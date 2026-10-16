package tinyconf

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"
)

// mapMockDriver returns values by the struct paths of the fields, the key of the value is the lower case path.
type mapMockDriver struct {
	name    string
	mu      sync.Mutex
	values  map[string]any
	changes chan struct{}
}

func newMapMockDriver(name string, values map[string]any) *mapMockDriver {
	return &mapMockDriver{name: name, values: values, changes: make(chan struct{})}
}

// GenDoc returns the values of the registered configs.
func (md *mapMockDriver) GenDoc(registers ...*Registered) string {
	var doc strings.Builder
	for _, register := range registers {
		fmt.Fprintf(&doc, "%+v\n", reflect.ValueOf(register.Config).Elem())
	}
	return doc.String()
}
func (md *mapMockDriver) GetName() string { return md.name }
func (md *mapMockDriver) GetValue(field fmap.Field) (*Value, error) {
	md.mu.Lock()
	defer md.mu.Unlock()
	value, ok := md.values[field.GetStructPath()]
	if !ok {
		return nil, ErrValueNotFound
	}
	if err, isErr := value.(error); isErr {
		return nil, err
	}
	return &Value{Source: strings.ToLower(field.GetStructPath()), Value: value}, nil
}

func (md *mapMockDriver) set(values map[string]any) {
	md.mu.Lock()
	md.values = values
	md.mu.Unlock()
	md.changes <- struct{}{}
}

func (md *mapMockDriver) Watch(ctx context.Context, onChange func()) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-md.changes:
			onChange()
		}
	}
}

// debugLogger records debug messages with all fields as strings.
type debugLogger struct {
	fields  []Field
	mu      *sync.Mutex
	records *[]string
}

func newDebugLogger() *debugLogger {
	return &debugLogger{mu: &sync.Mutex{}, records: &[]string{}}
}

func (l *debugLogger) Error(string, ...Field) {}
func (l *debugLogger) Warn(string, ...Field)  {}
func (l *debugLogger) Info(string, ...Field)  {}
func (l *debugLogger) Debug(msg string, fields ...Field) {
	if msg == "skip" {
		return
	}
	var record strings.Builder
	record.WriteString(msg + ":")
	for _, field := range append(append([]Field{}, l.fields...), fields...) {
		record.WriteString(" " + field.Key + "=" + field.Value.(string))
	}
	l.mu.Lock()
	*l.records = append(*l.records, record.String())
	l.mu.Unlock()
}

func (l *debugLogger) With(fields ...Field) Logger {
	return &debugLogger{fields: append(append([]Field{}, l.fields...), fields...), mu: l.mu, records: l.records}
}

// take returns recorded messages and clears them.
func (l *debugLogger) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	records := *l.records
	*l.records = nil
	return records
}

func TestManager_Parse_Log(t *testing.T) {
	type Config struct {
		Host     string
		Port     int
		Password string `hidden:"true"`
		Unset    string
	}
	log := newDebugLogger()
	defaults := newMapMockDriver("defaults", map[string]any{"Host": "localhost", "Port": 8080})
	file := newMapMockDriver("file", map[string]any{"Port": 9000, "Password": "pass"})
	c, err := New(WithLogger(log), WithDriver(defaults), WithDriver(file))
	assert.NoError(t, err)
	conf := &Config{Host: "initial"}
	assert.NoError(t, c.Register(conf))
	assert.NoError(t, c.Parse(conf))

	// initial values of all fields, then every override by every driver
	assert.Equal(t, []string{
		"initial: config=*tinyconf.Config field=Host value=initial",
		"initial: config=*tinyconf.Config field=Port value=0",
		"initial: config=*tinyconf.Config field=Password value=",
		"initial: config=*tinyconf.Config field=Unset value=",
		"override: config=*tinyconf.Config driver=defaults field=Host key=host value=localhost",
		"override: config=*tinyconf.Config driver=defaults field=Port key=port value=8080",
		"override: config=*tinyconf.Config driver=file field=Port key=port value=9000",
		"override: config=*tinyconf.Config driver=file field=Password key=password value=****",
	}, log.take())

	// the same values are not overridden
	assert.NoError(t, c.Parse(conf))
	assert.Equal(t, []string{
		"initial: config=*tinyconf.Config field=Host value=localhost",
		"initial: config=*tinyconf.Config field=Port value=9000",
		"initial: config=*tinyconf.Config field=Password value=****",
		"initial: config=*tinyconf.Config field=Unset value=",
		"override: config=*tinyconf.Config driver=defaults field=Port key=port value=8080",
		"override: config=*tinyconf.Config driver=file field=Port key=port value=9000",
	}, log.take())
}

func TestManager_Parse_SubConfigLog(t *testing.T) {
	type HTTP struct {
		Host string
		Port int
	}
	type Config struct {
		HTTP HTTP
	}
	log := newDebugLogger()
	c, err := New(WithLogger(log), WithDriver(newMapMockDriver("file", map[string]any{"HTTP.Port": 9000})))
	assert.NoError(t, err)
	assert.NoError(t, c.Register(&Config{}))

	// values of the sub config are initial values
	http := &HTTP{Host: "localhost", Port: 9000}
	assert.NoError(t, c.Parse(http))
	assert.Equal(t, HTTP{Host: "localhost", Port: 9000}, *http)
	assert.Equal(t, []string{
		"initial: config=*tinyconf.HTTP field=HTTP.Host value=localhost",
		"initial: config=*tinyconf.HTTP field=HTTP.Port value=9000",
	}, log.take())
}

func TestSubscribe_Log(t *testing.T) {
	type Config struct {
		Host     string
		Port     int
		Password string `hidden:"true"`
		Name     string
	}
	log := newDebugLogger()
	defaults := newMapMockDriver("defaults", map[string]any{"Host": "localhost", "Port": 8080})
	file := newMapMockDriver("file", map[string]any{"Port": 9000, "Password": "pass", "Name": "a"})
	c, err := New(WithLogger(log), WithDriver(defaults), WithDriver(file))
	assert.NoError(t, err)
	conf := &Config{}
	assert.NoError(t, c.Register(conf))
	assert.NoError(t, c.Parse(conf))
	log.take()
	configs, _ := subscribe[Config](t, c)

	// only changed fields are logged with the driver that has set the new value
	file.set(map[string]any{"Host": "example", "Password": "secret", "Name": "b"})
	receive(t, configs)
	assert.Equal(t, []string{
		"override: config=*tinyconf.Config driver=file field=Host key=host value=example",
		"override: config=*tinyconf.Config driver=defaults field=Port key=port value=8080",
		"override: config=*tinyconf.Config driver=file field=Password key=password value=******",
		"override: config=*tinyconf.Config driver=file field=Name key=name value=b",
	}, log.take())

	// the config with the error is not applied, nothing is logged on the debug level
	file.set(map[string]any{"Port": errors.New("broken value")})
	nothing(t, configs)
	assert.Empty(t, log.take())

	// changes are relative to the last config without errors;
	// Name has no value from drivers anymore and gets the initial value
	file.set(map[string]any{"Host": "example", "Password": "secret"})
	receive(t, configs)
	assert.Equal(t, []string{
		"override: config=*tinyconf.Config field=Name value=",
	}, log.take())
}

func TestSubscribe_SubConfigLog(t *testing.T) {
	type HTTP struct {
		Port int
	}
	type Config struct {
		HTTP HTTP
	}
	log := newDebugLogger()
	file := newMapMockDriver("file", map[string]any{"HTTP.Port": 9000})
	c, err := New(WithLogger(log), WithDriver(file))
	assert.NoError(t, err)
	assert.NoError(t, c.Register(&Config{}))
	https, _ := subscribe[HTTP](t, c)

	// the registered config is logged with the full paths of the fields
	file.set(map[string]any{"HTTP.Port": 9001})
	assert.Equal(t, 9001, receive(t, https).next.Port)
	assert.Equal(t, []string{
		"override: config=*tinyconf.Config driver=file field=HTTP.Port key=http.port value=9001",
	}, log.take())
}
