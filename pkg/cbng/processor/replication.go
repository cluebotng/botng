package processor

import (
	"context"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/database"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"sync"
	"time"
)

func ReplicationWatcher(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, ignoreReplicationDelay bool, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	logger := logrus.WithField("function", "processor.ReplicationWatcher")
	wg.Add(1)
	defer wg.Done()

	pending := map[string]*model.ProcessEvent{}
	mutex := &sync.Mutex{}

	timer := time.NewTicker(time.Second)
	for {
		select {
		// Every second update the stats & process the pending queue
		case <-timer.C:
			if mutex.TryLock() {
				func() {
					defer mutex.Unlock()
					ctx, span := metrics.OtelTracer.Start(context.Background(), "replication.ReplicationWatcher.timer")
					defer span.End()
					metrics.ReplicationWatcherPending.Set(float64(len(pending)))

					metrics.ProcessorsReplicationWatcherInUse.Inc()
					defer metrics.ProcessorsReplicationWatcherInUse.Dec()

					var replicationPoint int64
					if !ignoreReplicationDelay {
						var err error
						if replicationPoint, err = db.Replica.GetLatestChangeTimestamp(logger, ctx); err != nil {
							logger.Warnf("Failed to get current replication point: %+v", err)
							return
						}
					}

					_, loopSpan := metrics.OtelTracer.Start(ctx, "replication.ReplicationWatcher.timer.pending")
					defer loopSpan.End()
					for _, change := range pending {
						func() {
							_, span := metrics.OtelTracer.Start(context.Background(), "replication.ReplicationWatcher.pending.change")
							span.SetAttributes(attribute.String("uuid", change.Uuid))
							defer span.End()
							// If we're ignoring replication or are past the change in replication, kick off the process
							if ignoreReplicationDelay || change.ReceivedTime.Unix() >= replicationPoint {
								logger.Tracef("Change %v past replication point %v while pending (%v)", change.Uuid, replicationPoint, ignoreReplicationDelay)
								metrics.EditStatus.With(prometheus.Labels{"state": "wait_for_replication", "status": "success"}).Inc()
								metrics.ReplicationWatcherSuccess.Inc()
								outChangeFeed <- change
								delete(pending, change.Uuid)
								return
							}

							// If we've waited 2min, kill from pending
							if time.Now().Unix()-120 > change.ReceivedTime.Unix() {
								logger.WithFields(logrus.Fields{"uuid": change.Uuid}).Error("Change expired while pending")
								span.SetStatus(codes.Error, "Timeout while waiting for replication")
								metrics.EditStatus.With(prometheus.Labels{"state": "wait_for_replication", "status": "failed"}).Inc()
								metrics.ReplicationWatcherTimout.Inc()
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
}
