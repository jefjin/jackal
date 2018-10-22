/*
 * Copyright (c) 2018 Miguel Ángel Ortuño.
 * See the LICENSE file for more information.
 */

package application

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/ortuman/jackal/c2s"
	"github.com/ortuman/jackal/component"
	"github.com/ortuman/jackal/log"
	"github.com/ortuman/jackal/module"
	"github.com/ortuman/jackal/router"
	"github.com/ortuman/jackal/s2s"
	"github.com/ortuman/jackal/storage"
	"github.com/ortuman/jackal/version"
)

var logoStr = []string{
	`        __               __            __   `,
	`       |__|____    ____ |  | _______  |  |  `,
	`       |  \__  \ _/ ___\|  |/ /\__  \ |  |  `,
	`       |  |/ __ \\  \___|    <  / __ \|  |__`,
	`   /\__|  (____  /\___  >__|_ \(____  /____/`,
	`   \______|    \/     \/     \/     \/      `,
}

const usageStr = `
Usage: jackal [options]

Server Options:
    -c, --Config <file>    Configuration file path
Common Options:
    -h, --help             Show this message
    -v, --version          Show version
`

var initLogger = func(config *loggerConfig, stdOut io.Writer) (log.Logger, error) {
	var logFiles []io.WriteCloser
	if len(config.LogPath) > 0 {
		// create logFile intermediate directories.
		if err := os.MkdirAll(filepath.Dir(config.LogPath), os.ModePerm); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(config.LogPath, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
		if err != nil {
			return nil, err
		}
		logFiles = append(logFiles, f)
	}
	logger, err := log.New(config.Level, stdOut, logFiles...)
	if err != nil {
		return nil, err
	}
	return logger, nil
}

var initStorage = func(config *storage.Config) (storage.Storage, error) {
	return storage.New(config)
}

type Application struct {
	output   io.Writer
	logger   log.Logger
	storage  storage.Storage
	router   *router.Router
	mods     *module.Modules
	comps    *component.Components
	s2s      *s2s.S2S
	c2s      *c2s.C2S
	debugSrv *http.Server
}

func New(output io.Writer) *Application {
	return &Application{output: output}
}

func (a *Application) Run() error {
	var configFile string
	var showVersion, showUsage bool

	flag.BoolVar(&showUsage, "help", false, "Show this message")
	flag.BoolVar(&showUsage, "h", false, "Show this message")
	flag.BoolVar(&showVersion, "version", false, "Print version information.")
	flag.BoolVar(&showVersion, "v", false, "Print version information.")
	flag.StringVar(&configFile, "config", "/etc/jackal/jackal.yml", "Configuration file path.")
	flag.StringVar(&configFile, "c", "/etc/jackal/jackal.yml", "Configuration file path.")
	flag.Usage = func() {
		for i := range logoStr {
			fmt.Fprintf(a.output, "%s\n", logoStr[i])
		}
		fmt.Fprintf(a.output, "%s\n", usageStr)
	}
	flag.Parse()

	// print usage
	if showUsage {
		flag.Usage()
		return nil
	}
	// print version
	if showVersion {
		fmt.Fprintf(a.output, "jackal version: %v\n", version.ApplicationVersion)
		return nil
	}
	// load configuration
	var cfg Config
	err := cfg.FromFile(configFile)
	if err != nil {
		return err
	}
	// create PID file
	if err := a.createPIDFile(cfg.PIDFile); err != nil {
		return err
	}

	// initialize logger
	a.logger, err = initLogger(&cfg.Logger, a.output)
	if err != nil {
		return err
	}
	log.Set(a.logger)
	defer log.Unset()

	// initialize storage
	a.storage, err = initStorage(&cfg.Storage)
	if err != nil {
		return err
	}
	storage.Set(a.storage)
	defer storage.Unset()

	// initialize router
	a.router, err = router.New(&cfg.Router)
	if err != nil {
		return err
	}

	// initialize modules & components...
	a.mods = module.New(&cfg.Modules, a.router)
	defer a.mods.Close()

	a.comps = component.New(&cfg.Components, a.mods.DiscoInfo)
	defer a.comps.Close()

	a.printLogo()

	// start serving s2s...
	a.s2s = s2s.New(cfg.S2S, a.mods, a.router)
	if a.s2s.Enabled() {
		a.router.SetS2SOutProvider(a.s2s)
		a.s2s.Start()
		defer a.s2s.Stop()

	} else {
		log.Infof("s2s disabled")
	}

	// start serving c2s...
	a.c2s, err = c2s.New(cfg.C2S, a.mods, a.comps, a.router)
	if err != nil {
		return err
	}
	a.c2s.Start()
	defer a.c2s.Stop()

	// initialize debug server...
	if cfg.Debug.Port > 0 {
		if err := a.initDebugServer(cfg.Debug.Port); err != nil {
			return err
		}
		defer a.debugSrv.Close()
	}
	a.waitForStopSignal()
	return nil
}

func (a *Application) showVersion() {
	fmt.Fprintf(a.output, "jackal version: %v\n", version.ApplicationVersion)
}

func (a *Application) createPIDFile(pidFile string) error {
	if len(pidFile) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(pidFile), os.ModePerm); err != nil {
		return err
	}
	file, err := os.Create(pidFile)
	if err != nil {
		return err
	}
	defer file.Close()

	currentPid := os.Getpid()
	if _, err := file.WriteString(strconv.FormatInt(int64(currentPid), 10)); err != nil {
		return err
	}
	return nil
}

func (a *Application) printLogo() {
	for i := range logoStr {
		log.Infof("%s", logoStr[i])
	}
	log.Infof("")
	log.Infof("jackal %v\n", version.ApplicationVersion)
}

func (a *Application) initDebugServer(port int) error {
	a.debugSrv = &http.Server{}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	go a.debugSrv.Serve(ln)
	return nil
}

func (a *Application) waitForStopSignal() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	<-c
}
