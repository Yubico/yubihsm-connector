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
	"net/http"
	"os"
	"os/signal"
	"path"
	"regexp"
	"runtime"
	"strings"
	"syscall"

	yaml "gopkg.in/yaml.v2"

	"github.com/kardianos/service"

	log "github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// XXX(thorduri): Barf.
	serial string

	// Host header whitelisting
	hostHeaderWhitelisting bool
	hostHeaderWhitelist    = []string{"localhost", "localhost.", "127.0.0.1", "[::1]"}
)

type program struct {
	srv *http.Server
}

func (p *program) Start(s service.Service) error {
	addr := viper.GetString("listen")
	p.srv = &http.Server{Addr: addr}

	http.HandleFunc("/connector/status", middlewareWrapper(statusHandler))
	http.HandleFunc("/connector/api", middlewareWrapper(apiHandler))

	if viper.GetBool("seccomp") {
		log.Warn("seccomp support has been deprecated and the flag will be removed in future versions")
	}

	tls := false
	cert := viper.GetString("cert")
	key := viper.GetString("key")
	if cert != "" && key != "" {
		tls = true
	}

	log.WithFields(log.Fields{
		"pid":    os.Getpid(),
		"listen": addr,
		"TLS":    tls,
	}).Debug("takeoff")

	go func(tls bool) {
		if tls {
			if err := p.srv.ListenAndServeTLS(cert, key); err != nil {
				log.Printf("ListenAndServeTLS failure: %s", err)
			}
		} else {
			if err := p.srv.ListenAndServe(); err != nil {
				log.Printf("ListenAndServe failure: %s", err)
			}
		}
	}(tls)

	return nil
}

func (p *program) Stop(s service.Service) error {
	return p.srv.Shutdown(nil)
}

//go:generate go run -mod=vendor version.in.go
func main() {
	loggingInit(service.Interactive())
	if !service.Interactive() {
		if runtime.GOOS == "windows" {
			viper.AddConfigPath(path.Join(os.Getenv("ProgramData"), "YubiHSM"))
		} else {
			// These paths will work for most UNIXy platforms. macOS may need something else.
			configPaths := [2]string{"/etc", "/usr/local/etc"}
			for _, configPath := range configPaths {
				viper.AddConfigPath(path.Join(configPath, "yubihsm"))
			}
		}
	}

	svcConfig := &service.Config{
		Name:        "yhconsrv",
		DisplayName: "YubiHSM Connector Service",
		Description: "Implements the http-usb interface for the YubiHSM",
	}
	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
		return
	}

	signalChannel := make(chan os.Signal, 1)

	signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		signalEncountered := <-signalChannel
		log.Info("Shutting down.")

		// Put any process wide shutdown calls here
		usbclose("Process terminate")

		signal.Reset(signalEncountered)
		os.Exit(0)
	}()

	rootCmd := &cobra.Command{
		Use:           "yubihsm-connector",
		Long:          `YubiHSM Connector v` + Version.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if viper.GetBool("debug") {
				log.SetLevel(log.DebugLevel)
			}
			config := viper.GetString("config")
			if config != "" {
				viper.SetConfigFile(config)
			}
		},
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			if err = viper.ReadInConfig(); err != nil {
				if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
					return err
				}
			}

			certkeyErr := fmt.Errorf("cert and key must both be specified")
			if viper.GetString("cert") != "" && viper.GetString("key") == "" {
				return certkeyErr
			} else if viper.GetString("cert") == "" && viper.GetString("key") != "" {
				return certkeyErr
			}

			// XXX(thorduri): Barf.
			serial, err = ensureSerial(viper.GetString("serial"))
			if err != nil {
				return err
			}

			log.WithFields(log.Fields{
				"config":  viper.ConfigFileUsed(),
				"pid":     os.Getpid(),
				"seccomp": viper.GetBool("seccomp"),
				"syslog":  viper.GetBool("syslog"),
				"version": Version.String(),
				"cert":    viper.GetString("cert"),
				"key":     viper.GetString("key"),
				"serial":  serial,
			}).Debug("preflight complete")

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			return s.Run()
		},
	}
	rootCmd.PersistentFlags().StringP("config", "c", "", "config file")
	viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "debug output")
	viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))
	rootCmd.PersistentFlags().BoolP("seccomp", "s", false, "enable seccomp")
	viper.BindPFlag("seccomp", rootCmd.PersistentFlags().Lookup("seccomp"))
	rootCmd.PersistentFlags().StringP("cert", "", "", "certificate (X509)")
	viper.BindPFlag("cert", rootCmd.PersistentFlags().Lookup("cert"))
	rootCmd.PersistentFlags().StringP("key", "", "", "certificate key")
	viper.BindPFlag("key", rootCmd.PersistentFlags().Lookup("key"))
	rootCmd.PersistentFlags().StringP("serial", "", "", "device serial")
	viper.BindPFlag("serial", rootCmd.PersistentFlags().Lookup("serial"))
	rootCmd.PersistentFlags().StringP("listen", "l", "localhost:12345", "listen address")
	viper.BindPFlag("listen", rootCmd.PersistentFlags().Lookup("listen"))
	rootCmd.PersistentFlags().BoolP("syslog", "L", false, "log to syslog/eventlog")
	viper.BindPFlag("syslog", rootCmd.PersistentFlags().Lookup("syslog"))
	rootCmd.PersistentFlags().BoolVar(&hostHeaderWhitelisting, "enable-host-header-whitelist", false, "Enable Host header whitelisting")
	viper.BindPFlag("enable-host-whitelist", rootCmd.PersistentFlags().Lookup("enable-host-header-whitelist"))
	rootCmd.PersistentFlags().StringSliceVar(&hostHeaderWhitelist, "host-header-whitelist", hostHeaderWhitelist, "Host header whitelist")
	viper.BindPFlag("host-whitelist", rootCmd.PersistentFlags().Lookup("host-header-whitelist"))

	configCmd := &cobra.Command{
		Use: "config",
		Long: `YubiHSM Connector configuration

Most configuration knobs for the connector are not available at the command
line, and must be supplied via a configurtion file.

listen: localhost:12345
syslog: false
cert: /path/to/certificate.crt
key: /path/to/certificate.key
serial: 0123456789
`,
	}
	configCheckCmd := &cobra.Command{
		Use:           "check",
		Long:          `Syntax check configuration`,
		SilenceErrors: true,
		SilenceUsage:  true,
		Run: func(cmd *cobra.Command, args []string) {
			if err := viper.ReadInConfig(); err != nil {
				log.WithFields(log.Fields{
					"error":  err,
					"config": viper.ConfigFileUsed(),
				}).Fatal("syntax errors in configuration file")
			} else {
				log.Info("OK!")
			}
		},
	}
	configGenCmd := &cobra.Command{
		Use:           "generate",
		Long:          `Generate a skeleton configuration from default values`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			var buf []byte

			config := viper.AllSettings()
			delete(config, "debug")
			delete(config, "config")
			delete(config, "seccomp")

			if buf, err = yaml.Marshal(&config); err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "%s", buf)
			return nil
		},
	}

	versionCmd := &cobra.Command{
		Use:           "version",
		Long:          `Print program version`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			fmt.Fprintf(os.Stdout, "%s\n", Version.String())
			return nil
		},
	}

	installCmd := &cobra.Command{
		Use:  "install",
		Long: "Install YubiHSM Connector service",
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.Install()
		},
	}

	uninstallCmd := &cobra.Command{
		Use:  "uninstall",
		Long: "Uninstall YubiHSM Connector service",
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.Uninstall()
		},
	}

	startCmd := &cobra.Command{
		Use:  "start",
		Long: "Starts YubiHSM Connector service",
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.Start()
		},
	}

	stopCmd := &cobra.Command{
		Use:  "stop",
		Long: "Stops YubiHSM Connector service",
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.Stop()
		},
	}

	restartCmd := &cobra.Command{
		Use:  "restart",
		Long: "Restarts YubiHSM Connector service",
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.Restart()
		},
	}

	configCmd.AddCommand(configCheckCmd, configGenCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(restartCmd)

	viper.SetConfigName("yubihsm-connector")

	viper.SetEnvPrefix("YUBIHSM_CONNECTOR")
	viper.AutomaticEnv()

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

// XXX(thorduri): Barf.
var errInvalidSerial = fmt.Errorf("invalid device serial")

func ensureSerial(s string) (string, error) {
	if s == "" {
		return "", nil
	} else if len(s) > 10 {
		return "", errInvalidSerial
	}

	n := 10 - len(s)
	s = fmt.Sprintf("%s%s", strings.Repeat("0", n), s)
	matched, err := regexp.MatchString("^[0-9]{10}$", s)
	if err != nil {
		return "", err
	} else if !matched {
		return "", errInvalidSerial
	}

	return s, nil
}
