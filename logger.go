package tinyconf

import (
	"sync"
	"sync/atomic"
)

// deferredLimit is the maximum number of the records kept by the deferred logger, the oldest records are dropped.
const deferredLimit = 1000

// loggerBox stores the logger of the Manager in atomic.Pointer.
type loggerBox struct {
	Logger
}

// managerLogger is the logger of the Manager, it is replaced by SetLogger concurrently with the parsing and reloads.
type managerLogger struct {
	mu  sync.Mutex
	box atomic.Pointer[loggerBox]
}

var noop Logger = &noopLogger{}

// logger returns the current logger, noop logger if it is not set.
func (c *Manager) logger() Logger {
	if box := c.log.box.Load(); box != nil {
		return box.Logger
	}
	return noop
}

func (c *Manager) storeLogger(l Logger) {
	c.log.box.Store(&loggerBox{Logger: l})
}

// SetLogger replaces the logger of the Manager, nil disables logging.
// It is safe for concurrent use: Parse, reloads and other goroutines switch to the new logger.
// The records kept since WithDeferredLogger are written to the logger by the first call,
// so the application can create its logger from the parsed config and get the logs of this parsing too.
func (c *Manager) SetLogger(l Logger) {
	if l == nil {
		l = noop
	}
	c.log.mu.Lock()
	defer c.log.mu.Unlock()
	if deferred, ok := c.logger().(*deferredLogger); ok {
		deferred.buf.flush(c, l)
		return
	}
	c.storeLogger(l)
}

type logLevel uint8

const (
	debugLevel logLevel = iota
	infoLevel
	warnLevel
	errorLevel
)

type logRecord struct {
	level  logLevel
	msg    string
	with   []Field
	fields []Field
}

func (r logRecord) writeTo(l Logger) {
	if len(r.with) > 0 {
		l = l.With(r.with...)
	}
	switch r.level {
	case debugLevel:
		l.Debug(r.msg, r.fields...)
	case infoLevel:
		l.Info(r.msg, r.fields...)
	case warnLevel:
		l.Warn(r.msg, r.fields...)
	default:
		l.Error(r.msg, r.fields...)
	}
}

// deferredBuffer keeps the records of the deferred logger until the logger is set by Manager.SetLogger.
type deferredBuffer struct {
	mu      sync.Mutex
	records []logRecord
	dropped int
	// manager is set by the flush, the next records are written to the current logger of the manager
	manager *Manager
}

func (b *deferredBuffer) write(record logRecord) {
	b.mu.Lock()
	if b.manager != nil {
		b.mu.Unlock()
		// the logger was taken before SetLogger, for example by Parse running concurrently
		record.writeTo(b.manager.logger())
		return
	}
	defer b.mu.Unlock()
	if len(b.records) == deferredLimit {
		b.records[0] = logRecord{}
		b.records = b.records[1:]
		b.dropped++
	}
	b.records = append(b.records, record)
}

// flush sets the logger of the manager and writes the kept records to it.
// The records written concurrently wait for the end of the flush, so they are written after the kept ones.
func (b *deferredBuffer) flush(c *Manager, l Logger) {
	b.mu.Lock()
	defer b.mu.Unlock()
	c.storeLogger(l)
	if b.dropped > 0 {
		l.Warn("log records are dropped before the logger is set", LogField("count", b.dropped))
	}
	for _, record := range b.records {
		record.writeTo(l)
	}
	b.records, b.dropped, b.manager = nil, 0, c
}

// deferredLogger keeps the records in memory until the logger is set by Manager.SetLogger, see WithDeferredLogger.
type deferredLogger struct {
	buf  *deferredBuffer
	with []Field
}

func (l *deferredLogger) log(level logLevel, msg string, fields []Field) {
	l.buf.write(logRecord{level: level, msg: msg, with: l.with, fields: append([]Field(nil), fields...)})
}

func (l *deferredLogger) Debug(msg string, fields ...Field) { l.log(debugLevel, msg, fields) }
func (l *deferredLogger) Info(msg string, fields ...Field)  { l.log(infoLevel, msg, fields) }
func (l *deferredLogger) Warn(msg string, fields ...Field)  { l.log(warnLevel, msg, fields) }
func (l *deferredLogger) Error(msg string, fields ...Field) { l.log(errorLevel, msg, fields) }

func (l *deferredLogger) With(fields ...Field) Logger {
	with := make([]Field, 0, len(l.with)+len(fields))
	with = append(append(with, l.with...), fields...)
	return &deferredLogger{buf: l.buf, with: with}
}
