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

func loadSingleDistinctPagesCount(logger *logrus.Entry, change *model.ProcessEvent, configuration *config.Configuration, db *database.DatabaseConnection, outChangeFeed chan *model.ProcessEvent) error {
	timer := helpers.NewTimeLogger("loader.loadSingleDistinctPagesCount", map[string]interface{}{})
	defer timer.Done()

	// Load the user distinct pages count
	userDistinctPagesCount, err := db.Replica.GetUserDistinctPagesCount(logger, change.User.Username)
	if err != nil {
		// If the user has a super high edit count, then fake it out as a non-error....
		// This query will run successfully but take multiple mins, which we can't afford
		// Since the user has a super high edit count, we're going to skip reverting them anyway :shrug:
		if change.User.EditCount > 10000 {
			return nil
		}

		return err
	}

	change.User.DistinctPages = userDistinctPagesCount
	outChangeFeed <- change
	return nil
}

func LoadDistinctPagesCount(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	logger := logrus.WithField("function", "loader.LoadDistinctPagesCount")
	wg.Add(1)
	defer wg.Done()
	for {
		select {
		case change := <-inChangeFeed:
			metrics.LoaderUserDistinctPageCountInUse.Inc()
			startTime := time.Now()
			ev := libhoney.NewEvent()
			ev.AddField("cbng.function", "loader.loadSingleDistinctPagesCount")
			logger = logger.WithFields(logrus.Fields{"uuid": change.Uuid, "change": change})
			if err := loadSingleDistinctPagesCount(logger, change, configuration, db, outChangeFeed); err != nil {
				logger.Errorf(err.Error())
				ev.AddField("error", err.Error())
				r.SendDebug(fmt.Sprintf("%v # Failed to get user distinct pages count", change.FormatIrcChange()))
			}
			ev.AddField("duration_ms", time.Since(startTime).Nanoseconds()/1000000)
						if err := ev.Send(); err != nil {
				logger.Warnf("Failed to send to honeycomb: %+v", err)
			}
			metrics.LoaderUserDistinctPageCountInUse.Dec()
		}
	}
}
