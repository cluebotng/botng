package config

import (
	"context"
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/wikipedia"
	"github.com/sirupsen/logrus"
	"strings"
	"sync"
	"time"
)

type RunInstance struct {
	c          *Configuration
	w          *wikipedia.WikipediaApi
	reloadChan chan bool
}

func (r *RunInstance) TriggerReload() {
	r.reloadChan <- true
}

func (r *RunInstance) GetPageName() string {
	return fmt.Sprintf("User:%s/Run", strings.ReplaceAll(r.c.Wikipedia.Username, " ", "_"))
}

func (r *RunInstance) start(wg *sync.WaitGroup) {
	logger := logrus.WithField("function", "config.RunInstance.start")
	r.reload()

	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()

		timer := time.NewTicker(time.Minute)
		for {
			select {
			case <-timer.C:
				logger.Debugf("Reloading From Timer")
				r.reload()
			case <-r.reloadChan:
				logger.Debugf("Reloading From Trigger")
				r.reload()
			}
		}
	}(wg)
}

func (r *RunInstance) reload() {
	logger := logrus.WithField("function", "config.RunInstance.reload")

	ctx, span := metrics.OtelTracer.Start(context.Background(), "RunConfigurationReload")
	defer span.End()

	revision := r.w.GetPage(logger, ctx, r.GetPageName())
	if revision != nil {
		shouldRun := false
		if strings.Contains(strings.ToLower(revision.Data), "true") {
			shouldRun = true
		}
		if r.c.Dynamic.Run != shouldRun {
			logger.Infof("Updating run status to %v (%v)", shouldRun, revision.Data)
			r.c.Dynamic.Run = shouldRun
		}
	}
}

func NewRun(c *Configuration, w *wikipedia.WikipediaApi, wg *sync.WaitGroup) *RunInstance {
	i := RunInstance{
		c:          c,
		w:          w,
		reloadChan: make(chan bool),
	}
	i.start(wg)
	return &i
}
