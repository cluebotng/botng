package loader

import (
	"context"
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/database"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/cluebotng/botng/pkg/cbng/relay"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/codes"
	"sync"
)

func loadSingleUserEditCount(logger *logrus.Entry, ctx context.Context, change *model.ProcessEvent, configuration *config.Configuration, db *database.DatabaseConnection, outChangeFeed chan *model.ProcessEvent) error {

	// Load the user edit count
	userEditCount, err := db.Replica.GetUserEditCount(logger, ctx, change.User.Username)
	if err != nil {
		metrics.EditStatus.With(prometheus.Labels{"state": "lookup_user_edit_count", "status": "failed"}).Inc()
		return err
	}
	metrics.EditStatus.With(prometheus.Labels{"state": "lookup_user_edit_count", "status": "success"}).Inc()

	// Load the user registration time
	userRegTime, err := db.Replica.GetUserRegistrationTime(logger, ctx, change.User.Username)
	if err != nil {
		metrics.EditStatus.With(prometheus.Labels{"state": "lookup_user_registration_time", "status": "failed"}).Inc()
		return err
	}
	metrics.EditStatus.With(prometheus.Labels{"state": "lookup_user_registration_time", "status": "success"}).Inc()

	change.User.EditCount = userEditCount
	change.User.RegistrationTime = userRegTime
	outChangeFeed <- change
	return nil
}

func LoadUserEditCount(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	logger := logrus.WithField("function", "loader.LoadUserEditCount")

	wg.Add(1)
	defer wg.Done()
	for change := range inChangeFeed {
		metrics.LoaderUserEditCountInUse.Inc()
		ctx, span := metrics.OtelTracer.Start(change.TraceContext, "LoadUserEditCount")

		logger = logger.WithFields(logrus.Fields{"uuid": change.Uuid})
		if err := loadSingleUserEditCount(logger, ctx, change, configuration, db, outChangeFeed); err != nil {
			logger.Error(err.Error())
			span.SetStatus(codes.Error, err.Error())
			r.SendDebug(fmt.Sprintf("%v # Failed to get user edit count", change.FormatIrcChange()))
		}

		span.End()
		metrics.LoaderUserEditCountInUse.Dec()
	}
}
