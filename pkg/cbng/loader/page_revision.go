package loader

import (
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/cluebotng/botng/pkg/cbng/relay"
	"github.com/cluebotng/botng/pkg/cbng/wikipedia"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/codes"
	"sync"
)

func LoadPageRevision(wg *sync.WaitGroup, api *wikipedia.WikipediaApi, r *relay.Relays, inChangeFeed, outChangeFeed chan *model.ProcessEvent) {

	defer wg.Done()
	for change := range inChangeFeed {
		metrics.LoaderPageRevisionInUse.Inc()
		func(changeEvent *model.ProcessEvent) {
			change.EndActiveSpan()
			logger := change.Logger.WithField("function", "loader.LoadPageRevision")

			ctx, span := metrics.OtelTracer.Start(change.TraceContext, "LoadPageRevision")
			defer span.End()

			revisionData := api.GetRevision(logger, ctx, change.Common.Title, change.Current.Id)
			if revisionData == nil ||
				revisionData.Current.Timestamp == 0 ||
				revisionData.Current.Data == "" ||
				revisionData.Previous.Timestamp == 0 ||
				revisionData.Previous.Data == "" {
				metrics.EditStatus.With(prometheus.Labels{"state": "lookup_page_revisions", "status": "failed"}).Inc()
				logger.Error("failed to get complete revision data")
				span.SetStatus(codes.Error, "failed to get complete revision data")
				r.SendDebug(fmt.Sprintf("%v # Failed to get page revision", change.FormatIrcChange()))
			} else {
				change.Current = model.ProcessEventRevision{
					Timestamp: revisionData.Current.Timestamp,
					Text:      revisionData.Current.Data,
					Id:        revisionData.Current.Id,
					Username:  revisionData.Current.User,
				}
				change.Previous = model.ProcessEventRevision{
					Timestamp: revisionData.Previous.Timestamp,
					Text:      revisionData.Previous.Data,
					Id:        revisionData.Previous.Id,
					Username:  revisionData.Previous.User,
				}

				metrics.EditStatus.With(prometheus.Labels{"state": "lookup_page_revisions", "status": "success"}).Inc()
				change.StartNewActiveSpan("pending.ProcessScoringChangeEvents")
				outChangeFeed <- change
			}
		}(change)
		metrics.LoaderPageRevisionInUse.Dec()
	}
}
