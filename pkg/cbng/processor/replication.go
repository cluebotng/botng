package processor

import (
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/database"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

func ReplicationWatcher(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, ignoreReplicationDelay bool, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	logger := logrus.WithField("function", "processor.ReplicationWatcher")
	wg.Add(1)
	defer wg.Done()

	pending := map[string]*model.ProcessEvent{}

	for {
		select {
		// Every second update the stats & process the pending queue
		case <-time.Tick(time.Duration(time.Second)):
			metrics.ReplicationEventsPending.Set(float64(len(pending)))

			metrics.ProcessorsReplicationWatcherInUse.Inc()
			defer metrics.ProcessorsReplicationWatcherInUse.Dec()

			var replicationPoint int64
			if !ignoreReplicationDelay {
				var err error
				if replicationPoint, err = db.Replica.GetLatestChangeTimestamp(logger); err != nil {
					logger.Warnf("Failed to get current replication point: %+v", err)
					continue
				}
			}

			for _, change := range pending {
				// If we're ignoring replication or are past the change in replication, kick off the process
				if ignoreReplicationDelay || change.StartTime.Unix() >= replicationPoint {
					logger.Debugf("Change %v past replication point %v while pending (%v)", change.Uuid, replicationPoint, ignoreReplicationDelay)
					outChangeFeed <- change
					delete(pending, change.Uuid)
					continue
				}

				// If we've waited 30 seconds, kill from pending
				if time.Now().Unix()-30 > change.StartTime.Unix() {
					logger.Warnf("Change %v expired while pending", change.Uuid)
					delete(pending, change.Uuid)
					continue
				}

				// Else... we're still in pending
				logger.Debugf("Change %v still pending (%v < %v)", change.Uuid, change.StartTime.Unix(), replicationPoint)
			}

		case change := <-inChangeFeed:
			// Put the change feed into the pending map
			pending[change.Uuid] = change

			// Trigger the specific instances, if required
			if change.Common.Title == configuration.Instances.AngryOptInConfiguration.GetPageName() {
				configuration.Instances.AngryOptInConfiguration.TriggerReload()
			}
			if change.Common.Title == configuration.Instances.Run.GetPageName() {
				configuration.Instances.Run.TriggerReload()
			}
			if change.Common.Title == configuration.Instances.NamespaceOptIn.GetPageName() {
				configuration.Instances.NamespaceOptIn.TriggerReload()
			}
			if change.Common.Title == configuration.Instances.TFA.GetPageName() {
				configuration.Instances.TFA.TriggerReload()
			}
		}
	}
	fmt.Printf("REPLICATION WATCHER DIED\n")
}
