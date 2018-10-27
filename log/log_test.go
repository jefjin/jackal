/*
 * Copyright (c) 2018 Miguel Ángel Ortuño.
 * See the LICENSE file for more information.
 */

package log

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type logFile struct {
	bw      *bytes.Buffer
	closeCh chan bool
}

func newLogFile() *logFile { return &logFile{bw: bytes.NewBuffer(nil), closeCh: make(chan bool)} }

func (lf *logFile) Write(p []byte) (int, error) { return lf.bw.Write(p) }
func (lf *logFile) Close() error                { lf.closeCh <- true; return nil }

func TestDebugLog(t *testing.T) {
	bw, _, tearDown := setupLogger("debug")
	defer tearDown()

	Debugf("test debug log!")
	time.Sleep(time.Millisecond * 100)

	l := bw.String()
	require.True(t, strings.Contains(l, "[DBG]"))
	require.True(t, strings.Contains(l, "\U0001f50D"))
	require.True(t, strings.Contains(l, "test debug log!"))
}

func TestInfoLog(t *testing.T) {
	bw, _, tearDown := setupLogger("info")
	defer tearDown()

	Infof("test info log!")
	time.Sleep(time.Millisecond * 100)

	l := bw.String()
	require.True(t, strings.Contains(l, "[INF]"))
	require.True(t, strings.Contains(l, "\u2139\ufe0f"))
	require.True(t, strings.Contains(l, "test info log!"))
}

func TestWarningLog(t *testing.T) {
	bw, _, tearDown := setupLogger("warning")
	defer tearDown()

	Warnf("test warning log!")
	time.Sleep(time.Millisecond * 100)

	l := bw.String()
	require.True(t, strings.Contains(l, "[WRN]"))
	require.True(t, strings.Contains(l, "\u26a0\ufe0f"))
	require.True(t, strings.Contains(l, "test warning log!"))
}

func TestErrorLog(t *testing.T) {
	bw, _, tearDown := setupLogger("error")
	defer tearDown()

	Errorf("test error log!")
	time.Sleep(time.Millisecond * 100)

	l := bw.String()
	require.True(t, strings.Contains(l, "[ERR]"))
	require.True(t, strings.Contains(l, "\U0001f4a5"))
	require.True(t, strings.Contains(l, "test error log!"))

	bw.Reset()

	Error(errors.New("some error string"))
	time.Sleep(time.Millisecond * 100)

	l = bw.String()
	require.True(t, strings.Contains(l, "some error string"))
}

func TestFatalLog(t *testing.T) {
	var exited bool
	exitHandler = func() {
		exited = true
	}

	bw, _, tearDown := setupLogger("fatal")
	defer tearDown()

	Fatalf("test fatal log!")
	require.True(t, exited)

	l := bw.String()
	require.True(t, strings.Contains(l, "[FTL]"))
	require.True(t, strings.Contains(l, "\U0001f480"))
	require.True(t, strings.Contains(l, "test fatal log!"))

	bw.Reset()
	exited = false

	Fatal(errors.New("some error string"))

	l = bw.String()
	require.True(t, strings.Contains(l, "some error string"))
}

func TestLogFile(t *testing.T) {
	bw, lf, tearDown := setupLogger("debug")

	Debugf("test debug log!")
	time.Sleep(time.Millisecond * 100)

	require.Equal(t, bw.String(), lf.bw.String())

	// make sure file is closed
	tearDown()

	select {
	case <-lf.closeCh:
		require.True(t, true)
	case <-time.After(time.Second):
		require.FailNow(t, "log file has not been closed")
	}
}

func setupLogger(level string) (*bytes.Buffer, *logFile, func()) {
	output := bytes.NewBuffer(nil)
	logFile := newLogFile()
	l, _ := New(level, output, logFile)
	Set(l)
	return output, logFile, func() { Unset() }
}
