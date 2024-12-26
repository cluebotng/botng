package cluebot

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	_ "github.com/go-sql-driver/mysql"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/codes"
	"time"
)

type CluebotInstance struct {
	cfg config.SqlConfiguration
}

func NewCluebotInstance(configuration *config.Configuration) *CluebotInstance {
	ci := CluebotInstance{cfg: configuration.Sql.Cluebot}
	return &ci
}

func (ci *CluebotInstance) getDatabaseConnection() (*sql.DB, error) {
	logger := logrus.WithFields(logrus.Fields{
		"function": "database.cluebot.getDatabaseConnection",
	})

	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?timeout=1s", ci.cfg.Username, ci.cfg.Password, ci.cfg.Host, ci.cfg.Port, ci.cfg.Schema))
	if err != nil {
		logger.Errorf("Error connecting to MySQL: %v", err)
		return nil, err
	}
	db.SetMaxIdleConns(0)
	db.SetMaxOpenConns(0)

	logger.Tracef("Connected to %s:xxx@tcp(%s:%d)/%s", ci.cfg.Username, ci.cfg.Host, ci.cfg.Port, ci.cfg.Schema)
	return db, nil
}

func (ci *CluebotInstance) GenerateVandalismId(logger *logrus.Entry, ctx context.Context, user, title, reason, diffUrl string, previousId, currentId int64) (int64, error) {
	_, span := metrics.OtelTracer.Start(ctx, "cluebot.GenerateVandalismId")
	defer span.End()

	db, err := ci.getDatabaseConnection()
	if err != nil {
		logger.Errorf("Error connecting to db: %v", err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	defer db.Close()

	res, err := db.Exec("INSERT INTO `vandalism` (`id`,`user`,`article`,`heuristic`,`reason`,`diff`,`old_id`,`new_id`,`reverted`) VALUES (NULL, ?, ?, '', ?, ?, ?, ?, 0)", user, title, reason, diffUrl, previousId, currentId)
	if err != nil {
		logger.Errorf("Error running query: %v", err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}

	vandalismId, err := res.LastInsertId()
	if err != nil {
		logger.Errorf("Failed to get insert id: %v", err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}

	logger.Debugf("Generated id %v", vandalismId)
	return vandalismId, nil
}

func (ci *CluebotInstance) MarkVandalismRevertedSuccessfully(l *logrus.Entry, ctx context.Context, vandalismId int64) error {
	logger := l.WithFields(logrus.Fields{
		"function": "database.cluebot.MarkVandalismRevertedSuccessfully",
		"args": map[string]interface{}{
			"vandalismId": vandalismId,
		},
	})
	_, span := metrics.OtelTracer.Start(ctx, "cluebot.MarkVandalismRevertedSuccessfully")
	defer span.End()

	db, err := ci.getDatabaseConnection()
	if err != nil {
		logger.Errorf("Error connecting to db: %v", err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	defer db.Close()

	if _, err := db.Exec("UPDATE `vandalism` SET `reverted` = 1 WHERE `id` = ?", vandalismId); err != nil {
		logger.Errorf("Error running query: %v", err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	logger.Infoln("Updated reverted status (reverted)")
	return nil
}

func (ci *CluebotInstance) MarkVandalismRevertBeaten(l *logrus.Entry, ctx context.Context, vandalismId int64, pageTitle, diffUrl, beatenUser string) error {
	logger := l.WithFields(logrus.Fields{
		"function": "database.cluebot.MarkVandalismRevertBeaten",
		"args": map[string]interface{}{
			"vandalismId": vandalismId,
			"beatenUser":  beatenUser,
			"pageTitle":   pageTitle,
		},
	})
	_, span := metrics.OtelTracer.Start(ctx, "cluebot.MarkVandalismRevertBeaten")
	defer span.End()

	db, err := ci.getDatabaseConnection()
	if err != nil {
		logger.Errorf("Error connecting to db: %v", err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	defer db.Close()

	if _, err := db.Exec("UPDATE `vandalism` SET `reverted` = 0 WHERE `id` = ?", vandalismId); err != nil {
		logger.Errorf("Error running vandalism query: %v", err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if _, err := db.Exec("INSERT INTO `beaten` (`id`, `article`, `diff`, `user`) VALUES (NULL, ?, ?, ?)", pageTitle, diffUrl, beatenUser); err != nil {
		logger.Errorf("Error running beaten query: %v", err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	logger.Infoln("Updated reverted status (beaten)")
	return nil
}

func (ci *CluebotInstance) GetLastRevertTime(l *logrus.Entry, ctx context.Context, title, user string) (int64, error) {
	logger := l.WithFields(logrus.Fields{
		"function": "database.cluebot.GetLastRevertTime",
		"args": map[string]interface{}{
			"title": title,
			"user":  user,
		},
	})
	_, span := metrics.OtelTracer.Start(ctx, "cluebot.GetLastRevertTime")
	defer span.End()

	var revertTime int64
	db, err := ci.getDatabaseConnection()
	if err != nil {
		logger.Errorf("Error connecting to db: %v", err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	defer db.Close()

	rows, err := db.Query("SELECT `time` FROM `last_revert` WHERE title=? AND user=?", title, user)
	if err != nil {
		logger.Infof("Error running query: %v", err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		defer rows.Close()
		if !rows.Next() {
			logger.Infof("No data found for query")
		} else {
			if err := rows.Scan(&revertTime); err != nil {
				logger.Errorf("Error reading rows for query: %v", err)
				span.SetStatus(codes.Error, err.Error())
			}
		}
	}

	return revertTime, nil
}

func (ci *CluebotInstance) SaveRevertTime(l *logrus.Entry, ctx context.Context, title, user string) error {
	logger := l.WithFields(logrus.Fields{
		"function": "database.cluebot.SaveRevertTime",
		"args": map[string]interface{}{
			"title": title,
			"user":  user,
		},
	})
	_, span := metrics.OtelTracer.Start(ctx, "cluebot.SaveRevertTime")
	defer span.End()

	revertTime := time.Now().UTC().Unix()
	db, err := ci.getDatabaseConnection()
	if err != nil {
		logger.Errorf("Error connecting to db: %v", err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	defer db.Close()

	rows, err := db.Query("INSERT INTO `last_revert` (`title`, `user`, `time`) "+
		"VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE `time`=`time`", title, user, revertTime)
	if err != nil {
		logger.Infof("Error running query: %v", err)
		span.SetStatus(codes.Error, err.Error())
		return err
	} else {
		defer rows.Close()
		if rows.Next() {
			if err := rows.Scan(&revertTime); err != nil {
				logger.Errorf("Error reading rows for query: %v", err)
				span.SetStatus(codes.Error, err.Error())
				return err
			}
		}
	}

	return nil
}

func (ci *CluebotInstance) PurgeOldRevertTimes() error {
	logger := logrus.WithFields(logrus.Fields{
		"function": "database.cluebot.PurgeOldRevertTimes",
	})
	_, span := metrics.OtelTracer.Start(context.Background(), "database.cluebot.PurgeOldRevertTimes")
	defer span.End()

	db, err := ci.getDatabaseConnection()
	if err != nil {
		logger.Errorf("Error connecting to db: %v", err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	defer db.Close()

	_, err = db.Exec("DELETE FROM `last_revert` WHERE `time` < ?", time.Now().UTC().Unix()-(config.RecentRevertThreshold+10))
	if err != nil {
		logger.Warnf("Error purging database: %v", err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}
