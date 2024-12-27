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

func LoadUserRegistrationTime(wg *sync.WaitGroup, db *database.DatabaseConnection, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	wg.Add(1)
	defer wg.Done()
	for change := range inChangeFeed {
		metrics.LoaderUserRegistrationInUse.Inc()
		func(changeEvent *model.ProcessEvent) {
			change.EndActiveSpan()

			logger := change.Logger.WithField("function", "loader.LoadUserRegistrationTime")

			_, span := metrics.OtelTracer.Start(change.TraceContext, "LoadUserRegistrationTime")
			defer span.End()

			userRegTime, err := db.Replica.GetUserRegistrationTime(logger, change.User.Username)
			if err != nil {
				metrics.EditStatus.With(prometheus.Labels{"state": "lookup_user_registration_time", "status": "failed"}).Inc()
				logger.Error(err.Error())
				span.SetStatus(codes.Error, err.Error())
				r.SendDebug(fmt.Sprintf("%v # Failed to get user edit count", change.FormatIrcChange()))
			} else {
				metrics.EditStatus.With(prometheus.Labels{"state": "lookup_user_registration_time", "status": "success"}).Inc()
				change.User.RegistrationTime = userRegTime
				change.StartNewActiveSpan("pending.LoadDistinctPagesCount")
				outChangeFeed <- change
			}
		}(change)
		metrics.LoaderUserRegistrationInUse.Dec()
	}
}
