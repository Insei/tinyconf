package tree

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"

	"github.com/insei/tinyconf/v2"
	"github.com/insei/tinyconf/v2/convert"
)

func TestLookup(t *testing.T) {
	tests := []struct {
		name          string
		yamlMap       map[string]any
		getField      func() fmap.Field
		expectedValue any
		wantErr       bool
	}{
		{
			name:    "NonExistingYamlTag",
			yamlMap: nil,
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test int
				}]()
				return storage.MustFind("Test")
			},
			expectedValue: nil,
			wantErr:       true,
		},
		{
			name:    "NonExistingValueInNonExistingYaml",
			yamlMap: nil,
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test int `yaml:"nonexisting"`
				}]()
				return storage.MustFind("Test")
			},
			expectedValue: nil,
			wantErr:       true,
		},
		{
			name:    "NonExistingFieldInExistingYaml",
			yamlMap: map[string]any{"existent": "value"},
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test int `yaml:"nonexistent"`
				}]()
				return storage.MustFind("Test")
			},
			expectedValue: nil,
			wantErr:       true,
		},
		{
			name:    "ExistingNonEmptyField",
			yamlMap: map[string]any{"existent": "value"},
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test int `yaml:"existent"`
				}]()
				return storage.MustFind("Test")
			},
			expectedValue: "value",
			wantErr:       false,
		},
		{
			name:    "NestedExistingNonEmptyField",
			yamlMap: map[string]any{"existent": map[string]any{"nested": "value"}},
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test struct {
						Nested string `yaml:"nested"`
					} `yaml:"existent"`
				}]()
				return storage.MustFind("Test.Nested")
			},
			expectedValue: "value",
			wantErr:       false,
		},
		{
			name:    "NestedNonExistingField",
			yamlMap: map[string]any{"existent": map[string]any{"nested": "value"}},
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test struct {
						Nonested string `yaml:"nonested"`
					} `yaml:"existent"`
				}]()
				return storage.MustFind("Test.Nonested")
			},
			expectedValue: nil,
			wantErr:       true,
		},
		{
			name:    "NestedNonExistingFieldWithEmptyMap",
			yamlMap: map[string]any{"existent": "value"},
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test struct {
						Nonested string `yaml:"nonesting"`
					} `yaml:"existent"`
				}]()
				return storage.MustFind("Test.Nonested")
			},
			expectedValue: nil,
			wantErr:       true,
		},
		{
			name:    "NestedMissingParentDoesNotFallbackToRoot",
			yamlMap: map[string]any{"nested": "root value"},
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test struct {
						Nested string `yaml:"nested"`
					} `yaml:"nonexistent"`
				}]()
				return storage.MustFind("Test.Nested")
			},
			expectedValue: nil,
			wantErr:       true,
		},
		{
			name:    "DeepNestedMissingParentDoesNotFallbackToGrandparent",
			yamlMap: map[string]any{"existent": map[string]any{"nested": "parent value"}},
			getField: func() fmap.Field {
				storage, _ := fmap.Get[struct {
					Test struct {
						Middle struct {
							Nested string `yaml:"nested"`
						} `yaml:"nonexistent"`
					} `yaml:"existent"`
				}]()
				return storage.MustFind("Test.Middle.Nested")
			},
			expectedValue: nil,
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Lookup(tt.getField(), "yaml", tt.yamlMap)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedValue, got)
		})
	}
}

func TestMerge(t *testing.T) {
	dst := map[string]any{
		"http":  map[string]any{"host": "localhost", "port": 80, "tls": map[string]any{"enabled": false}},
		"hosts": []any{"a", "b"},
		"name":  "base",
		"db":    "dsn",
	}
	src := map[string]any{
		"http":  map[string]any{"port": 8080, "tls": map[string]any{"enabled": true}},
		"hosts": []any{"c"},
		"db":    map[string]any{"host": "db"},
		"new":   1,
	}
	Merge(dst, src)
	assert.Equal(t, map[string]any{
		"http":  map[string]any{"host": "localhost", "port": 8080, "tls": map[string]any{"enabled": true}},
		"hosts": []any{"c"},
		"name":  "base",
		"db":    map[string]any{"host": "db"},
		"new":   1,
	}, dst)
}

func TestExpandEnv(t *testing.T) {
	t.Setenv("TINYCONF_TREE_SET", "value")
	t.Setenv("TINYCONF_TREE_EMPTY", "")
	tests := map[string]string{
		"${TINYCONF_TREE_SET}":                     "value",
		"a ${TINYCONF_TREE_SET} b":                 "a value b",
		"${TINYCONF_TREE_SET:-default}":            "value",
		"${TINYCONF_TREE_UNSET}":                   "",
		"${TINYCONF_TREE_UNSET:-default}":          "default",
		"${TINYCONF_TREE_EMPTY:-default}":          "default",
		"${TINYCONF_TREE_UNSET:-}":                 "",
		"$${TINYCONF_TREE_SET}":                    "${TINYCONF_TREE_SET}",
		"$TINYCONF_TREE_SET":                       "$TINYCONF_TREE_SET",
		"pa$$word and $5":                          "pa$word and $5",
		"${TINYCONF_TREE_SET}${TINYCONF_TREE_SET}": "valuevalue",
	}
	for in, want := range tests {
		assert.Equal(t, want, ExpandEnv(in), in)
	}
}

func testDecode(data []byte) (map[string]any, error) {
	result := make(map[string]any)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, errors.New("incorrect line")
		}
		result[parts[0]] = parts[1]
	}
	return result, nil
}

type testConfig struct {
	Name string `test:"name"`
	Port int    `test:"port"`
	Skip string
}

type countSource struct {
	opened int
	data   string
}

func (s *countSource) source() Source {
	return Source{Name: "count", Open: func(context.Context) (io.ReadCloser, error) {
		s.opened++
		return io.NopCloser(strings.NewReader(s.data)), nil
	}}
}

func TestDriver(t *testing.T) {
	storage, err := fmap.Get[testConfig]()
	assert.NoError(t, err)
	name, port, skip := storage.MustFind("Name"), storage.MustFind("Port"), storage.MustFind("Skip")

	t.Run("values", func(t *testing.T) {
		driver := New("test", testDecode, []Source{
			DataSource("first", []byte("name=first\nport=80")),
			DataSource("second", []byte("name=second")),
		}, false)
		assert.Equal(t, "test", driver.GetName())
		assert.NoError(t, driver.Load())

		val, err := driver.GetValue(name)
		assert.NoError(t, err)
		assert.Equal(t, &tinyconf.Value{Source: "name", Value: "second"}, val)

		val, err = driver.GetValue(port)
		assert.NoError(t, err)
		assert.Equal(t, 80, val.Value)

		_, err = driver.GetValue(skip)
		assert.ErrorIs(t, err, tinyconf.ErrIncorrectTagSettings)
	})

	t.Run("value not found", func(t *testing.T) {
		driver := New("test", testDecode, []Source{DataSource("first", []byte("name=first"))}, false)
		_, err := driver.GetValue(port)
		assert.ErrorIs(t, err, tinyconf.ErrValueNotFound)
	})

	t.Run("incorrect value", func(t *testing.T) {
		driver := New("test", testDecode, []Source{DataSource("first", []byte("port=http"))}, false)
		_, err := driver.GetValue(port)
		assert.Error(t, err)
		assert.False(t, errors.Is(err, tinyconf.ErrValueNotFound))
	})

	t.Run("source is read once per load", func(t *testing.T) {
		source := &countSource{data: "name=first\nport=80"}
		driver := New("test", testDecode, []Source{source.source()}, false)
		// without Load source is read on the first GetValue
		_, err := driver.GetValue(name)
		assert.NoError(t, err)
		_, err = driver.GetValue(port)
		assert.NoError(t, err)
		assert.Equal(t, 1, source.opened)

		source.data = "name=second"
		assert.NoError(t, driver.Load())
		val, err := driver.GetValue(name)
		assert.NoError(t, err)
		assert.Equal(t, "second", val.Value)
		assert.Equal(t, 2, source.opened)
	})

	t.Run("env expansion", func(t *testing.T) {
		t.Setenv("TINYCONF_TREE_NAME", "from env")
		driver := New("test", testDecode, []Source{DataSource("first", []byte("name=${TINYCONF_TREE_NAME}"))}, true)
		val, err := driver.GetValue(name)
		assert.NoError(t, err)
		assert.Equal(t, "from env", val.Value)
	})

	t.Run("sources", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "config")
		assert.NoError(t, os.WriteFile(filePath, []byte("name=file"), 0o600))
		fsys := fstest.MapFS{"config": {Data: []byte("port=8080")}}
		absent := FileSource(filepath.Join(dir, "absent"))

		driver := New("test", testDecode, []Source{FileSource(filePath), FSSource(fsys, "config")}, false)
		assert.NoError(t, driver.Load())
		val, err := driver.GetValue(name)
		assert.NoError(t, err)
		assert.Equal(t, "file", val.Value)
		val, err = driver.GetValue(port)
		assert.NoError(t, err)
		assert.Equal(t, 8080, val.Value)

		driver = New("test", testDecode, []Source{absent}, false)
		assert.ErrorIs(t, driver.Load(), fs.ErrNotExist)
		_, err = driver.GetValue(name)
		assert.ErrorIs(t, err, fs.ErrNotExist)

		absent.Optional = true
		driver = New("test", testDecode, []Source{absent, FSSource(fsys, "absent")}, false)
		assert.ErrorIs(t, driver.Load(), fs.ErrNotExist)

		absentFS := FSSource(fsys, "absent")
		absentFS.Optional = true
		driver = New("test", testDecode, []Source{absent, absentFS}, false)
		assert.NoError(t, driver.Load())
		_, err = driver.GetValue(name)
		assert.ErrorIs(t, err, tinyconf.ErrValueNotFound)

		driver = New("test", testDecode, []Source{DataSource("bad", []byte("incorrect"))}, false)
		assert.Error(t, driver.Load())
	})
}

func TestDriver_Watch(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "config")
	absentPath := filepath.Join(dir, "absent")
	assert.NoError(t, os.WriteFile(filePath, []byte("name=first"), 0o600))

	options := &Options{WatchInterval: 5 * time.Millisecond}
	options.AddFiles(filePath, absentPath, "")
	options.AddData("data", []byte("port=80"))
	options.Optional = true
	driver := options.NewDriver("test", testDecode)

	ctx, cancel := context.WithCancel(context.Background())
	changes, done := make(chan struct{}, 10), make(chan error, 1)
	go func() {
		done <- driver.Watch(ctx, func() { changes <- struct{}{} })
	}()
	waitChange := func() {
		select {
		case <-changes:
		case <-time.After(5 * time.Second):
			t.Fatal("change is not detected")
		}
	}
	// nothing is changed
	select {
	case <-changes:
		t.Fatal("unexpected change")
	case <-time.After(50 * time.Millisecond):
	}

	assert.NoError(t, os.WriteFile(filePath, []byte("name=second value"), 0o600))
	waitChange()
	assert.NoError(t, driver.Load())
	storage, err := fmap.Get[testConfig]()
	assert.NoError(t, err)
	val, err := driver.GetValue(storage.MustFind("Name"))
	assert.NoError(t, err)
	assert.Equal(t, "second value", val.Value)

	// absent file is created
	assert.NoError(t, os.WriteFile(absentPath, []byte("port=8080"), 0o600))
	waitChange()
	// file is removed
	assert.NoError(t, os.Remove(filePath))
	waitChange()

	cancel()
	assert.NoError(t, <-done)

	// changes made between Load and Watch are detected
	assert.NoError(t, driver.Load())
	assert.NoError(t, os.WriteFile(filePath, []byte("name=third"), 0o600))
	ctx, cancel = context.WithCancel(context.Background())
	go func() {
		done <- driver.Watch(ctx, func() { changes <- struct{}{} })
	}()
	waitChange()
	cancel()
	assert.NoError(t, <-done)
	assert.Equal(t, DefaultWatchInterval, New("test", testDecode, nil, false).watchInterval)
}

func TestDriver_WatchSources(t *testing.T) {
	notify, fail := make(chan struct{}), make(chan struct{})
	options := &Options{WatchInterval: time.Hour}
	options.AddSource(Source{
		Name: "notified",
		Open: func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("name=value")), nil
		},
		Watch: func(ctx context.Context, onChange func()) error {
			for {
				select {
				case <-ctx.Done():
					return nil
				case <-notify:
					onChange()
				}
			}
		},
	})
	options.AddSource(Source{
		Name: "failed",
		Open: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("")), nil },
		Watch: func(ctx context.Context, onChange func()) error {
			select {
			case <-ctx.Done():
				return nil
			case <-fail:
				return errors.New("connection lost")
			}
		},
	})
	driver := options.NewDriver("test", testDecode)
	assert.NoError(t, driver.Load())

	changes, done := make(chan struct{}, 10), make(chan error, 1)
	go func() {
		done <- driver.Watch(context.Background(), func() { changes <- struct{}{} })
	}()
	// the source reports the change by itself
	notify <- struct{}{}
	<-changes
	// the error of the source stops watching
	fail <- struct{}{}
	err := <-done
	assert.EqualError(t, err, "failed to watch failed: connection lost")
}

func TestDoc(t *testing.T) {
	type config struct {
		Name   string `test:"name"`
		Secret string `test:"secret" hidden:"true"`
		Skip   *int   `test:"skip"`
		HTTP   struct {
			Port int `test:"port"`
		} `test:"http"`
		NoTag struct {
			Port int `test:"port"`
		}
	}
	conf := &config{Name: "app", Secret: "secret"}
	conf.HTTP.Port = 80
	storage, err := fmap.GetFrom(conf)
	assert.NoError(t, err)
	doc := Doc([]*tinyconf.Registered{{Storage: storage, Config: conf}}, "test", func(v any, _ fmap.Field) any {
		return convert.ToTree(v, "test")
	})
	// hidden values are empty, nil values and fields under the parents without the tag are skipped
	assert.Equal(t, map[string]any{"name": "app", "secret": "", "http": map[string]any{"port": 80}}, doc)
}

func TestDriver_WatchFileEvents(t *testing.T) {
	watch := func(t *testing.T, driver *Driver) <-chan struct{} {
		t.Helper()
		ctx, cancel := context.WithCancel(context.Background())
		changes, done := make(chan struct{}, 10), make(chan error, 1)
		go func() { done <- driver.Watch(ctx, func() { changes <- struct{}{} }) }()
		t.Cleanup(func() {
			cancel()
			assert.NoError(t, <-done)
		})
		// the watcher is started
		time.Sleep(50 * time.Millisecond)
		return changes
	}
	wait := func(t *testing.T, changes <-chan struct{}) {
		t.Helper()
		select {
		case <-changes:
		case <-time.After(5 * time.Second):
			t.Fatal("change is not detected")
		}
	}
	nothing := func(t *testing.T, changes <-chan struct{}) {
		t.Helper()
		select {
		case <-changes:
			t.Fatal("unexpected change")
		case <-time.After(300 * time.Millisecond):
		}
	}

	t.Run("changes are detected by the events without polling", func(t *testing.T) {
		filePath := filepath.Join(t.TempDir(), "config")
		assert.NoError(t, os.WriteFile(filePath, []byte("name=first"), 0o600))
		options := &Options{WatchInterval: time.Hour}
		options.AddFiles(filePath)
		// the delay is long for the slow machines of CI, it is restored after watching is stopped
		original := eventsDelay
		eventsDelay = time.Second
		t.Cleanup(func() { eventsDelay = original })
		changes := watch(t, options.NewDriver("test", testDecode))

		// several writes of the editor are one change
		for _, content := range []string{"name=", "name=sec", "name=second"} {
			assert.NoError(t, os.WriteFile(filePath, []byte(content), 0o600))
		}
		wait(t, changes)
		nothing(t, changes)

		// other files of the directory do not change the state
		assert.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(filePath), "other"), []byte("other"), 0o600))
		nothing(t, changes)
	})

	t.Run("symlinks are replaced like in ConfigMap volumes", func(t *testing.T) {
		dir := t.TempDir()
		write := func(version, content string) {
			assert.NoError(t, os.MkdirAll(filepath.Join(dir, version), 0o700))
			assert.NoError(t, os.WriteFile(filepath.Join(dir, version, "config"), []byte(content), 0o600))
			// ..data is replaced atomically by the rename of the new symlink
			assert.NoError(t, os.Symlink(version, filepath.Join(dir, "..data_tmp")))
			assert.NoError(t, os.Rename(filepath.Join(dir, "..data_tmp"), filepath.Join(dir, "..data")))
		}
		write("v1", "name=first")
		assert.NoError(t, os.Symlink(filepath.Join("..data", "config"), filepath.Join(dir, "config")))
		options := &Options{WatchInterval: time.Hour}
		options.AddFiles(filepath.Join(dir, "config"))
		driver := options.NewDriver("test", testDecode)
		assert.NoError(t, driver.Load())
		changes := watch(t, driver)

		write("v2", "name=second value")
		wait(t, changes)
	})

	t.Run("polling without the events", func(t *testing.T) {
		filePath := filepath.Join(t.TempDir(), "config")
		assert.NoError(t, os.WriteFile(filePath, []byte("name=first"), 0o600))
		for name, setup := range map[string]func(t *testing.T) []string{
			"watcher is not created": func(t *testing.T) []string {
				original := newFileWatcher
				newFileWatcher = func() (*fsnotify.Watcher, error) { return nil, errors.New("too many open files") }
				t.Cleanup(func() { newFileWatcher = original })
				return []string{filePath}
			},
			"directory is absent": func(t *testing.T) []string {
				return []string{filepath.Join(t.TempDir(), "absent", "config")}
			},
			"watcher is closed": func(t *testing.T) []string {
				original := newFileWatcher
				newFileWatcher = func() (*fsnotify.Watcher, error) {
					watcher, err := original()
					go func() {
						time.Sleep(10 * time.Millisecond)
						_ = watcher.Close()
					}()
					return watcher, err
				}
				t.Cleanup(func() { newFileWatcher = original })
				return []string{filePath}
			},
		} {
			t.Run(name, func(t *testing.T) {
				files := setup(t)
				options := &Options{WatchInterval: 5 * time.Millisecond, Optional: true}
				options.AddFiles(files...)
				changes := watch(t, options.NewDriver("test", testDecode))
				assert.NoError(t, os.MkdirAll(filepath.Dir(files[0]), 0o700))
				assert.NoError(t, os.WriteFile(files[0], []byte("name="+name), 0o600))
				wait(t, changes)
			})
		}
	})
}

func TestSetPath(t *testing.T) {
	doc := map[string]any{}
	assert.NoError(t, SetPath(doc, []string{"http", "port"}, "80"))
	assert.NoError(t, SetPath(doc, []string{"http", "host"}, "localhost"))
	// the last value wins, nil value creates the object only
	assert.NoError(t, SetPath(doc, []string{"http", "port"}, "8080"))
	assert.NoError(t, SetPath(doc, []string{"empty"}, nil))
	assert.NoError(t, SetPath(doc, []string{"http"}, nil))
	assert.Equal(t, map[string]any{"http": map[string]any{"port": "8080", "host": "localhost"}, "empty": map[string]any{}}, doc)

	assert.EqualError(t, SetPath(doc, []string{"http"}, "value"), "http is an object, it can not have a value")
	assert.EqualError(t, SetPath(doc, []string{"http", "port", "number"}, "80"), "http.port has a value, it can not be an object")
}
