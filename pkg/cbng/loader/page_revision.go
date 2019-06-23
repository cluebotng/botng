package loader

import (
	"errors"
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/helpers"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/cluebotng/botng/pkg/cbng/relay"
	"github.com/cluebotng/botng/pkg/cbng/wikipedia"
	"github.com/honeycombio/libhoney-go"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

func loadSinglePageRevision(logger *logrus.Entry, change *model.ProcessEvent, api *wikipedia.WikipediaApi, outChangeFeed chan *model.ProcessEvent) error {
	timer := helpers.NewTimeLogger("loader.loadSinglePageRevision", map[string]interface{}{})
	defer timer.Done()

	// Pull the revisions from the API
	revisionData := api.GetRevision(logger, change.Common.Title, change.Current.Id)
	logger = logrus.WithField("revision", revisionData)
	if revisionData == nil ||
		revisionData.Current.Timestamp == 0 ||
		revisionData.Current.Data == "" ||
		revisionData.Previous.Timestamp == 0 ||
		revisionData.Previous.Data == "" {
		return errors.New("failed to get complete revision data")
	}

	change.Current = model.ProcessEventRevision{
		Timestamp: revisionData.Current.Timestamp,
		Text:      revisionData.Current.Data,
		Id:        revisionData.Current.Id,
	}
	change.Previous = model.ProcessEventRevision{
		Timestamp: revisionData.Previous.Timestamp,
		Text:      revisionData.Previous.Data,
		Id:        revisionData.Previous.Id,
	}

	outChangeFeed <- change
	return nil
}

func LoadPageRevision(wg *sync.WaitGroup, api *wikipedia.WikipediaApi, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {
	logger := logrus.WithField("function", "loader.LoadPageRevision")
	wg.Add(1)
	defer wg.Done()
	for {
		select {
		case change := <-inChangeFeed:
			metrics.LoaderPageRevisionInUse.Inc()
			startTime := time.Now()
			ev := libhoney.NewEvent()
			ev.AddField("cbng.function", "loader.loadSinglePageRevision")
			logger = logger.WithFields(logrus.Fields{"uuid": change.Uuid, "change": change})
			if err := loadSinglePageRevision(logger, change, api, outChangeFeed); err != nil {
				logger.Errorf(err.Error())
				ev.AddField("error", err.Error())
				r.SendDebug(fmt.Sprintf("%v # Failed to get page revision", change.FormatIrcChange()))
			}
			ev.AddField("duration_ms", time.Since(startTime).Nanoseconds()/1000000)
						if err := ev.Send(); err != nil {
				logger.Warnf("Failed to send to honeycomb: %+v", err)
			}
			metrics.LoaderPageRevisionInUse.Dec()
		}
	}
}
