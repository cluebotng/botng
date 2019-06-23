package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var ChangeEventReceived prometheus.Counter
var ChangeEventSkipped prometheus.Counter
var ChangeEventsAccepted prometheus.Counter
var ReplicationEventsPending prometheus.Gauge

var PendingReplicationWatcher prometheus.Gauge
var PendingPageMetadataLoader prometheus.Gauge
var PendingPageRecentEditCountLoader prometheus.Gauge
var PendingPageRecentRevertCountLoader prometheus.Gauge
var PendingUserEditCountLoader prometheus.Gauge
var PendingUserWarnsCountLoader prometheus.Gauge
var PendingUserDistinctPagesCountLoader prometheus.Gauge
var PendingRevisionLoader prometheus.Gauge
var PendingTriggerProcessor prometheus.Gauge
var PendingScoringProcessor prometheus.Gauge
var PendingRevertProcessor prometheus.Gauge

var PendingIrcSpamNotifications prometheus.Gauge
var PendingIrcDebugNotifications prometheus.Gauge
var PendingIrcRevertNotifications prometheus.Gauge

var ProcessorsTriggerInUse prometheus.Gauge
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

var IrcSentDebug prometheus.Counter
var IrcSentSpam prometheus.Counter
var IrcSentRevert prometheus.Counter

func init() {
	ChangeEventReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cbng_change_event_received",
		Help: "The total number of change events received from the feed",
	})
	ChangeEventSkipped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cbng_change_event_skipped",
		Help: "The total number of change events skipped from the feed",
	})
	ChangeEventsAccepted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cbng_change_event_accepted",
		Help: "The total number of change events accepted from the feed",
	})

	ReplicationEventsPending = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_change_event_pending_replication",
		Help: "The total number of change events pending replication",
	})
	PendingReplicationWatcher = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_pending_replication",
		Help: "The total number of change events pending replication to catch up",
	})

	PendingPageMetadataLoader = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_pending_page_metadata_loader",
		Help: "The total number of change events pending to be processed for page metadata loading",
	})
	PendingPageRecentEditCountLoader = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_pending_page_recent_edit_count_loader",
		Help: "The total number of change events pending to be processed for page recent edit count loading",
	})
	PendingPageRecentRevertCountLoader = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_pending_page_recent_revert_count_loader",
		Help: "The total number of change events pending to be processed for page recent revert count loading",
	})
	PendingUserEditCountLoader = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_pending_user_edit_count_loader",
		Help: "The total number of change events pending to be processed for user edit count loading",
	})
	PendingUserWarnsCountLoader = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_pending_user_warns_count_loader",
		Help: "The total number of change events pending to be processed for user warns count loading",
	})
	PendingUserDistinctPagesCountLoader = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_pending_user_distinct_pages_count_loader",
		Help: "The total number of change events pending to be processed for user distinct pages count loading",
	})
	PendingRevisionLoader = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_pending_revision_loader",
		Help: "The total number of change events pending to be processed for page revision loading",
	})
	PendingTriggerProcessor = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_pending_trigger_processor",
		Help: "The total number of change events pending to be processed for triggers",
	})
	PendingScoringProcessor = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_pending_scoring_processor",
		Help: "The total number of change events pending to be processed for scoring",
	})
	PendingRevertProcessor = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_pending_revert_processor",
		Help: "The total number of change events pending to be processed for reverts",
	})

	PendingIrcSpamNotifications = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_irc_pending_spam",
		Help: "The total number of irc spam notifications pending to be sent",
	})
	PendingIrcDebugNotifications = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_irc_pending_debug",
		Help: "The total number of irc debug notifications pending to be sent",
	})
	PendingIrcRevertNotifications = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_irc_pending_revert",
		Help: "The total number of irc revert notifications pending to be sent",
	})

	ProcessorsTriggerInUse = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_processor_active_trigger",
		Help: "The total number of trigger processors used",
	})
	ProcessorsScoringInUse = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_processor_active_scoring",
		Help: "The total number of scoring processors used",
	})
	ProcessorsRevertInUse = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_processor_active_revert",
		Help: "The total number of revert processors used",
	})
	ProcessorsReplicationWatcherInUse = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_processor_active_replication_watcher",
		Help: "The total number of replication watcher processors used",
	})

	LoaderPageMetadataInUse = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_loader_active_page_metadata",
		Help: "The total number of page metadata loaders used",
	})
	LoaderPageRecentEditCountInUse = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_loader_active_page_recent_edit_count",
		Help: "The total number of page recent edit count loaders used",
	})
	LoaderPageRecentRevertCountInUse = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_loader_active_recent_revert_count",
		Help: "The total number of page recent revert count loaders used",
	})
	LoaderUserEditCountInUse = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_loader_active_user_edit_count",
		Help: "The total number of user edit count loaders used",
	})
	LoaderUserDistinctPageCountInUse = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_loader_active_user_distinct_page_count",
		Help: "The total number of user distinct page count loaders used",
	})
	LoaderUserWarnsCountInUse = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_loader_active_user_warns_count",
		Help: "The total number of user warns count loaders used",
	})
	LoaderPageRevisionInUse = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cbng_loader_active_page_revision",
		Help: "The total number of page revision loaders used",
	})

	IrcSentDebug = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cbng_irc_sent_debug",
		Help: "The total number of debug messages sent",
	})
	IrcSentSpam = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cbng_irc_sent_spam",
		Help: "The total number of spam messages sent",
	})
	IrcSentRevert = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cbng_irc_sent_revert",
		Help: "The total number of revert messages sent",
	})
}