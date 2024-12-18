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

func loadSinglePageMetadata(logger *logrus.Entry, ctx context.Context, change *model.ProcessEvent, configuration *config.Configuration, db *database.DatabaseConnection, outChangeFeed chan *model.ProcessEvent) error {

	// Skip namespaces we're not interested in
	if change.Common.NamespaceId != 0 && !helpers.StringItemInSlice(change.Common.Namespace, configuration.Dynamic.NamespaceOptIn) {
		logger.Debugf("Skipping change due to namespace: %s (%d)", change.Common.Namespace, change.Common.NamespaceId)
		metrics.EditStatus.With(prometheus.Labels{"state": "verify_namespace", "status": "skipped"}).Inc()
		return nil
	}
	metrics.EditStatus.With(prometheus.Labels{"state": "verify_namespace", "status": "success"}).Inc()

	// Load the page created metadata
	pageCreatedUser, pageCreatedTimestamp, err := db.Replica.GetPageCreatedTimeAndUser(logger, ctx, change.Common.NamespaceId, helpers.PageTitleWithoutNamespace(change.Common.Title))
	if err != nil {
		metrics.EditStatus.With(prometheus.Labels{"state": "lookup_page_metadata", "status": "failed"}).Inc()
		return err
	}

	metrics.EditStatus.With(prometheus.Labels{"state": "lookup_page_metadata", "status": "success"}).Inc()
	change.Common.Creator = pageCreatedUser
	change.Common.PageMadeTime = pageCreatedTimestamp
	outChangeFeed <- change
	return nil
}

func LoadPageMetadata(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	logger := logrus.WithField("function", "loader.LoadPageMetadata")
	wg.Add(1)
	defer wg.Done()
	for {
		change := <-inChangeFeed
		metrics.LoaderPageMetadataInUse.Inc()
		ctx, span := metrics.OtelTracer.Start(context.Background(), "loader.LoadPageMetadata")
		span.SetAttributes(attribute.String("uuid", change.Uuid))

		logger = logger.WithFields(logrus.Fields{"uuid": change.Uuid})
		if err := loadSinglePageMetadata(logger, ctx, change, configuration, db, outChangeFeed); err != nil {
			logger.Error(err.Error())
			span.SetStatus(codes.Error, err.Error())
			r.SendDebug(fmt.Sprintf("%v # Failed to get page metadata", change.FormatIrcChange()))
		}

		span.End()
		metrics.LoaderPageMetadataInUse.Dec()
	}
}
