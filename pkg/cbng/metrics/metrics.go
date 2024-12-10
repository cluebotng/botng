package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var ReplicationWatcherPending prometheus.Gauge
var ReplicationWatcherTimout prometheus.Counter
var ReplicationWatcherSuccess prometheus.Counter

var PendingPageMetadataLoader prometheus.Gauge
var PendingPageRecentEditCountLoader prometheus.Gauge
var PendingPageRecentRevertCountLoader prometheus.Gauge
var PendingUserEditCountLoader prometheus.Gauge
var PendingUserWarnsCountLoader prometheus.Gauge
var PendingUserDistinctPagesCountLoader prometheus.Gauge
var PendingRevisionLoader prometheus.Gauge
var PendingScoringProcessor prometheus.Gauge
var PendingRevertProcessor prometheus.Gauge

var IrcNotificationsPending *prometheus.GaugeVec
var IrcNotificationsSent *prometheus.CounterVec

var EditStatus *prometheus.CounterVec
var RevertStatus *prometheus.CounterVec

var ProcessorsScoringInUse prometheus.Gauge
var ProcessorsRevertInUse prometheus.Gauge
var ProcessorsReplicationWatcherInUse prometheus.Gauge

var LoaderPageMetadataInUse prometheus.Gauge
var LoaderPageRecentEditCountInUse prometheus.Gauge
var LoaderPageRecentRevertCountInUse prometheus.Gauge
var LoaderUserEditCountInUse prometheus.Gauge
var LoaderUserDistinctPageCountInUse prometheus.Gauge
var LoaderUserWarnsCountInUse prometheus.Gauge
var LoaderPageRevisionInUse prometheus.Gauge

var ReplicaStats *prometheus.GaugeVec

var OtelTracer trace.Tracer

func init() {
	OtelTracer = otel.Tracer("ClueBot NG")

	EditStatus = promauto.NewCounterVec(prometheus.CounterOpts{Name: "cbng_event_state"}, []string{"state", "status"})
	RevertStatus = promauto.NewCounterVec(prometheus.CounterOpts{Name: "cbng_revert_state"}, []string{"state", "status", "meta"})

	PendingPageMetadataLoader = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_loader", ConstLabels: prometheus.Labels{"status": "pending", "loader": "page_metadata"}})
	PendingPageRecentEditCountLoader = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_loader", ConstLabels: prometheus.Labels{"status": "pending", "loader": "page_recent_edit_count"}})
	PendingPageRecentRevertCountLoader = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_loader", ConstLabels: prometheus.Labels{"status": "pending", "loader": "page_recent_revert_count"}})
	PendingUserEditCountLoader = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_loader", ConstLabels: prometheus.Labels{"status": "pending", "loader": "user_edit_count"}})
	PendingUserDistinctPagesCountLoader = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_loader", ConstLabels: prometheus.Labels{"status": "pending", "loader": "user_distinct_page_count"}})
	PendingUserWarnsCountLoader = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_loader", ConstLabels: prometheus.Labels{"status": "pending", "loader": "user_warns_count"}})
	PendingRevisionLoader = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_loader", ConstLabels: prometheus.Labels{"status": "pending", "loader": "page_revisions"}})

	LoaderPageMetadataInUse = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_loader", ConstLabels: prometheus.Labels{"status": "active", "loader": "page_metadata"}})
	LoaderPageRecentEditCountInUse = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_loader", ConstLabels: prometheus.Labels{"status": "active", "loader": "page_recent_edit_count"}})
	LoaderPageRecentRevertCountInUse = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_loader", ConstLabels: prometheus.Labels{"status": "active", "loader": "page_recent_revert_count"}})
	LoaderUserEditCountInUse = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_loader", ConstLabels: prometheus.Labels{"status": "active", "loader": "user_edit_count"}})
	LoaderUserDistinctPageCountInUse = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_loader", ConstLabels: prometheus.Labels{"status": "active", "loader": "user_distinct_page_count"}})
	LoaderUserWarnsCountInUse = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_loader", ConstLabels: prometheus.Labels{"status": "active", "loader": "user_warns_count"}})
	LoaderPageRevisionInUse = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_loader", ConstLabels: prometheus.Labels{"status": "active", "loader": "page_revisions"}})

	PendingScoringProcessor = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_processor", ConstLabels: prometheus.Labels{"status": "pending", "processor": "scoring"}})
	PendingRevertProcessor = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_processor", ConstLabels: prometheus.Labels{"status": "pending", "processor": "revert"}})

	ProcessorsScoringInUse = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_processor", ConstLabels: prometheus.Labels{"status": "active", "processor": "scoring"}})
	ProcessorsRevertInUse = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_processor", ConstLabels: prometheus.Labels{"status": "active", "processor": "revert"}})
	ProcessorsReplicationWatcherInUse = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_processor", ConstLabels: prometheus.Labels{"status": "active", "processor": "replication"}})

	ReplicationWatcherPending = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_database_replication", ConstLabels: prometheus.Labels{"status": "pending"}})
	ReplicationWatcherTimout = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_database_replication", ConstLabels: prometheus.Labels{"status": "timeout"}})
	ReplicationWatcherSuccess = promauto.NewGauge(prometheus.GaugeOpts{Name: "cbng_database_replication", ConstLabels: prometheus.Labels{"status": "success"}})

	IrcNotificationsPending = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "cbng_irc_notifications_pending"}, []string{"channel"})
	IrcNotificationsSent = promauto.NewCounterVec(prometheus.CounterOpts{Name: "cbng_irc_notifications_sent"}, []string{"channel"})

	ReplicaStats = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "cbng_database_replica"}, []string{"instance", "metric"})
}
