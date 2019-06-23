package replica

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/helpers"
	_ "github.com/go-sql-driver/mysql"
	"github.com/sirupsen/logrus"
	"net"
	"strings"
)

type ReplicaInstance struct {
	config config.ReplicaSqlConfiguration
	cur    *sql.DB
	connectionId string
}

func NewReplicaInstance(configuration *config.Configuration) *ReplicaInstance {
	ri := ReplicaInstance{config: configuration.Sql.Replica}
	if err := ri.ConnectToDatabase(); err != nil {
		panic(err)
	}
	return &ri
}

func (ri *ReplicaInstance) ConnectToDatabase() (error) {
	logger := logrus.WithFields(logrus.Fields{
		"function": "database.replica.getDatabaseConnection",
	})
	timer := helpers.NewTimeLogger("database.replica.getDatabaseConnection", map[string]interface{}{
		"username": ri.config.Username,
	})
	defer timer.Done()

	cur, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?timeout=1s", ri.config.Username, ri.config.Password, ri.config.Host, ri.config.Port, ri.config.Schema))
	if err != nil {
		logger.Fatalf("Error connecting to MySQL: %v", err)
	}
	cur.SetMaxIdleConns(1)
	cur.SetMaxOpenConns(1)

	logger.Tracef("Connected to %s:xxx@tcp(%s:%d)/%s", ri.config.Username, ri.config.Host, ri.config.Port, ri.config.Schema)
	ri.cur = cur
	if err := ri.cur.QueryRow("SELECT CONNECTION_ID()").Scan(&ri.connectionId); err != nil {
		return err
	}
	return nil
}

func (ri *ReplicaInstance) DisconnectFromDatabase() error {
	if ri.connectionId != "" {
		ri.cur.Exec("KILL CONNECTION ?", ri.connectionId)
		ri.connectionId = ""
	}
	return ri.cur.Close()
}

func (ri *ReplicaInstance) GetPageCreatedTimeAndUser(l *logrus.Entry, namespaceId int64, title string) (string, int64, error) {
	logger := l.WithFields(logrus.Fields{
		"function": "database.replica.GetPageCreatedTimeAndUser",
		"args": map[string]interface{}{
			"namespaceId": namespaceId,
			"title":       title,
		},
	})

	timer := helpers.NewTimeLogger("database.replica.GetPageCreatedTimeAndUser", map[string]interface{}{
		"namespaceId": namespaceId,
		"title":       title,
	})
	defer timer.Done()

	var timestamp int64
	var user string
	rows, err := ri.cur.Query("SELECT `rev_timestamp`, `actor_name` FROM `page` "+
		"JOIN `revision` ON `rev_page` = `page_id` "+
		"JOIN `actor` ON `actor_id` = `rev_actor` "+
		"WHERE `page_namespace` = ? AND `page_title` = ? "+
		"ORDER BY `rev_id` "+
		"LIMIT 1", namespaceId, title)
	if err != nil {
		return user, timestamp, err
	}
	defer rows.Close()

	if !rows.Next() {
		return user, timestamp, errors.New("No rows found")
	}

	if err := rows.Scan(&timestamp, &user); err != nil {
		return user, timestamp, err
	}

	logger.Debugf("Found user '%v', timestamp '%v'", user, timestamp)
	return user, timestamp, nil
}

func (ri *ReplicaInstance) GetPageRecentEditCount(l *logrus.Entry, namespaceId int64, title string, timestamp int64) (int64, error) {
	logger := l.WithFields(logrus.Fields{
		"function": "database.replica.GetPageRecentEditCount",
		"args": map[string]interface{}{
			"namespaceId": namespaceId,
			"title":       title,
			"timestamp":   timestamp,
		},
	})

	timer := helpers.NewTimeLogger("database.replica.GetPageRecentEditCount", map[string]interface{}{
		"namespaceId": namespaceId,
		"title":       title,
		"timestamp":   timestamp,
	})
	defer timer.Done()

	var recentEditCount int64
	rows, err := ri.cur.Query("SELECT COUNT(*) as count FROM `page` "+
		"JOIN `revision` ON `rev_page` = `page_id` "+
		"WHERE `page_namespace` = ? AND `page_title` = ? AND `rev_timestamp` > ?", namespaceId, title, timestamp)

	if err != nil {
		return recentEditCount, err
	}
	defer rows.Close()

	if !rows.Next() {
		return recentEditCount, errors.New("No rows found")
	}

	if err := rows.Scan(&recentEditCount); err != nil {
		return recentEditCount, err
	}

	logger.Debugf("Found number of recent edits: %v", recentEditCount)
	return recentEditCount, nil
}

func (ri *ReplicaInstance) GetPageRecentRevertCount(l *logrus.Entry, namespaceId int64, title string, timestamp int64) (int64, error) {
	logger := l.WithFields(logrus.Fields{
		"function": "database.replica.GetPageRecentRevertCount",
		"args": map[string]interface{}{
			"namespaceId": namespaceId,
			"title":       title,
			"timestamp":   timestamp,
		},
	})

	timer := helpers.NewTimeLogger("database.replica.GetPageRecentRevertCount", map[string]interface{}{
		"namespaceId": namespaceId,
		"title":       title,
		"timestamp":   timestamp,
	})
	defer timer.Done()

	var recentRevertCount int64
	rows, err := ri.cur.Query("SELECT COUNT(*) as count FROM `page` "+
		"JOIN `revision` ON `rev_page` = `page_id` "+
		"JOIN `comment` ON `comment_id` = `rev_comment_id` "+
		"WHERE `page_namespace` = ? AND `page_title` = ? AND `rev_timestamp` > ? AND `comment_text` "+
		"LIKE 'Revert%'", namespaceId, title, timestamp)

	if err != nil {
		return recentRevertCount, err
	}
	defer rows.Close()

	if !rows.Next() {
		// The page has never been reverted
		return 0, nil
	}

	if err := rows.Scan(&recentRevertCount); err != nil {
		return recentRevertCount, err
	}

	logger.Debugf("Found number of recent reverts: %v", recentRevertCount)
	return recentRevertCount, nil
}

func (ri *ReplicaInstance) GetUserEditCount(l *logrus.Entry, user string) (int64, error) {
	logger := l.WithFields(logrus.Fields{
		"function": "database.replica.GetUserEditCount",
		"args": map[string]interface{}{
			"user": user,
		},
	})

	timer := helpers.NewTimeLogger("database.replica.GetUserEditCount", map[string]interface{}{
		"user": user,
	})
	defer timer.Done()

	var editCount int64
	if net.ParseIP(user) != nil {
		logger.Debugf("Querying revision_userindex for anonymous user")
		rows, err := ri.cur.Query("SELECT COUNT(*) AS `user_editcount` FROM `revision_userindex` "+
			"WHERE `rev_actor` = "+
			"(SELECT actor_id FROM actor WHERE `actor_name` = ?)", user)
		if err != nil {
			return editCount, err
		}
		defer rows.Close()

		if !rows.Next() {
			return editCount, errors.New("No rows found")
		}

		if err := rows.Scan(&editCount); err != nil {
			return editCount, err
		}
	} else {
		logger.Debugf("Querying user_editcount for user")
		userCountRows, err := ri.cur.Query("SET STATEMENT max_statement_time=1 "+
			"FOR SELECT `user_editcount` FROM `user` WHERE `user_name` = ?", user)
		if err != nil {
			return editCount, err
		}
		defer userCountRows.Close()

		if !userCountRows.Next() {
			return editCount, errors.New("No rows found")
		}

		if err := userCountRows.Scan(&editCount); err != nil {
			return editCount, err
		}
	}

	return editCount, nil
}

func (ri *ReplicaInstance) GetUserRegistrationTime(l *logrus.Entry, user string) (int64, error) {
	logger := l.WithFields(logrus.Fields{
		"function": "database.replica.GetUserEditCount",
		"args": map[string]interface{}{
			"user": user,
		},
	})

	timer := helpers.NewTimeLogger("database.replica.GetUserEditCount", map[string]interface{}{
		"user": user,
	})
	defer timer.Done()

	var registrationTime int64
	// Anon users have no registration time so are a noop
	if net.ParseIP(user) == nil {
		logger.Debugf("Using registered lookup")
		userRegRows, err := ri.cur.Query("SELECT `user_registration` FROM `user` WHERE `user_name` = ? AND `user_registration` is not NULL", user)
		if err != nil {
			return registrationTime, err
		}
		defer userRegRows.Close()

		if userRegRows.Next() {
			if err := userRegRows.Scan(&registrationTime); err != nil {
				return registrationTime, err
			}
		} else {
			logger.Debugf("Querying (fallback) revision_userindex for registered user")
			userRevRows, err := ri.cur.Query("SELECT `rev_timestamp` FROM `revision_userindex` WHERE `rev_actor` = "+
				"(SELECT actor_id FROM actor WHERE `actor_name` = ?) "+
				" ORDER BY `rev_timestamp` LIMIT 0,1", user)
			if err != nil {
				return registrationTime, err
			}
			defer userRevRows.Close()

			if !userRevRows.Next() {
				return registrationTime, errors.New("No rows found")
			}

			if err := userRevRows.Scan(&registrationTime); err != nil {
				return registrationTime, err
			}
		}

	}
	logger.Debugf("Found registration time '%v'", registrationTime)
	return registrationTime, nil
}

func (ri *ReplicaInstance) GetUserWarnCount(l *logrus.Entry, user string) (int64, error) {
	logger := l.WithFields(logrus.Fields{
		"function": "database.replica.GetUserWarnCount",
		"args": map[string]interface{}{
			"user": user,
		},
	})

	timer := helpers.NewTimeLogger("database.replica.GetUserWarnCount", map[string]interface{}{
		"user": user,
	})
	defer timer.Done()

	var warningCount int64
	rows, err := ri.cur.Query("SELECT COUNT(*) as count FROM `page` "+
		"JOIN `revision` ON `rev_page` = `page_id` "+
		"JOIN `comment` ON `comment_id` = `rev_comment_id` "+
		"WHERE `page_namespace` = 3 AND `page_title` = ? AND "+
		"(`comment_text` LIKE '%warning%' OR "+
		"`comment_text` LIKE 'General note: Nonconstructive%')", strings.ReplaceAll(user, " ", "_"))
	if err != nil {
		return warningCount, err
	}
	defer rows.Close()

	if !rows.Next() {
		// User has never been warned
		return 0, nil
	}

	if err := rows.Scan(&warningCount); err != nil {
		return warningCount, err
	}

	logger.Debugf("Found number of warnings: %v", warningCount)
	return warningCount, nil
}

func (ri *ReplicaInstance) GetUserDistinctPagesCount(l *logrus.Entry, user string) (int64, error) {
	logger := l.WithFields(logrus.Fields{
		"function": "database.replica.GetUserDistinctPagesCount",
		"args": map[string]interface{}{
			"user": user,
		},
	})

	timer := helpers.NewTimeLogger("database.replica.GetUserDistinctPagesCount", map[string]interface{}{
		"user": user,
	})
	defer timer.Done()

	var distinctPageCount int64
	rows, err := ri.cur.Query("SELECT COUNT(DISTINCT rev_page) AS count FROM `revision_userindex` WHERE `rev_actor` = "+
		"(SELECT actor_id FROM actor WHERE `actor_name` = ?)", strings.ReplaceAll(user, " ", "_"))
	if err != nil {
		return distinctPageCount, err
	}
	defer rows.Close()

	if !rows.Next() {
		return distinctPageCount, errors.New("No rows found")
	}

	if err := rows.Scan(&distinctPageCount); err != nil {
		return distinctPageCount, err
	}

	logger.Debugf("Found number of distinct pages: %v", distinctPageCount)
	return distinctPageCount, nil
}

func (ri *ReplicaInstance) GetLatestChangeTimestamp(l *logrus.Entry) (int64, error) {
	logger := l.WithFields(logrus.Fields{
		"function": "database.replica.GetLatestChangeTimestamp",
	})

	timer := helpers.NewTimeLogger("database.replica.GetLatestChangeTimestamp", map[string]interface{}{})
	defer timer.Done()

	var replicationDelay []uint8
	rows, err := ri.cur.Query("SELECT UNIX_TIMESTAMP(MAX(rc_timestamp)) FROM `recentchanges`")
	if err != nil {
		logger.Warnf("Failed to query replication delay: %+v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return int64(0), errors.New("Found no results for replication delay query")
	}

	if err := rows.Scan(&replicationDelay); err != nil {
		return int64(0), fmt.Errorf("Failed to read replication delay: %+v", err)
	}

	if len(replicationDelay) == 0 {
		return int64(0), fmt.Errorf("No replication delay data: %+v", replicationDelay)
	}

	return int64(replicationDelay[0]), nil
}
