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

package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func uuidv4() (string, error) {
	uuid, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}

	return uuid.String(), nil
}

type statusReponse struct {
	http.ResponseWriter
	status int
}

func (r *statusReponse) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(p)
}

func (r *statusReponse) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func middlewareWrapper(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id, err = uuidv4()
			if err != nil {
				id = "-"
			}
			r.Header.Set("X-Request-ID", id)
		}
		ip := r.Header.Get("X-Real-IP")
		if ip == "" {
			s := strings.Split(r.RemoteAddr, ":")
			ip = s[0]
			r.Header.Set("X-Real-IP", ip)
		}

		clog := log.WithFields(log.Fields{
			"X-Request-ID":   id,
			"X-Real-IP":      ip,
			"RemoteAddr":     r.RemoteAddr,
			"Method":         r.Method,
			"Content-Length": r.ContentLength,
			"Content-Type":   r.Header.Get("Content-Type"),
			"User-Agent":     r.UserAgent(),
			"URI":            r.URL.RequestURI(),
		})

		defer func() {
			if rcv := recover(); rcv != nil {
				clog.WithField("panic", rcv).Error("recovered from handler panic")
				http.Error(
					w,
					http.StatusText(http.StatusInternalServerError),
					http.StatusInternalServerError,
				)
			}
		}()

		if hostHeaderAllowlisting && !validateHost(r.Host) {
			clog.WithField("host", r.Host).Error("host not in allowlist")
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}

		response := &statusReponse{
			ResponseWriter: w,
		}
		response.Header().Add("X-Request-ID", id)

		now := time.Now()
		next.ServeHTTP(response, r)
		latency := time.Since(now)

		fields := log.Fields{
			"latency":    latency,
			"StatusCode": response.status,
		}
		if response.status != http.StatusOK {
			clog.WithFields(fields).Error("error in handling request")
		} else {
			clog.WithFields(fields).Info("handled request")
		}

	})
}

func statusHandler(w http.ResponseWriter, r *http.Request, timeout time.Duration, serial string) {
	var err error

	if r.Method != "GET" {
		w.Header().Set("Allow", "GET")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed),
			http.StatusMethodNotAllowed)
		return
	}

	cid := r.Header.Get("X-Request-ID")
	clog := log.WithFields(log.Fields{
		"X-Request-ID": cid,
	})

	var status string
	if err = usbReopen(cid, timeout, serial); err != nil {
		status = "NO_DEVICE"
		clog.WithError(err).Warn("status failed to open usb device")
	} else {
		status = "OK"
	}

	// Deal with address/port in ycshell.
	split := strings.Split(viper.GetString("listen"), ":")

	fmt.Fprintf(w, "status=%s\n", status)
	if serial == "" {
		fmt.Fprintf(w, "serial=*\n")
	} else {
		fmt.Fprintf(w, "serial=%s\n", serial)
	}
	fmt.Fprintf(w, "version=%s\n", Version.String())
	fmt.Fprintf(w, "pid=%d\n", os.Getpid())
	fmt.Fprintf(w, "address=%s\n", split[0])
	fmt.Fprintf(w, "port=%s\n", split[1])
}

func apiHandler(w http.ResponseWriter, r *http.Request, timeout time.Duration, serial string) {
	var buf []byte
	var n int
	var err error

	cid := r.Header.Get("X-Request-ID")
	clog := log.WithFields(log.Fields{
		"X-Request-ID": cid,
	})

	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed),
			http.StatusMethodNotAllowed)
		return
	}

	if buf, err = ioutil.ReadAll(r.Body); err != nil {
		clog.WithError(err).Error("failed reading incoming request")
		http.Error(w, http.StatusText(http.StatusInternalServerError),
			http.StatusInternalServerError)
		return
	}

	if buf, err = usbProxy(buf, cid, timeout, serial); err != nil {
		clog.WithError(err).Error("failed usb proxy")
		http.Error(w, http.StatusText(http.StatusInternalServerError),
			http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	if n, err = w.Write(buf); err != nil {
		clog.WithError(err).Error("failed response write")
		http.Error(w, http.StatusText(http.StatusInternalServerError),
			http.StatusInternalServerError)
		return
	}
	if n != len(buf) {
		clog.WithError(err).WithFields(log.Fields{
			"n":   n,
			"len": len(buf),
		}).Error("partial response write")
		http.Error(w, http.StatusText(http.StatusInternalServerError),
			http.StatusInternalServerError)
		return
	}
}

func extractHost(addr string) string {
	if strings.Contains(addr, ":") {
		idx := strings.LastIndex(addr, ":")
		idx2 := 0
		// if this is a v6 adress we need to discover in a sane way if it has port or not
		if strings.Contains(addr, "]") {
			idx2 = strings.Index(addr, "]")
		}
		if idx > idx2 {
			addr = addr[:idx]
		}
	}
	return addr
}

func validateHost(addr string) bool {
	host := extractHost(addr)
	for _, h := range hostHeaderAllowlist {
		if h == host {
			return true
		}
	}
	return false
}
