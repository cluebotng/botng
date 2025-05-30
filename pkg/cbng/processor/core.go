package processor

import (
	"context"
	"encoding/xml"
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
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
			EditType:               "change",
			EditId:                 pe.Current.Id,
			Comment:                pe.Comment,
			User:                   pe.User.Username,
			UserEditCount:          pe.User.EditCount,
			UserDistinctPagesCount: pe.User.DistinctPages,
			UserWarningsCount:      pe.User.Warns,
			PreviousUser:           pe.Previous.Username,
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
			Previous: WPEditRevision{
				Text:      pe.Previous.Text,
				Timestamp: pe.Previous.Timestamp,
			},
		},
	}

	return xml.Marshal(data)
}

func isVandalism(l *logrus.Entry, parentCtx context.Context, configuration *config.Configuration, pe *model.ProcessEvent) (bool, error) {
	logger := l.WithField("function", "processor.isVandalism")
	_, span := metrics.OtelTracer.Start(parentCtx, "core.isVandalism")
	defer span.End()

	xmlData, err := generateXML(pe)
	if err != nil {
		logger.Errorf("Could not generate xml: %v", err)
		span.SetStatus(codes.Error, err.Error())
		return false, err
	}
	logger = logger.WithField("request", xmlData)

	coreUrl := fmt.Sprintf("%s:%d", configuration.Core.Host, configuration.Core.Port)
	logger.Tracef("Connecting to %v", coreUrl)

	dialer := net.Dialer{Timeout: time.Second * 5}
	conn, err := dialer.Dial("tcp", coreUrl)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		logger.Errorf("Could not connect (%v): %v", coreUrl, err)
		return false, err
	}
	defer conn.Close()

	if _, err := conn.Write(xmlData); err != nil {
		span.SetStatus(codes.Error, err.Error())
		logger.Infof("Could not write payload: %v", err)
		return false, err
	}
	response := []byte{}
	tmp := make([]byte, 4096)
	for {
		n, err := conn.Read(tmp)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			logger.Warnf("Could not read response: %v", err)
			return false, err
		}

		response = append(response, tmp[:n]...)
		if strings.Contains(string(response), "</WPEditSet>") {
			break
		}
	}
	logger = logger.WithField("response", response)

	editSet := model.WPEditScoreSet{}
	if err := xml.Unmarshal(response, &editSet); err != nil {
		span.SetStatus(codes.Error, err.Error())
		logger.Warnf("Could not decode response: %v", err)
		return false, err
	}

	logger.Debugf("Core response; Vandalism: %v, Score: %v", editSet.WPEdit.ThinkVandalism, editSet.WPEdit.Score)
	span.SetAttributes(attribute.Float64("core.vandalism.score", editSet.WPEdit.Score))
	span.SetAttributes(attribute.Bool("core.vandalism.result", editSet.WPEdit.ThinkVandalism))

	pe.VandalismScore = editSet.WPEdit.Score
	return editSet.WPEdit.ThinkVandalism, nil
}
