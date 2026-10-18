package tinyconf

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"
)

// errorMockDriver returns the error for the field Denied, its Load and Watch return the errors from the fields.
type errorMockDriver struct {
	loadErr  error
	watchErr error
}

func (md *errorMockDriver) GenDoc(...*Registered) string { return "" }
func (md *errorMockDriver) GetName() string              { return "errors" }
func (md *errorMockDriver) Load() error                  { return md.loadErr }
func (md *errorMockDriver) GetValue(field fmap.Field) (*Value, error) {
	switch field.GetStructPath() {
	case "Denied":
		return nil, fmt.Errorf("%w: no permission to read denied", ErrAccessDenied)
	case "Failed":
		return nil, errors.New("connection lost")
	}
	return &Value{Source: "key", Value: "value"}, nil
}

func (md *errorMockDriver) Watch(context.Context, func()) error {
	return md.watchErr
}

// errorLogger records error messages with the details.
type errorLogger struct {
	noopLogger
	mu     *sync.Mutex
	errors *[]string
}

func (l *errorLogger) Error(msg string, fields ...Field) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, field := range fields {
		if field.Key == "details" {
			msg += ": " + field.Value.(string)
		}
	}
	*l.errors = append(*l.errors, msg)
}

func (l *errorLogger) With(...Field) Logger { return l }

func TestManager_AccessDenied(t *testing.T) {
	type Config struct {
		Denied string
		Failed string
	}
	var logged []string
	log := &errorLogger{mu: &sync.Mutex{}, errors: &logged}
	driver := &errorMockDriver{}
	c, err := New(WithDriver(driver), WithLogger(log))
	assert.NoError(t, err)
	conf := &Config{}
	assert.NoError(t, c.Register(conf))

	// access denied is logged by its own message and can be checked by errors.Is
	err = c.Parse(context.Background(), conf)
	assert.ErrorIs(t, err, ErrAccessDenied)
	assert.Equal(t, []string{
		"access denied: access denied: no permission to read denied",
		"failed: connection lost",
	}, logged)

	logged = nil
	driver.loadErr = fmt.Errorf("%w: no permission to read the source", ErrAccessDenied)
	assert.ErrorIs(t, c.Parse(context.Background(), conf), ErrAccessDenied)
	driver.loadErr = errors.New("source is not available")
	assert.Error(t, c.Parse(context.Background(), conf))
	assert.Equal(t, []string{
		"access denied: access denied: no permission to read the source",
		"failed to load: source is not available",
	}, logged)

	for watchErr, message := range map[error]string{
		fmt.Errorf("%w: no permission to watch", ErrAccessDenied): "access denied: access denied: no permission to watch",
		errors.New("watching is failed"):                          "failed to watch: watching is failed",
	} {
		log.mu.Lock()
		logged = nil
		log.mu.Unlock()
		// every case with its own manager, watching is started by the first subscription
		c, err := New(WithDriver(&errorMockDriver{watchErr: watchErr}), WithLogger(log))
		assert.NoError(t, err)
		assert.NoError(t, c.Register(conf))
		ctx, cancel := context.WithCancel(context.Background())
		assert.NoError(t, Subscribe(ctx, c, func(prev, next *Config) {}))
		assert.Eventually(t, func() bool {
			log.mu.Lock()
			defer log.mu.Unlock()
			return len(logged) == 1 && logged[0] == message
		}, 5*time.Second, 10*time.Millisecond)
		cancel()
	}
}
