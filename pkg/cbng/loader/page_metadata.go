package loader

import (
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/database"
	"github.com/cluebotng/botng/pkg/cbng/helpers"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/cluebotng/botng/pkg/cbng/relay"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/codes"
	"sync"
)

func LoadPageMetadata(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	wg.Add(1)
	defer wg.Done()
	for {
		change := <-inChangeFeed
		metrics.LoaderPageMetadataInUse.Inc()
		func(changeEvent *model.ProcessEvent) {
			change.EndActiveSpan()

			ctx, span := metrics.OtelTracer.Start(change.TraceContext, "LoadPageMetadata")
			defer span.End()

			logger := change.Logger.WithField("function", "loader.LoadPageMetadata")

			pageCreatedUser, pageCreatedTimestamp, err := db.Replica.GetPageCreatedTimeAndUser(logger, ctx, change.Common.NamespaceId, helpers.PageTitleWithoutNamespace(change.Common.Title))
			if err != nil {
				metrics.EditStatus.With(prometheus.Labels{"state": "lookup_page_metadata", "status": "failed"}).Inc()
				logger.Error(err.Error())
				span.SetStatus(codes.Error, err.Error())
				r.SendDebug(fmt.Sprintf("%v # Failed to get page metadata", change.FormatIrcChange()))
			} else {
				metrics.EditStatus.With(prometheus.Labels{"state": "lookup_page_metadata", "status": "success"}).Inc()
				change.Common.Creator = pageCreatedUser
				change.Common.PageMadeTime = pageCreatedTimestamp
				change.StartNewActiveSpan("pending.LoadPageRecentEditCount")
				outChangeFeed <- change
			}
		}(change)
		metrics.LoaderPageMetadataInUse.Dec()
	}
}
