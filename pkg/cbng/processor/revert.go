package processor

import (
	"context"
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/database"
	"github.com/cluebotng/botng/pkg/cbng/helpers"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/cluebotng/botng/pkg/cbng/relay"
	"github.com/cluebotng/botng/pkg/cbng/wikipedia"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"regexp"
	"strings"
	"sync"
	"time"
)

func revertChange(l *logrus.Entry, parentCtx context.Context, api *wikipedia.WikipediaApi, change *model.ProcessEvent, configuration *config.Configuration, mysqlVandalismId int64) bool {
	logger := l.WithFields(logrus.Fields{
		"function": "processor.revertChange",
		"args": map[string]interface{}{
			"mysqlVandalismId": mysqlVandalismId,
		},
	})
	ctx, span := metrics.OtelTracer.Start(parentCtx, "revert.revertChange")
	defer span.End()

	var revertRevision *wikipedia.Revision
	for _, revision := range *api.GetRevisionHistory(logger, ctx, change.Common.Title, change.Current.Id) {
		if revision.User != change.User.Username {
			revertRevision = &revision
			break
		}
	}
	if revertRevision == nil {
		logger.Infof("Failed to find revert revision")
		metrics.RevertStatus.With(prometheus.Labels{"state": "revert", "status": "failed", "meta": "lookup_revision"}).Inc()
		return false
	}

	if change.User.Username == configuration.Wikipedia.Username || helpers.StringItemInSlice(change.User.Username, configuration.Bot.Friends) {
		logger.Infof("Revert revision is self or a friend: %v", revertRevision)
		metrics.RevertStatus.With(prometheus.Labels{"state": "revert", "status": "failed", "meta": "revision_is_friend"}).Inc()
		return false
	}

	revComment := "older version"
	if revertRevision.Id != 0 {
		metrics.RevertStatus.With(prometheus.Labels{"state": "revert", "status": "failed", "meta": "revision_is_old"}).Inc()
		revComment = fmt.Sprintf("version by %s", revertRevision.User)
	}

	comment := fmt.Sprintf("Reverting possible vandalism by [[Special:Contribs/%s|%s]] to %s. [[WP:CBFP|Report False Positive?]] Thanks, [[WP:%s|%s]]. (%d) (Bot)",
		change.User.Username, change.User.Username,
		revComment,
		configuration.Wikipedia.Username,
		configuration.Wikipedia.Username,
		mysqlVandalismId,
	)

	if !api.Rollback(logger, ctx, helpers.PageTitle(change.Common.Namespace, change.Common.Title), change.User.Username, comment) {
		metrics.RevertStatus.With(prometheus.Labels{"state": "revert", "status": "failed", "meta": "api"}).Inc()
		return false
	}

	metrics.RevertStatus.With(prometheus.Labels{"state": "revert", "status": "success", "meta": ""}).Inc()
	return true
}

func doWarn(l *logrus.Entry, parentCtx context.Context, api *wikipedia.WikipediaApi, r *relay.Relays, change *model.ProcessEvent, configuration *config.Configuration, mysqlVandalismId int64) bool {
	logger := l.WithFields(logrus.Fields{
		"function": "processor.doWarn",
		"args": map[string]interface{}{
			"mysqlVandalismId": mysqlVandalismId,
		},
	})
	ctx, span := metrics.OtelTracer.Start(parentCtx, "revert.doWarn")
	defer span.End()

	report := fmt.Sprintf("[[%s]] was [%s changed] by [[Special:Contributions/%s|%s]] [[User:%s|(u)]] [[User talk:%s|(t)]] ANN scored at %f on %s",
		strings.ReplaceAll("File:", ":File:", change.TitleWithNamespace()),
		change.GetDiffUrl(),
		change.User.Username, change.User.Username,
		change.User.Username,
		change.User.Username,
		change.VandalismScore,
		time.Now().Format(time.RFC3339),
	)

	warningLevel := api.GetWarningLevel(logger, ctx, change.User.Username)
	logger.Infof("Found current warning level for user: %v", warningLevel)
	if warningLevel >= 4 {
		page := api.GetPage(logger, ctx, "Wikipedia:Administrator_intervention_against_vandalism/TB2")
		if page == nil {
			logger.Warnf("Failed to fetch current AIV")
			metrics.EditStatus.With(prometheus.Labels{"state": "avi_report", "status": "failed"}).Inc()
			return false
		}
		if strings.Contains(page.Data, change.User.Username) {
			logger.Infof("User already reported to AIV")
			metrics.EditStatus.With(prometheus.Labels{"state": "avi_report", "status": "skipped"}).Inc()
			return false
		}

		notice := fmt.Sprintf("* {{%s|%s}} - %s (Automated) ~~~~",
			helpers.AivUserVandalType(change.User.Username),
			change.User.Username,
			report,
		)
		comment := fmt.Sprintf("Automatically reporting [[Special:Contributions/%s]]. (bot)", change.User.Username)

		logger.Infof("Reporting user to AIV")
		if !api.AppendToPage(logger, ctx, "Wikipedia:Administrator_intervention_against_vandalism/TB2", notice, comment) {
			metrics.EditStatus.With(prometheus.Labels{"state": "avi_report", "status": "failed"}).Inc()
			return false
		}
		r.SendSpam(fmt.Sprintf("Reporting to AIV %s (%d)", change.User.Username, warningLevel))
		metrics.EditStatus.With(prometheus.Labels{"state": "avi_report", "status": "success"}).Inc()
		return true
	} else {
		warning := fmt.Sprintf("{{subst:User:%s/Warnings/Warning", configuration.Wikipedia.Username)
		warning += fmt.Sprintf("|1=%d", warningLevel)
		warning += fmt.Sprintf("|2=%s", strings.ReplaceAll("File:", ":File:", change.Common.Title))
		warning += fmt.Sprintf("|3=%s", report)
		warning += fmt.Sprintf(" <!{{subst:ns:0}}-- MySQL ID: %d --{{subst:ns:0}}>", mysqlVandalismId)
		warning += fmt.Sprintf("|4=%d}} ~~~~", mysqlVandalismId)
		comment := fmt.Sprintf("Warning [[Special:Contributions/%s|%s]] - #%d", change.User.Username, change.User.Username, warningLevel)

		logger.Infof("Warning user")
		if !api.AppendToPage(logger, ctx, fmt.Sprintf("User Talk:%s", change.User.Username), warning, comment) {
			metrics.EditStatus.With(prometheus.Labels{"state": "user_warning", "status": "failure"}).Inc()
			return false
		}
		metrics.EditStatus.With(prometheus.Labels{"state": "user_warning", "status": "success"}).Inc()
		r.SendSpam(fmt.Sprintf("Warning %s (%d)", change.User.Username, warningLevel))
		return true
	}
}

func shouldRevert(l *logrus.Entry, parentCtx context.Context, configuration *config.Configuration, db *database.DatabaseConnection, change *model.ProcessEvent) bool {
	logger := l.WithField("function", "processor.shouldRevert")
	ctx, span := metrics.OtelTracer.Start(parentCtx, "revert.shouldRevert")
	defer span.End()

	change.RevertReason = "Default Revert"

	if !configuration.Bot.Run {
		logger.Infof("Not reverting due to running disabled locally")
		change.RevertReason = "Run Disabled"
		metrics.RevertStatus.With(prometheus.Labels{"state": "should_revert", "status": "failed", "meta": "local_config"}).Inc()
		return false
	}

	if !configuration.Dynamic.Run {
		logger.Infof("Not reverting due to running disabled remotely")
		change.RevertReason = "Run Disabled"
		metrics.RevertStatus.With(prometheus.Labels{"state": "should_revert", "status": "failed", "meta": "remote_config"}).Inc()
		return false
	}

	if change.User.Username == configuration.Wikipedia.Username {
		logger.Infof("Not reverting due to self change")
		change.RevertReason = "User is myself"
		metrics.RevertStatus.With(prometheus.Labels{"state": "should_revert", "status": "failed", "meta": "self_edit"}).Inc()
		return false
	}

	if configuration.Bot.Angry {
		logger.Infof("Reverting due to angry mode")
		change.RevertReason = "Angry-reverting in angry mode"
		metrics.RevertStatus.With(prometheus.Labels{"state": "should_revert", "status": "success", "meta": "angry"}).Inc()
		return true
	}

	if strings.Contains(change.Current.Text, "{{nobots}}") {
		logger.Infof("Not reverting due to nobots")
		change.RevertReason = "Exclusion compliance"
		metrics.RevertStatus.With(prometheus.Labels{"state": "should_revert", "status": "failed", "meta": "nobots"}).Inc()
		return false
	}

	for _, name := range []string{
		configuration.Wikipedia.Username,
		strings.ReplaceAll(configuration.Wikipedia.Username, " ", "_"),
	} {
		noBotsDenyRegex := regexp.MustCompile(`{{bots\s*\|\s*deny\s*=[^}]*(` + regexp.QuoteMeta(name) + `|\*)[^}]*}}`)
		if noBotsDenyRegex.MatchString(change.Current.Text) {
			logger.Infof("Not reverting due to bots deny")
			change.RevertReason = "Exclusion compliance"
			metrics.RevertStatus.With(prometheus.Labels{"state": "should_revert", "status": "failed", "meta": "exclusion_deny"}).Inc()
			return false
		}

		noBotsAllows := regexp.MustCompile(`{{bots\s*\|\s*allow\s*=([^}]*)}}`).FindAllStringSubmatch(change.Current.Text, 1)
		if len(noBotsAllows) == 1 {
			if !strings.Contains(noBotsAllows[0][1], name) {
				logger.Infof("Not reverting due to no bots allow")
				change.RevertReason = "Exclusion compliance"
				metrics.RevertStatus.With(prometheus.Labels{"state": "should_revert", "status": "failed", "meta": "exclusion_allow"}).Inc()
				return false
			}
		}
	}

	if change.User.Username == change.Common.Creator {
		logger.Infof("Not reverting due to page creator being the user")
		change.RevertReason = "User is creator"
		metrics.RevertStatus.With(prometheus.Labels{"state": "should_revert", "status": "failed", "meta": "common_creator"}).Inc()
		return false
	}

	if change.User.EditCount > 50 {
		userWarnRatio := float64(change.User.Warns / change.User.EditCount)
		if userWarnRatio < 0.1 {
			logger.Infof("Not reverting due to user edit count")
			change.RevertReason = "User has edit count"
			metrics.RevertStatus.With(prometheus.Labels{"state": "should_revert", "status": "failed", "meta": "high_edit_count"}).Inc()
			return false
		}
		logger.Infof("Found user edit count, but high warns (%f)", userWarnRatio)
		change.RevertReason = "User has edit count, but warns > 10%"
		metrics.RevertStatus.With(prometheus.Labels{"state": "should_revert", "status": "success", "meta": "edit_count_warn_perc"}).Inc()
		return true
	}

	if change.Common.Title == configuration.Dynamic.TFA {
		logger.Infof("Reverting due to page being TFA")
		change.RevertReason = "Angry-reverting on TFA"
		metrics.RevertStatus.With(prometheus.Labels{"state": "should_revert", "status": "success", "meta": "angry_tfa"}).Inc()
		return true
	}

	for _, page := range configuration.Dynamic.AngryOptinPages {
		if change.Common.Title == page {
			logger.Infof("Reverting due to angry optin")
			change.RevertReason = "Angry-reverting on angry-optin"
			metrics.RevertStatus.With(prometheus.Labels{"state": "should_revert", "status": "success", "meta": "angry_opt_in"}).Inc()
			return true
		}
	}

	// If we reverted this user/page before in the last 24 hours, don't
	lastRevertTime := db.ClueBot.GetLastRevertTime(logger, ctx, change.Common.Title, change.User.Username)
	if lastRevertTime != 0 && lastRevertTime > time.Now().UTC().Unix()-config.RecentRevertThreshold {
		change.RevertReason = "Reverted before"
		metrics.RevertStatus.With(prometheus.Labels{"state": "should_revert", "status": "failed", "meta": "recent_revert"}).Inc()
		return false
	}

	metrics.RevertStatus.With(prometheus.Labels{"state": "should_revert", "status": "success", "meta": "fallback"}).Inc()
	return true
}

func processSingleRevertChange(logger *logrus.Entry, parentCtx context.Context, change *model.ProcessEvent, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, api *wikipedia.WikipediaApi) error {
	logger.Infof("Processing revert.....")
	ctx, parentSpan := metrics.OtelTracer.Start(parentCtx, "revert.processSingleRevertChange")
	defer parentSpan.End()

	// This is Vandalism, first generate an id
	mysqlVandalismId, err := db.ClueBot.GenerateVandalismId(logger,
		ctx,
		change.User.Username,
		change.Common.Title,
		fmt.Sprintf("ANN scored at %f", change.VandalismScore),
		change.GetDiffUrl(),
		change.Previous.Id,
		change.Current.Id)
	if err != nil {
		r.SendSpam(fmt.Sprintf("%s # %f # %s # Not reverted", change.FormatIrcChange(), change.VandalismScore, change.RevertReason))
		return fmt.Errorf("failed to generate vandalism id: %v", err)
	}
	logger.Infof("Generated vandalism id %v", mysqlVandalismId)

	// Log the revert time for later
	if err := db.ClueBot.SaveRevertTime(logger, ctx, change.Common.Title, change.User.Username); err != nil {
		logger.Warnf("Failed to save revert time: %v", err)
	}

	// Revert or not
	if !shouldRevert(logger, ctx, configuration, db, change) {
		metrics.EditStatus.With(prometheus.Labels{"state": "revert", "status": "skipped"}).Inc()
		logger.Infof("Should not revert: %s", change.RevertReason)
		r.SendSpam(fmt.Sprintf("%s # %f # %s # Not reverted", change.FormatIrcChange(), change.VandalismScore, change.RevertReason))
		return nil
	}
	logger.Infof("Should revert: %s", change.RevertReason)

	if revertChange(logger, ctx, api, change, configuration, mysqlVandalismId) {
		metrics.EditStatus.With(prometheus.Labels{"state": "revert", "status": "success"}).Inc()
		logger.Infof("Reverted successfully")
		doWarn(logger, ctx, api, r, change, configuration, mysqlVandalismId)
		db.ClueBot.MarkVandalismRevertedSuccessfully(logger, ctx, mysqlVandalismId)

		r.SendRevert(fmt.Sprintf("%s (Reverted) (%s) (%d s)", change.FormatIrcRevert(), change.RevertReason, time.Now().Unix()-change.ReceivedTime.Unix()))
		r.SendSpam(fmt.Sprintf("%s # %f # %s # Reverted", change.FormatIrcChange(), change.VandalismScore, change.RevertReason))
	} else {
		logger.Infof("Failed to revert")
		revision := api.GetPage(logger, ctx, helpers.PageTitle(change.Common.Namespace, change.Common.Title))
		if revision != nil {
			if change.User.Username == revision.User {
				metrics.EditStatus.With(prometheus.Labels{"state": "revert", "status": "self_beaten"}).Inc()
			} else {
				metrics.EditStatus.With(prometheus.Labels{"state": "revert", "status": "beaten"}).Inc()
				change.RevertReason = fmt.Sprintf("Beaten by %s", revision.User)
				db.ClueBot.MarkVandalismRevertBeaten(logger, ctx, mysqlVandalismId, change.Common.Title, change.GetDiffUrl(), revision.User)

				r.SendRevert(fmt.Sprintf("%s (Not Reverted) (%s) (%d s)", change.FormatIrcRevert(), change.RevertReason, time.Now().Unix()-change.ReceivedTime.Unix()))
				r.SendSpam(fmt.Sprintf("%s # %f # %s # Not Reverted", change.FormatIrcChange(), change.VandalismScore, change.RevertReason))
			}
		} else {
			metrics.EditStatus.With(prometheus.Labels{"state": "revert", "status": "failed"}).Inc()
		}
	}
	return nil
}

func ProcessRevertChangeEvents(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, api *wikipedia.WikipediaApi, inChangeFeed chan *model.ProcessEvent) {
	logger := logrus.WithField("function", "processor.ProcessRevertChangeEvents")
	wg.Add(1)
	defer wg.Done()
	for change := range inChangeFeed {
		metrics.ProcessorsRevertInUse.Inc()
		ctx, span := metrics.OtelTracer.Start(context.Background(), "processor.ProcessRevertChangeEvents")
		span.SetAttributes(attribute.String("uuid", change.Uuid))

		logger = logger.WithFields(logrus.Fields{"uuid": change.Uuid})
		if err := processSingleRevertChange(logger, ctx, change, configuration, db, r, api); err != nil {
			logger.Error(err.Error())
			span.SetStatus(codes.Error, err.Error())
		}

		span.End()
		metrics.ProcessorsRevertInUse.Dec()
	}
}
