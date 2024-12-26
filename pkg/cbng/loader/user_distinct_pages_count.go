package loader

import (
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/database"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/cluebotng/botng/pkg/cbng/relay"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/codes"
	"sync"
)

func LoadDistinctPagesCount(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	wg.Add(1)
	defer wg.Done()
	for change := range inChangeFeed {
		metrics.LoaderUserDistinctPageCountInUse.Inc()
		func(changeEvent *model.ProcessEvent) {
			change.EndActiveSpan()
			logger := change.Logger.WithField("function", "loader.LoadDistinctPagesCount")

			_, span := metrics.OtelTracer.Start(change.TraceContext, "LoadDistinctPagesCount")
			defer span.End()

			userDistinctPagesCount, err := db.Replica.GetUserDistinctPagesCount(logger, change.User.Username)
			if err != nil {
				metrics.EditStatus.With(prometheus.Labels{"state": "lookup_user_distinct_count", "status": "failed"}).Inc()
				logger.Error(err.Error())
				span.SetStatus(codes.Error, err.Error())
				r.SendDebug(fmt.Sprintf("%v # Failed to get user distinct pages count", change.FormatIrcChange()))
			} else {
				metrics.EditStatus.With(prometheus.Labels{"state": "lookup_user_distinct_count", "status": "passed"}).Inc()
				change.User.DistinctPages = userDistinctPagesCount
				change.StartNewActiveSpan("pending.LoadUserWarnsCount")
				outChangeFeed <- change
			}
		}(change)
		metrics.LoaderUserDistinctPageCountInUse.Dec()
	}
}
