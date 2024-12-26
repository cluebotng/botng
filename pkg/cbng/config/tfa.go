package config

import (
	"context"
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
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

	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()

		timer := time.NewTicker(time.Hour)
		for {
			select {
			case <-timer.C:
				logger.Debugf("Reloading From Timer")
				t.reload()
			case <-t.reloadChan:
				logger.Debugf("Reloading From Trigger")
				t.reload()
			}
		}
	}(wg)
}

func (t *TFAInstance) reload() {
	logger := logrus.WithField("function", "config.TFAInstance.reload")

	ctx, span := metrics.OtelTracer.Start(context.Background(), "TFAConfigurationReload")
	defer span.End()

	revision := t.w.GetPage(logger, ctx, t.GetPageName())
	if revision != nil {
		article := regexp.MustCompile(`{{TFAFULL\|([^}]+)}}`).FindAllStringSubmatch(revision.Data, 1)
		if len(article) != 1 {
			logger.Errorf("Failed to find TFA: '%v'", revision.Data)
			return
		}

		if t.c.Dynamic.TFA != article[0][1] {
			logger.Infof("Updating TFA to '%v'", article[0][1])
			t.c.Dynamic.TFA = article[0][1]
		}
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
