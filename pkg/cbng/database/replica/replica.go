package replica

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/cluebotng/botng/pkg/cbng/metrics"
	_ "github.com/go-sql-driver/mysql"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/codes"
	"math/rand"
	"net"
	"strings"
	"time"
)

type ReplicaInstance struct {
	handlers []*sql.DB
}

func NewReplicaInstance(configuration *config.Configuration) *ReplicaInstance {
	logger := logrus.WithFields(logrus.Fields{
		"function": "database.replica.getDatabaseConnection",
	})

	var handlers []*sql.DB
	for _, replica := range configuration.Sql.Replica {
		db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?timeout=1s", replica.Username, replica.Password, replica.Host, replica.Port, replica.Schema))
		if err != nil {
			logger.Fatalf("Error connecting to MySQL: %v", err)
		}
		db.SetConnMaxLifetime(time.Minute * 5)
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(0)

		if err := db.Ping(); err != nil {
			logger.Warnf("Could not use connection to MySQL: %v", err)
			continue
		}

		logger.Tracef("Connected to %s:xxx@tcp(%s:%d)/%s", replica.Username, replica.Host, replica.Port, replica.Schema)
		handlers = append(handlers, db)
	}

	ri := ReplicaInstance{handlers: handlers}
	return &ri
}

func (ri *ReplicaInstance) getHandle() *sql.DB {
	logger := logrus.WithFields(logrus.Fields{
		"function": "database.replica.DisconnectFromDatabase",
	})
	numberOfHandlers := len(ri.handlers)
	if numberOfHandlers > 0 {
		return ri.handlers[rand.Intn(len(ri.handlers))]
	}
	logger.Warnf("Could not find handler: %d", numberOfHandlers)
	return nil
}

func (ri *ReplicaInstance) DisconnectFromDatabase() {
	logger := logrus.WithFields(logrus.Fields{
		"function": "database.replica.DisconnectFromDatabase",
	})
	for _, handler := range ri.handlers {
		if err := handler.Close(); err != nil {
			logger.Warnf("Error closing connection to MySQL: %v", err)
		}
	}
}

func (ri *ReplicaInstance) GetPageCreatedTimeAndUser(l *logrus.Entry, ctx context.Context, namespaceId int64, title string) (string, int64, error) {
	logger := l.WithFields(logrus.Fields{"function": "database.replica.GetPageCreatedTimeAndUser", "args": map[string]interface{}{"namespaceId": namespaceId, "title": title}})
	_, span := metrics.OtelTracer.Start(ctx, "database.replica.ReplicaInstance.GetPageCreatedTimeAndUser")
	defer span.End()

	var timestamp int64
	var user string
	rows, err := ri.getHandle().Query("SET STATEMENT max_statement_time=10 FOR "+
		"SELECT `rev_timestamp`, `actor_name` FROM `page` "+
		"JOIN `revision` ON `rev_page` = `page_id` "+
		"JOIN `actor` ON `actor_id` = `rev_actor` "+
		"WHERE `page_namespace` = ? AND `page_title` = ? "+
		"ORDER BY `rev_id` "+
		"LIMIT 1", namespaceId, title)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return user, timestamp, err
	}
	defer rows.Close()

	if !rows.Next() {
		return user, timestamp, errors.New("No rows found")
	}

	if err := rows.Scan(&timestamp, &user); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return user, timestamp, err
	}

	logger.Debugf("Found creator %v @ %v", user, timestamp)
	return user, timestamp, nil
}

func (ri *ReplicaInstance) GetPageRecentEditCount(l *logrus.Entry, ctx context.Context, namespaceId int64, title string, timestamp int64) (int64, error) {
	logger := l.WithFields(logrus.Fields{
		"function": "database.replica.GetPageRecentEditCount",
		"args": map[string]interface{}{
			"namespaceId": namespaceId,
			"title":       title,
			"timestamp":   timestamp,
		},
	})
	_, span := metrics.OtelTracer.Start(ctx, "database.replica.ReplicaInstance.GetLatestChangeTimestamp")
	defer span.End()

	var recentEditCount int64
	rows, err := ri.getHandle().Query("SET STATEMENT max_statement_time=10 FOR "+
		"SELECT COUNT(*) as count FROM `page` "+
		"JOIN `revision` ON `rev_page` = `page_id` "+
		"WHERE `page_namespace` = ? AND `page_title` = ? AND `rev_timestamp` > ?", namespaceId, title, timestamp)

	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return recentEditCount, err
	}
	defer rows.Close()

	// No recent edits
	if !rows.Next() {
		span.SetStatus(codes.Error, "No rows found")
		return 0, nil
	}

	if err := rows.Scan(&recentEditCount); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return recentEditCount, err
	}

	logger.Debugf("Found number of recent edits: %v", recentEditCount)
	return recentEditCount, nil
}

func (ri *ReplicaInstance) GetPageRecentRevertCount(l *logrus.Entry, ctx context.Context, namespaceId int64, title string, timestamp int64) (int64, error) {
	logger := l.WithFields(logrus.Fields{
		"function": "database.replica.GetPageRecentRevertCount",
		"args": map[string]interface{}{
			"namespaceId": namespaceId,
			"title":       title,
			"timestamp":   timestamp,
		},
	})
	_, span := metrics.OtelTracer.Start(ctx, "database.replica.ReplicaInstance.GetPageRecentRevertCount")
	defer span.End()

	var recentRevertCount int64
	rows, err := ri.getHandle().Query("SET STATEMENT max_statement_time=10 FOR "+
		"SELECT COUNT(*) as count FROM `page` "+
		"JOIN `revision` ON `rev_page` = `page_id` "+
		"JOIN `comment` ON `comment_id` = `rev_comment_id` "+
		"WHERE `page_namespace` = ? AND `page_title` = ? AND `rev_timestamp` > ? AND `comment_text` "+
		"LIKE 'Revert%'", namespaceId, title, timestamp)

	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return recentRevertCount, err
	}
	defer rows.Close()

	if !rows.Next() {
		// The page has never been reverted
		logger.Debug("Found no reverts")
		return 0, nil
	}

	if err := rows.Scan(&recentRevertCount); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return recentRevertCount, err
	}

	logger.Debugf("Found number of recent reverts: %v", recentRevertCount)
	return recentRevertCount, nil
}

func (ri *ReplicaInstance) GetUserEditCount(l *logrus.Entry, parentCtx context.Context, user string) (int64, error) {
	logger := l.WithFields(logrus.Fields{
		"function": "database.replica.GetUserEditCount",
		"args": map[string]interface{}{
			"user": user,
		},
	})
	ctx, parentSpan := metrics.OtelTracer.Start(parentCtx, "database.replica.ReplicaInstance.GetUserEditCount")
	defer parentSpan.End()

	var editCount int64
	if net.ParseIP(user) != nil {
		_, span := metrics.OtelTracer.Start(ctx, "database.replica.ReplicaInstance.GetUserEditCount.registered")
		defer span.End()

		logger.Debugf("Querying revision_userindex for anonymous user")
		rows, err := ri.getHandle().Query("SET STATEMENT max_statement_time=10 FOR "+
			"SELECT COUNT(*) AS `user_editcount` FROM `revision_userindex` "+
			"WHERE `rev_actor` = "+
			"(SELECT actor_id FROM actor WHERE `actor_name` = ?)", user)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			return editCount, err
		}
		defer rows.Close()

		if !rows.Next() {
			logger.Debug("Found no edits")
			return 0, nil
		}

		if err := rows.Scan(&editCount); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return editCount, err
		}
	} else {
		_, span := metrics.OtelTracer.Start(ctx, "database.replica.ReplicaInstance.GetUserEditCount.anonymous")
		defer span.End()

		logger.Debugf("Querying user_editcount for user")
		userCountRows, err := ri.getHandle().Query("SET STATEMENT max_statement_time=10 FOR "+
			"SET STATEMENT max_statement_time=10 FOR "+
			"SELECT `user_editcount` FROM `user` WHERE `user_name` = ?", user)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			return editCount, err
		}
		defer userCountRows.Close()

		if !userCountRows.Next() {
			logger.Debug("Found no edits")
			return 0, nil
		}

		if err := userCountRows.Scan(&editCount); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return editCount, err
		}
	}

	return editCount, nil
}

func (ri *ReplicaInstance) GetUserRegistrationTime(l *logrus.Entry, parentCtx context.Context, user string) (int64, error) {
	logger := l.WithFields(logrus.Fields{
		"function": "database.replica.GetUserEditCount",
		"args": map[string]interface{}{
			"user": user,
		},
	})
	ctx, span := metrics.OtelTracer.Start(parentCtx, "database.replica.ReplicaInstance.GetUserRegistrationTime")
	defer span.End()

	var registrationTime int64
	// Anon users have no registration time so are a noop
	if net.ParseIP(user) == nil {
		logger.Debugf("Using registered lookup")
		userRegRows, err := ri.getHandle().Query("SET STATEMENT max_statement_time=10 FOR "+
			"SELECT `user_registration` FROM `user` WHERE `user_name` = ? AND `user_registration` is not NULL", user)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			return registrationTime, err
		}
		defer userRegRows.Close()

		if userRegRows.Next() {
			if err := userRegRows.Scan(&registrationTime); err != nil {
				span.SetStatus(codes.Error, err.Error())
				return registrationTime, err
			}
		} else {
			_, subSpan := metrics.OtelTracer.Start(ctx, "database.replica.ReplicaInstance.GetUserRegistrationTime.fallback")
			defer subSpan.End()
			logger.Debugf("Querying (fallback) revision_userindex for registered user")
			userRevRows, err := ri.getHandle().Query("SET STATEMENT max_statement_time=10 FOR "+
				"SELECT `rev_timestamp` FROM `revision_userindex` WHERE `rev_actor` = "+
				"(SELECT actor_id FROM actor WHERE `actor_name` = ?) "+
				" ORDER BY `rev_timestamp` LIMIT 0,1", user)
			if err != nil {
				subSpan.SetStatus(codes.Error, err.Error())
				return registrationTime, err
			}
			defer userRevRows.Close()

			if !userRevRows.Next() {
				subSpan.SetStatus(codes.Error, "No edits found for user")
				return registrationTime, errors.New("no edits found for user")
			}

			if err := userRevRows.Scan(&registrationTime); err != nil {
				subSpan.SetStatus(codes.Error, err.Error())
				return registrationTime, err
			}
		}
	}
	logger.Debugf("Found registration time: %v", registrationTime)
	return registrationTime, nil
}

func (ri *ReplicaInstance) GetUserWarnCount(l *logrus.Entry, ctx context.Context, user string) (int64, error) {
	logger := l.WithFields(logrus.Fields{
		"function": "database.replica.GetUserWarnCount",
		"args": map[string]interface{}{
			"user": user,
		},
	})
	_, span := metrics.OtelTracer.Start(ctx, "database.replica.ReplicaInstance.GetUserWarnCount")
	defer span.End()

	var warningCount int64
	rows, err := ri.getHandle().Query("SET STATEMENT max_statement_time=10 FOR "+
		"SELECT COUNT(*) as count FROM `page` "+
		"JOIN `revision` ON `rev_page` = `page_id` "+
		"JOIN `comment` ON `comment_id` = `rev_comment_id` "+
		"WHERE `page_namespace` = 3 AND `page_title` = ? AND "+
		"(`comment_text` LIKE '%warning%' OR "+
		"`comment_text` LIKE 'General note: Nonconstructive%')", strings.ReplaceAll(user, " ", "_"))
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return warningCount, err
	}
	defer rows.Close()

	if !rows.Next() {
		// User has never been warned
		return 0, nil
	}

	if err := rows.Scan(&warningCount); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return warningCount, err
	}

	logger.Debugf("Found number of warnings: %v", warningCount)
	return warningCount, nil
}

func (ri *ReplicaInstance) GetUserDistinctPagesCount(l *logrus.Entry, ctx context.Context, user string) (int64, error) {
	logger := l.WithFields(logrus.Fields{
		"function": "database.replica.GetUserDistinctPagesCount",
		"args": map[string]interface{}{
			"user": user,
		},
	})
	_, span := metrics.OtelTracer.Start(ctx, "database.replica.ReplicaInstance.GetUserDistinctPagesCount")
	defer span.End()

	var distinctPageCount int64
	rows, err := ri.getHandle().Query("SET STATEMENT max_statement_time=10 FOR "+
		"SELECT COUNT(DISTINCT rev_page) AS count FROM `revision_userindex` WHERE `rev_actor` = "+
		"(SELECT actor_id FROM actor WHERE `actor_name` = ?)", strings.ReplaceAll(user, " ", "_"))
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return distinctPageCount, err
	}
	defer rows.Close()

	if !rows.Next() {
		return 0, nil
	}

	if err := rows.Scan(&distinctPageCount); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return distinctPageCount, err
	}

	logger.Debugf("Found number of distinct pages: %v", distinctPageCount)
	return distinctPageCount, nil
}

func (ri *ReplicaInstance) GetLatestChangeTimestamp(l *logrus.Entry) (int64, error) {
	logger := l.WithFields(logrus.Fields{"function": "database.replica.ReplicaInstance.GetLatestChangeTimestamp"})
	var replicationDelay []uint8
	rows, err := ri.getHandle().Query("SET STATEMENT max_statement_time=10 FOR " +
		"SELECT UNIX_TIMESTAMP(MAX(rc_timestamp)) FROM `recentchanges`")
	if err != nil {
		logger.Errorf("Failed to query replication delay: %+v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return int64(0), errors.New("no results for replication delay query")
	}

	if err := rows.Scan(&replicationDelay); err != nil {
		return int64(0), fmt.Errorf("failed to read replication delay: %+v", err)
	}

	if len(replicationDelay) == 0 {
		return int64(0), fmt.Errorf("no replication delay data: %+v", replicationDelay)
	}

	return int64(replicationDelay[0]), nil
}

func (ri *ReplicaInstance) UpdateMetrics() {
	for i, handler := range ri.handlers {
		stats := handler.Stats()
		metrics.ReplicaStats.With(prometheus.Labels{"instance": fmt.Sprintf("%d", i), "metric": "max_open"}).Set(float64(stats.MaxOpenConnections))
		metrics.ReplicaStats.With(prometheus.Labels{"instance": fmt.Sprintf("%d", i), "metric": "idle"}).Set(float64(stats.Idle))
		metrics.ReplicaStats.With(prometheus.Labels{"instance": fmt.Sprintf("%d", i), "metric": "open"}).Set(float64(stats.OpenConnections))
		metrics.ReplicaStats.With(prometheus.Labels{"instance": fmt.Sprintf("%d", i), "metric": "in_use"}).Set(float64(stats.InUse))
		metrics.ReplicaStats.With(prometheus.Labels{"instance": fmt.Sprintf("%d", i), "metric": "wait"}).Set(float64(stats.WaitCount))
		metrics.ReplicaStats.With(prometheus.Labels{"instance": fmt.Sprintf("%d", i), "metric": "wait_duration"}).Set(float64(stats.WaitDuration))
		metrics.ReplicaStats.With(prometheus.Labels{"instance": fmt.Sprintf("%d", i), "metric": "idle_closed"}).Set(float64(stats.MaxIdleClosed))
		metrics.ReplicaStats.With(prometheus.Labels{"instance": fmt.Sprintf("%d", i), "metric": "idle_time_closed"}).Set(float64(stats.MaxIdleTimeClosed))
		metrics.ReplicaStats.With(prometheus.Labels{"instance": fmt.Sprintf("%d", i), "metric": "lifetime_closed"}).Set(float64(stats.MaxLifetimeClosed))
	}
}
