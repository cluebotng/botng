package processor

import (
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/honeycombio/libhoney-go"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

func processSingleTriggerChange(logger *logrus.Entry, change *model.ProcessEvent, configuration *config.Configuration, outChangeFeed chan *model.ProcessEvent) {
	// Configuration pages
	if change.Common.Namespace == "User" && change.Common.Title == fmt.Sprintf("User:%s/Run", configuration.Wikipedia.Username) {
		logger.Infof("Triggering run configuration reload due to page change: %v", change.Common.Title)
		configuration.Instances.Run.TriggerReload()
	}

	if change.Common.Namespace == "User" && change.Common.Title == fmt.Sprintf("User:%s/Optin", configuration.Wikipedia.Username) {
		logger.Infof("Triggering namespace opt-in configuration reload due to page change: %v", change.Common.Title)
		configuration.Instances.NamespaceOptIn.TriggerReload()
	}

	if change.Common.Namespace == "User" && change.Common.Title == fmt.Sprintf("User:%s/AngryOptin", configuration.Wikipedia.Username) {
		logger.Infof("Triggering angry opt-in configuration reload due to page change: %v", change.Common.Title)
		configuration.Instances.AngryOptInConfiguration.TriggerReload()
	}

	// Pre-filtering passed
	metrics.ChangeEventsAccepted.Inc()
	outChangeFeed <- change
}

func ProcessTriggerChangeEvents(wg *sync.WaitGroup, configuration *config.Configuration, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	logger := logrus.WithField("function", "processor.ProcessTriggerChangeEvents")
	wg.Add(1)
	defer wg.Done()
	for {
		select {
		case change := <-inChangeFeed:
			metrics.ProcessorsTriggerInUse.Inc()
			startTime := time.Now()
			ev := libhoney.NewEvent()
			ev.AddField("cbng.function", "processor.processSingleChange")
			logger = logger.WithFields(logrus.Fields{"uuid": change.Uuid, "change": change})
			processSingleTriggerChange(logger, change, configuration, outChangeFeed)
			ev.AddField("duration_ms", time.Since(startTime).Nanoseconds()/1000000)
						if err := ev.Send(); err != nil {
				logger.Warnf("Failed to send to honeycomb: %+v", err)
			}
			metrics.ProcessorsTriggerInUse.Dec()
		}
	}
}
