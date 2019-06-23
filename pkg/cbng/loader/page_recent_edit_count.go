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

func loadSinglePageRecentEditCount(logger *logrus.Entry, change *model.ProcessEvent, configuration *config.Configuration, db *database.DatabaseConnection, outChangeFeed chan *model.ProcessEvent) error {
	timer := helpers.NewTimeLogger("loader.loadSinglePageRecentEditCount", map[string]interface{}{})
	defer timer.Done()

	// Load the page recent edit count
	pageRecentEditCount, err := db.Replica.GetPageRecentEditCount(logger, change.Common.NamespaceId, helpers.PageTitleWithoutNamespace(change.Common.Title), change.StartTime.Unix())
	if err != nil {
		return err
	}

	change.Common.NumRecentEdits = pageRecentEditCount
	outChangeFeed <- change
	return nil
}

func LoadPageRecentEditCount(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	logger := logrus.WithField("function", "loader.LoadPageRecentEditCount")
	wg.Add(1)
	defer wg.Done()
	for {
		select {
		case change := <-inChangeFeed:
			metrics.LoaderPageRecentEditCountInUse.Inc()
			startTime := time.Now()
			ev := libhoney.NewEvent()
			ev.AddField("cbng.function", "loader.loadSinglePageRecentEditCount")
			logger = logger.WithFields(logrus.Fields{"uuid": change.Uuid, "change": change})
			if err := loadSinglePageRecentEditCount(logger, change, configuration, db, outChangeFeed); err != nil {
				logger.Errorf(err.Error())
				ev.AddField("error", err.Error())
				r.SendDebug(fmt.Sprintf("%v # Failed to get page recent edit count", change.FormatIrcChange()))
			}
			ev.AddField("duration_ms", time.Since(startTime).Nanoseconds()/1000000)
						if err := ev.Send(); err != nil {
				logger.Warnf("Failed to send to honeycomb: %+v", err)
			}
			metrics.LoaderPageRecentEditCountInUse.Dec()
		}
	}
}
