package wikipedia

import (
	"encoding/json"
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/helpers"
	"github.com/sirupsen/logrus"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

type RevisionData struct {
	Previous Revision
	Current  Revision
}

type RevisionHistory []Revision

type Revision struct {
	Id        int64
	Timestamp int64
	Data      string
	User      string
}

type WikipediaApi struct {
	username string
	password string
	client   *http.Client
}

func NewWikipediaApi(username, password string) *WikipediaApi {
	logger := logrus.WithField("function", "wikipedia.NewWikipediaApi")
	timer := helpers.NewTimeLogger("wikipedia.NewWikipediaApi", map[string]interface{}{})
	defer timer.Done()

	cookieJar, err := cookiejar.New(nil)
	if err != nil {
		logger.Panicf("Failed to generate cookie jar: %v", err)
	}
	client := &http.Client{
		Jar:     cookieJar,
		Timeout: time.Second * 10,
		Transport: &http.Transport{
			DisableKeepAlives:   false,
			MaxConnsPerHost:     100,
			MaxIdleConns:        10,
			TLSHandshakeTimeout: time.Second * 2,
			IdleConnTimeout:     time.Second,
		},
	}

	api := WikipediaApi{
		username: username,
		password: password,
		client:   client,
	}
	if err := api.login(); err != nil {
		logger.Panicf("Failed to login to wikipedia: %v", err)
	}
	return &api
}

func (w *WikipediaApi) attemptLogin(reqData url.Values) (bool, *string) {
	logger := logrus.WithField("function", "wikipedia.WikipediaApi.attemptLogin")
	timer := helpers.NewTimeLogger("wikipedia.WikipediaApi.attemptLogin", map[string]interface{}{})
	defer timer.Done()

	logger.Tracef("Attempting login")
	response, err := w.client.PostForm("https://en.wikipedia.org/w/api.php", reqData)
	if err != nil {
		logger.Infof("Failed to login: %v", err)
		return false, nil
	}
	defer response.Body.Close()

	if response.StatusCode != 200 || response.Body == nil {
		logger.Infof("Error response received: %+v", response)
		return false, nil
	}

	data := map[string]interface{}{}
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		logger.Infof("Failed to read token login response: %v", err)
		return false, nil
	}

	result := data["login"].(map[string]interface{})["result"].(string)
	if result == "Success" {
		logger.Tracef("Got Success")
		return true, nil
	}

	if data["login"].(map[string]interface{})["result"].(string) == "NeedToken" {
		logger.Tracef("Got NeedToken")
		loginToken := data["login"].(map[string]interface{})["token"].(string)
		return false, &loginToken
	}
	return false, nil
}

func (w *WikipediaApi) login() error {
	logger := logrus.WithField("function", "wikipedia.WikipediaApi.login")
	timer := helpers.NewTimeLogger("wikipedia.WikipediaApi.login", map[string]interface{}{})
	defer timer.Done()

	success, loginToken := w.attemptLogin(url.Values{
		"action":     []string{"login"},
		"format":     []string{"json"},
		"lgname":     []string{w.username},
		"lgpassword": []string{w.password},
	})
	if success {
		logger.Infof("Logged into Wikipedia (no token)")
		return nil
	}

	if loginToken != nil {
		success, _ = w.attemptLogin(url.Values{
			"action":     []string{"login"},
			"format":     []string{"json"},
			"lgname":     []string{w.username},
			"lgpassword": []string{w.password},
			"lgtoken":    []string{*loginToken},
		})
		if success {
			logger.Infof("Logged into Wikipedia (token)")
			return nil
		}
	}

	return nil
	//return errors.New("Failed to login to Wikipedia")
}

func (w *WikipediaApi) GetRevisionHistory(l *logrus.Entry, page string, revId int64) *RevisionHistory {
	logger := l.WithFields(logrus.Fields{
		"function": "wikipedia.WikipediaApi.GetRevisionHistory",
		"args": map[string]interface{}{
			"page":  page,
			"revId": revId,
		},
	})
	timer := helpers.NewTimeLogger("wikipedia.WikipediaApi.GetRevisionHistory", map[string]interface{}{
		"page":  page,
		"revId": revId,
	})
	defer timer.Done()

	logger.Tracef("Starting request")
	response, err := w.client.Get(fmt.Sprintf("https://en.wikipedia.org/w/api.php?action=query&rawcontinue=1&prop=revisions&titles=%s&rvstartid=%d&rvlimit=5&rvslots=*&rvprop=timestamp|user|content|ids&format=json", url.QueryEscape(page), revId))
	if err != nil {
		logger.Infof("Failed to query page revisions (%s, %d): %v", page, revId, err)
		return nil
	}
	defer response.Body.Close()

	data := map[string]interface{}{}
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		logger.Infof("Failed to read page revisions (%s, %d): %v", page, revId, err)
		return nil
	}
	logger.Tracef("Got response")

	revisions := RevisionHistory{}
	for _, value := range data["query"].(map[string]interface{})["pages"].(map[string]interface{}) {
		if value.(map[string]interface{})["revisions"] == nil {
			return nil
		}
		for _, revision := range value.(map[string]interface{})["revisions"].([]interface{}) {
			revisionData := Revision{}
			revisionData.Data = revision.(map[string]interface{})["slots"].(map[string]interface{})["main"].(map[string]interface{})["*"].(string)
			revisionData.Id = int64(revision.(map[string]interface{})["revid"].(float64))
			revisionData.User = revision.(map[string]interface{})["user"].(string)
			timestampCurrent := revision.(map[string]interface{})["timestamp"].(string)
			if val, err := time.Parse("2006-01-02T15:04:05Z", timestampCurrent); err != nil {
				logger.Infof("Failed to decode revision timestamp (%s): %v", timestampCurrent, err)
			} else {
				revisionData.Timestamp = val.Unix()
			}
			revisions = append(revisions, revisionData)
		}
	}
	return &revisions
}

func (w *WikipediaApi) GetRevision(l *logrus.Entry, page string, revId int64) *RevisionData {
	logger := l.WithFields(logrus.Fields{
		"function": "wikipedia.WikipediaApi.GetRevision",
		"args": map[string]interface{}{
			"page":  page,
			"revId": revId,
		},
	})
	timer := helpers.NewTimeLogger("wikipedia.WikipediaApi.GetRevision", map[string]interface{}{
		"page":  page,
		"revId": revId,
	})
	defer timer.Done()

	logger.Tracef("Starting request")
	response, err := w.client.Get(fmt.Sprintf("https://en.wikipedia.org/w/api.php?action=query&rawcontinue=1&prop=revisions&titles=%s&rvstartid=%d&rvlimit=2&rvslots=*&rvprop=timestamp|user|content|ids&format=json", url.QueryEscape(page), revId))
	if err != nil {
		logger.Infof("Failed to query page revisions: %v", err)
		return nil
	}
	defer response.Body.Close()

	data := map[string]interface{}{}
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		logger.Infof("Failed to read page revisions: %v", err)
		return nil
	}
	logger.Tracef("Got response")

	if data["query"] == nil {
		logger.Infof("Found no query result: %v", data)
		return nil
	}

	for _, value := range data["query"].(map[string]interface{})["pages"].(map[string]interface{}) {
		if value.(map[string]interface{})["revisions"] == nil {
			logger.Infof("Found no pages: %v", data)
			return nil
		}
		revisions := value.(map[string]interface{})["revisions"].([]interface{})

		if len(revisions) != 2 {
			logger.Warnf("Not enough revisions: %v", data)
			return nil
		}
		revisionData := RevisionData{}

		currentData := revisions[0].(map[string]interface{})["slots"].(map[string]interface{})["main"].(map[string]interface{})["*"]
		if currentData == nil {
			logger.Warnf("No current revision data found: %+v", revisions[0])
			return nil
		}

		revisionData.Current.Data = currentData.(string)
		revisionData.Current.Id = int64(revisions[0].(map[string]interface{})["revid"].(float64))
		revisionData.Current.User = revisions[0].(map[string]interface{})["user"].(string)
		timestampCurrent := revisions[0].(map[string]interface{})["timestamp"].(string)

		previousTime, err := time.Parse("2006-01-02T15:04:05Z", timestampCurrent)
		if err != nil {
			logger.Warnf("Failed to decode revision timestamp (%s): %v", timestampCurrent, err)
			return nil
		}
		revisionData.Current.Timestamp = previousTime.Unix()

		previousData := revisions[1].(map[string]interface{})["slots"].(map[string]interface{})["main"].(map[string]interface{})["*"]
		if previousData == nil {
			logger.Warnf("No previous revision data found: %+v", revisions[1])
			return nil
		}
		revisionData.Previous.Data = previousData.(string)
		revisionData.Previous.Id = int64(revisions[1].(map[string]interface{})["revid"].(float64))
		revisionData.Previous.User = revisions[1].(map[string]interface{})["user"].(string)
		timestampPrevious := revisions[1].(map[string]interface{})["timestamp"].(string)

		currentTime, err := time.Parse("2006-01-02T15:04:05Z", timestampPrevious)
		if err != nil {
			logger.Warnf("Failed to decode revision timestamp (%s): %v", timestampPrevious, err)
			return nil
		}
		revisionData.Previous.Timestamp = currentTime.Unix()

		return &revisionData
	}
	return nil
}

func (w *WikipediaApi) GetPage(l *logrus.Entry, name string) *Revision {
	logger := l.WithFields(logrus.Fields{
		"function": "wikipedia.WikipediaApi.GetPage",
		"args": map[string]interface{}{
			"name": name,
		},
	})
	timer := helpers.NewTimeLogger("wikipedia.WikipediaApi.GetPage", map[string]interface{}{
		"name": name,
	})
	defer timer.Done()

	logger.Tracef("Starting request")
	response, err := w.client.Get(fmt.Sprintf("https://en.wikipedia.org/w/api.php?action=query&rawcontinue=1&prop=revisions&titles=%s&rvlimit=1&rvslots=*&rvprop=timestamp|user|content|ids&format=json&meta=userinfo&rvdir=older", url.QueryEscape(name)))
	if err != nil {
		logger.Infof("Failed to query page revisions %s: %v", name, err)
		return nil
	}
	defer response.Body.Close()

	data := map[string]interface{}{}
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		logger.Infof("Failed to read page revisions %s: %v", name, err)
		return nil
	}
	logger.Tracef("Got response")

	for _, value := range data["query"].(map[string]interface{})["pages"].(map[string]interface{}) {
		if value.(map[string]interface{})["revisions"] == nil {
			logger.Infof("Found no revisions for %v: %v", name, err)
			return nil
		}
		revisions := value.(map[string]interface{})["revisions"].([]interface{})

		revisionData := Revision{}
		revisionData.Data = revisions[0].(map[string]interface{})["slots"].(map[string]interface{})["main"].(map[string]interface{})["*"].(string)
		revisionData.Id = int64(revisions[0].(map[string]interface{})["revid"].(float64))
		revisionData.User = revisions[0].(map[string]interface{})["user"].(string)
		timestampCurrent := revisions[0].(map[string]interface{})["timestamp"].(string)
		if val, err := time.Parse("2006-01-02T15:04:05Z", timestampCurrent); err != nil {
			logger.Infof("Failed to decode revision timestamp (%s): %v", timestampCurrent, err)
		} else {
			revisionData.Timestamp = val.Unix()
		}
		return &revisionData
	}
	return nil
}

func (w *WikipediaApi) getRollbackToken(l *logrus.Entry) *string {
	logger := l.WithField("function", "wikipedia.WikipediaApi.getRollbackToken")
	timer := helpers.NewTimeLogger("wikipedia.WikipediaApi.getRollbackToken", map[string]interface{}{})
	defer timer.Done()

	logger.Tracef("Starting request")
	response, err := w.client.Get("https://en.wikipedia.org/w/api.php?action=query&meta=tokens&type=rollback&format=json")
	if err != nil {
		logger.Infof("Failed to request rollback token: %v", err)
		return nil
	}
	defer response.Body.Close()

	data := map[string]interface{}{}
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		logger.Infof("Failed to read rollback token response: %v", err)
		return nil
	}
	logger.Tracef("Got response")

	token := data["query"].(map[string]interface{})["tokens"].(map[string]interface{})["rollbacktoken"].(string)
	return &token
}

func (w *WikipediaApi) getCsrfToken(l *logrus.Entry) *string {
	logger := l.WithField("function", "wikipedia.WikipediaApi.getCsrfToken")
	timer := helpers.NewTimeLogger("wikipedia.WikipediaApi.getCsrfToken", map[string]interface{}{})
	defer timer.Done()

	logger.Tracef("Starting request")
	response, err := w.client.Get("https://en.wikipedia.org/w/api.php?action=query&meta=tokens&format=json")
	if err != nil {
		logger.Infof("Failed to request csrf token: %v", err)
		return nil
	}
	defer response.Body.Close()

	data := map[string]interface{}{}
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		logger.Infof("Failed to read csrf token response: %v", err)
		return nil
	}
	logger.Tracef("Got response")

	fmt.Printf("+%v\n\n", data)
	token := data["query"].(map[string]interface{})["tokens"].(map[string]interface{})["csrftoken"].(string)
	return &token
}

func (w *WikipediaApi) Rollback(l *logrus.Entry, title, user, comment string) bool {
	logger := l.WithFields(logrus.Fields{
		"function": "wikipedia.WikipediaApi.Rollback",
		"args": map[string]interface{}{
			"title":   title,
			"user":    user,
			"comment": comment,
		},
	})
	timer := helpers.NewTimeLogger("wikipedia.WikipediaApi.Rollback", map[string]interface{}{
		"title":   title,
		"user":    user,
		"comment": comment,
	})
	defer timer.Done()

	rollbackToken := w.getRollbackToken(logger)
	if rollbackToken == nil {
		logger.Infof("Failed to get token for rolling back %v (%v)", title, user)
		return false
	}
	// TODO: Remove
	logger.Infof("Would have rolled back")
	return true

	logger.Tracef("Starting request")
	response, err := w.client.PostForm("https://en.wikipedia.org/w/api.php", url.Values{
		"action":  []string{"rollback"},
		"format":  []string{"json"},
		"title":   []string{title},
		"user":    []string{user},
		"summary": []string{comment},
		"token":   []string{*rollbackToken},
	})
	if err != nil {
		logger.Infof("Failed to request rollback: %v", err)
		return false
	}
	defer response.Body.Close()

	data := map[string]interface{}{}
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		logger.Infof("Failed to read rollback response: %v", err)
		return false
	}
	logger.Tracef("Got response")

	if data["error"] != nil {
		if data["error"].(map[string]interface{})["code"].(string) == "badtoken" {
			logger.Infof("Got bad token, re-trying after login")
			if err := w.login(); err != nil {
				logger.Panicf("Failed to login to wikipedia: %v", err)
			}
			return w.Rollback(logger, title, user, comment)
		}
		logger.Infof("Error during rollback: %+v", data)
		return false
	}

	logger.Infof("Completed Rollback: %+v", data)
	return true
}

func (w *WikipediaApi) GetWarningLevel(l *logrus.Entry, user string) int {
	logger := l.WithFields(logrus.Fields{
		"function": "wikipedia.WikipediaApi.GetWarningLevel",
		"args": map[string]interface{}{
			"user": user,
		},
	})
	timer := helpers.NewTimeLogger("wikipedia.WikipediaApi.GetWarningLevel", map[string]interface{}{
		"user": user,
	})
	defer timer.Done()

	page := w.GetPage(logger, fmt.Sprintf("User talk:%s", user))

	matches := regexp.MustCompile(`<!-- Template:uw-[a-z]*(\d)(im)? -->.*(\d{2}:\d{2}, \d+ [a-zA-Z]+ \d{4} \(UTC\))`).FindAllStringSubmatch(page.Data, -1)
	level := 0
	for _, match := range matches {
		mlevel, err := strconv.Atoi(match[1])
		if err != nil {
			logger.Warnf("Failed to parse '%v' into int: %v", match[1], err)
			continue
		}
		t, err := time.Parse("15:04, 02 January 2006 (MST)", match[2])
		if err != nil {
			logger.Warnf("Failed to parse '%v' into time: %v", match[2], err)
			continue
		}
		if mlevel > level && t.Second() <= (2*24*60*60) {
			level = mlevel
		}
	}
	return level
}

func (w *WikipediaApi) AppendToPage(l *logrus.Entry, title, message, comment string) bool {
	logger := l.WithFields(logrus.Fields{
		"function": "wikipedia.WikipediaApi.AppendToPage",
		"args": map[string]interface{}{
			"title":   title,
			"message": message,
			"comment": comment,
		},
	})
	timer := helpers.NewTimeLogger("wikipedia.WikipediaApi.AppendToPage", map[string]interface{}{
		"title":   title,
		"message": message,
		"comment": comment,
	})
	defer timer.Done()

	// TODO: Remove
	logger.Infof("Would have appended to page")
	return true

	page := w.GetPage(logger, title)
	if page == nil {
		logger.Warnf("Could not fetch current page data")
		return false
	}
	newData := fmt.Sprintf("%s\n\n%s", page.Data, message)
	return w.WritePage(logger, title, newData, comment)
}

func (w *WikipediaApi) WritePage(l *logrus.Entry, title, content, comment string) bool {
	logger := l.WithFields(logrus.Fields{
		"function": "wikipedia.WikipediaApi.WritePage",
		"args": map[string]interface{}{
			"title":   title,
			"content": content,
			"comment": comment,
		},
	})
	timer := helpers.NewTimeLogger("wikipedia.WikipediaApi.WritePage", map[string]interface{}{
		"title":   title,
		"content": content,
		"comment": comment,
	})
	defer timer.Done()

	editToken := w.getCsrfToken(logger)
	if editToken == nil {
		logger.Infof("Failed to get csrf token for %v", title)
		return false
	}

	logger.Tracef("Starting request")
	response, err := w.client.PostForm("https://en.wikipedia.org/w/api.php", url.Values{
		"action":   []string{"edit"},
		"format":   []string{"json"},
		"title":    []string{title},
		"text":     []string{content},
		"summary":  []string{comment},
		"token":    []string{*editToken},
		"notminor": []string{"1"},
	})
	if err != nil {
		logger.Infof("Failed to request edit: %v", err)
		return false
	}
	defer response.Body.Close()

	data := map[string]interface{}{}
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		logger.Infof("Failed to read edit response: %v", err)
		return false
	}
	logger.Tracef("Got response")

	if data["error"] != nil {
		if data["error"].(map[string]interface{})["code"].(string) == "badtoken" {
			logger.Infof("Got bad token, re-trying after login")
			if err := w.login(); err != nil {
				logger.Panicf("Failed to login to wikipedia: %v", err)
			}
			return w.WritePage(logger, title, content, comment)
		}
		logger.Infof("Error during edit: %+v", data)
		return false
	}

	logger.Infof("Completed edit: %+v", data)
	return true
}
