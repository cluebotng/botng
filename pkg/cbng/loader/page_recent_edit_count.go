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

func loadSinglePageRecentEditCount(logger *logrus.Entry, ctx context.Context, change *model.ProcessEvent, configuration *config.Configuration, db *database.DatabaseConnection, outChangeFeed chan *model.ProcessEvent) error {

	// Load the page recent edit count
	pageRecentEditCount, err := db.Replica.GetPageRecentEditCount(logger, ctx, change.Common.NamespaceId, helpers.PageTitleWithoutNamespace(change.Common.Title), change.ReceivedTime.Unix()-14*86400)
	if err != nil {
		metrics.EditStatus.With(prometheus.Labels{"state": "lookup_page_recent_edits", "status": "failed"}).Inc()
		return err
	}

	metrics.EditStatus.With(prometheus.Labels{"state": "lookup_page_recent_edits", "status": "success"}).Inc()
	change.Common.NumRecentEdits = pageRecentEditCount
	outChangeFeed <- change
	return nil
}

func LoadPageRecentEditCount(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	logger := logrus.WithField("function", "loader.LoadPageRecentEditCount")
	wg.Add(1)
	defer wg.Done()
	for {
		change := <-inChangeFeed
		metrics.LoaderPageRecentEditCountInUse.Inc()
		ctx, span := metrics.OtelTracer.Start(context.Background(), "loader.LoadPageRecentEditCount")
		span.SetAttributes(attribute.String("uuid", change.Uuid))

		logger = logger.WithFields(logrus.Fields{"uuid": change.Uuid})
		if err := loadSinglePageRecentEditCount(logger, ctx, change, configuration, db, outChangeFeed); err != nil {
			logger.Error(err.Error())
			span.SetStatus(codes.Error, err.Error())
			r.SendDebug(fmt.Sprintf("%v # Failed to get page recent edit count", change.FormatIrcChange()))
		}

		span.End()
		metrics.LoaderPageRecentEditCountInUse.Dec()
	}
}
