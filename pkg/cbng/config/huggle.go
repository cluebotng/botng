package config

import (
	"context"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/wikipedia"
	"github.com/sirupsen/logrus"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"
)

type HuggleConfigurationInstance struct {
	c          *Configuration
	w          *wikipedia.WikipediaApi
	reloadChan chan bool
}

func (h *HuggleConfigurationInstance) TriggerReload() {
	h.reloadChan <- true
}

func (h *HuggleConfigurationInstance) start(wg *sync.WaitGroup) {
	logger := logrus.WithField("function", "config.HuggleConfigurationInstance.start")
	h.reload()

	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()

		timer := time.NewTicker(time.Hour)
		for {
			select {
			case <-timer.C:
				logger.Debugf("Reloading From Timer")
				h.reload()
			case <-h.reloadChan:
				logger.Debugf("Reloading From Trigger")
				h.reload()
			}
		}
	}(wg)
}

func (h *HuggleConfigurationInstance) reload() {
	logger := logrus.WithField("function", "config.HuggleConfigurationInstance.reload")

	_, span := metrics.OtelTracer.Start(context.Background(), "HuggleConfigurationReload")
	defer span.End()

	response, err := http.Get("https://huggle.bena.rocks/?action=read&wp=en.wikipedia.org")
	if err != nil {
		logger.Errorf("Failed to build request: %v", err)
		return
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		logger.Errorf("Failed to read response: %v", err)
		return
	}

	if response.StatusCode != 200 {
		logger.Errorf("HTTP failure: %v", response.StatusCode)
		return
	}

	whitelist := []string{}
	for _, user := range strings.Split(string(body), "|") {
		if user != "" && user != "<!-- list -->" {
			whitelist = append(whitelist, user)
		}
	}

	if reflect.DeepEqual(h.c.Dynamic.HuggleUserWhitelist, whitelist) {
		logger.Infof("Updating Huggle User Whitelist: %+v", whitelist)
		h.c.Dynamic.HuggleUserWhitelist = whitelist
	}
}

func NewHuggleConfiguration(c *Configuration, w *wikipedia.WikipediaApi, wg *sync.WaitGroup) *HuggleConfigurationInstance {
	i := HuggleConfigurationInstance{
		c:          c,
		w:          w,
		reloadChan: make(chan bool),
	}
	i.start(wg)
	return &i
}
