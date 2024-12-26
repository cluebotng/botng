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

func loadSingleUserWarnsCount(logger *logrus.Entry, ctx context.Context, change *model.ProcessEvent, configuration *config.Configuration, db *database.DatabaseConnection, outChangeFeed chan *model.ProcessEvent) error {

	// Load the user warns count
	userWarnCount, err := db.Replica.GetUserWarnCount(logger, ctx, change.User.Username)
	if err != nil {
		metrics.EditStatus.With(prometheus.Labels{"state": "lookup_user_warning_count", "status": "failed"}).Inc()
		return err
	}

	metrics.EditStatus.With(prometheus.Labels{"state": "lookup_user_warning_count", "status": "success"}).Inc()
	change.User.Warns = userWarnCount
	outChangeFeed <- change
	return nil
}

func LoadUserWarnsCount(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	logger := logrus.WithField("function", "loader.LoadUserWarnsCount")
	wg.Add(1)
	defer wg.Done()
	for change := range inChangeFeed {
		metrics.LoaderUserWarnsCountInUse.Inc()
		ctx, span := metrics.OtelTracer.Start(change.TraceContext, "LoadUserWarnsCount")

		logger = logger.WithFields(logrus.Fields{"uuid": change.Uuid})
		if err := loadSingleUserWarnsCount(logger, ctx, change, configuration, db, outChangeFeed); err != nil {
			logger.Error(err.Error())
			span.SetStatus(codes.Error, err.Error())
			r.SendDebug(fmt.Sprintf("%v # Failed to get user warns count", change.FormatIrcChange()))
		}

		span.End()
		metrics.LoaderUserWarnsCountInUse.Dec()
	}
}
