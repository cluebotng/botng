package config

import (
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/helpers"
	"github.com/cluebotng/botng/pkg/cbng/wikipedia"
	"github.com/sirupsen/logrus"
	"regexp"
	"sync"
	"time"
)

type TFAInstance struct {
	c          *Configuration
	w          *wikipedia.WikipediaApi
	reloadChan chan bool
}

func (t *TFAInstance) TriggerReload() {
	t.reloadChan <- true
}

func (t *TFAInstance) GetPageName() string {
	return fmt.Sprintf("Wikipedia:Today's featured article/%s", time.Now().Format("January 2, 2006"))
}

func (t *TFAInstance) start(wg *sync.WaitGroup) {
	logger := logrus.WithField("function", "config.TFAInstance.start")
	t.reload()
	go func(wg *sync.WaitGroup) {
		wg.Add(1)
		defer wg.Done()
		for {
			select {
			case <-time.Tick(time.Duration(time.Hour)):
				logger.Infof("Reloading From Timer")
				t.reload()
			case <-t.reloadChan:
				logger.Infof("Reloading From Trigger")
				t.reload()
			}
		}
	}(wg)
}

func (t *TFAInstance) reload() {
	logger := logrus.WithField("function", "config.TFAInstance.reload")
	timer := helpers.NewTimeLogger("config.TFAInstance.reload", map[string]interface{}{})
	defer timer.Done()

	revision := t.w.GetPage(logger, t.GetPageName())
	if revision != nil {
		article := regexp.MustCompile(`{{TFAFULL\|([^}]+)}}`).FindAllStringSubmatch(revision.Data, 1)
		if len(article) != 1 {
			logger.Warnf("Failed to find TFA: '%v'", revision.Data)
			return
		}
		logger.Infof("Updated TFA to '%v'", article[0][1])
		t.c.Dynamic.TFA = article[0][1]
	}
}

func NewTFA(c *Configuration, w *wikipedia.WikipediaApi, wg *sync.WaitGroup) *TFAInstance {
	i := TFAInstance{
		c:          c,
		w:          w,
		reloadChan: make(chan bool),
	}
	i.start(wg)
	return &i
}
