package tinyconf

// Secret is a string value that is not printed: String, GoString and MarshalText return ****** for not empty values,
// so the secret is masked when the application prints or logs the whole config by fmt, encoding/json, yaml, zap or slog.
// The value is parsed from drivers like a usual string, Value returns it.
//
//	type Config struct {
//		Password tinyconf.Secret `env:"DB_PASSWORD"`
//	}
//
//	db.Connect(user, conf.Password.Value())
type Secret string

const secretMask = "******"

// Value returns the value of the secret.
func (s Secret) Value() string {
	return string(s)
}

func (s Secret) mask() string {
	if s == "" {
		// the empty value is shown, it is useful to see that the secret is not set
		return ""
	}
	return secretMask
}

// String returns ****** for not empty secret, it is used by fmt.
func (s Secret) String() string {
	return s.mask()
}

// GoString returns ****** for not empty secret, it is used by fmt with %#v.
func (s Secret) GoString() string {
	return `"` + s.mask() + `"`
}

// MarshalText returns ****** for not empty secret, it is used by encoding/json, yaml and other encoders.
func (s Secret) MarshalText() ([]byte, error) {
	return []byte(s.mask()), nil
}
