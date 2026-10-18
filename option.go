package tinyconf

type driverOption struct {
	driver Driver
}

func (o driverOption) apply(config *Manager) {
	if o.driver != nil {
		config.drivers = append(config.drivers, o.driver)
	}
}

func countDrivers(opts ...Option) uint8 {
	var driversCount uint8 = 0
	for _, opt := range opts {
		if _, ok := opt.(driverOption); ok {
			driversCount++
		}
	}
	return driversCount
}

func WithDriver(driver Driver) Option {
	return driverOption{driver: driver}
}

type loggerOption struct {
	logger Logger
}

func (o loggerOption) apply(config *Manager) {
	if o.logger != nil {
		config.storeLogger(o.logger)
	}
}

// WithLogger sets the logger of the Manager, the logger can be replaced later by Manager.SetLogger.
func WithLogger(logger Logger) Option {
	return loggerOption{logger: logger}
}

type deferredLoggerOption struct{}

func (deferredLoggerOption) apply(config *Manager) {
	config.storeLogger(&deferredLogger{buf: &deferredBuffer{}})
}

// WithDeferredLogger keeps the log records in memory until the logger is set by Manager.SetLogger,
// then they are written to it. It solves the case of the logger configured by the config itself:
// the config is parsed before the logger exists, and the logs of this parsing are not lost.
// Up to 1000 records are kept, the oldest are dropped.
// The last of WithLogger and WithDeferredLogger is used.
func WithDeferredLogger() Option {
	return deferredLoggerOption{}
}
