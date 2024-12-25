package loader

import (
	"context"
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/database"
	"github.com/cluebotng/botng/pkg/cbng/helpers"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/cluebotng/botng/pkg/cbng/relay"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"sync"
)

func loadSinglePageRecentRevertCount(logger *logrus.Entry, ctx context.Context, change *model.ProcessEvent, configuration *config.Configuration, db *database.DatabaseConnection, outChangeFeed chan *model.ProcessEvent) error {
	// Load the page recent revert count
	pageRecentRevertCount, err := db.Replica.GetPageRecentRevertCount(logger, ctx, change.Common.NamespaceId, helpers.PageTitleWithoutNamespace(change.Common.Title), change.ReceivedTime.Unix())
	if err != nil {
		metrics.EditStatus.With(prometheus.Labels{"state": "lookup_page_recent_reverts", "status": "failed"}).Inc()
		return err
	}

	metrics.EditStatus.With(prometheus.Labels{"state": "lookup_page_recent_reverts", "status": "success"}).Inc()
	change.Common.NumRecentRevisions = pageRecentRevertCount
	outChangeFeed <- change
	return nil
}

func LoadPageRecentRevertCount(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	logger := logrus.WithField("function", "loader.LoadPageRecentRevertCount")
	wg.Add(1)
	defer wg.Done()
	for change := range inChangeFeed {
		metrics.LoaderPageRecentRevertCountInUse.Inc()
		ctx, span := metrics.OtelTracer.Start(change.TraceContext, "loader.LoadPageRecentRevertCount")
		span.SetAttributes(attribute.String("uuid", change.Uuid))

		logger = logger.WithFields(logrus.Fields{"uuid": change.Uuid})
		if err := loadSinglePageRecentRevertCount(logger, ctx, change, configuration, db, outChangeFeed); err != nil {
			logger.Error(err.Error())
			span.SetStatus(codes.Error, err.Error())
			r.SendDebug(fmt.Sprintf("%v # Failed to get page recent revert count", change.FormatIrcChange()))
		}

		span.End()
		metrics.LoaderPageRecentRevertCountInUse.Dec()
	}
}
