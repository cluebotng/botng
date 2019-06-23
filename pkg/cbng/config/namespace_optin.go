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

type NamespaceOptInInstance struct {
	c          *Configuration
	w          *wikipedia.WikipediaApi
	reloadChan chan bool
}

func (n *NamespaceOptInInstance) TriggerReload() {
	n.reloadChan <- true
}

func (n *NamespaceOptInInstance) GetPageName() string {
	return fmt.Sprintf("User:%s/Optin", strings.ReplaceAll(n.c.Wikipedia.Username, " ", "_"))
}

func (n *NamespaceOptInInstance) start(wg *sync.WaitGroup) {
	logger := logrus.WithField("function", "config.NamespaceOptInInstance.start")
	n.reload()
	go func(wg *sync.WaitGroup) {
		wg.Add(1)
		defer wg.Done()
		for {
			select {
			case <-time.Tick(time.Duration(time.Hour)):
				logger.Infof("Reloading From Timer")
				n.reload()
			case <-n.reloadChan:
				logger.Infof("Reloading From Trigger")
				n.reload()
			}
		}
	}(wg)
}

func (n *NamespaceOptInInstance) reload() {
	logger := logrus.WithField("function", "config.NamespaceOptInInstance.reload")
	timer := helpers.NewTimeLogger("config.NamespaceOptInInstance.reload", map[string]interface{}{})
	defer timer.Done()

	revision := n.w.GetPage(logger, n.GetPageName())
	if revision != nil {
		pages := []string{}
		for _, line := range strings.Split(revision.Data, "\n") {
			m := regexp.MustCompile(`^\* \[\[(.+)\]\] \-`).FindAllStringSubmatch(line, 1)
			if len(m) == 1 {
				pages = append(pages, m[0][1])
			}
		}
		logger.Infof("Updated Namespace Opt-in pages")
		logger.Debugf("Namespace Opt-in Pages Now: %+v", pages)
		n.c.Dynamic.NamespaceOptIn = pages
	}
}

func NewNamespaceOptIn(c *Configuration, w *wikipedia.WikipediaApi, wg *sync.WaitGroup) *NamespaceOptInInstance {
	i := NamespaceOptInInstance{
		c:          c,
		w:          w,
		reloadChan: make(chan bool),
	}
	i.start(wg)
	return &i
}
