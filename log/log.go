/*
 * Copyright (c) 2018 Miguel Ángel Ortuño.
 * See the LICENSE file for more information.
 */

package log

import (
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const logChanBufferSize = 512

const projectFolder = "jackal"

var exitHandler = func() { os.Exit(-1) }

// singleton interface
var (
	instMu      sync.RWMutex
	inst        *Logger
	initialized bool
)

// Logger object is used to log messages for a specific system or application component.
type Logger struct {
	level     LogLevel
	outWriter io.Writer
	f         *os.File
	recCh     chan record
	closeCh   chan bool
	b         strings.Builder
}

func newLogger(cfg *Config, outWriter io.Writer) (*Logger, error) {
	l := &Logger{
		level:     cfg.Level,
		outWriter: outWriter,
	}
	if len(cfg.LogPath) > 0 {
		// create logFile intermediate directories.
		if err := os.MkdirAll(filepath.Dir(cfg.LogPath), os.ModePerm); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(cfg.LogPath, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
		if err != nil {
			return nil, err
		}
		l.f = f
	}
	l.recCh = make(chan record, logChanBufferSize)
	l.closeCh = make(chan bool)
	go l.loop()
	return l, nil
}

// Initialize initializes the default log subsystem.
func Initialize(cfg *Config) {
	instMu.Lock()
	defer instMu.Unlock()
	if initialized {
		return
	}
	l, err := newLogger(cfg, os.Stdout)
	if err != nil {
		log.Fatalf("%v", err)
	}
	inst = l
	initialized = true
}

func instance() *Logger {
	instMu.RLock()
	defer instMu.RUnlock()
	return inst
}

// Shutdown shuts down log sub system.
// This method should be used only for testing purposes.
func Shutdown() {
	instMu.Lock()
	defer instMu.Unlock()
	if !initialized {
		return
	}
	inst.closeCh <- true
	inst = nil
	initialized = false
}

// Debugf logs a 'debug' message to the log file
// and echoes it to the console.
func Debugf(format string, args ...interface{}) {
	if inst := instance(); inst != nil && inst.level <= DebugLevel {
		ci := getCallerInfo()
		inst.writeLog(ci.pkg, ci.filename, ci.line, format, DebugLevel, true, args...)
	}
}

// Infof logs an 'info' message to the log file
// and echoes it to the console.
func Infof(format string, args ...interface{}) {
	if inst := instance(); inst != nil && inst.level <= InfoLevel {
		ci := getCallerInfo()
		inst.writeLog(ci.pkg, ci.filename, ci.line, format, InfoLevel, true, args...)
	}
}

// Warnf logs a 'warning' message to the log file
// and echoes it to the console.
func Warnf(format string, args ...interface{}) {
	if inst := instance(); inst != nil && inst.level <= WarningLevel {
		ci := getCallerInfo()
		inst.writeLog(ci.pkg, ci.filename, ci.line, format, WarningLevel, true, args...)
	}
}

// Errorf logs an 'error' message to the log file
// and echoes it to the console.
func Errorf(format string, args ...interface{}) {
	if inst := instance(); inst != nil && inst.level <= ErrorLevel {
		ci := getCallerInfo()
		inst.writeLog(ci.pkg, ci.filename, ci.line, format, ErrorLevel, true, args...)
	}
}

// Fatalf logs a 'fatal' message to the log file
// and echoes it to the console.
// Application should terminate after logging.
func Fatalf(format string, args ...interface{}) {
	if inst := instance(); inst != nil {
		ci := getCallerInfo()
		inst.writeLog(ci.pkg, ci.filename, ci.line, format, FatalLevel, false, args...)
	}
}

// Error logs an 'error' value to the log file
// and echoes it to the console.
func Error(err error) {
	if inst := instance(); inst != nil && inst.level <= ErrorLevel {
		ci := getCallerInfo()
		inst.writeLog(ci.pkg, ci.filename, ci.line, "%v", ErrorLevel, true, err)
	}
}

// Fatal logs an 'error' value to the log file
// and echoes it to the console.
// Application should terminate after logging.
func Fatal(err error) {
	ci := getCallerInfo()
	inst.writeLog(ci.pkg, ci.filename, ci.line, "%v", FatalLevel, true, err)
}

type callerInfo struct {
	pkg      string
	filename string
	line     int
}

type record struct {
	level      LogLevel
	pkg        string
	file       string
	line       int
	log        string
	continueCh chan struct{}
}

func (l *Logger) writeLog(pkg, file string, line int, format string, level LogLevel, async bool, args ...interface{}) {
	entry := record{
		level:      level,
		pkg:        pkg,
		file:       file,
		line:       line,
		log:        fmt.Sprintf(format, args...),
		continueCh: make(chan struct{}),
	}
	select {
	case l.recCh <- entry:
		if !async {
			<-entry.continueCh // wait until done
		}
	default:
		break // avoid blocking...
	}
}

func (l *Logger) loop() {
	for {
		select {
		case rec := <-l.recCh:
			l.b.Reset()

			l.b.WriteString(time.Now().Format("2006-01-02 15:04:05"))
			l.b.WriteString(" ")
			l.b.WriteString(logLevelGlyph(rec.level))
			l.b.WriteString(" [")
			l.b.WriteString(logLevelAbbreviation(rec.level))
			l.b.WriteString("] ")

			l.b.WriteString(rec.pkg)
			if len(rec.pkg) > 0 {
				l.b.WriteString("/")
			}
			l.b.WriteString(rec.file)
			l.b.WriteString(":")
			l.b.WriteString(strconv.Itoa(rec.line))
			l.b.WriteString(" - ")
			l.b.WriteString(rec.log)
			l.b.WriteString("\n")

			line := l.b.String()
			if l.f != nil {
				l.f.WriteString(line)
			}
			switch rec.level {
			case DebugLevel, WarningLevel, InfoLevel, ErrorLevel:
				fmt.Fprintf(l.outWriter, line)
			case FatalLevel:
				fmt.Fprintf(l.outWriter, line)
				exitHandler()
			}
			close(rec.continueCh)

		case <-l.closeCh:
			if l.f != nil {
				l.f.Close()
			}
			return
		}
	}
}

func getCallerInfo() callerInfo {
	ci := callerInfo{}
	_, file, ln, ok := runtime.Caller(2)
	if ok {
		ci.pkg = filepath.Base(path.Dir(file))
		if ci.pkg == projectFolder {
			ci.pkg = ""
		}
		filename := filepath.Base(file)
		ci.filename = strings.TrimSuffix(filename, filepath.Ext(filename))
		ci.line = ln
	} else {
		ci.filename = "???"
		ci.pkg = "???"
	}
	return ci
}

func logLevelAbbreviation(level LogLevel) string {
	switch level {
	case DebugLevel:
		return "DBG"
	case InfoLevel:
		return "INF"
	case WarningLevel:
		return "WRN"
	case ErrorLevel:
		return "ERR"
	case FatalLevel:
		return "FTL"
	default:
		// should not be reached
		return ""
	}
}

func logLevelGlyph(level LogLevel) string {
	switch level {
	case DebugLevel:
		return "\U0001f50D"
	case InfoLevel:
		return "\u2139\ufe0f"
	case WarningLevel:
		return "\u26a0\ufe0f"
	case ErrorLevel:
		return "\U0001f4a5"
	case FatalLevel:
		return "\U0001f480"
	default:
		// should not be reached
		return ""
	}
}
