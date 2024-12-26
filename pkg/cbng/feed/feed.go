package feed

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/helpers"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/prometheus/client_golang/prometheus"
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
		httpChange := httpChangeEvent{}
		if err := json.Unmarshal([]byte(line[5:]), &httpChange); err != nil {
			logger.Warnf("Decoding failed: %v", err)
			metrics.FeedStatus.With(prometheus.Labels{"status": "decoding_failed"}).Inc()
			return
		}
		logger.Tracef("Received: %+v", httpChange)
		metrics.FeedStatus.With(prometheus.Labels{"status": "decoded"}).Inc()

		if httpChange.Type != "edit" {
			metrics.FeedStatus.With(prometheus.Labels{"status": "rejected_type"}).Inc()
			return
		}

		if httpChange.ServerName != configuration.Wikipedia.Host {
			metrics.FeedStatus.With(prometheus.Labels{"status": "rejected_server"}).Inc()
			return
		}

		namespace := strings.TrimRight(httpChange.Namespace, ":")
		if namespace == "" {
			namespace = "main"
		}

		// Skip namespaces we're not interested in
		if httpChange.NamespaceId != 0 && !helpers.StringItemInSlice(namespace, configuration.Dynamic.NamespaceOptIn) {
			logger.Debugf("Skipping change due to namespace: %s (%d)", namespace, httpChange.NamespaceId)
			metrics.FeedStatus.With(prometheus.Labels{"status": "rejected_namespace"}).Inc()
			return
		}

		changeUUID := uuid.NewV4().String()
		metrics.FeedStatus.With(prometheus.Labels{"status": "received"}).Inc()

		_, span := metrics.OtelTracer.Start(context.Background(), "handleEdit")
		span.SetAttributes(attribute.String("uuid", changeUUID))
		defer span.End()

		change := model.ProcessEvent{
			TraceContext: trace.ContextWithRemoteSpanContext(
				context.Background(),
				trace.NewSpanContext(trace.SpanContextConfig{
					TraceID:    span.SpanContext().TraceID(),
					SpanID:     span.SpanContext().SpanID(),
					TraceFlags: trace.FlagsSampled,
				}),
			),
			Uuid:         changeUUID,
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

		// Otherwise send for processing
		logger.WithFields(logrus.Fields{"uuid": change.Uuid, "change": change}).Debug("Received new event")
		metrics.EditStatus.With(prometheus.Labels{"state": "received_new", "status": "success"}).Inc()
		changeFeed <- &change
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
