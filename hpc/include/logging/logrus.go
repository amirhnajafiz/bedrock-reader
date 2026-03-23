package logging

import (
	"time"

	"github.com/sirupsen/logrus"
)

// SetLogger sets the logger using input vars, it defaults to text logs
// on debug level unless otherwise specified.
func SetLogger(level string, jsonLog bool) {
	logrus.SetLevel(logrus.DebugLevel)

	if level != "" {
		llev, err := logrus.ParseLevel(level)
		if err != nil {
			logrus.Fatalf("cannot set LOG_LEVEL to %q", level)
		}

		logrus.SetLevel(llev)
	}

	if jsonLog {
		logrus.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})
	} else {
		logrus.SetFormatter(&BacktickFormatter{
			TimestampFormat: time.RFC3339,
		})
	}
}
