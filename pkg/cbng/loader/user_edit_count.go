package loader

import (
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/database"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/cluebotng/botng/pkg/cbng/relay"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/codes"
	"net"
	"sync"
)

func LoadUserEditCount(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	wg.Add(1)
	defer wg.Done()
	for change := range inChangeFeed {
		metrics.LoaderUserEditCountInUse.Inc()
		func(changeEvent *model.ProcessEvent) {
			change.EndActiveSpan()
			logger := change.Logger.WithField("function", "loader.LoadUserEditCount")

			_, span := metrics.OtelTracer.Start(change.TraceContext, "LoadUserEditCount")
			defer span.End()

			var f func(l *logrus.Entry, user string) (int64, error)
			if net.ParseIP(change.User.Username) != nil {
				f = db.Replica.GetAnonymousUserEditCount
			} else {
				f = db.Replica.GetRegisteredUserEditCount
			}

			userEditCount, err := f(logger, change.User.Username)
			if err != nil {
				metrics.EditStatus.With(prometheus.Labels{"state": "lookup_anonymous_user_edit_count", "status": "failed"}).Inc()
				logger.Error(err.Error())
				span.SetStatus(codes.Error, err.Error())
				r.SendDebug(fmt.Sprintf("%v # Failed to get user edit count", change.FormatIrcChange()))
			} else {
				metrics.EditStatus.With(prometheus.Labels{"state": "lookup_anonymous_user_edit_count", "status": "success"}).Inc()
				change.User.EditCount = userEditCount
				change.StartNewActiveSpan("pending.LoadUserRegistrationTime")
				outChangeFeed <- change
			}
		}(change)
		metrics.LoaderUserEditCountInUse.Dec()
	}
}
