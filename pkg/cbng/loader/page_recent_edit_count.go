package loader

import (
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/database"
	"github.com/cluebotng/botng/pkg/cbng/helpers"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/cluebotng/botng/pkg/cbng/relay"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/codes"
	"sync"
)

func LoadPageRecentEditCount(wg *sync.WaitGroup, db *database.DatabaseConnection, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	wg.Add(1)
	defer wg.Done()
	for {
		change := <-inChangeFeed
		metrics.LoaderPageRecentEditCountInUse.Inc()
		func(changeEvent *model.ProcessEvent) {
			change.EndActiveSpan()

			_, span := metrics.OtelTracer.Start(change.TraceContext, "LoadPageRecentEditCount")
			defer span.End()

			logger := change.Logger.WithField("function", "loader.LoadPageRecentEditCount")

			pageRecentEditCount, err := db.Replica.GetPageRecentEditCount(logger, change.Common.NamespaceId, helpers.PageTitleWithoutNamespace(change.Common.Title), change.ReceivedTime.Unix()-14*86400)
			if err != nil {
				metrics.EditStatus.With(prometheus.Labels{"state": "lookup_page_recent_edits", "status": "failed"}).Inc()
				logger.Error(err.Error())
				span.SetStatus(codes.Error, err.Error())
				r.SendDebug(fmt.Sprintf("%v # Failed to get page recent edit count", change.FormatIrcChange()))
			} else {
				metrics.EditStatus.With(prometheus.Labels{"state": "lookup_page_recent_edits", "status": "success"}).Inc()
				change.Common.NumRecentEdits = pageRecentEditCount
				change.StartNewActiveSpan("pending.LoadPageRecentRevertCount")
				outChangeFeed <- change
			}
		}(change)
		metrics.LoaderPageRecentEditCountInUse.Dec()
	}
}
