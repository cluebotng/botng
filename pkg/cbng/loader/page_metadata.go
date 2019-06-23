package loader

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

func loadSinglePageMetadata(logger *logrus.Entry, change *model.ProcessEvent, configuration *config.Configuration, db *database.DatabaseConnection, outChangeFeed chan *model.ProcessEvent) error {
	timer := helpers.NewTimeLogger("loader.loadSinglePageMetadata", map[string]interface{}{})
	defer timer.Done()

	// Skip namespaces we're not interested in
	if change.Common.NamespaceId != 0 && !helpers.StringItemInSlice(change.Common.Namespace, configuration.Dynamic.NamespaceOptIn) {
		logger.Debugf("Skipping change due to namespace: %v (%v)", change.Common.Namespace, change.Common.NamespaceId)
		metrics.ChangeEventSkipped.Inc()
		return nil
	}

	// Load the page created metadata
	pageCreatedUser, pageCreatedTimestamp, err := db.Replica.GetPageCreatedTimeAndUser(logger, change.Common.NamespaceId, helpers.PageTitleWithoutNamespace(change.Common.Title))
	if err != nil {
		return err
	}

	change.Common.Creator = pageCreatedUser
	change.Common.PageMadeTime = pageCreatedTimestamp
	outChangeFeed <- change
	return nil
}

func LoadPageMetadata(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	logger := logrus.WithField("function", "loader.LoadPageMetadata")
	wg.Add(1)
	defer wg.Done()
	for {
		select {
		case change := <-inChangeFeed:
			metrics.LoaderPageMetadataInUse.Inc()
			startTime := time.Now()
			ev := libhoney.NewEvent()
			ev.AddField("cbng.function", "loader.loadSinglePageMetadata")
			logger = logger.WithFields(logrus.Fields{"uuid": change.Uuid, "change": change})
			if err := loadSinglePageMetadata(logger, change, configuration, db, outChangeFeed); err != nil {
				logger.Errorf(err.Error())
				ev.AddField("error", err.Error())
				r.SendDebug(fmt.Sprintf("%v # Failed to get page metadata", change.FormatIrcChange()))
			}
			ev.AddField("duration_ms", time.Since(startTime).Nanoseconds()/1000000)
						if err := ev.Send(); err != nil {
				logger.Warnf("Failed to send to honeycomb: %+v", err)
			}
			metrics.LoaderPageMetadataInUse.Dec()
		}
	}
}
