package feed

import (
	"bufio"
	"encoding/json"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	"github.com/cluebotng/botng/pkg/cbng/model"
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
	"net/http"
	"strings"
	"sync"
	"time"
)

func ConsumeHttpChangeEvents(wg *sync.WaitGroup, configuration *config.Configuration, changeFeed chan<- *model.ProcessEvent) {
	logger := logrus.WithFields(logrus.Fields{
		"function": "feed.ConsumeHttpChangeEvents",
	})
	wg.Add(1)
	defer wg.Done()

	type httpChangeEventLength struct {
		New int64
		Old int64
	}

	type httpChangeEventRevision struct {
		New int64
		Old int64
	}

	type httpChangeEvent struct {
		Type        string
		Namespace   string `json:"-"`
		NamespaceId int64 `json:"namespace"`
		Title       string
		Comment     string
		User        string
		Length      httpChangeEventLength
		Revision    httpChangeEventRevision
		ServerName  string `json:"server_name"`
	}

	logger.Info("Connecting to feed")
	req, err := http.NewRequest("GET", "https://stream.wikimedia.org/v2/stream/recentchange", nil)
	if err != nil {
		logger.Fatalf("Could not build request: %v", err)
	}

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		logger.Fatalf("Request failed: %v", err)
	}

	reader := bufio.NewReader(res.Body)
	defer res.Body.Close()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			logger.Fatalf("Reading failed: %v", err)
		}

		parts := strings.Split(line, ": ")
		if len(parts) == 2 && parts[0] == "data" {
			httpChange := httpChangeEvent{}
			if err := json.Unmarshal([]byte(parts[1]), &httpChange); err != nil {
				logger.Warnf("Decoding failed: %v", err)
				continue
			}
			logger.Tracef("Received: %+v", httpChange)

			if httpChange.Type == "edit" && httpChange.ServerName == configuration.Wikipedia.Host {
				change := model.ProcessEvent{
					Uuid:      uuid.NewV4().String(),
					StartTime: time.Now().UTC(),
					Common: model.ProcessEventCommon{
						Namespace:   strings.TrimRight(httpChange.Namespace, ":"),
						NamespaceId: httpChange.NamespaceId,
						Title:       httpChange.Title,
					},
					Comment: httpChange.Comment,
					User: model.ProcessEventUser{
						Username: httpChange.User,
					},
					Length: httpChange.Length.New - httpChange.Length.Old,

					Current: model.ProcessEventRevision{
						Id: httpChange.Revision.New,
					},
					Previous: model.ProcessEventRevision{
						Id: httpChange.Revision.Old,
					},
				}
				logger.Tracef("Decoded change: %+v", change)
				metrics.ChangeEventReceived.Inc()
				select {
				case changeFeed <- &change:
				default:
					logger.Warnf("Failed to write to change feed")
				}
			}
		}
	}
}
