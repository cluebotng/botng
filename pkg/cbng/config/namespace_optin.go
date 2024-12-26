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

	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()

		timer := time.NewTicker(time.Hour)
		for {
			select {
			case <-timer.C:
				logger.Debugf("Reloading From Timer")
				n.reload()
			case <-n.reloadChan:
				logger.Debugf("Reloading From Trigger")
				n.reload()
			}
		}
	}(wg)
}

func (n *NamespaceOptInInstance) reload() {
	logger := logrus.WithField("function", "config.NamespaceOptInInstance.reload")

	ctx, span := metrics.OtelTracer.Start(context.Background(), "NamespaceConfigurationReload")
	defer span.End()

	revision := n.w.GetPage(logger, ctx, n.GetPageName())
	if revision != nil {
		pages := []string{}
		for _, line := range strings.Split(revision.Data, "\n") {
			m := regexp.MustCompile(`^\* \[\[(.+)\]\] \-`).FindAllStringSubmatch(line, 1)
			if len(m) == 1 {
				pages = append(pages, m[0][1])
			}
		}

		if reflect.DeepEqual(n.c.Dynamic.NamespaceOptIn, pages) {
			logger.Infof("Updating Namespace Opt-in pages to: %+v", pages)
			n.c.Dynamic.NamespaceOptIn = pages
		}
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
