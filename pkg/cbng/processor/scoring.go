package processor

import (
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/cluebotng/botng/pkg/cbng/relay"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
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

func ProcessScoringChangeEvents(wg *sync.WaitGroup, configuration *config.Configuration, r *relay.Relays, inChangeFeed chan *model.ProcessEvent, outChangeFeed chan *model.ProcessEvent) {
	wg.Add(1)
	defer wg.Done()
	for change := range inChangeFeed {
		metrics.ProcessorsScoringInUse.Inc()
		func(changeEvent *model.ProcessEvent) {
			change.EndActiveSpan()
			logger := change.Logger.WithField("function", "processor.ProcessScoringChangeEvents")

			ctx, span := metrics.OtelTracer.Start(change.TraceContext, "ProcessScoringChangeEvents")
			defer span.End()

			isVandalism, err := isVandalism(logger, ctx, configuration, change)
			if err != nil {
				metrics.EditStatus.With(prometheus.Labels{"state": "score_edit", "status": "failed_to_classify"}).Inc()
				logger.Error(err.Error())
				span.SetStatus(codes.Error, err.Error())
				r.SendDebug(fmt.Sprintf("%v # Failed to score change", change.FormatIrcChange()))
				return
			}

			if !isVandalism {
				logger.Infof("Is not vandalism (scored at %f)", change.VandalismScore)
				r.SendSpam(fmt.Sprintf("%s # %f # Below threshold # Not reverted", change.FormatIrcChange(), change.VandalismScore))
				metrics.EditStatus.With(prometheus.Labels{"state": "score_edit", "status": "classified_as_not_vandalism"}).Inc()
				return
			}
			logger.Infof("Is vandalism (scored at %f)", change.VandalismScore)

			if isWhitelisted(logger, configuration, change.User.Username) {
				logger.Infof("User is whitelisted, not reverting")
				r.SendSpam(fmt.Sprintf("%s # %f # Whitelisted # Not reverted", change.FormatIrcChange(), change.VandalismScore))
				metrics.EditStatus.With(prometheus.Labels{"state": "score_edit", "status": "skipped_due_to_whitelist"}).Inc()
				return
			}

			logger.Infof("User is not whitelisted")
			metrics.EditStatus.With(prometheus.Labels{"state": "score_edit", "status": "classified_as_vandalism"}).Inc()
			change.StartNewActiveSpan("pending.ProcessRevertChangeEvents")
			outChangeFeed <- change
		}(change)
		metrics.ProcessorsScoringInUse.Dec()
	}
}
