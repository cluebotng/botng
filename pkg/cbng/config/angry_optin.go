package config

import (
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/helpers"
	"github.com/cluebotng/botng/pkg/cbng/wikipedia"
	"github.com/sirupsen/logrus"
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
	go func(wg *sync.WaitGroup) {
		wg.Add(1)
		defer wg.Done()
		for {
			select {
			case <-time.Tick(time.Duration(time.Hour)):
				logger.Infof("Reloading From Timer")
				a.reload()
			case <-a.reloadChan:
				logger.Infof("Reloading From Trigger")
				a.reload()
			}
		}
	}(wg)
}

func (a *AngryOptInConfigurationInstance) reload() {
	logger := logrus.WithField("function", "config.AngryOptInConfigurationInstance.reload")
	timer := helpers.NewTimeLogger("config.AngryOptInConfigurationInstance.reload", map[string]interface{}{})
	defer timer.Done()

	revision := a.w.GetPage(logger, a.GetPageName())
	if revision != nil {
		pages := []string{}
		for _, line := range strings.Split(revision.Data, "\n") {
			m := regexp.MustCompile(`^\* \[\[(.+)\]\] \-`).FindAllStringSubmatch(line, 1)
			if len(m) == 1 {
				pages = append(pages, m[0][1])
			} else {
				logger.Debugf("Ignoring angry option line: '%v'", line)
			}
		}
		logger.Infof("Updated Angry Opt-in pages")
		logger.Debugf("Angry Opt-in Pages Now: %+v", pages)
		a.c.Dynamic.AngryOptinPages = pages
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
