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

func LoadPageRecentRevertCount(wg *sync.WaitGroup, db *database.DatabaseConnection, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	wg.Add(1)
	defer wg.Done()
	for change := range inChangeFeed {

		metrics.LoaderPageRecentRevertCountInUse.Inc()
		func(changeEvent *model.ProcessEvent) {
			change.EndActiveSpan()
			logger := change.Logger.WithField("function", "loader.LoadPageRecentRevertCount")

			_, span := metrics.OtelTracer.Start(change.TraceContext, "LoadPageRecentRevertCount")
			defer span.End()

			pageRecentRevertCount, err := db.Replica.GetPageRecentRevertCount(logger, change.Common.NamespaceId, helpers.PageTitleWithoutNamespace(change.Common.Title), change.ReceivedTime.Unix()-config.RecentChangeWindow)
			if err != nil {
				metrics.EditStatus.With(prometheus.Labels{"state": "lookup_page_recent_reverts", "status": "failed"}).Inc()
				logger.Error(err.Error())
				span.SetStatus(codes.Error, err.Error())
				r.SendDebug(fmt.Sprintf("%v # Failed to get page recent revert count", change.FormatIrcChange()))
			} else {
				metrics.EditStatus.With(prometheus.Labels{"state": "lookup_page_recent_reverts", "status": "success"}).Inc()
				change.Common.NumRecentRevisions = pageRecentRevertCount
				change.StartNewActiveSpan("pending.LoadUserEditCount")
				outChangeFeed <- change
			}
		}(change)
		metrics.LoaderPageRecentRevertCountInUse.Dec()
	}
}
