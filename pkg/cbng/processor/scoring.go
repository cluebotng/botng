package processor

import (
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/database"
	"github.com/cluebotng/botng/pkg/cbng/helpers"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/cluebotng/botng/pkg/cbng/relay"
	"github.com/honeycombio/libhoney-go"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

func isWhitelisted(l *logrus.Entry, configuration *config.Configuration, user string) bool {
	logger := l.WithFields(logrus.Fields{
		"function": "processor.isWhitelisted",
		"args": map[string]interface{}{
			"user": user,
		},
	})
	timer := helpers.NewTimeLogger("processor.isWhitelisted", map[string]interface{}{
		"user": user,
	})
	defer timer.Done()

	for _, wuser := range configuration.Dynamic.HuggleUserWhitelist {
		if user == wuser {
			logger.Infof("Found user whitelisted in Huggle")
			return true
		}
	}
	return false
}

func processSingleScoringChange(logger *logrus.Entry, change *model.ProcessEvent, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, outChangeFeed chan *model.ProcessEvent) error {
	logger = logger.WithFields(logrus.Fields{
		"change": change,
	})

	isVandalism, err := isVandalism(logger, configuration, db, change)
	if err != nil {
		return fmt.Errorf("Failed to score vandalism: %v", err)
	}
	if !isVandalism {
		logger.Infof("Is not vandalism (scored at %f)", change.VandalismScore)
		r.SendSpam(fmt.Sprintf("%s # %f # Below threshold # Not reverted", change.FormatIrcChange(), change.VandalismScore))
		return nil
	}
	logger.Infof("Is vandalism (scored at %f)", change.VandalismScore)

	if isWhitelisted(logger, configuration, change.User.Username) {
		logger.Infof("User is whitelisted, not reverting")
		r.SendSpam(fmt.Sprintf("%s # %f # Whitelisted # Not reverted", change.FormatIrcChange(), change.VandalismScore))
		return nil
	}
	logger.Infof("User is not whitelisted")
	outChangeFeed <- change
	return nil
}

func ProcessScoringChangeEvents(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, inChangeFeed chan *model.ProcessEvent, outChangeFeed chan *model.ProcessEvent) {
	logger := logrus.WithField("function", "processor.ProcessScoringChangeEvents")
	wg.Add(1)
	defer wg.Done()
	for {
		select {
		case change := <-inChangeFeed:
			metrics.ProcessorsScoringInUse.Inc()
			startTime := time.Now()
			ev := libhoney.NewEvent()
			ev.AddField("cbng.function", "processor.processSingleChange")
			logger = logger.WithFields(logrus.Fields{"uuid": change.Uuid, "change": change})
			if err := processSingleScoringChange(logger, change, configuration, db, r, outChangeFeed); err != nil {
				logger.Errorf(err.Error())
				ev.AddField("error", err.Error())
				r.SendDebug(fmt.Sprintf("%v # Failed to score change", change.FormatIrcChange()))
			}
			ev.AddField("duration_ms", time.Since(startTime).Nanoseconds()/1000000)
						if err := ev.Send(); err != nil {
				logger.Warnf("Failed to send to honeycomb: %+v", err)
			}
			metrics.ProcessorsScoringInUse.Dec()
		}
	}
}
