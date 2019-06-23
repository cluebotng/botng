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

func loadSingleUserEditCount(logger *logrus.Entry, change *model.ProcessEvent, configuration *config.Configuration, db *database.DatabaseConnection, outChangeFeed chan *model.ProcessEvent) error {
	timer := helpers.NewTimeLogger("loader.loadSingleUserEditCount", map[string]interface{}{})
	defer timer.Done()

	// Load the user edit count
	userEditCount, err := db.Replica.GetUserEditCount(logger, change.User.Username)
	if err != nil {
		return err
	}

	// Load the user registration time
	userRegTime, err := db.Replica.GetUserRegistrationTime(logger, change.User.Username)
	if err != nil {
		return err
	}

	change.User.EditCount = userEditCount
	change.User.RegistrationTime = userRegTime
	outChangeFeed <- change
	return nil
}

func LoadUserEditCount(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	logger := logrus.WithField("function", "loader.LoadUserEditCount")
	wg.Add(1)
	defer wg.Done()
	for {
		select {
		case change := <-inChangeFeed:
			metrics.LoaderUserEditCountInUse.Inc()
			startTime := time.Now()
			ev := libhoney.NewEvent()
			ev.AddField("cbng.function", "loader.loadSingleUserEditCount")
			logger = logger.WithFields(logrus.Fields{"uuid": change.Uuid, "change": change})
			if err := loadSingleUserEditCount(logger, change, configuration, db, outChangeFeed); err != nil {
				logger.Errorf(err.Error())
				ev.AddField("error", err.Error())
				r.SendDebug(fmt.Sprintf("%v # Failed to get user edit count", change.FormatIrcChange()))
			}
			ev.AddField("duration_ms", time.Since(startTime).Nanoseconds()/1000000)
						if err := ev.Send(); err != nil {
				logger.Warnf("Failed to send to honeycomb: %+v", err)
			}
			metrics.LoaderUserEditCountInUse.Dec()
		}
	}
}
