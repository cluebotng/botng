package main

import (
	"context"
	"crypto/tls"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/database"
	"github.com/cluebotng/botng/pkg/cbng/feed"
	"github.com/cluebotng/botng/pkg/cbng/loader"
	"github.com/cluebotng/botng/pkg/cbng/logging"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/cluebotng/botng/pkg/cbng/processor"
	"github.com/cluebotng/botng/pkg/cbng/relay"
	"github.com/cluebotng/botng/pkg/cbng/wikipedia"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
	"net/http"
	"sync"
	"time"
)

func RunMetricPoller(wg *sync.WaitGroup, toPageMetadataLoader, toPageRecentEditCountLoader, toPageRecentRevertCountLoader, toUserEditCountLoader, toUserRegistrationLoader, toUserWarnsCountLoader, toUserDistinctPagesCountLoader, toRevisionLoader, toScoringProcessor, toRevertProcessor chan *model.ProcessEvent, r *relay.Relays, db *database.DatabaseConnection) {
	wg.Add(1)
	defer wg.Done()

	timer := time.NewTicker(time.Second)
	for range timer.C {
		metrics.PendingPageMetadataLoader.Set(float64(len(toPageMetadataLoader)))
		metrics.PendingPageRecentEditCountLoader.Set(float64(len(toPageRecentEditCountLoader)))
		metrics.PendingPageRecentRevertCountLoader.Set(float64(len(toPageRecentRevertCountLoader)))
		metrics.PendingUserEditCountLoader.Set(float64(len(toUserEditCountLoader)))
		metrics.PendingUserWarnsCountLoader.Set(float64(len(toUserWarnsCountLoader)))
		metrics.PendingUserRegistrationLoader.Set(float64(len(toUserRegistrationLoader)))
		metrics.PendingUserDistinctPagesCountLoader.Set(float64(len(toUserDistinctPagesCountLoader)))
		metrics.PendingRevisionLoader.Set(float64(len(toRevisionLoader)))
		metrics.PendingScoringProcessor.Set(float64(len(toScoringProcessor)))
		metrics.PendingRevertProcessor.Set(float64(len(toRevertProcessor)))

		metrics.IrcNotificationsPending.With(prometheus.Labels{"channel": "debug"}).Set(float64(r.GetPendingDebugMessages()))
		metrics.IrcNotificationsPending.With(prometheus.Labels{"channel": "revert"}).Set(float64(r.GetPendingRevertMessages()))
		metrics.IrcNotificationsPending.With(prometheus.Labels{"channel": "spam"}).Set(float64(r.GetPendingSpamMessages()))
	}
}

func RunDatabasePurger(wg *sync.WaitGroup, db *database.DatabaseConnection) {
	wg.Add(1)
	defer wg.Done()

	timer := time.NewTicker(time.Hour)
	for range timer.C {
		func() {
			ctx, span := metrics.OtelTracer.Start(context.Background(), "DatabasePurger")
			defer span.End()
			db.ClueBot.PurgeOldRevertTimes(ctx)
		}()
	}
}

func setupTracing(configuration *config.Configuration, debugMetrics bool) {
	traceResourceOptions := []resource.Option{
		resource.WithAttributes(semconv.ServiceNameKey.String("ClueBot NG")),
	}

	traceSampler := trace.AlwaysSample()
	if !debugMetrics && configuration.Honey.SampleRate > 0 && configuration.Honey.SampleRate < 1 {
		traceSampler = trace.TraceIDRatioBased(configuration.Honey.SampleRate)
		traceResourceOptions = append(traceResourceOptions, resource.WithAttributes(attribute.Float64("SampleRate", (1/configuration.Honey.SampleRate))))
	}

	traceResource, err := resource.New(
		context.Background(),
		traceResourceOptions...,
	)
	if err != nil {
		logrus.Fatalf("failed to init otlptrace provider: %s", err)
	}

	traceProviderOptions := []trace.TracerProviderOption{
		trace.WithResource(traceResource),
		trace.WithSampler(traceSampler),
	}
	if configuration.Honey.Key != "" {
		spanExporter, err := otlptrace.New(
			context.Background(),
			otlptracehttp.NewClient(
				otlptracehttp.WithTLSClientConfig(&tls.Config{}),
				otlptracehttp.WithEndpoint("api.honeycomb.io"),
				otlptracehttp.WithHeaders(map[string]string{
					"x-honeycomb-team": configuration.Honey.Key,
				}),
				otlptracehttp.WithCompression(otlptracehttp.GzipCompression),
			),
		)
		if err != nil {
			logrus.Fatalf("failed to init otlptrace provider: %s", err)
		}
		bsp := trace.NewBatchSpanProcessor(spanExporter)
		traceProviderOptions = append(traceProviderOptions, trace.WithSpanProcessor(bsp))
	}

	if debugMetrics {
		spanExporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			logrus.Fatalf("failed to init stdouttrace provider: %e", err)
		}
		bsp := trace.NewBatchSpanProcessor(spanExporter)
		traceProviderOptions = append(traceProviderOptions, trace.WithSpanProcessor(bsp))
	}

	tp := trace.NewTracerProvider(traceProviderOptions...)
	otel.SetTracerProvider(tp)
}

func main() {
	var wg sync.WaitGroup
	var debugLogging bool
	var traceLogging bool
	var debugMetrics bool
	var useIrcRelay bool
	var ignoreReplicationDelay bool
	var processors int
	var sqlLoaders int
	var httpLoaders int
	var changeId int64

	pflag.BoolVar(&debugLogging, "debug", false, "Should we log debug info")
	pflag.BoolVar(&traceLogging, "trace", false, "Should we log trace info")
	pflag.BoolVar(&debugMetrics, "debug-metrics", false, "Should we log metrics")
	pflag.BoolVar(&useIrcRelay, "irc-relay", false, "Should we use enable the IRC relay")
	pflag.BoolVar(&ignoreReplicationDelay, "no-replication-check", false, "Should we disable the replication monitoring")
	pflag.IntVar(&processors, "processors", 5, "Number of processors to use")
	pflag.IntVar(&sqlLoaders, "sql-loaders", 20, "Number of SQL loaders to use")
	pflag.IntVar(&httpLoaders, "http-loaders", 20, "Number of HTTP loaders to use")
	pflag.Int64Var(&changeId, "process-id", 0, "Process a single ID, rather than feed")
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

	configuration := config.NewConfiguration()
	logrus.AddHook(logging.NewLogFileHook(configuration.Logging.File))

	setupTracing(configuration, debugMetrics)
	go logging.PruneOldLogFiles(&wg, configuration)

	wg.Add(1)
	go func() {
		defer wg.Done()
		http.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(":8118", nil); err != nil {
			logrus.Fatalf("failed to serve metrics: %s", err)
		}
	}()

	api := wikipedia.NewWikipediaApi(
		configuration.Wikipedia.Username,
		configuration.Wikipedia.Password,
		configuration.Bot.ReadOnly,
	)
	configuration.LoadDynamic(&wg, api)

	r := relay.NewRelays(&wg, useIrcRelay, configuration.Irc.Server, configuration.Irc.Port, configuration.Irc.Username, configuration.Irc.Password, configuration.Irc.Channel)
	db := database.NewDatabaseConnection(configuration)

	// Processing channels
	toReplicationWatcher := make(chan *model.ProcessEvent, 10000)
	toPageMetadataLoader := make(chan *model.ProcessEvent, 10000)
	toPageRecentEditCountLoader := make(chan *model.ProcessEvent, 10000)
	toPageRecentRevertCountLoader := make(chan *model.ProcessEvent, 10000)
	toUserEditCountLoader := make(chan *model.ProcessEvent, 10000)
	toUserRegistrationLoader := make(chan *model.ProcessEvent, 10000)
	toUserWarnsCountLoader := make(chan *model.ProcessEvent, 10000)
	toUserDistinctPagesCountLoader := make(chan *model.ProcessEvent, 10000)
	toRevisionLoader := make(chan *model.ProcessEvent, 10000)

	toScoringProcessor := make(chan *model.ProcessEvent, 10000)
	toRevertProcessor := make(chan *model.ProcessEvent, 10000)

	go RunMetricPoller(&wg, toPageMetadataLoader, toPageRecentEditCountLoader, toPageRecentRevertCountLoader, toUserEditCountLoader, toUserRegistrationLoader, toUserWarnsCountLoader, toUserDistinctPagesCountLoader, toRevisionLoader, toScoringProcessor, toRevertProcessor, r, db)
	go RunDatabasePurger(&wg, db)

	if changeId > 0 {
		go feed.EmitSingleEdit(api, changeId, toReplicationWatcher)
	} else {
		go feed.ConsumeHttpChangeEvents(&wg, configuration, toReplicationWatcher)
	}

	go processor.ReplicationWatcher(&wg, configuration, db, ignoreReplicationDelay, toReplicationWatcher, toPageMetadataLoader)

	for i := 0; i < sqlLoaders; i++ {
		go loader.LoadPageMetadata(&wg, db, r, toPageMetadataLoader, toPageRecentEditCountLoader)
		go loader.LoadPageRecentEditCount(&wg, db, r, toPageRecentEditCountLoader, toPageRecentRevertCountLoader)
		go loader.LoadPageRecentRevertCount(&wg, db, r, toPageRecentRevertCountLoader, toUserEditCountLoader)
		go loader.LoadUserEditCount(&wg, db, r, toUserEditCountLoader, toUserRegistrationLoader)
		go loader.LoadUserRegistrationTime(&wg, db, r, toUserRegistrationLoader, toUserWarnsCountLoader)
		go loader.LoadDistinctPagesCount(&wg, db, r, toUserWarnsCountLoader, toUserDistinctPagesCountLoader)
		go loader.LoadUserWarnsCount(&wg, db, r, toUserDistinctPagesCountLoader, toRevisionLoader)
	}

	for i := 0; i < httpLoaders; i++ {
		go loader.LoadPageRevision(&wg, api, r, toRevisionLoader, toScoringProcessor)
	}

	for i := 0; i < processors; i++ {
		go processor.ProcessScoringChangeEvents(&wg, configuration, r, toScoringProcessor, toRevertProcessor)
		go processor.ProcessRevertChangeEvents(&wg, configuration, db, r, api, toRevertProcessor)
	}

	wg.Wait()
}
