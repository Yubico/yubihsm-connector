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

//go:build generate

package main

import (
	"embed"
	"errors"
	"fmt"
	"github.com/Masterminds/semver"
	"log"
	"os"
	"runtime"
	"text/template"
	"time"
)

//go:embed *.tmpl
var embedFs embed.FS

type VersionInput struct {
	Version string

	Major int64
	Minor int64
	Patch int64
	Build int64

	PreRelease string
	Metadata   string
}

func main() {
	envVersion, exists := os.LookupEnv("VERSION")
	if envVersion == "" || !exists {
		log.Fatal(errors.New("failed to read env var VERSION"))
	}
	newVersion, err := semver.NewVersion(envVersion)
	if err != nil {
		log.Fatal(fmt.Errorf("failed to read valid version from env var VERSION=%s: %w", envVersion, err))
	}

	versionInput := VersionInput{
		Version: newVersion.Original(),

		Major: newVersion.Major(),
		Minor: newVersion.Minor(),
		Patch: newVersion.Patch(),
		Build: 0,

		PreRelease: newVersion.Prerelease(),
		Metadata:   newVersion.Metadata(),
	}

	if err := templateIt("version.go", versionInput); err != nil {
		log.Fatal(err)
	}

	if runtime.GOOS == "windows" {
		if err := templateIt("versioninfo.json", versionInput); err != nil {
			log.Fatal(err)
		}
	}
}

func templateIt(file string, vi VersionInput) error {
	outFile, err := os.Create(file)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", file, err)
	}
	defer outFile.Close()

	templateFile := file + ".tmpl"
	funcMap := template.FuncMap{"now": time.Now}
	parsedFile, err := template.New(templateFile).Funcs(funcMap).ParseFS(embedFs, templateFile)
	if err != nil {
		return fmt.Errorf("failed to parse template file %s: %w", templateFile, err)
	}

	if err := parsedFile.Execute(outFile, vi); err != nil {
		return fmt.Errorf("failed to generate output file %s: %w", outFile.Name(), err)
	}
	return nil
}
