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

	log "github.com/sirupsen/logrus"

	logrus_evlog "github.com/notdpate/evloghook"
)

func loggingInit(interactive bool) error {
	if interactive {
		log.SetFormatter(&log.TextFormatter{DisableColors: true})
	} else {
		log.SetOutput(ioutil.Discard)
		log.SetFormatter(&log.JSONFormatter{})

		hook, err := logrus_evlog.NewEventLogHook("YubiHSM Connector")
		if err != nil {
			return err
		}
		log.AddHook(hook)
	}

	return nil
}
