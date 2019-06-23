package main

import (
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/database"
	"github.com/cluebotng/botng/pkg/cbng/feed"
	"github.com/cluebotng/botng/pkg/cbng/helpers"
	"github.com/cluebotng/botng/pkg/cbng/loader"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/cluebotng/botng/pkg/cbng/processor"
	"github.com/cluebotng/botng/pkg/cbng/relay"
	"github.com/cluebotng/botng/pkg/cbng/wikipedia"
	"github.com/honeycombio/libhoney-go"
	"github.com/honeycombio/libhoney-go/transmission"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"gopkg.in/natefinch/lumberjack.v2"
	"net/http"
	"sync"
	"time"
)

func RunMetricPoller(wg *sync.WaitGroup, toReplicationWatcher, toPageMetadataLoader, toPageRecentEditCountLoader, toPageRecentRevertCountLoader, toUserEditCountLoader, toUserWarnsCountLoader, toUserDistinctPagesCountLoader, toRevisionLoader, toTriggerProcessor, toScoringProcessor, toRevertProcessor chan *model.ProcessEvent, r *relay.Relays) {
	wg.Add(1)
	defer wg.Done()
	for range time.Tick(time.Duration(time.Second)) {
		metrics.PendingReplicationWatcher.Set(float64(len(toReplicationWatcher)))
		metrics.PendingPageMetadataLoader.Set(float64(len(toPageMetadataLoader)))
		metrics.PendingPageRecentEditCountLoader.Set(float64(len(toPageRecentEditCountLoader)))
		metrics.PendingPageRecentRevertCountLoader.Set(float64(len(toPageRecentRevertCountLoader)))
		metrics.PendingUserEditCountLoader.Set(float64(len(toUserEditCountLoader)))
		metrics.PendingUserWarnsCountLoader.Set(float64(len(toUserWarnsCountLoader)))
		metrics.PendingUserDistinctPagesCountLoader.Set(float64(len(toUserDistinctPagesCountLoader)))
		metrics.PendingRevisionLoader.Set(float64(len(toRevisionLoader)))
		metrics.PendingTriggerProcessor.Set(float64(len(toTriggerProcessor)))
		metrics.PendingScoringProcessor.Set(float64(len(toScoringProcessor)))
		metrics.PendingRevertProcessor.Set(float64(len(toRevertProcessor)))
		metrics.PendingIrcSpamNotifications.Set(float64(r.GetPendingSpamMessages()))
		metrics.PendingIrcDebugNotifications.Set(float64(r.GetPendingDebugMessages()))
		metrics.PendingIrcRevertNotifications.Set(float64(r.GetPendingRevertMessages()))
	}
}

func main() {
	var wg sync.WaitGroup
	var debugLogging bool
	var traceLogging bool
	var useIrcRelay bool
	var ignoreReplicationDelay bool
	var processors int
	var sqlLoaders int
	var httpLoaders int

	pflag.BoolVar(&debugLogging, "debug", false, "Should we log debug info")
	pflag.BoolVar(&traceLogging, "trace", false, "Should we log trace info")
	pflag.BoolVar(&useIrcRelay, "irc-relay", false, "Should we use enable the IRC relay")
	pflag.BoolVar(&ignoreReplicationDelay, "no-replication-check", false, "Should we disable the replication monitoring")
	pflag.IntVar(&processors, "processors", 500, "Number of processors to use")
	pflag.IntVar(&sqlLoaders, "sql-loaders", 10, "Number of SQL loaders to use")
	pflag.IntVar(&httpLoaders, "http-loaders", 100, "Number of HTTP loaders to use")
	pflag.Parse()

	if traceLogging {
		logrus.SetLevel(logrus.TraceLevel)
	} else if debugLogging {
		logrus.SetLevel(logrus.DebugLevel)
	} else {
		logrus.SetLevel(logrus.InfoLevel)
	}

	logrus.SetFormatter(&logrus.JSONFormatter{
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "timestamp",
			logrus.FieldKeyFunc:  "_caller",
			logrus.FieldKeyFile:  "_file",
			logrus.FieldKeyLevel: "level",
			logrus.FieldKeyMsg:   "message",
		},
	})
	logrus.AddHook(helpers.NewLogFileHook(&lumberjack.Logger{
		Filename:   "cbng.log",
		MaxBackups: 31,
		MaxAge:     1,
		Compress:   true,
	}))

	configuration := config.NewConfiguration()

	honeyConfig := libhoney.Config{
		WriteKey: configuration.Honey.Key,
		Dataset:  "ClueBot NG",
	}
	if configuration.Honey.Key == "" {
		honeyConfig.Transmission = &transmission.DiscardSender{}
	}

	if err := libhoney.Init(honeyConfig); err != nil {
		logrus.Fatalf("Failed to init honeycomb: %v", err)
	}
	defer libhoney.Close()

	api := wikipedia.NewWikipediaApi(configuration.Wikipedia.Username, configuration.Wikipedia.Password)
	configuration.LoadDynamic(&wg, api)

	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(":8118", nil)

	r := relay.NewRelays(&wg, useIrcRelay, configuration.Irc.Relay.Server, configuration.Irc.Relay.Port, configuration.Irc.Relay.Username, configuration.Irc.Relay.Password, configuration.Irc.Relay.Channel)
	db := database.NewDatabaseConnection(configuration)

	// Processing channels
	toReplicationWatcher := make(chan *model.ProcessEvent, 10000)
	toPageMetadataLoader := make(chan *model.ProcessEvent, 10000)
	toPageRecentEditCountLoader := make(chan *model.ProcessEvent, 10000)
	toPageRecentRevertCountLoader := make(chan *model.ProcessEvent, 10000)
	toUserEditCountLoader := make(chan *model.ProcessEvent, 10000)
	toUserWarnsCountLoader := make(chan *model.ProcessEvent, 10000)
	toUserDistinctPagesCountLoader := make(chan *model.ProcessEvent, 10000)
	toRevisionLoader := make(chan *model.ProcessEvent, 10000)

	toTriggerProcessor := make(chan *model.ProcessEvent, 10000)
	toScoringProcessor := make(chan *model.ProcessEvent, 10000)
	toRevertProcessor := make(chan *model.ProcessEvent, 10000)

	go RunMetricPoller(&wg, toReplicationWatcher, toPageMetadataLoader, toPageRecentEditCountLoader, toPageRecentRevertCountLoader, toUserEditCountLoader, toUserWarnsCountLoader, toUserDistinctPagesCountLoader, toRevisionLoader, toTriggerProcessor, toScoringProcessor, toRevertProcessor, r)

	go feed.ConsumeHttpChangeEvents(&wg, configuration, toReplicationWatcher)
	go processor.ReplicationWatcher(&wg, configuration, db, ignoreReplicationDelay, toReplicationWatcher, toPageMetadataLoader)

	for i := 0; i < sqlLoaders; i++ {
		go loader.LoadPageMetadata(&wg, configuration, db, r, toPageMetadataLoader, toPageRecentEditCountLoader)
		go loader.LoadPageRecentEditCount(&wg, configuration, db, r, toPageRecentEditCountLoader, toPageRecentRevertCountLoader)
		go loader.LoadPageRecentRevertCount(&wg, configuration, db, r, toPageRecentRevertCountLoader, toUserEditCountLoader)
		go loader.LoadUserEditCount(&wg, configuration, db, r, toUserEditCountLoader, toUserWarnsCountLoader)
		go loader.LoadDistinctPagesCount(&wg, configuration, db, r, toUserWarnsCountLoader, toUserDistinctPagesCountLoader)
		go loader.LoadUserWarnsCount(&wg, configuration, db, r, toUserDistinctPagesCountLoader, toRevisionLoader)
	}

	for i := 0; i < httpLoaders; i++ {
		go loader.LoadPageRevision(&wg, api, r, toRevisionLoader, toScoringProcessor)
	}

	for i := 0; i < processors; i++ {
		go processor.ProcessTriggerChangeEvents(&wg, configuration, toTriggerProcessor, toPageMetadataLoader)
		go processor.ProcessScoringChangeEvents(&wg, configuration, db, r, toScoringProcessor, toRevertProcessor)
		go processor.ProcessRevertChangeEvents(&wg, configuration, db, r, api, toRevertProcessor)
	}

	wg.Wait()
}
