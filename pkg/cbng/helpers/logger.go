package helpers

import (
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

type LogFileHook struct {
	logger *lumberjack.Logger
}

func NewLogFileHook(logger *lumberjack.Logger) *LogFileHook {
	return &LogFileHook{
		logger: logger,
	}
}

func (hook *LogFileHook) Fire(entry *logrus.Entry) error {
	line, err := entry.String()
	if err != nil {
		return err
	}

	_, err = hook.logger.Write([]byte(line))
	if err != nil {
		return err
	}

	return nil
}

func (hook *LogFileHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.DebugLevel,
		logrus.InfoLevel,
		logrus.WarnLevel,
		logrus.ErrorLevel,
		logrus.FatalLevel,
		logrus.PanicLevel,
	}
}
