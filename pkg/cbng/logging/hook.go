package logging

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"strings"
	"sync"
	"time"
)

func formatLogFileName(baseName string, relativeTime string) string {
	if strings.Contains(baseName, "%s") {
		return fmt.Sprintf(baseName, relativeTime)
	}
	return baseName
}

type LogFileHook struct {
	logFile     string
	fileHandles map[string]*os.File
	mutex       sync.Mutex
}

func NewLogFileHook(logFile string) *LogFileHook {
	LogFileHook := &LogFileHook{
		logFile:     logFile,
		fileHandles: make(map[string]*os.File),
		mutex:       sync.Mutex{},
	}
	go LogFileHook.closeOldFileHandlers()
	return LogFileHook
}

func (hook *LogFileHook) Fire(entry *logrus.Entry) error {
	if line, err := entry.Bytes(); err != nil {
		return err
	} else {
		if err := hook.write(line); err != nil {
			return err
		}
	}
	return nil
}

func (hook *LogFileHook) getCurrentFileName() string {
	return formatLogFileName(hook.logFile, time.Now().Format("20060102"))
}

func (hook *LogFileHook) getFileHandler() (*os.File, error) {
	currentFileName := hook.getCurrentFileName()
	if hook.fileHandles[currentFileName] == nil {
		hook.mutex.Lock()
		defer hook.mutex.Unlock()
		f, err := os.OpenFile(currentFileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
		if err != nil {
			return nil, err
		}
		hook.fileHandles[currentFileName] = f
	}
	return hook.fileHandles[currentFileName], nil
}

func (hook *LogFileHook) write(line []byte) error {
	if fileHandler, err := hook.getFileHandler(); err != nil {
		return err
	} else {
		if _, err := fileHandler.Write(line); err != nil {
			return err
		}
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

func (hook *LogFileHook) closeOldFileHandlers() {
	timer := time.NewTicker(time.Hour)
	for range timer.C {
		currentFileName := hook.getCurrentFileName()
		for name, file := range hook.fileHandles {
			if name != currentFileName {
				hook.mutex.Lock()
				if err := file.Close(); err != nil {
					logrus.Errorf("Failed to close current log file: %v", err)
				}
				delete(hook.fileHandles, name)
				hook.mutex.Unlock()
			}
		}
	}
}
