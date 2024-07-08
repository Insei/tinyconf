package tinyconf

type noopLogger struct{}

func (l *noopLogger) Debug(string, ...Field) {}
func (l *noopLogger) Error(string, ...Field) {}
func (l *noopLogger) Warn(string, ...Field)  {}
func (l *noopLogger) Info(string, ...Field)  {}
func (l *noopLogger) With(...Field) Logger   { return l }
