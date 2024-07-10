package logger

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/insei/tinyconf"
)

// define a utility to capture stdout
func captureStdout(f func()) string {
	old := os.Stdout // keep backup of the real stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	outC := make(chan string)
	// copy the output in a separate goroutine so printing can't block indefinitely
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r) // nolint:errcheck
		outC <- buf.String()
	}()

	// back to normal state
	w.Close()
	os.Stdout = old // restoring the real stdout
	out := <-outC

	return out
}

func TestMsgMethod(t *testing.T) {
	tests := []struct {
		logLevel    level
		msgLevel    string
		expectedOut string
	}{
		{DEBUG, "DEBUG", "DEBUG"},
		{ERROR, "INFO", ""},
		{INFO, "DEBUG", ""},
		{INFO, "INFO", "INFO"},
	}

	for _, tt := range tests {
		t.Run(tt.expectedOut, func(t *testing.T) {
			logger := &fmtLogger{lvl: tt.logLevel}
			out := captureStdout(func() { logger.msg(tt.msgLevel, "Some message") })
			if tt.expectedOut != "" && !strings.Contains(out, tt.expectedOut) {
				t.Errorf("Expected output containing '%s', but got '%s'", tt.expectedOut, out)
			}
			if tt.expectedOut == "" && out != "" {
				t.Errorf("Expected no output, but got '%s'", out)
			}
		})
	}
}

func TestDebug(t *testing.T) {
	msg := "debug"
	logger := &fmtLogger{lvl: DEBUG}

	out := captureStdout(func() {
		logger.Debug(msg)
	})

	if !strings.Contains(out, msg) {
		t.Errorf("Expected '%s', but got '%s'", msg, out)
	}
}

func TestError(t *testing.T) {
	msg := "error"
	logger := &fmtLogger{lvl: ERROR}

	out := captureStdout(func() {
		logger.Error(msg)
	})

	if !strings.Contains(out, msg) {
		t.Errorf("Expected '%s', but got '%s'", msg, out)
	}
}

func TestWarning(t *testing.T) {
	msg := "warning"
	logger := &fmtLogger{lvl: WARN}

	out := captureStdout(func() {
		logger.Warn(msg)
	})

	if !strings.Contains(out, msg) {
		t.Errorf("Expected '%s', but got '%s'", msg, out)
	}
}

func TestInfo(t *testing.T) {
	msg := "info"
	logger := &fmtLogger{lvl: INFO}

	out := captureStdout(func() {
		logger.Info(msg)
	})

	if !strings.Contains(out, msg) {
		t.Errorf("Expected '%s', but got '%s'", msg, out)
	}
}

func TestWith(t *testing.T) {
	field := tinyconf.LogField("key", "value")
	logger := &fmtLogger{lvl: DEBUG}

	newLogger := logger.With(field)

	if newLogger.(*fmtLogger).fields[0] != field {
		t.Error("fields don't match in logger.With()")
	}
}

func TestLevelFromString(t *testing.T) {
	tests := []struct {
		in  string
		out level
	}{
		{"DEBUG", DEBUG},
		{"INFO", INFO},
		{"WARN", WARN},
		{"ERROR", ERROR},
		{"FATAL", FATAL},
		{"TRACE", TRACE},
		{"SOMETHING", level(100)},
	}

	for _, tt := range tests {
		t.Run(string(tt.out), func(t *testing.T) {
			if result := levelFromString(tt.in); result != tt.out {
				t.Errorf("Expected level '%d', but got '%d'", tt.out, result)
			}
		})
	}
}

func TestNewFmtLogger(t *testing.T) {
	logger := NewFmtLogger(DEBUG)

	_, ok := logger.(*fmtLogger)
	if !ok {
		t.Error("The type of logger is not '*fmtLogger'")
	}
}
