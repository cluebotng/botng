package logging

import (
	"github.com/cluebotng/botng/pkg/cbng/config"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func executeLogFilePrune(configuration *config.Configuration) {
	logFileNamesToRetain := map[string]bool{}
	for i := 0; i < configuration.Logging.Keep; i++ {
		logFileName := formatLogFileName(configuration.Logging.File, time.Now().AddDate(0, 0, -i).Format("20060102"))
		logFileNamesToRetain[logFileName] = true
	}
	logrus.Tracef("Calculated log files to keep: %v", logFileNamesToRetain)

	if matches, err := filepath.Glob(strings.Replace(configuration.Logging.File, "%s", "*", 1)); err != nil {
		logrus.Errorf("Failed to check for log file entries: %v", err)
	} else {
		for _, logFileName := range matches {
			if logFileNamesToRetain[logFileName] {
				logrus.Tracef("Skipping retained log file: %s", logFileName)
				continue
			}

			if err := os.Remove(logFileName); err != nil {
				logrus.Warnf("Failed to remove log file %s: %v", logFileName, err)
			} else {
				logrus.Tracef("Removed log file %s", logFileName)
			}
		}
	}
}

func PruneOldLogFiles(wg *sync.WaitGroup, configuration *config.Configuration) {

	defer wg.Done()

	if configuration.Logging.Keep == 0 {
		logrus.Info("Skipping log pruning due to keep")
		return
	}

	if !strings.Contains(configuration.Logging.File, "%s") {
		logrus.Infof("Skipping log pruning due to non-rotating file: %s", configuration.Logging.File)
		return
	}

	// On startup
	executeLogFilePrune(configuration)

	// Then regularly
	timer := time.NewTicker(time.Hour)
	for {
		<-timer.C
		executeLogFilePrune(configuration)
	}
}
