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
	"fmt"
	"math"
	"sync"
	"time"
	"unsafe"

	log "github.com/sirupsen/logrus"
)

// #cgo CFLAGS: -DUNICODE -D_UNICODE
// #cgo LDFLAGS: -lwinusb -lsetupapi -luuid
// #include "usb_windows.h"
import "C"

var device struct {
	ctx C.PDEVICE_CONTEXT
	mtx sync.Mutex
}

func (e C.DWORD) Error() string {
	return fmt.Sprintf("Windows Error: 0x%x", uint(e))
}

const (
	SUCCESS                 C.DWORD = C.ERROR_SUCCESS
	ERROR_INVALID_STATE     C.DWORD = C.ERROR_INVALID_STATE
	ERROR_INVALID_HANDLE    C.DWORD = C.ERROR_INVALID_HANDLE
	ERROR_INVALID_PARAMETER C.DWORD = C.ERROR_INVALID_PARAMETER
	ERROR_OUTOFMEMORY       C.DWORD = C.ERROR_OUTOFMEMORY
	ERROR_GEN_FAILURE       C.DWORD = C.ERROR_GEN_FAILURE
	ERROR_OBJECT_NOT_FOUND  C.DWORD = C.ERROR_OBJECT_NOT_FOUND
	ERROR_NOT_SUPPORTED     C.DWORD = C.ERROR_NOT_SUPPORTED
	ERROR_SHARING_VIOLATION C.DWORD = C.ERROR_SHARING_VIOLATION
	ERROR_BAD_COMMAND       C.DWORD = C.ERROR_BAD_COMMAND
)

func winusbError(err error) error {
	if err != SUCCESS {
		return err
	}
	return nil
}

func usbopen(cid string, timeout time.Duration, serial string) (err error) {
	if device.ctx != nil {
		log.WithField("Correlation-ID", cid).Debug("usb context already open")
		return nil
	}

	var timeoutMs int64 = timeout.Milliseconds()
	if timeoutMs > math.MaxUint32 {
		log.Fatalf("timeout must fit in a uint32")
	}

	var ms C.ULONG = C.ulong(uint32(timeoutMs))
	if serial != "" {
		cSerial := C.CString(serial)
		defer C.free(unsafe.Pointer(cSerial))

		err = winusbError(C.usbOpen(0x1050, 0x0030, cSerial, &device.ctx, ms))
	} else {
		err = winusbError(C.usbOpen(0x1050, 0x0030, nil, &device.ctx, ms))
	}

	if device.ctx == nil {
		err = fmt.Errorf("device not found")
	}

	return err
}

func usbclose(cid string) {
	if device.ctx != nil {
		C.usbClose(&device.ctx)
	}
}

func usbreopen(cid string, why error, timeout time.Duration, serial string) (err error) {
	log.WithFields(log.Fields{
		"Correlation-ID": cid,
		"why":            why,
	}).Debug("reopening usb context")

	usbclose(cid)
	return usbopen(cid, timeout, serial)
}

func usbReopen(cid string, why error, timeout time.Duration, serial string) (err error) {
	device.mtx.Lock()
	defer device.mtx.Unlock()

	if err = usbopen(cid, timeout, serial); err != nil {
		return err
	}

	for {
		if err = winusbError(C.usbCheck(device.ctx, 0x1050, 0x0030)); err != nil {
			log.WithFields(log.Fields{
				"Correlation-ID": cid,
				"Error":          err,
			}).Debug("Couldn't check usb context")

			if err = usbreopen(cid, why, timeout, serial); err != nil {
				return err
			}
			continue
		}

		break
	}

	return nil
}

func usbwrite(buf []byte, cid string) (err error) {
	var n C.ULONG

	if err = winusbError(C.usbWrite(
		device.ctx,
		(*C.UCHAR)(unsafe.Pointer(&buf[0])),
		C.ULONG(len(buf)),
		&n)); err != nil {
		goto out
	}

out:
	log.WithFields(log.Fields{
		"Correlation-ID": cid,
		"n":              uint(n),
		"err":            err,
		"len":            len(buf),
		"buf":            buf,
	}).Debug("usb endpoint write")

	return err
}

func usbread(cid string) (buf []byte, err error) {
	var n C.ULONG

	buf = make([]byte, 8192)

	if err = winusbError(C.usbRead(
		device.ctx,
		(*C.UCHAR)(unsafe.Pointer(&buf[0])),
		C.ULONG(len(buf)),
		&n)); err != nil {
		buf = buf[:0]
		goto out
	}
	buf = buf[:n]

out:
	log.WithFields(log.Fields{
		"Correlation-ID": cid,
		"n":              uint(n),
		"err":            err,
		"len":            len(buf),
		"buf":            buf,
	}).Debug("usb endpoint read")

	return buf, err
}

func usbProxy(req []byte, cid string, timeout time.Duration, serial string) (resp []byte, err error) {
	device.mtx.Lock()
	defer device.mtx.Unlock()

	if err = usbopen(cid, timeout, serial); err != nil {
		return nil, err
	}

	for {
		if err = usbwrite(req, cid); err != nil {
			if err = usbreopen(cid, err, timeout, serial); err != nil {
				return nil, err
			}
			continue
		}

		if resp, err = usbread(cid); err != nil {
			if err = usbreopen(cid, err, timeout, serial); err != nil {
				return nil, err
			}
			continue
		}

		break
	}

	return resp, err
}
