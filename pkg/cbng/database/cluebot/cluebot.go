package cluebot

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/helpers"
	_ "github.com/go-sql-driver/mysql"
	"github.com/sirupsen/logrus"
	"time"
)

type CluebotInstance struct {
	cfg config.CluebotSqlConfiguration
}

func NewCluebotInstance(configuration *config.Configuration) *CluebotInstance {
	ri := CluebotInstance{cfg: configuration.Sql.Cluebot}
	return &ri
}

func (ci *CluebotInstance) getDatabaseConnection() *sql.DB {
	logger := logrus.WithFields(logrus.Fields{
		"function": "database.cluebot.getDatabaseConnection",
	})
	timer := helpers.NewTimeLogger("database.cluebot.getDatabaseConnection", map[string]interface{}{})
	defer timer.Done()

	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?timeout=1s", ci.cfg.Username, ci.cfg.Password, ci.cfg.Host, ci.cfg.Port, ci.cfg.Schema))
	if err != nil {
		logger.Fatalf("Error connecting to MySQL: %v", err)
	}
	db.SetMaxIdleConns(0)
	db.SetMaxOpenConns(0)

	logger.Tracef("Connected to %s:xxx@tcp(%s:%d)/%s", ci.cfg.Username, ci.cfg.Host, ci.cfg.Port, ci.cfg.Schema)
	return db
}

func (ci *CluebotInstance) GenerateVandalismId(l *logrus.Entry, user, title, reason, diffUrl string, previousId, currentId int64) (int64, error) {
	logger := l.WithFields(logrus.Fields{
		"function": "database.cluebot.GenerateVandalismId",
		"args": map[string]interface{}{
			"user":       user,
			"title":      title,
			"reason":     reason,
			"diffUrl":    diffUrl,
			"previousId": previousId,
			"currentId":  currentId,
		},
	})
	timer := helpers.NewTimeLogger("database.cluebot.GenerateVandalismId", map[string]interface{}{
		"user":       user,
		"title":      title,
		"reason":     reason,
		"diffUrl":    diffUrl,
		"previousId": previousId,
		"currentId":  currentId,
	})
	defer timer.Done()

	var vandalismId int64

	db := ci.getDatabaseConnection()
	defer db.Close()

	res, err := db.Exec( "INSERT INTO `vandalism` (`id`,`user`,`article`,`heuristic`,`reason`,`diff`,`old_id`,`new_id`,`reverted`) VALUES (NULL, ?, ?, '', ?, ?, ?, ?, 0)", user, title, reason, diffUrl, previousId, currentId)
	if err != nil {
		logger.Infof("Error running query: %v", err)
		return vandalismId, err
	}
	if vandalismId, err := res.LastInsertId(); err != nil {
		logger.Infof("Failed to get insert id: %v", err)
		return vandalismId, err
	}

	logger.Debugf("Generated id %v", vandalismId)
	return vandalismId, nil
}

func (ci *CluebotInstance) MarkVandalismRevertedSuccessfully(l *logrus.Entry, vandalismId int64) {
	logger := l.WithFields(logrus.Fields{
		"function": "database.cluebot.MarkVandalismRevertedSuccessfully",
		"args": map[string]interface{}{
			"vandalismId": vandalismId,
		},
	})
	timer := helpers.NewTimeLogger("database.cluebot.MarkVandalismRevertedSuccessfully", map[string]interface{}{
		"vandalismId": vandalismId,
	})
	defer timer.Done()

	db := ci.getDatabaseConnection()
	defer db.Close()

	if _, err := db.Exec("UPDATE `vandalism` SET `reverted` = 1 WHERE `id` = ?", vandalismId); err != nil {
		logger.Errorf("Error running query: %v", err)
		return
	}
	logger.Infoln("Updated reverted status (reverted)")
}

func (ci *CluebotInstance) MarkVandalismRevertBeaten(l *logrus.Entry, vandalismId int64, pageTitle, diffUrl, beatenUser string) {
	logger := l.WithFields(logrus.Fields{
		"function": "database.cluebot.MarkVandalismRevertBeaten",
		"args": map[string]interface{}{
			"vandalismId": vandalismId,
			"beatenUser":  beatenUser,
			"pageTitle":   pageTitle,
		},
	})
	timer := helpers.NewTimeLogger("database.cluebot.MarkVandalismRevertBeaten", map[string]interface{}{
		"vandalismId": vandalismId,
		"beatenUser":  beatenUser,
		"pageTitle":   pageTitle,
	})
	defer timer.Done()

	db := ci.getDatabaseConnection()
	defer db.Close()

	if _, err := db.Exec("UPDATE `vandalism` SET `reverted` = 0 WHERE `id` = ?", vandalismId); err != nil {
		logger.Errorf("Error running vandalism query: %v", err)
		return
	}

	if _, err := db.Exec("INSERT INTO `beaten` (`id`, `article`, `diff`, `user`) VALUES (NULL, ?, ?, ?)", pageTitle, diffUrl, beatenUser); err != nil {
		logger.Errorf("Error running beaten query: %v", err)
		return
	}
	logger.Infoln("Updated reverted status (beaten)")
}

func (ci *CluebotInstance) GetServiceHost(l *logrus.Entry, service string) string {
	logger := l.WithFields(logrus.Fields{
		"function": "database.cluebot.GetServiceHost",
		"args": map[string]interface{}{
			"service": service,
		},
	})
	timer := helpers.NewTimeLogger("database.cluebot.GetServiceHost", map[string]interface{}{
		"service": service,
	})
	defer timer.Done()

	var host string

	db := ci.getDatabaseConnection()
	defer db.Close()

	rows, err := db.Query("SELECT `node` from `cluster_node` where type=?", service)
	if err != nil {
		logger.Infof("Error running query: %v", err)
	} else {
		defer rows.Close()
		if !rows.Next() {
			logger.Infof("No data found for query")
		} else {
			if err := rows.Scan(&host); err != nil {
				logger.Warnf("Error reading rows for query: %v", err)
			}
		}
	}

	return host
}

func (ci *CluebotInstance) GetLastRevertTime(l *logrus.Entry, title, user string) int64 {
	logger := l.WithFields(logrus.Fields{
		"function": "database.cluebot.GetLastRevertTime",
		"args": map[string]interface{}{
			"title": title,
			"user":  user,
		},
	})
	timer := helpers.NewTimeLogger("database.cluebot.GetServiceHost", map[string]interface{}{
		"title": title,
		"user":  user,
	})
	defer timer.Done()

	var revertTime int64
	ctx, _ := context.WithTimeout(context.Background(), time.Millisecond*300)
	db := ci.getDatabaseConnection()
	defer db.Close()

	rows, err := db.QueryContext(ctx, "SELECT `time` FROM `last_revert` WHERE title=? AND user=?", title, user)
	if err != nil {
		logger.Infof("Error running query: %v", err)
	} else {
		defer rows.Close()
		if !rows.Next() {
			logger.Infof("No data found for query")
		} else {
			if err := rows.Scan(&revertTime); err != nil {
				logger.Warnf("Error reading rows for query: %v", err)
			}
		}
	}

	return revertTime
}

func (ci *CluebotInstance) SaveRevertTime(l *logrus.Entry, title, user string) int64 {
	logger := l.WithFields(logrus.Fields{
		"function": "database.cluebot.SaveRevertTime",
		"args": map[string]interface{}{
			"title": title,
			"user":  user,
		},
	})
	timer := helpers.NewTimeLogger("database.cluebot.GetServiceHost", map[string]interface{}{
		"title": title,
		"user":  user,
	})
	defer timer.Done()

	var revertTime int64
	ctx, _ := context.WithTimeout(context.Background(), time.Millisecond*300)
	db := ci.getDatabaseConnection()
	defer db.Close()

	rows, err := db.QueryContext(ctx, "INSERT INTO `last_revert` (`title`, `user`, `time`) "+
		"VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE `time`=`time`", title, user, time.Now().UTC().Unix())
	if err != nil {
		logger.Infof("Error running query: %v", err)
	} else {
		defer rows.Close()
		if !rows.Next() {
			logger.Infof("No data found for query")
		} else {
			if err := rows.Scan(&revertTime); err != nil {
				logger.Warnf("Error reading rows for query: %v", err)
			}
		}
	}

	return revertTime
}
