package processor

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

func isWhitelisted(l *logrus.Entry, configuration *config.Configuration, user string) bool {
	logger := l.WithFields(logrus.Fields{
		"function": "processor.isWhitelisted",
		"args": map[string]interface{}{
			"user": user,
		},
	})

	for _, wuser := range configuration.Dynamic.HuggleUserWhitelist {
		if user == wuser {
			logger.Infof("Found user whitelisted in Huggle")
			return true
		}
	}
	return false
}

func processSingleScoringChange(logger *logrus.Entry, ctx context.Context, change *model.ProcessEvent, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, outChangeFeed chan *model.ProcessEvent) error {
	isVandalism, err := isVandalism(logger, ctx, configuration, db, change)
	if err != nil {
		metrics.EditStatus.With(prometheus.Labels{"state": "score_edit", "status": "failed_to_classify"}).Inc()
		return fmt.Errorf("failed to score vandalism: %v", err)
	}
	if !isVandalism {
		logger.Infof("Is not vandalism (scored at %f)", change.VandalismScore)
		r.SendSpam(fmt.Sprintf("%s # %f # Below threshold # Not reverted", change.FormatIrcChange(), change.VandalismScore))
		metrics.EditStatus.With(prometheus.Labels{"state": "score_edit", "status": "classified_as_not_vandalism"}).Inc()
		return nil
	}
	logger.Infof("Is vandalism (scored at %f)", change.VandalismScore)

	if isWhitelisted(logger, configuration, change.User.Username) {
		logger.Infof("User is whitelisted, not reverting")
		r.SendSpam(fmt.Sprintf("%s # %f # Whitelisted # Not reverted", change.FormatIrcChange(), change.VandalismScore))
		metrics.EditStatus.With(prometheus.Labels{"state": "score_edit", "status": "skipped_due_to_whitelist"}).Inc()
		return nil
	}
	logger.Infof("User is not whitelisted")
	metrics.EditStatus.With(prometheus.Labels{"state": "score_edit", "status": "classified_as_vandalism"}).Inc()
	outChangeFeed <- change
	return nil
}

func ProcessScoringChangeEvents(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, inChangeFeed chan *model.ProcessEvent, outChangeFeed chan *model.ProcessEvent) {
	logger := logrus.WithField("function", "processor.ProcessScoringChangeEvents")
	wg.Add(1)
	defer wg.Done()
	for change := range inChangeFeed {
		metrics.ProcessorsScoringInUse.Inc()
		ctx, span := metrics.OtelTracer.Start(context.Background(), "processor.ProcessScoringChangeEvents")
		span.SetAttributes(attribute.String("uuid", change.Uuid))

		logger = logger.WithFields(logrus.Fields{"uuid": change.Uuid})
		if err := processSingleScoringChange(logger, ctx, change, configuration, db, r, outChangeFeed); err != nil {
			logger.Error(err.Error())
			span.SetStatus(codes.Error, err.Error())
			r.SendDebug(fmt.Sprintf("%v # Failed to score change", change.FormatIrcChange()))
		}

		span.End()
		metrics.ProcessorsScoringInUse.Dec()
	}
}
