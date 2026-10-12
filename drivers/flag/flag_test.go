package flag

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"

	"github.com/insei/tinyconf"
)

type testConfig struct {
	HTTP struct {
		Host    string        `flag:"http-host" doc:"http host"`
		Port    int           `flag:"http-port" doc:"http port"`
		Timeout time.Duration `flag:"http-timeout"`
	}
	Debug    bool           `flag:"debug" doc:"debug mode"`
	DebugPtr *bool          `flag:"debug-ptr"`
	Hosts    []string       `flag:"host" doc:"hosts"`
	Paths    []string       `flag:"path" separator:":"`
	Labels   map[string]int `flag:"labels"`
	Password string         `flag:"password,omitempty" doc:"password" hidden:"true"`
	Excluded string         `flag:"-"`
	Skipped  string
}

func TestFlagDriver_GetValue(t *testing.T) {
	storage, err := fmap.Get[testConfig]()
	assert.NoError(t, err)
	enabled := true
	tests := []struct {
		name    string
		args    []string
		path    string
		want    any
		source  string
		wantErr error
		anyErr  bool
	}{
		{name: "single dash with equal sign", args: []string{"-http-host=localhost"}, path: "HTTP.Host", want: "localhost", source: "-http-host"},
		{name: "double dash with equal sign", args: []string{"--http-port=80"}, path: "HTTP.Port", want: 80, source: "-http-port"},
		{name: "single dash with separate value", args: []string{"-http-port", "80"}, path: "HTTP.Port", want: 80, source: "-http-port"},
		{name: "double dash with separate value", args: []string{"--http-timeout", "5s"}, path: "HTTP.Timeout", want: 5 * time.Second, source: "-http-timeout"},
		{name: "negative value", args: []string{"--http-port", "-1"}, path: "HTTP.Port", want: -1, source: "-http-port"},
		{name: "empty value", args: []string{"--http-host="}, path: "HTTP.Host", want: "", source: "-http-host"},
		{name: "value with equal sign", args: []string{"--http-host=a=b"}, path: "HTTP.Host", want: "a=b", source: "-http-host"},
		{name: "last flag wins", args: []string{"--http-port=80", "--http-port=8080"}, path: "HTTP.Port", want: 8080, source: "-http-port"},
		{name: "among other arguments", args: []string{"run", "-v", "--other", "value", "--http-port=80", "file"}, path: "HTTP.Port", want: 80, source: "-http-port"},
		{name: "bool without value", args: []string{"--debug", "--http-port=80"}, path: "Debug", want: true, source: "-debug"},
		{name: "bool with value", args: []string{"--debug=false"}, path: "Debug", want: false, source: "-debug"},
		{name: "bool does not use next argument", args: []string{"--debug", "false"}, path: "Debug", want: true, source: "-debug"},
		{name: "pointer to bool without value", args: []string{"--debug-ptr"}, path: "DebugPtr", want: &enabled, source: "-debug-ptr"},
		{name: "slice", args: []string{"--host=a,b"}, path: "Hosts", want: []string{"a", "b"}, source: "-host"},
		{name: "slice with repeated flags", args: []string{"--host=a", "--host", "b,c"}, path: "Hosts", want: []string{"a", "b", "c"}, source: "-host"},
		{name: "slice with repeated flags and custom separator", args: []string{"--path=/bin", "--path=/usr/bin"}, path: "Paths", want: []string{"/bin", "/usr/bin"}, source: "-path"},
		{name: "map", args: []string{"--labels=a:1,b:2"}, path: "Labels", want: map[string]int{"a": 1, "b": 2}, source: "-labels"},
		{name: "tag with options", args: []string{"--password=secret"}, path: "Password", want: "secret", source: "-password"},
		{name: "not provided", args: []string{"--http-host=localhost"}, path: "HTTP.Port", wantErr: tinyconf.ErrValueNotFound},
		{name: "prefix of the other flag", args: []string{"--http-port-extra=1"}, path: "HTTP.Port", wantErr: tinyconf.ErrValueNotFound},
		{name: "after terminator", args: []string{"--", "--http-port=80"}, path: "HTTP.Port", wantErr: tinyconf.ErrValueNotFound},
		{name: "positional argument", args: []string{"http-port=80", "-"}, path: "HTTP.Port", wantErr: tinyconf.ErrValueNotFound},
		{name: "without tag", args: []string{"--Skipped=1"}, path: "Skipped", wantErr: tinyconf.ErrIncorrectTagSettings},
		{name: "excluded", args: []string{"---=1"}, path: "Excluded", wantErr: tinyconf.ErrIncorrectTagSettings},
		{name: "without value", args: []string{"--http-port"}, path: "HTTP.Port", anyErr: true},
		{name: "incorrect value", args: []string{"--http-port=http"}, path: "HTTP.Port", anyErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver, err := New(WithArgs(tt.args))
			assert.NoError(t, err)
			val, err := driver.GetValue(storage.MustFind(tt.path))
			switch {
			case tt.wantErr != nil:
				assert.ErrorIs(t, err, tt.wantErr)
			case tt.anyErr:
				assert.Error(t, err)
				assert.False(t, errors.Is(err, tinyconf.ErrValueNotFound))
				assert.False(t, errors.Is(err, tinyconf.ErrIncorrectTagSettings))
			default:
				assert.NoError(t, err)
				assert.Equal(t, &tinyconf.Value{Source: tt.source, Value: tt.want}, val)
			}
		})
	}
}

func TestFlagDriver_Parse(t *testing.T) {
	driver, err := New(WithArgs([]string{"--http-host", "localhost", "--debug", "--host=a", "--host=b"}))
	assert.NoError(t, err)
	assert.Equal(t, "flag", driver.GetName())
	manager, err := tinyconf.New(tinyconf.WithDriver(driver))
	assert.NoError(t, err)

	conf := &testConfig{}
	conf.HTTP.Port = 80
	assert.NoError(t, manager.Register(conf))
	assert.NoError(t, manager.Parse(conf))

	want := &testConfig{Debug: true, Hosts: []string{"a", "b"}}
	want.HTTP.Host, want.HTTP.Port = "localhost", 80
	assert.Equal(t, want, conf)
}

func TestFlagDriver_GenDoc(t *testing.T) {
	conf := &testConfig{Hosts: []string{"a", "b"}, Password: "secret", Excluded: "excluded", Skipped: "skipped"}
	conf.HTTP.Host, conf.HTTP.Port = "localhost", 80
	storage, err := fmap.GetFrom(conf)
	assert.NoError(t, err)

	driver, err := New()
	assert.NoError(t, err)
	register := &tinyconf.Registered{Storage: storage, Config: conf}
	assert.Equal(t, "  -http-host\n    \thttp host (default localhost)\n"+
		"  -http-port\n    \thttp port (default 80)\n"+
		"  -http-timeout\n    \t (default 0s)\n"+
		"  -debug\n    \tdebug mode (default false)\n"+
		"  -debug-ptr\n    \t\n"+
		"  -host\n    \thosts (default a,b)\n"+
		"  -path\n    \t\n"+
		"  -labels\n    \t\n"+
		"  -password\n    \tpassword\n", driver.GenDoc(register, register))
}

func TestNew(t *testing.T) {
	type config struct {
		Name string `flag:"tinyconf-test-name"`
	}
	storage, err := fmap.Get[config]()
	assert.NoError(t, err)

	args := os.Args
	t.Cleanup(func() { os.Args = args })
	os.Args = []string{"program", "--tinyconf-test-name=from args"}

	driver, err := New()
	assert.NoError(t, err)
	val, err := driver.GetValue(storage.MustFind("Name"))
	assert.NoError(t, err)
	assert.Equal(t, "from args", val.Value)
}
