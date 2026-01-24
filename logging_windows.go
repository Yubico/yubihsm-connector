// Copyright 2016-2018 Yubico AB
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build windows

package main

import (
	"io/ioutil"

	"golang.org/x/sys/windows/svc/eventlog"

	log "github.com/sirupsen/logrus"

)

func loggingInit(interactive bool) error {
	if interactive {
		log.SetFormatter(&log.TextFormatter{DisableColors: true})
	} else {
		log.SetOutput(ioutil.Discard)
		log.SetFormatter(&log.JSONFormatter{})

		hook, err := NewEventLogHook("YubiHSM Connector")
		if err != nil {
			return err
		}
		log.AddHook(hook)
	}

	return nil
}

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
func (h *EventLogHook) Fire(entry *log.Entry) error {
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
	case log.PanicLevel:
		return logger.Error(eventID+2, message)
	case log.FatalLevel:
		return logger.Error(eventID+1, message)
	case log.ErrorLevel:
		return logger.Error(eventID, message)
	case log.WarnLevel:
		return logger.Warning(eventID, message)
	case log.InfoLevel:
		return logger.Info(eventID, message)
	case log.DebugLevel:
		return logger.Info(eventID+1, message)
	default:
		return nil
	}
}

// Levels returns the available logging levels.
func (h *EventLogHook) Levels() []log.Level {
	return []log.Level{
		log.PanicLevel,
		log.FatalLevel,
		log.ErrorLevel,
		log.WarnLevel,
		log.InfoLevel,
		log.DebugLevel,
	}
}