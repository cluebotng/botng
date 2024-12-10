package processor

import (
	"context"
	"encoding/xml"
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/database"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/codes"
	"net"
	"strings"
	"time"
)

func generateXML(pe *model.ProcessEvent) ([]byte, error) {
	type WPEditCommon struct {
		PageMadeTime         int64  `xml:"page_made_time"`
		Title                string `xml:"title"`
		Namespace            string `xml:"namespace"`
		Creator              string `xml:"creator"`
		NumerOfRecentEdits   int64  `xml:"num_recent_edits"`
		NumerOfRecentReverts int64  `xml:"num_recent_reversions"`
	}
	type WPEditRevision struct {
		Timestamp int64  `xml:"timestamp"`
		Text      string `xml:"text"`
	}
	type WPEdit struct {
		EditType               string         `xml:"EditType"`
		EditId                 int64          `xml:"EditID"`
		Comment                string         `xml:"comment"`
		User                   string         `xml:"user"`
		UserEditCount          int64          `xml:"user_edit_count"`
		UserDistinctPagesCount int64          `xml:"user_distinct_pages"`
		UserWarningsCount      int64          `xml:"user_warns"`
		PreviousUser           string         `xml:"prev_user"`
		UserRegistrationTime   int64          `xml:"user_reg_time"`
		Common                 WPEditCommon   `xml:"common"`
		Current                WPEditRevision `xml:"current"`
		Previous               WPEditRevision `xml:"previous"`
	}
	type WPEditSet struct {
		WPEdit WPEdit
	}

	data := WPEditSet{
		WPEdit: WPEdit{
			EditType:               pe.EditType,
			EditId:                 pe.EditId,
			Comment:                pe.Comment,
			User:                   pe.User.Username,
			UserEditCount:          pe.User.EditCount,
			UserDistinctPagesCount: pe.User.DistinctPages,
			UserWarningsCount:      pe.User.Warns,
			PreviousUser:           pe.PreviousUser,
			UserRegistrationTime:   pe.User.RegistrationTime,
			Common: WPEditCommon{
				PageMadeTime:         pe.Common.PageMadeTime,
				Title:                pe.Common.Title,
				Namespace:            pe.Common.Namespace,
				Creator:              pe.Common.Creator,
				NumerOfRecentEdits:   pe.Common.NumRecentEdits,
				NumerOfRecentReverts: pe.Common.NumRecentRevisions,
			},
			Current: WPEditRevision{
				Text:      pe.Current.Text,
				Timestamp: pe.Current.Timestamp,
			},
		},
	}

	return xml.Marshal(data)
}

func isVandalism(l *logrus.Entry, parentCtx context.Context, configuration *config.Configuration, db *database.DatabaseConnection, pe *model.ProcessEvent) (bool, error) {
	logger := l.WithField("function", "processor.isVandalism")
	ctx, parentSpan := metrics.OtelTracer.Start(parentCtx, "core.isVandalism")
	defer parentSpan.End()

	coreHost := configuration.Core.Host
	if coreHost == "" {
		coreHost = db.ClueBot.GetServiceHost(logger, "core")
	}

	_, XmlSpan := metrics.OtelTracer.Start(ctx, "core.isVandalism.generateXML")
	xmlData, err := generateXML(pe)
	if err != nil {
		XmlSpan.SetStatus(codes.Error, err.Error())
		logger.Errorf("Could not generate xml: %v", err)
		return false, err
	}
	XmlSpan.End()

	_, scoreSpan := metrics.OtelTracer.Start(ctx, "core.isVandalism.score")
	defer scoreSpan.End()

	coreUrl := fmt.Sprintf("%s:%d", coreHost, configuration.Core.Port)
	logger.Tracef("Connecting to %v", coreUrl)

	dialer := net.Dialer{Timeout: time.Second * 10}
	conn, err := dialer.Dial("tcp", coreUrl)
	if err != nil {
		scoreSpan.SetStatus(codes.Error, err.Error())
		logger.Errorf("Could not connect (%v): %v", coreUrl, err)
		return false, err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(time.Second * 2)); err != nil {
		scoreSpan.SetStatus(codes.Error, err.Error())
		logger.Errorf("Could not set deadline: %v", err)
		return false, err
	}
	if err := conn.SetReadDeadline(time.Now().Add(time.Second * 2)); err != nil {
		scoreSpan.SetStatus(codes.Error, err.Error())
		logger.Errorf("Could not set read deadline: %v", err)
		return false, err
	}

	if _, err := conn.Write(xmlData); err != nil {
		scoreSpan.SetStatus(codes.Error, err.Error())
		logger.Infof("Could not write payload: %v", err)
		return false, err
	}

	response := []byte{}
	tmp := make([]byte, 4096)
	i := 0
	for {
		n, err := conn.Read(tmp)
		if err != nil {
			scoreSpan.SetStatus(codes.Error, err.Error())
			logger.Warnf("Could not read response: %v", err)
			return false, err
		}

		response = append(response, tmp[:n]...)
		if strings.Contains(string(response), "</WPEditSet>") {
			break
		}
		i += 1
	}
	logger = logger.WithField("response", response)

	editSet := model.WPEditScoreSet{}
	if err := xml.Unmarshal(response, &editSet); err != nil {
		scoreSpan.SetStatus(codes.Error, err.Error())
		logger.Warnf("Could not decode response: %v", err)
		return false, err
	}

	logger.Debugf("Core response; Vandalism: %v, Score: %v", editSet.WPEdit.ThinkVandalism, editSet.WPEdit.Score)
	pe.VandalismScore = editSet.WPEdit.Score
	return editSet.WPEdit.ThinkVandalism, nil
}
