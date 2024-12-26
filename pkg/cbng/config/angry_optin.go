package config

import (
	"context"
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/wikipedia"
	"github.com/sirupsen/logrus"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"
)

type AngryOptInConfigurationInstance struct {
	c          *Configuration
	w          *wikipedia.WikipediaApi
	reloadChan chan bool
}

func (a *AngryOptInConfigurationInstance) TriggerReload() {
	a.reloadChan <- true
}

func (a *AngryOptInConfigurationInstance) GetPageName() string {
	return fmt.Sprintf("User:%s/AngryOptin", strings.ReplaceAll(a.c.Wikipedia.Username, " ", "_"))
}

func (a *AngryOptInConfigurationInstance) start(wg *sync.WaitGroup) {
	logger := logrus.WithField("function", "config.AngryOptInConfigurationInstance.start")
	a.reload()

	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()

		timer := time.NewTicker(time.Hour)
		for {
			select {
			case <-timer.C:
				logger.Debugf("Reloading From Timer")
				a.reload()
			case <-a.reloadChan:
				logger.Debugf("Reloading From Trigger")
				a.reload()
			}
		}
	}(wg)
}

func (a *AngryOptInConfigurationInstance) reload() {
	logger := logrus.WithField("function", "config.AngryOptInConfigurationInstance.reload")

	ctx, span := metrics.OtelTracer.Start(context.Background(), "AngryOptInConfigurationReload")
	defer span.End()

	revision := a.w.GetPage(logger, ctx, a.GetPageName())
	if revision != nil {
		pages := []string{}
		for _, line := range strings.Split(revision.Data, "\n") {
			m := regexp.MustCompile(`^\* \[\[(.+)\]\] \-`).FindAllStringSubmatch(line, 1)
			if len(m) == 1 {
				pages = append(pages, m[0][1])
			} else {
				logger.Tracef("Ignoring angry option line: '%v'", line)
			}
		}

		if reflect.DeepEqual(a.c.Dynamic.AngryOptinPages, pages) {
			logger.Infof("Updating Angry Opt-in pages to: %+v", pages)
			a.c.Dynamic.AngryOptinPages = pages
		}
	}
}

func NewAngryOptInConfiguration(c *Configuration, w *wikipedia.WikipediaApi, wg *sync.WaitGroup) *AngryOptInConfigurationInstance {
	i := AngryOptInConfigurationInstance{
		c:          c,
		w:          w,
		reloadChan: make(chan bool),
	}
	i.start(wg)
	return &i
}
