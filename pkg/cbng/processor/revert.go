package processor

import (
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/database"
	"github.com/cluebotng/botng/pkg/cbng/helpers"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	"github.com/cluebotng/botng/pkg/cbng/relay"
	"github.com/cluebotng/botng/pkg/cbng/wikipedia"
	"github.com/honeycombio/libhoney-go"
	"github.com/sirupsen/logrus"
	"regexp"
	"strings"
	"sync"
	"time"
)

func revertChange(l *logrus.Entry, api *wikipedia.WikipediaApi, change *model.ProcessEvent, configuration *config.Configuration, mysqlVandalismId int64) bool {
	logger := l.WithFields(logrus.Fields{
		"function": "processor.revertChange",
		"args": map[string]interface{}{
			"mysqlVandalismId": mysqlVandalismId,
		},
	})
	timer := helpers.NewTimeLogger("processor.revertChange", map[string]interface{}{
		"mysqlVandalismId": mysqlVandalismId,
	})
	defer timer.Done()

	var revertRevision *wikipedia.Revision
	for _, revision := range *api.GetRevisionHistory(logger, change.Common.Title, change.Current.Id) {
		if revision.User != change.User.Username {
			revertRevision = &revision
			break
		}
	}
	if revertRevision == nil {
		logger.Infof("Failed to find revert revision")
		return false
	}

	if change.User.Username == configuration.Wikipedia.Username || helpers.StringItemInSlice(change.User.Username, configuration.Bot.Friends) {
		logger.Infof("Revert revision is self or a friend: %v", revertRevision)
		return false
	}

	revComment := "older version"
	if revertRevision.Id != 0 {
		revComment = fmt.Sprintf("version by %+v", revertRevision.User)
	}

	comment := fmt.Sprintf("Reverting possible vandalism by [[Special:Contribs/%+v|%+v]] to %+v. [[WP:CBFP|Report False Positive?]] Thanks, [[WP:CBNG|%+v]]. (%v) (Bot)",
		change.User, change.User,
		revComment,
		configuration.Wikipedia.Username,
		mysqlVandalismId,
	)

	return api.Rollback(logger, helpers.PageTitle(change.Common.Namespace, change.Common.Title), change.User.Username, comment)
}

func doWarn(l *logrus.Entry, api *wikipedia.WikipediaApi, r *relay.Relays, change *model.ProcessEvent, configuration *config.Configuration, mysqlVandalismId int64) bool {
	logger := l.WithFields(logrus.Fields{
		"function": "processor.doWarn",
		"args": map[string]interface{}{
			"mysqlVandalismId": mysqlVandalismId,
		},
	})

	report := fmt.Sprintf("[[%s]] was [%s changed] by [[Special:Contributions/%s|%s]] [[User:%s|(u)]] [[User talk:%s|(t)]] ANN scored at %f on %s",
		strings.ReplaceAll("File:", ":File:", change.TitleWithNamespace()),
		change.GetDiffUrl(),
		change.User.Username, change.User.Username,
		change.User.Username,
		change.User.Username,
		change.VandalismScore,
		time.Now().Format(time.RFC3339),
	)

	warningLevel := api.GetWarningLevel(logger, change.User.Username)
	logger.Infof("Found current warning level for user: %v", warningLevel)
	if warningLevel >= 4 {
		page := api.GetPage(logger, "Wikipedia:Administrator_intervention_against_vandalism/TB2")
		if page == nil {
			logger.Warnf("Failed to fetch current AIV")
			return false
		}
		if strings.Contains(page.Data, change.User.Username) {
			logger.Infof("User already reported to AIV")
			return false
		}

		notice := fmt.Sprintf("* {{%s|%s}} - %s (Automated) ~~~~",
			helpers.AivUserVandalType(change.User.Username),
			change.User.Username,
			report,
		)
		comment := fmt.Sprintf("Automatically reporting [[Special:Contributions/%s]]. (bot)", change.User.Username)

		logger.Infof("Reporting user to AIV")
		api.AppendToPage(logger, "Wikipedia:Administrator_intervention_against_vandalism/TB2", notice, comment)
		r.SendSpam(fmt.Sprintf("Reporting to AIV %s (%s)", change.User, warningLevel))
	} else {
		warning := fmt.Sprintf("{{subst:User:%s/Warnings/Warning", configuration.Wikipedia.Username)
		warning += fmt.Sprintf("|1=%d", warningLevel)
		warning += fmt.Sprintf("|2=%s", strings.ReplaceAll("File:", ":File:", change.Common.Title))
		warning += fmt.Sprintf("|3=%s", report)
		warning += fmt.Sprintf(" <!{{subst:ns:0}}-- MySQL ID: %d --{{subst:ns:0}}>", mysqlVandalismId)
		warning += fmt.Sprintf("|4=%d}} ~~~~", mysqlVandalismId)
		comment := fmt.Sprintf("Warning [[Special:Contributions/%s|%s]] - #%d", change.User.Username, change.User.Username, warningLevel)

		logger.Infof("Warning user")
		api.AppendToPage(logger, fmt.Sprintf("User Talk:%s", change.User.Username), warning, comment)
		r.SendSpam(fmt.Sprintf("Warning %s (%s)", change.User, warningLevel))
	}
	return false
}

func shouldRevert(l *logrus.Entry, configuration *config.Configuration, db *database.DatabaseConnection, change *model.ProcessEvent) bool {
	logger := l.WithField("function", "processor.shouldRevert")
	timer := helpers.NewTimeLogger("processor.shouldRevert", map[string]interface{}{})
	defer timer.Done()

	change.RevertReason = "Default Revert"

	if !configuration.Bot.Run {
		logger.Infof("Not reverting due to running disabled locally")
		change.RevertReason = "Run Disabled"
		return false
	}

	if !configuration.Dynamic.Run {
		logger.Infof("Not reverting due to running disabled remotely")
		change.RevertReason = "Run Disabled"
		return false
	}

	if change.User.Username == configuration.Wikipedia.Username {
		logger.Infof("Not reverting due to self change")
		change.RevertReason = "User is myself"
		return false
	}

	if configuration.Bot.Angry {
		logger.Infof("Reverting due to angry mode")
		change.RevertReason = "Angry-reverting in angry mode"
		return true
	}

	if strings.Contains(change.Current.Text, "{{nobots}}") {
		logger.Infof("Not reverting due to nobots")
		change.RevertReason = "Exclusion compliance"
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
			return false
		}

		noBotsAllows := regexp.MustCompile(`{{bots\s*\|\s*allow\s*=([^}]*)}}`).FindAllStringSubmatch(change.Current.Text, 1)
		if len(noBotsAllows) == 1 {
			if !strings.Contains(noBotsAllows[0][1], name) {
				logger.Infof("Not reverting due to no bots allow")
				change.RevertReason = "Exclusion compliance"
				return false
			}
		}
	}

	if change.User.Username == change.Common.Creator {
		logger.Infof("Not reverting due to page creator being the user")
		change.RevertReason = "User is creator"
		return false
	}

	if change.User.EditCount > 50 {
		userWarnRatio := float64(change.User.Warns / change.User.EditCount)
		if userWarnRatio < 0.1 {
			logger.Infof("Not reverting due to user edit count")
			change.RevertReason = "User has edit count"
			return false
		}
		logger.Infof("Found user edit count, but high warns (%+v)", userWarnRatio)
		change.RevertReason = "User has edit count, but warns > 10%"
	}

	if change.Common.Title == configuration.Dynamic.TFA {
		logger.Infof("Reverting due to page being TFA")
		change.RevertReason = "Angry-reverting on TFA"
		return true
	}

	for _, page := range configuration.Dynamic.AngryOptinPages {
		if change.Common.Title == page {
			logger.Infof("Reverting due to angry optin")
			change.RevertReason = "Angry-reverting on angry-optin"
			return true
		}
	}

	// If we reverted this user/page before in the last 24 hours, don't
	lastRevertTime := db.ClueBot.GetLastRevertTime(logger, change.Common.Title, change.User.Username)
	if lastRevertTime != 0 && lastRevertTime < 86400 {
		change.RevertReason = "Reverted before"
		return false
	}

	return false
}

func processSingleRevertChange(logger *logrus.Entry, change *model.ProcessEvent, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, api *wikipedia.WikipediaApi) error {
	logger = logger.WithFields(logrus.Fields{
		"change": change,
	})
	logger.Infof("Processing revert.....")

	// This is Vandalism, first generate an id
	mysqlVandalismId, err := db.ClueBot.GenerateVandalismId(logger,
		change.User.Username,
		change.Common.Title,
		fmt.Sprintf("ANN scored at %+v", change.VandalismScore),
		change.GetDiffUrl(),
		change.Previous.Id,
		change.Current.Id)
	if err != nil {
		r.SendSpam(fmt.Sprintf("%s # %f # %s # Not reverted", change.FormatIrcChange(), change.VandalismScore, change.RevertReason))
		return fmt.Errorf("Failed to generate vandalism id: %v", err)
	}
	logger.Infof("Generated vandalism id %v", mysqlVandalismId)

	// Revert or not
	if !shouldRevert(logger, configuration, db, change) {
		logger.Infof("Should not revert: %s", change.RevertReason)
		r.SendSpam(fmt.Sprintf("%s # %f # %s # Not reverted", change.FormatIrcChange(), change.VandalismScore, change.RevertReason))
		return nil
	}
	logger.Infof("Should revert: %s", change.RevertReason)

	if revertChange(logger, api, change, configuration, mysqlVandalismId) {
		logger.Infof("Reverted successfully")
		doWarn(logger, api, r, change, configuration, mysqlVandalismId)
		db.ClueBot.MarkVandalismRevertedSuccessfully(logger, mysqlVandalismId)

		r.SendRevert(fmt.Sprintf("%s (Reverted) (%s) (%s s)", change.FormatIrcRevert(), change.RevertReason, time.Now().Unix()-change.StartTime.Unix()))
		r.SendSpam(fmt.Sprintf("%s # %f # %s # Reverted", change.FormatIrcChange(), change.VandalismScore, change.RevertReason))
	} else {
		logger.Infof("Failed to revert")
		revision := api.GetPage(logger, helpers.PageTitle(change.Common.Namespace, change.Common.Title))
		if revision != nil && change.User.Username != revision.User {
			change.RevertReason = fmt.Sprintf("Beaten by %s", revision.User)
			db.ClueBot.MarkVandalismRevertBeaten(logger, mysqlVandalismId, change.Common.Title, change.GetDiffUrl(), revision.User)

			r.SendRevert(fmt.Sprintf("%s (Not Reverted) (%s) (%s s)", change.FormatIrcRevert(), change.RevertReason, time.Now().Unix()-change.StartTime.Unix()))
			r.SendSpam(fmt.Sprintf("%s # %f # %s # Not Reverted", change.FormatIrcChange(), change.VandalismScore, change.RevertReason))
		}
	}
	return nil
}

func ProcessRevertChangeEvents(wg *sync.WaitGroup, configuration *config.Configuration, db *database.DatabaseConnection, r *relay.Relays, api *wikipedia.WikipediaApi, inChangeFeed chan *model.ProcessEvent) {
	logger := logrus.WithField("function", "processor.ProcessRevertChangeEvents")
	wg.Add(1)
	defer wg.Done()
	for {
		select {
		case change := <-inChangeFeed:
			metrics.ProcessorsRevertInUse.Inc()
			startTime := time.Now()
			ev := libhoney.NewEvent()
			ev.AddField("cbng.function", "processor.processSingleChange")
			logger = logger.WithFields(logrus.Fields{"uuid": change.Uuid, "change": change})
			if err := processSingleRevertChange(logger, change, configuration, db, r, api); err != nil {
				logger.Errorf(err.Error())
				ev.AddField("error", err.Error())
			}
			ev.AddField("duration_ms", time.Since(startTime).Nanoseconds()/1000000)
			if err := ev.Send(); err != nil {
				logger.Warnf("Failed to send to honeycomb: %+v", err)
			}
			metrics.ProcessorsRevertInUse.Dec()
		}
	}
}
