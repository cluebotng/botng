package feed

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/helpers"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/cluebotng/botng/pkg/cbng/wikipedia"
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
	Timestamp   int64
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

		receivedTime := time.Now().UTC()
		changeTime := time.Unix(httpChange.Timestamp, 0)

		_, span := metrics.OtelTracer.Start(context.Background(), "handleEdit")
		span.SetAttributes(attribute.String("uuid", changeUUID))
		span.SetAttributes(attribute.Int64("time_until_received", receivedTime.Unix()-changeTime.Unix()))
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
			Logger:       logger.WithFields(logrus.Fields{"uuid": changeUUID}),
			ActiveSpan:   nil,
			Uuid:         changeUUID,
			ReceivedTime: receivedTime,
			ChangeTime:   changeTime,
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
		logger.WithFields(logrus.Fields{
			"uuid": change.Uuid,
			"change": map[string]interface{}{
				"user":      change.User.Username,
				"namespace": change.Common.Namespace,
				"title":     change.Common.Title,
				"oldid":     change.Previous.Id,
				"curid":     change.Current.Id,
			},
		}).Info("Received new event")
		metrics.EditStatus.With(prometheus.Labels{"state": "received_new", "status": "success"}).Inc()

		change.StartNewActiveSpan("pending.Replication")
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

func EmitSingleEdit(api *wikipedia.WikipediaApi, changeId int64, changeFeed chan<- *model.ProcessEvent) {
	logger := logrus.WithFields(logrus.Fields{"function": "feed.EmitSingleEdit"})

	revisionMeta := api.GetRevisionMetadata(logger, changeId)
	if revisionMeta == nil {
		logger.Fatalf("Could not get revision metadata")
		return
	}

	revisionHistory := *api.GetRevisionHistory(logger, context.Background(), revisionMeta.Title, changeId)
	if len(revisionHistory) < 2 {
		logger.Fatalf("Could not get revision history")
		return
	}

	changeUUID := uuid.NewV4().String()
	receivedTime := time.Now().UTC()
	changeTime := time.Unix(revisionMeta.Timestamp, 0)

	_, span := metrics.OtelTracer.Start(context.Background(), "handleEdit")
	span.SetAttributes(attribute.String("uuid", changeUUID))
	span.SetAttributes(attribute.Int64("time_until_received", receivedTime.Unix()-changeTime.Unix()))
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
		Logger:       logger.WithFields(logrus.Fields{"uuid": changeUUID}),
		ActiveSpan:   nil,
		Uuid:         changeUUID,
		ReceivedTime: receivedTime,
		ChangeTime:   changeTime,
		Common: model.ProcessEventCommon{
			Namespace:   helpers.NameSpaceIdToName(revisionMeta.NamespaceId),
			NamespaceId: revisionMeta.NamespaceId,
			Title:       revisionMeta.Title,
		},
		Comment: revisionMeta.Comment,
		User: model.ProcessEventUser{
			Username: revisionMeta.User,
		},
		Length: revisionMeta.Size,

		Current: model.ProcessEventRevision{
			Id: int64(changeId),
		},
		Previous: model.ProcessEventRevision{
			Id: revisionHistory[1].Id,
		},
	}

	// Otherwise send for processing
	logger.WithFields(logrus.Fields{
		"uuid": change.Uuid,
		"change": map[string]interface{}{
			"user":      change.User.Username,
			"namespace": change.Common.Namespace,
			"title":     change.Common.Title,
			"oldid":     change.Previous.Id,
			"curid":     change.Current.Id,
		},
	}).Info("Received new event")

	change.StartNewActiveSpan("pending.Replication")
	changeFeed <- &change
}
