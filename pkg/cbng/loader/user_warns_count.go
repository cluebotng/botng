package loader

import (
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/database"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/cluebotng/botng/pkg/cbng/relay"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/codes"
	"sync"
)

func LoadUserWarnsCount(wg *sync.WaitGroup, db *database.DatabaseConnection, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	wg.Add(1)
	defer wg.Done()
	for change := range inChangeFeed {
		metrics.LoaderUserWarnsCountInUse.Inc()
		func(changeEvent *model.ProcessEvent) {
			change.EndActiveSpan()
			logger := change.Logger.WithField("function", "loader.LoadUserWarnsCount")

			_, span := metrics.OtelTracer.Start(change.TraceContext, "LoadUserWarnsCount")
			defer span.End()

			userWarnCount, err := db.Replica.GetUserWarnCount(logger, change.User.Username)
			if err != nil {
				metrics.EditStatus.With(prometheus.Labels{"state": "lookup_user_warning_count", "status": "failed"}).Inc()
				logger.Error(err.Error())
				span.SetStatus(codes.Error, err.Error())
				r.SendDebug(fmt.Sprintf("%v # Failed to get user warns count", change.FormatIrcChange()))
			} else {
				metrics.EditStatus.With(prometheus.Labels{"state": "lookup_user_warning_count", "status": "success"}).Inc()
				change.User.Warns = userWarnCount
				change.StartNewActiveSpan("pending.LoadPageRevision")
				outChangeFeed <- change
			}
		}(change)
		metrics.LoaderUserWarnsCountInUse.Dec()
	}
}
