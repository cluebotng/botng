package config

import (
	"github.com/cluebotng/botng/pkg/cbng/helpers"
	"github.com/cluebotng/botng/pkg/cbng/wikipedia"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
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
	go func(wg *sync.WaitGroup) {
		wg.Add(1)
		defer wg.Done()
		for {
			select {
			case <-time.Tick(time.Duration(time.Hour)):
				logger.Infof("Reloading From Timer")
				h.reload()
			case <-h.reloadChan:
				logger.Infof("Reloading From Trigger")
				h.reload()
			}
		}
	}(wg)
}

func (h *HuggleConfigurationInstance) reload() {
	logger := logrus.WithField("function", "config.HuggleConfigurationInstance.reload")
	timer := helpers.NewTimeLogger("config.HuggleConfigurationInstance.reload", map[string]interface{}{})
	defer timer.Done()

	response, err := http.Get("https://huggle-wl.wmflabs.org/?action=read&wp=en.wikipedia.org")
	if err != nil {
		logger.Warnf("Failed to build request: %v", err)
		return
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		logger.Warnf("Failed to read response: %v", err)
		return
	}

	if response.StatusCode != 200 {
		logger.Warnf("HTTP failure: %v", response.StatusCode)
		return
	}

	whitelist := []string{}
	for _, user := range strings.Split(string(body), "|") {
		if user != "" && user != "<!-- list -->" {
			whitelist = append(whitelist, user)
		}
	}
	logger.Infof("Updated Huggle User Whitelist")
	logger.Debugf("Huggle User Whitelist Users Now: %+v", whitelist)
	h.c.Dynamic.HuggleUserWhitelist = whitelist
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
