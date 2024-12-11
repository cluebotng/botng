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
	"github.com/honeycombio/otel-config-go/otelconfig"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"gopkg.in/natefinch/lumberjack.v2"
	"net/http"
	"os"
	"sync"
	"time"
)

func RunMetricPoller(wg *sync.WaitGroup, toPageMetadataLoader, toPageRecentEditCountLoader, toPageRecentRevertCountLoader, toUserEditCountLoader, toUserWarnsCountLoader, toUserDistinctPagesCountLoader, toRevisionLoader, toScoringProcessor, toRevertProcessor chan *model.ProcessEvent, r *relay.Relays, db *database.DatabaseConnection) {
	wg.Add(1)
	defer wg.Done()

	timer := time.NewTicker(time.Second)
	for range timer.C {
		metrics.PendingPageMetadataLoader.Set(float64(len(toPageMetadataLoader)))
		metrics.PendingPageRecentEditCountLoader.Set(float64(len(toPageRecentEditCountLoader)))
		metrics.PendingPageRecentRevertCountLoader.Set(float64(len(toPageRecentRevertCountLoader)))
		metrics.PendingUserEditCountLoader.Set(float64(len(toUserEditCountLoader)))
		metrics.PendingUserWarnsCountLoader.Set(float64(len(toUserWarnsCountLoader)))
		metrics.PendingUserDistinctPagesCountLoader.Set(float64(len(toUserDistinctPagesCountLoader)))
		metrics.PendingRevisionLoader.Set(float64(len(toRevisionLoader)))
		metrics.PendingScoringProcessor.Set(float64(len(toScoringProcessor)))
		metrics.PendingRevertProcessor.Set(float64(len(toRevertProcessor)))

		metrics.IrcNotificationsPending.With(prometheus.Labels{"channel": "debug"}).Set(float64(r.GetPendingDebugMessages()))
		metrics.IrcNotificationsPending.With(prometheus.Labels{"channel": "revert"}).Set(float64(r.GetPendingRevertMessages()))
		metrics.IrcNotificationsPending.With(prometheus.Labels{"channel": "spam"}).Set(float64(r.GetPendingSpamMessages()))

		db.UpdateMetrics()
	}
}

func RunDatabasePurger(wg *sync.WaitGroup, db *database.DatabaseConnection) {
	wg.Add(1)
	defer wg.Done()

	timer := time.NewTicker(time.Hour)
	for range timer.C {
		db.ClueBot.PurgeOldRevertTimes()
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
	pflag.IntVar(&processors, "processors", 20, "Number of processors to use")
	pflag.IntVar(&sqlLoaders, "sql-loaders", 150, "Number of SQL loaders to use")
	pflag.IntVar(&httpLoaders, "http-loaders", 150, "Number of HTTP loaders to use")
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

	logFile := "botng.log"
	if value, ok := os.LookupEnv("BOTNG_LOG"); ok {
		logFile = value
	}
	logrus.AddHook(helpers.NewLogFileHook(&lumberjack.Logger{
		Filename:   logFile,
		MaxBackups: 31,
		MaxAge:     1,
		Compress:   true,
	}))

	configuration := config.NewConfiguration()

	wg.Add(1)
	go func() {
		defer wg.Done()
		http.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(":8118", nil); err != nil {
			logrus.Fatalf("failed to serve metrics: %s", err)
		}
	}()

	var otelOptions = []otelconfig.Option{
		otelconfig.WithServiceName("ClueBot NG"),
	}
	if configuration.Honey.Key != "" {
		otelOptions = append(otelOptions, otelconfig.WithExporterProtocol(otelconfig.ProtocolHTTPProto))
		otelOptions = append(otelOptions, otelconfig.WithExporterEndpoint("https://api.honeycomb.io"))
		otelOptions = append(otelOptions, otelconfig.WithHeaders(map[string]string{
			"x-honeycomb-team": configuration.Honey.Key,
		}))
	} else {
		otelOptions = append(otelOptions, otelconfig.WithTracesEnabled(false))
		otelOptions = append(otelOptions, otelconfig.WithMetricsEnabled(false))
	}

	otelShutdown, err := otelconfig.ConfigureOpenTelemetry(otelOptions...)
	if err != nil {
		logrus.Fatalf("failed to init OTel SDK: %e", err)
	}
	defer otelShutdown()

	api := wikipedia.NewWikipediaApi(
		configuration.Wikipedia.Username,
		configuration.Wikipedia.Password,
		configuration.Bot.ReadOnly,
	)
	configuration.LoadDynamic(&wg, api)

	r := relay.NewRelays(&wg, useIrcRelay, configuration.Irc.Server, configuration.Irc.Port, configuration.Irc.Username, configuration.Irc.Password, configuration.Irc.Channel)
	db := database.NewDatabaseConnection(configuration)
	defer db.Disconnect()

	// Processing channels
	toReplicationWatcher := make(chan *model.ProcessEvent, 10000)
	toPageMetadataLoader := make(chan *model.ProcessEvent, 10000)
	toPageRecentEditCountLoader := make(chan *model.ProcessEvent, 10000)
	toPageRecentRevertCountLoader := make(chan *model.ProcessEvent, 10000)
	toUserEditCountLoader := make(chan *model.ProcessEvent, 10000)
	toUserWarnsCountLoader := make(chan *model.ProcessEvent, 10000)
	toUserDistinctPagesCountLoader := make(chan *model.ProcessEvent, 10000)
	toRevisionLoader := make(chan *model.ProcessEvent, 10000)

	toScoringProcessor := make(chan *model.ProcessEvent, 10000)
	toRevertProcessor := make(chan *model.ProcessEvent, 10000)

	go RunMetricPoller(&wg, toPageMetadataLoader, toPageRecentEditCountLoader, toPageRecentRevertCountLoader, toUserEditCountLoader, toUserWarnsCountLoader, toUserDistinctPagesCountLoader, toRevisionLoader, toScoringProcessor, toRevertProcessor, r, db)
	go RunDatabasePurger(&wg, db)

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
		go processor.ProcessScoringChangeEvents(&wg, configuration, db, r, toScoringProcessor, toRevertProcessor)
		go processor.ProcessRevertChangeEvents(&wg, configuration, db, r, api, toRevertProcessor)
	}

	wg.Wait()
}
