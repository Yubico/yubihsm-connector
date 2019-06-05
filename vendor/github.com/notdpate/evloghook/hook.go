// +build windows

// Package evloghook to send logs via Windwows Event Log
package evloghook

import (
	"golang.org/x/sys/windows/svc/eventlog"

	"github.com/sirupsen/logrus"
)

const levels = eventlog.Error | eventlog.Warning | eventlog.Info

// EventLogHook to send logs via Windwows Event Log.
type EventLogHook struct {
	name string
}

// NewEventLogHook creates a hook to be added to an instance of logger
func NewEventLogHook(name string) (*EventLogHook, error) {
	l, err := eventlog.Open(name)
	if err != nil {
		return nil, err
	}
	defer l.Close()

	return &EventLogHook{
		name: name,
	}, nil
}

// Fire is called when a log event is fired.
func (h *EventLogHook) Fire(entry *logrus.Entry) error {
	logger, err := eventlog.Open(h.name)
	if err != nil {
		return err
	}
	defer logger.Close()

	const eventID = 1001
	message, err := entry.String()
	if err != nil {
		return err
	}

	switch entry.Level {
	case logrus.PanicLevel:
		return logger.Error(eventID+2, message)
	case logrus.FatalLevel:
		return logger.Error(eventID+1, message)
	case logrus.ErrorLevel:
		return logger.Error(eventID, message)
	case logrus.WarnLevel:
		return logger.Warning(eventID, message)
	case logrus.InfoLevel:
		return logger.Info(eventID, message)
	case logrus.DebugLevel:
		return logger.Info(eventID+1, message)
	default:
		return nil
	}
}

// Levels returns the available logging levels.
func (h *EventLogHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
		logrus.WarnLevel,
		logrus.InfoLevel,
		logrus.DebugLevel,
	}
}
