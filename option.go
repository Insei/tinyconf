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
		config.log = o.logger
	}
}

func WithLogger(logger Logger) Option {
	return loggerOption{logger: logger}
}
