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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"sync"
)

func loadSingleDistinctPagesCount(logger *logrus.Entry, ctx context.Context, change *model.ProcessEvent, configuration *config.Configuration, db *database.DatabaseConnection, outChangeFeed chan *model.ProcessEvent) error {

	// Load the user distinct pages count
	userDistinctPagesCount, err := db.Replica.GetUserDistinctPagesCount(logger, ctx, change.User.Username)
	if err != nil {
		metrics.EditStatus.With(prometheus.Labels{"state": "lookup_user_distinct_count", "status": "failed"}).Inc()
		return err
	}

	metrics.EditStatus.With(prometheus.Labels{"state": "lookup_user_distinct_count", "status": "passed"}).Inc()
	change.User.DistinctPages = userDistinctPagesCount
	outChangeFeed <- change
	return nil
}

func LoadDistinctPagesCount(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	logger := logrus.WithField("function", "loader.LoadDistinctPagesCount")
	wg.Add(1)
	defer wg.Done()
	for change := range inChangeFeed {
		metrics.LoaderUserDistinctPageCountInUse.Inc()
		ctx, span := metrics.OtelTracer.Start(context.Background(), "loader.LoadDistinctPagesCount")
		span.SetAttributes(attribute.String("uuid", change.Uuid))

		logger = logger.WithFields(logrus.Fields{"uuid": change.Uuid})
		if err := loadSingleDistinctPagesCount(logger, ctx, change, configuration, db, outChangeFeed); err != nil {
			logger.Error(err.Error())
			span.SetStatus(codes.Error, err.Error())
			r.SendDebug(fmt.Sprintf("%v # Failed to get user distinct pages count", change.FormatIrcChange()))
		}

		span.End()
		metrics.LoaderUserDistinctPageCountInUse.Dec()
	}
}
