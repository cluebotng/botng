package processor

import (
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/database"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/codes"
	"sync"
	"time"
)

func ReplicationWatcher(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, ignoreReplicationDelay bool, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	wg.Add(1)
	defer wg.Done()

	pending := map[string]*model.ProcessEvent{}
	mutex := &sync.Mutex{}

	timer := time.NewTicker(time.Second)
	for {
		select {
		// Every second update the stats & process the pending queue
		case <-timer.C:
			logger := logrus.WithField("function", "processor.ReplicationWatcher")
			if mutex.TryLock() {
				func() {
					defer mutex.Unlock()
					metrics.ReplicationWatcherPending.Set(float64(len(pending)))

					metrics.ProcessorsReplicationWatcherInUse.Inc()
					defer metrics.ProcessorsReplicationWatcherInUse.Dec()

					var replicationPoint int64
					if !ignoreReplicationDelay {
						var err error
						if replicationPoint, err = db.Replica.GetLatestChangeTimestamp(logger); err != nil {
							logger.Warnf("Failed to get current replication point: %+v", err)
							return
						}
					}

					for _, change := range pending {
						logger := change.Logger.WithField("function", "processor.ReplicationWatcher")
						func() {
							// If we're ignoring replication or are past the change in replication, kick off the process
							if ignoreReplicationDelay || change.ChangeTime.Unix() >= replicationPoint {
								logger.Tracef("Change %v past replication point %v while pending (%v)", change.Uuid, replicationPoint, ignoreReplicationDelay)
								metrics.EditStatus.With(prometheus.Labels{"state": "wait_for_replication", "status": "success"}).Inc()
								metrics.ReplicationWatcherSuccess.Inc()

								change.StartNewActiveSpan("pending.LoadPageMetadata")
								outChangeFeed <- change
								delete(pending, change.Uuid)
								return
							}

							// If we've waited 2min, kill from pending
							if time.Now().Unix()-120 > change.ReceivedTime.Unix() {
								logger.WithFields(logrus.Fields{"uuid": change.Uuid}).Error("Change expired while pending")
								metrics.EditStatus.With(prometheus.Labels{"state": "wait_for_replication", "status": "failed"}).Inc()
								metrics.ReplicationWatcherTimout.Inc()

								change.EndActiveSpanInError(codes.Error, "Timeout while waiting for replication")
								delete(pending, change.Uuid)
								return
							}

							// Else... we're still in pending
							logger.Debugf("Change %v still pending (%v < %v)", change.Uuid, change.ReceivedTime.Unix(), replicationPoint)
						}()
					}
				}()
			}

		case change := <-inChangeFeed:
			logger := change.Logger.WithField("function", "processor.ReplicationWatcher")

			// Put the change feed into the pending map
			pending[change.Uuid] = change

			// Trigger the specific instances, if required
			if change.Common.Title == configuration.Instances.AngryOptInConfiguration.GetPageName() {
				logger.Infof("Triggering Angry Opt-in reload")
				configuration.Instances.AngryOptInConfiguration.TriggerReload()
			}
			if change.Common.Title == configuration.Instances.Run.GetPageName() {
				logger.Infof("Triggering Run reload")
				configuration.Instances.Run.TriggerReload()
			}
			if change.Common.Title == configuration.Instances.NamespaceOptIn.GetPageName() {
				logger.Infof("Triggering Namespace Opt-in reload")
				configuration.Instances.NamespaceOptIn.TriggerReload()
			}
			if change.Common.Title == configuration.Instances.TFA.GetPageName() {
				logger.Infof("Triggering TFA reload")
				configuration.Instances.TFA.TriggerReload()
			}
		}
	}
}
