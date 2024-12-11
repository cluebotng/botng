package feed

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/prometheus/client_golang/prometheus"
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"net/http"
	"strings"
	"sync"
	"time"
)

type httpChangeEventLength struct {
	New int64
	Old int64
}

type httpChangeEventRevision struct {
	New int64
	Old int64
}

type httpChangeEvent struct {
	Type        string
	Namespace   string `json:"-"`
	NamespaceId int64  `json:"namespace"`
	Title       string
	Comment     string
	User        string
	Length      httpChangeEventLength
	Revision    httpChangeEventRevision
	ServerName  string `json:"server_name"`
}

func handleLine(logger *logrus.Entry, line string, configuration *config.Configuration, changeFeed chan<- *model.ProcessEvent) {
	if len(line) > 5 && line[0:5] == "data:" {
		rootCtx, rootSpan := metrics.OtelTracer.Start(context.Background(), "feed.ConsumeHttpChangeEvents.event")
		rootUUID := uuid.NewV4().String()
		rootSpan.SetAttributes(attribute.String("uuid", rootUUID))
		defer rootSpan.End()

		_, decodeSpan := metrics.OtelTracer.Start(rootCtx, "feed.ConsumeHttpChangeEvents.event.unmarshal")
		httpChange := httpChangeEvent{}
		if err := json.Unmarshal([]byte(line[5:]), &httpChange); err != nil {
			logger.Warnf("Decoding failed: %v", err)
			decodeSpan.SetStatus(codes.Error, err.Error())
			decodeSpan.End()
			return
		}
		decodeSpan.End()
		logger.Tracef("Received: %+v", httpChange)

		if httpChange.Type == "edit" && httpChange.ServerName == configuration.Wikipedia.Host {
			_, emitterSpan := metrics.OtelTracer.Start(rootCtx, "feed.ConsumeHttpChangeEvents.event.emit")
			defer emitterSpan.End()

			namespace := strings.TrimRight(httpChange.Namespace, ":")
			if namespace == "" {
				namespace = "main"
			}

			change := model.ProcessEvent{
				Uuid:         rootUUID,
				ReceivedTime: time.Now().UTC(),
				Common: model.ProcessEventCommon{
					Namespace:   namespace,
					NamespaceId: httpChange.NamespaceId,
					Title:       httpChange.Title,
				},
				Comment: httpChange.Comment,
				User: model.ProcessEventUser{
					Username: httpChange.User,
				},
				Length: httpChange.Length.New - httpChange.Length.Old,

				Current: model.ProcessEventRevision{
					Id: httpChange.Revision.New,
				},
				Previous: model.ProcessEventRevision{
					Id: httpChange.Revision.Old,
				},
			}

			logger.WithFields(logrus.Fields{"uuid": change.Uuid, "change": change}).Debug("Received new event")
			metrics.EditStatus.With(prometheus.Labels{"state": "received_new", "status": "success"}).Inc()
			changeFeed <- &change
		}
	}
}

func streamFeed(logger *logrus.Entry, configuration *config.Configuration, changeFeed chan<- *model.ProcessEvent) bool {
	logger.Info("Connecting to feed")
	req, err := http.NewRequest("GET", "https://stream.wikimedia.org/v2/stream/mediawiki.recentchange", nil)
	if err != nil {
		logger.Errorf("Could not build request: %v", err)
		return false
	}

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		logger.Errorf("Request failed: %v", err)
		return false
	}

	reader := bufio.NewReader(res.Body)
	defer res.Body.Close()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			logger.Errorf("Reading failed: %v", err)
			break
		}

		handleLine(logger, line, configuration, changeFeed)
	}
	return true
}

func ConsumeHttpChangeEvents(wg *sync.WaitGroup, configuration *config.Configuration, changeFeed chan<- *model.ProcessEvent) {
	logger := logrus.WithFields(logrus.Fields{"function": "feed.ConsumeHttpChangeEvents"})
	wg.Add(1)
	defer wg.Done()

	attempts := 0
	for {
		if streamFeed(logger, configuration, changeFeed) {
			attempts = 0
		}
		attempts++

		logger.Infof("Stream returned, trying to reconnect (attempt %v)", attempts)
		time.Sleep(time.Duration(attempts) * time.Second)
	}
}
