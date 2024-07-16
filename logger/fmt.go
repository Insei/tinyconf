package logger

import (
	"fmt"

	"github.com/insei/tinyconf"
)

type level uint8

const (
	FATAL = level(0)
	ERROR = level(1)
	WARN  = level(3)
	INFO  = level(4)
	DEBUG = level(5)
	TRACE = level(6)
)

type fmtLogger struct {
	lvl    level
	fields []tinyconf.Field
}

func levelFromString(s string) level {
	switch s {
	case "DEBUG":
		return DEBUG
	case "ERROR":
		return ERROR
	case "WARN":
		return WARN
	case "INFO":
		return INFO
	case "FATAL":
		return FATAL
	case "TRACE":
		return TRACE

	}
	return level(100)
}

func (t *fmtLogger) msg(lvl, msg string, fld ...tinyconf.Field) {
	msgLvl := levelFromString(lvl)
	if t.lvl < msgLvl {
		return
	}
	fields := make([]tinyconf.Field, 0, len(fld)+len(t.fields))
	fields = append(t.fields, fld...)
	_ = fields
	strFields := ""
	for _, f := range fields {
		strFields += fmt.Sprintf("{%s: %v} ", f.Key, f.Value)
	}
	fmt.Printf("[%s] %s: %s\n", lvl, msg, strFields)
}

func (t *fmtLogger) Debug(msg string, fld ...tinyconf.Field) {
	t.msg("DEBUG", msg, fld...)
}

func (t *fmtLogger) Error(msg string, fld ...tinyconf.Field) {
	t.msg("ERROR", msg, fld...)
}
func (t *fmtLogger) Warn(msg string, fld ...tinyconf.Field) {
	t.msg("WARN", msg, fld...)
}

func (t *fmtLogger) Info(msg string, fld ...tinyconf.Field) {
	t.msg("INFO", msg, fld...)
}

func (t *fmtLogger) With(flds ...tinyconf.Field) tinyconf.Logger {
	fields := make([]tinyconf.Field, 0, len(flds)+len(t.fields))
	copy(t.fields, fields)
	fields = append(fields, flds...)
	return &fmtLogger{lvl: t.lvl, fields: fields}
}

func NewFmtLogger(lvl level) tinyconf.Logger {
	return &fmtLogger{lvl: lvl}
}
