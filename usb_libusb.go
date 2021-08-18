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

// +build !windows

package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/gousb"
	log "github.com/sirupsen/logrus"
)

var state struct {
	ctx       *gousb.Context
	device    *gousb.Device
	config    *gousb.Config
	iface     *gousb.Interface
	wendpoint *gousb.OutEndpoint
	rendpoint *gousb.InEndpoint

	mtx sync.Mutex
}

func usbopen(cid string, serial string) (err error) {
	if state.ctx == nil {
		log.WithField("Correlation-ID", cid).Debug("usb context not yet open")
		state.ctx = gousb.NewContext()
		if state.ctx == nil {
			return fmt.Errorf("unable to create a usb context")
		}
	}
	if state.device != nil {
		log.WithField("Correlation-ID", cid).Debug("usb device already open")
		return nil
	}

	var devs []*gousb.Device
	devs, err = state.ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		if desc.Vendor == 0x1050 && desc.Product == 0x0030 {
			return true
		}
		return false
	})

	// If len(devs) > 0 we're happy even if there are errors, because
	// gousb will try to open all the devices that match, but will also
	// return the last of any error encountered when interacting with
	// *any* device, even the ones we're not interested in.
	if len(devs) == 0 && err != nil {
		goto out
	}

	for _, dev := range devs {
		serialnumber, err := dev.SerialNumber()
		if err != nil {
			log.WithFields(log.Fields{
				"Correlation-ID": cid,
				"Device":         dev,
				"Error":          err,
			}).Debug("Couldn't read serial number from device")

			dev.Close()
			continue
		}
		fields := log.Fields{
			"Correlation-ID": cid,
			"Device-Serial":  serialnumber,
			"Wanted-Serial":  serial,
		}
		if serial != "" && serial != serialnumber {
			log.WithFields(fields).Debug("Device skipped for non-matching serial")
			dev.Close()
		} else {
			log.WithFields(fields).Debug("Returning a matched device")
			if state.device != nil {
				// A new matching device will override the previously selected
				// one, close the one we're overriding.
				state.device.Close()
			}
			state.device = dev
		}
	}
	if state.device == nil {
		err = fmt.Errorf("device not found")
		goto out
	}
	state.device.ControlTimeout = 5 * time.Second

	state.config, err = state.device.Config(1)
	if err != nil {
		goto out
	}

	state.iface, err = state.config.Interface(0, 0)
	if err != nil {
		goto out
	}

	state.wendpoint, err = state.iface.OutEndpoint(0x1)
	if err != nil {
		goto out
	}

	state.rendpoint, err = state.iface.InEndpoint(0x81)
	if err != nil {
		goto out
	}

	usbread(cid, 1*time.Millisecond)

	return nil

out:
	usbclose(cid)
	return err
}

func usbclose(cid string) {
	if state.iface != nil {
		state.iface.Close()
		state.iface = nil
	}
	if state.config != nil {
		state.config.Close()
		state.config = nil
	}
	if state.device != nil {
		state.device.Close()
		state.device = nil
	}
}

func usbreopen(cid string, why error, serial string) (err error) {
	log.WithFields(log.Fields{
		"Correlation-ID": cid,
		"why":            why,
	}).Debug("reopening usb context")

	usbclose(cid)
	return usbopen(cid, serial)
}

func usbCheck(cid string, serial string) (err error) {
	state.mtx.Lock()
	defer state.mtx.Unlock()

	if err = usbopen(cid, serial); err != nil {
		return err
	}

	for {
		if _, err := state.device.SerialNumber(); err != nil {
			log.WithFields(log.Fields{
				"Correlation-ID": cid,
				"Error":          err,
			}).Debug("Couldn't read serial number from device")

			if err = usbreopen(cid, err, serial); err != nil {
				return err
			}
			continue
		}

		break
	}

	return nil
}

func usbwrite(buf []byte, cid string) (err error) {
	var n int
	var ctx context.Context

	ctx = context.Background()
   	if n, err = state.wendpoint.WriteContext(ctx, buf); err != nil {
		goto out
	}
	if len(buf)%64 == 0 {
		var empty []byte
		if n, err = state.wendpoint.WriteContext(ctx, empty); err != nil {
			goto out
		}
	}

out:
	log.WithFields(log.Fields{
		"Correlation-ID": cid,
		"n":              n,
		"err":            err,
		"len":            len(buf),
		"buf":            buf,
	}).Debug("usb endpoint write")

	return err
}

func usbread(cid string, timeout time.Duration) (buf []byte, err error) {
	var n int
	var ctx context.Context

	buf = make([]byte, 8192)
	ctx = context.Background()
    	if timeout > 0 {
    		var cancel func()
    		ctx, cancel = context.WithTimeout(ctx, timeout)
    		defer cancel()
    	}
    	if n, err = state.rendpoint.ReadContext(ctx, buf); err != nil {
		buf = buf[:0]
		goto out
	}
	buf = buf[:n]

out:
	log.WithFields(log.Fields{
		"Correlation-ID": cid,
		"n":              n,
		"err":            err,
		"len":            len(buf),
		"buf":            buf,
	}).Debug("usb endpoint read")

	return buf, err
}

func usbProxy(req []byte, cid string, serial string) (resp []byte, err error) {
	state.mtx.Lock()
	defer state.mtx.Unlock()

	if err = usbopen(cid, serial); err != nil {
		return nil, err
	}

	for i := 0; i < 2; i++ {
		if err = usbwrite(req, cid); err != nil {
			if err2 := usbreopen(cid, err, serial); err2 != nil {
				return nil, err2
			}
			continue
		}

		resp, err = usbread(cid, 0)
		break
	}

	return resp, err
}
