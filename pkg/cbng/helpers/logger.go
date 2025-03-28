package helpers

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"strings"
	"sync"
	"time"
)

type RotatingLogFileHook struct {
	logFile     string
	fileHandles map[string]*os.File
	mutex       sync.Mutex
}

func NewRotatingRotatingLogFileHook(logFile string) *RotatingLogFileHook {
	rotatingLogFileHook := &RotatingLogFileHook{
		logFile:     logFile,
		fileHandles: make(map[string]*os.File),
		mutex:       sync.Mutex{},
	}
	go rotatingLogFileHook.closeOldFileHandlers()
	return rotatingLogFileHook
}

func (hook *RotatingLogFileHook) Fire(entry *logrus.Entry) error {
	if line, err := entry.Bytes(); err != nil {
		return err
	} else {
		if err := hook.write(line); err != nil {
			return err
		}
	}
	return nil
}

func (hook *RotatingLogFileHook) getCurrentFileName() string {
	currentFileName := hook.logFile
	if strings.HasSuffix(hook.logFile, ".log") {
		prefix, _ := strings.CutSuffix(hook.logFile, ".log")
		currentFileName = fmt.Sprintf("%s-%s.log", prefix, time.Now().Format("20060102"))
	}
	return currentFileName
}

func (hook *RotatingLogFileHook) getFileHandler() (*os.File, error) {
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

func (hook *RotatingLogFileHook) write(line []byte) error {
	if fileHandler, err := hook.getFileHandler(); err != nil {
		return err
	} else {
		if _, err := fileHandler.Write(line); err != nil {
			return err
		}
	}
	return nil
}

func (hook *RotatingLogFileHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.DebugLevel,
		logrus.InfoLevel,
		logrus.WarnLevel,
		logrus.ErrorLevel,
		logrus.FatalLevel,
		logrus.PanicLevel,
	}
}

func (hook *RotatingLogFileHook) closeOldFileHandlers() {
	timer := time.NewTicker(time.Hour)
	for range timer.C {
		currentFileName := hook.getCurrentFileName()
		for name, file := range hook.fileHandles {
			if name != currentFileName {
				hook.mutex.Lock()
				file.Close()
				delete(hook.fileHandles, name)
				hook.mutex.Unlock()
			}
		}
	}
}
