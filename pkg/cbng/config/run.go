package config

import (
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/helpers"
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
	go func(wg *sync.WaitGroup) {
		wg.Add(1)
		defer wg.Done()
		for {
			select {
			case <-time.Tick(time.Duration(time.Minute)):
				logger.Infof("Reloading From Timer")
				r.reload()
			case <-r.reloadChan:
				logger.Infof("Reloading From Trigger")
				r.reload()
			}
		}
	}(wg)
}

func (r *RunInstance) reload() {
	logger := logrus.WithField("function", "config.RunInstance.reload")
	timer := helpers.NewTimeLogger("config.RunInstance.reload", map[string]interface{}{})
	defer timer.Done()

	revision := r.w.GetPage(logger, r.GetPageName())
	if revision != nil {
		shouldRun := false
		if strings.Contains(strings.ToLower(revision.Data), "true") {
			shouldRun = true
		}
		logger.Infof("Updated status to %v (%v)", shouldRun, revision.Data)
		r.c.Dynamic.Run = shouldRun
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
