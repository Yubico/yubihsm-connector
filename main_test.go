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
	"testing"
)

type ensureSerialTest struct {
	iserial string
	oserial string
	err     error
}

var ensureSerialTests = []ensureSerialTest{
	{
		"",
		"",
		nil,
	},
	{
		"12345",
		"0000012345",
		nil,
	},
	{
		"abcdef",
		"",
		errInvalidSerial,
	},
	{
		"12345678901234567890",
		"",
		errInvalidSerial,
	},
	{
		"-1",
		"",
		errInvalidSerial,
	},
}

func TestEnsureSerial(t *testing.T) {
	for i, test := range ensureSerialTests {
		serial, err := ensureSerial(test.iserial)
		if err != test.err {
			t.Fatalf("ensureSerialTest %d: got %v: expected: %v", i, err, test.err)
			continue
		} else if err != nil {
			continue
		}
		if serial != test.oserial {
			t.Fatalf("ensureSerial %d: got %q: expected %q",
				i, serial, test.oserial)
		}
	}
}
