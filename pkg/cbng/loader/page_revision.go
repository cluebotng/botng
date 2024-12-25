package loader

import (
	"context"
	"errors"
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/cluebotng/botng/pkg/cbng/relay"
	"github.com/cluebotng/botng/pkg/cbng/wikipedia"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"sync"
)

func loadSinglePageRevision(logger *logrus.Entry, ctx context.Context, change *model.ProcessEvent, api *wikipedia.WikipediaApi, outChangeFeed chan *model.ProcessEvent) error {

	// Pull the revisions from the API
	revisionData := api.GetRevision(logger, ctx, change.Common.Title, change.Current.Id)
	if revisionData == nil ||
		revisionData.Current.Timestamp == 0 ||
		revisionData.Current.Data == "" ||
		revisionData.Previous.Timestamp == 0 ||
		revisionData.Previous.Data == "" {
		metrics.EditStatus.With(prometheus.Labels{"state": "lookup_page_revisions", "status": "failed"}).Inc()
		return errors.New("failed to get complete revision data")
	}

	change.Current = model.ProcessEventRevision{
		Timestamp: revisionData.Current.Timestamp,
		Text:      revisionData.Current.Data,
		Id:        revisionData.Current.Id,
		Username:  revisionData.Current.User,
	}
	change.Previous = model.ProcessEventRevision{
		Timestamp: revisionData.Previous.Timestamp,
		Text:      revisionData.Previous.Data,
		Id:        revisionData.Previous.Id,
		Username:  revisionData.Previous.User,
	}

	metrics.EditStatus.With(prometheus.Labels{"state": "lookup_page_revisions", "status": "success"}).Inc()
	outChangeFeed <- change
	return nil
}

func LoadPageRevision(wg *sync.WaitGroup, api *wikipedia.WikipediaApi, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	logger := logrus.WithField("function", "loader.LoadPageRevision")
	wg.Add(1)
	defer wg.Done()
	for change := range inChangeFeed {
		metrics.LoaderPageRevisionInUse.Inc()
		ctx, span := metrics.OtelTracer.Start(change.TraceContext, "loader.LoadPageRevision")
		span.SetAttributes(attribute.String("uuid", change.Uuid))

		logger = logger.WithFields(logrus.Fields{"uuid": change.Uuid})
		if err := loadSinglePageRevision(logger, ctx, change, api, outChangeFeed); err != nil {
			logger.Error(err.Error())
			span.SetStatus(codes.Error, err.Error())
			r.SendDebug(fmt.Sprintf("%v # Failed to get page revision", change.FormatIrcChange()))
		}

		span.End()
		metrics.LoaderPageRevisionInUse.Dec()
	}
}
