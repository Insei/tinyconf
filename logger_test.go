package tinyconf

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// levelLogger records the messages of all levels with the fields, it is safe for concurrent use.
type levelLogger struct {
	fields  []Field
	mu      *sync.Mutex
	records *[]string
}

func newLevelLogger() *levelLogger {
	return &levelLogger{mu: &sync.Mutex{}, records: &[]string{}}
}

func (l *levelLogger) record(level, msg string, fields []Field) {
	var record strings.Builder
	record.WriteString(level + " " + msg + ":")
	for _, field := range append(append([]Field{}, l.fields...), fields...) {
		record.WriteString(fmt.Sprintf(" %s=%v", field.Key, field.Value))
	}
	l.mu.Lock()
	*l.records = append(*l.records, record.String())
	l.mu.Unlock()
}

func (l *levelLogger) Debug(msg string, fields ...Field) { l.record("debug", msg, fields) }
func (l *levelLogger) Info(msg string, fields ...Field)  { l.record("info", msg, fields) }
func (l *levelLogger) Warn(msg string, fields ...Field)  { l.record("warn", msg, fields) }
func (l *levelLogger) Error(msg string, fields ...Field) { l.record("error", msg, fields) }

func (l *levelLogger) With(fields ...Field) Logger {
	return &levelLogger{fields: append(append([]Field{}, l.fields...), fields...), mu: l.mu, records: l.records}
}

func (l *levelLogger) get() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), *l.records...)
}

func TestWithDeferredLogger_Parse(t *testing.T) {
	type Config struct {
		Port int
		Host string
	}
	parse := func(opts ...Option) *Manager {
		driver := &pathMockDriver{values: map[string]any{"Port": 9090}, errs: map[string]error{"Host": ErrIncorrectTagSettings}}
		c, err := New(append([]Option{WithDriver(driver)}, opts...)...)
		require.NoError(t, err)
		conf := &Config{}
		require.NoError(t, c.Register(conf))
		require.NoError(t, c.Parse(context.Background(), conf))
		return c
	}

	// the records of the parsing before SetLogger are the same as with the logger set from the start
	expected := newLevelLogger()
	parse(WithLogger(expected))
	require.NotEmpty(t, expected.get())

	deferred := newLevelLogger()
	c := parse(WithDeferredLogger())
	assert.Empty(t, deferred.get())
	c.SetLogger(deferred)
	assert.Equal(t, expected.get(), deferred.get())
	assert.Same(t, deferred, c.logger())
}

func TestDeferredLogger_LevelsAndFields(t *testing.T) {
	c, err := New(WithDeferredLogger())
	require.NoError(t, err)
	log := c.logger()
	parent := log.With(LogField("a", 1))
	child := parent.With(LogField("b", 2))
	fields := []Field{LogField("c", 3)}
	log.Debug("debug", fields...)
	parent.Info("info", fields...)
	child.Warn("warn", fields...)
	child.Error("error")
	// the fields are copied, the caller can reuse the slice
	fields[0] = LogField("changed", 0)

	l := newLevelLogger()
	c.SetLogger(l)
	assert.Equal(t, []string{
		"debug debug: c=3",
		"info info: a=1 c=3",
		"warn warn: a=1 b=2 c=3",
		"error error: a=1 b=2",
	}, l.get())
}

func TestDeferredLogger_Limit(t *testing.T) {
	c, err := New(WithDeferredLogger())
	require.NoError(t, err)
	for i := 0; i < deferredLimit+5; i++ {
		c.logger().Info(fmt.Sprint(i))
	}
	l := newLevelLogger()
	c.SetLogger(l)
	records := l.get()
	require.Len(t, records, deferredLimit+1)
	assert.Equal(t, "warn log records are dropped before the logger is set: count=5", records[0])
	assert.Equal(t, "info 5:", records[1])
	assert.Equal(t, fmt.Sprintf("info %d:", deferredLimit+4), records[deferredLimit])
}

func TestDeferredLogger_AfterSetLogger(t *testing.T) {
	c, err := New(WithDeferredLogger())
	require.NoError(t, err)
	// the logger is taken before SetLogger, like by Parse running concurrently
	log := c.logger().With(LogField("field", "Port"))

	first := newLevelLogger()
	c.SetLogger(first)
	log.Info("late")
	assert.Equal(t, []string{"info late: field=Port"}, first.get())

	// the records go to the current logger of the manager
	second := newLevelLogger()
	c.SetLogger(second)
	log.Info("later")
	assert.Equal(t, []string{"info later: field=Port"}, second.get())
	assert.Len(t, first.get(), 1)
}

func TestManager_SetLogger(t *testing.T) {
	c, err := New(WithLogger(newLevelLogger()))
	require.NoError(t, err)
	l := newLevelLogger()
	c.SetLogger(l)
	assert.Same(t, l, c.logger())

	c.SetLogger(nil)
	assert.Same(t, noop, c.logger())

	// nil drops the kept records and disables logging
	c, err = New(WithDeferredLogger())
	require.NoError(t, err)
	c.logger().Info("dropped")
	c.SetLogger(nil)
	assert.Same(t, noop, c.logger())
}

func TestWithLogger_Order(t *testing.T) {
	l := newLevelLogger()
	c, err := New(WithDeferredLogger(), WithLogger(l))
	require.NoError(t, err)
	assert.Same(t, l, c.logger())

	c, err = New(WithLogger(l), WithDeferredLogger())
	require.NoError(t, err)
	assert.IsType(t, &deferredLogger{}, c.logger())
}

func TestManager_SetLogger_Concurrent(t *testing.T) {
	type Config struct {
		Port int
	}
	c, err := New(WithDriver(&concurrentDriver{}), WithDeferredLogger())
	require.NoError(t, err)
	conf := &Config{}
	require.NoError(t, c.Register(conf))

	// less than deferredLimit with the records of Parse, so nothing is dropped
	const writers, records = 4, 100
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			for j := 0; j < records; j++ {
				c.logger().With(LogField("writer", i)).Info("record")
			}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 10; i++ {
			assert.NoError(t, c.Parse(context.Background(), &Config{}))
		}
	}()

	l := newLevelLogger()
	close(start)
	c.SetLogger(l)
	c.SetLogger(l)
	wg.Wait()

	// no record is lost: kept ones are written by SetLogger, the next ones are written directly
	count := 0
	for _, record := range l.get() {
		if strings.HasPrefix(record, "info record:") {
			count++
		}
	}
	assert.Equal(t, writers*records, count)
}

// concurrentDriver returns the value for every field, it is safe for concurrent use.
type concurrentDriver struct{}

func (concurrentDriver) GenDoc(...*Registered) string { return "" }
func (concurrentDriver) GetName() string              { return "concurrent" }
func (concurrentDriver) GetValue(fmap.Field) (*Value, error) {
	return &Value{Source: "concurrent", Value: 1}, nil
}
