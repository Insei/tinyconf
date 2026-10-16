package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcetcd "github.com/testcontainers/testcontainers-go/modules/etcd"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/drivers/etcd"
	"github.com/insei/tinyconf/drivers/yaml"
)

const etcdImage = "quay.io/coreos/etcd:v3.5.17"

// etcdServer is an etcd node or cluster in Docker.
type etcdServer struct {
	container *tcetcd.EtcdContainer
	endpoints []string
}

func startEtcd(t *testing.T, opts ...testcontainers.ContainerCustomizer) *etcdServer {
	t.Helper()
	ctx := context.Background()
	c, err := tcetcd.Run(ctx, etcdImage, opts...)
	testcontainers.CleanupContainer(t, c)
	require.NoError(t, err)
	endpoints, err := c.ClientEndpoints(ctx)
	require.NoError(t, err)
	return &etcdServer{container: c, endpoints: endpoints}
}

// etcdctl runs etcdctl in the container of the first node, the image has no shell.
func (s *etcdServer) etcdctl(t *testing.T, args ...string) string {
	t.Helper()
	code, output, err := s.container.Exec(context.Background(), append([]string{"etcdctl"}, args...))
	require.NoError(t, err)
	out, _ := io.ReadAll(output)
	require.Equal(t, 0, code, "etcdctl %s: %s", strings.Join(args, " "), out)
	return string(out)
}

func (s *etcdServer) put(t *testing.T, key, value string) {
	s.etcdctl(t, "put", "--", key, value)
}

// script runs the shell script on the host, etcdctl of the script runs in the container of the first node.
func (s *etcdServer) script(t *testing.T, script string) {
	t.Helper()
	script = "etcdctl() { docker exec " + s.container.GetContainerID() + " etcdctl \"$@\"; }\n" + script
	cmd := exec.Command("sh", "-c", script)
	// the docker CLI uses the same docker as testcontainers
	cmd.Env = append(os.Environ(), "DOCKER_HOST=unix://"+testcontainers.MustExtractDockerSocket(context.Background()))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

type etcdDatabase struct {
	Host     string `etcd:"host"`
	Port     int    `etcd:"port"`
	Password string `etcd:"password" hidden:"true"`
}

type etcdConfig struct {
	Name     string            `etcd:"name"`
	Database etcdDatabase      `etcd:"database"`
	Hosts    []string          `etcd:"hosts"`
	Labels   map[string]string `etcd:"labels"`
	Timeout  time.Duration     `etcd:"timeout"`
}

func parseEtcd(t *testing.T, driver tinyconf.Driver) (*etcdConfig, *tinyconf.Manager, error) {
	t.Helper()
	manager := newManager(t, driver)
	conf := &etcdConfig{}
	require.NoError(t, manager.Register(conf))
	return conf, manager, manager.Parse(conf)
}

func TestEtcd(t *testing.T) {
	srv := startEtcd(t)

	t.Run("key per field", func(t *testing.T) {
		srv.put(t, "keys/name", "app")
		srv.put(t, "keys/database/host", "db")
		srv.put(t, "keys/database/port", "5432")
		srv.put(t, "keys/hosts", `a,"b,c"`)
		srv.put(t, "keys/labels", "env:prod")
		srv.put(t, "keys/timeout", "1m30s")
		srv.put(t, "keys-other/name", "other")

		driver, err := etcd.New("keys", etcd.WithEndpoints(srv.endpoints...))
		require.NoError(t, err)
		conf, manager, err := parseEtcd(t, driver)
		require.NoError(t, err)
		assert.Equal(t, etcdConfig{
			Name:     "app",
			Database: etcdDatabase{Host: "db", Port: 5432},
			Hosts:    []string{"a", "b,c"},
			Labels:   map[string]string{"env": "prod"},
			Timeout:  90 * time.Second,
		}, *conf)
		assert.Contains(t, manager.Sources(conf), tinyconf.FieldSource{Field: "Database.Port", Driver: "etcd", Source: "keys/database/port"})
	})

	t.Run("gendoc commands put the values to etcd", func(t *testing.T) {
		driver, err := etcd.New("gendoc", etcd.WithEndpoints(srv.endpoints...))
		require.NoError(t, err)
		manager := newManager(t, driver)
		conf := &etcdConfig{
			Name:     "it's my app",
			Database: etcdDatabase{Host: "db", Port: -1},
			Hosts:    []string{"a b", "c"},
			Labels:   map[string]string{"env": "prod"},
			Timeout:  time.Minute,
		}
		require.NoError(t, manager.Register(conf))

		// the commands are parsed by the shell and run by etcdctl in the container
		srv.script(t, manager.GenDoc("etcd"))

		parsed, _, err := parseEtcd(t, driver)
		require.NoError(t, err)
		assert.Equal(t, *conf, *parsed)
	})

	t.Run("document merged with file", func(t *testing.T) {
		type config struct {
			Name     string `yaml:"name"`
			Database struct {
				Host string `yaml:"host"`
				Port int    `yaml:"port"`
			} `yaml:"database"`
		}
		srv.put(t, "document/config", "name: etcd\ndatabase:\n  port: 6432\n")
		file := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(file, []byte("name: file\ndatabase:\n  host: db\n  port: 5432\n"), 0o600))

		driver, err := yaml.New(file, yaml.WithSource(etcd.Document("document/config", etcd.WithEndpoints(srv.endpoints...))))
		require.NoError(t, err)
		manager := newManager(t, driver)
		conf := &config{}
		require.NoError(t, manager.Register(conf))
		require.NoError(t, manager.Parse(conf))
		assert.Equal(t, "etcd", conf.Name)
		assert.Equal(t, "db", conf.Database.Host)
		assert.Equal(t, 6432, conf.Database.Port)
	})

	t.Run("reload", func(t *testing.T) {
		srv.put(t, "reload/name", "first")
		srv.put(t, "reload/database/port", "5432")
		driver, err := etcd.New("reload", etcd.WithEndpoints(srv.endpoints...))
		require.NoError(t, err)
		_, manager, err := parseEtcd(t, driver)
		require.NoError(t, err)
		configs := subscribe[etcdConfig](t, manager)
		databases := subscribe[etcdDatabase](t, manager)

		srv.put(t, "reload/name", "second")
		assert.Equal(t, "second", next(t, configs).Name)
		noReload(t, databases)

		// the subscribers of the whole config get the changes of its parts too
		srv.put(t, "reload/database/port", "6432")
		assert.Equal(t, 6432, next(t, databases).Port)
		assert.Equal(t, 6432, next(t, configs).Database.Port)

		// keys out of the prefix do not cause reloads, the deleted key gets the initial value
		srv.put(t, "reload-other/name", "other")
		srv.etcdctl(t, "del", "reload/database/port")
		assert.Equal(t, 0, next(t, databases).Port)
		assert.Equal(t, "second", next(t, configs).Name)
		noReload(t, configs)
	})

	t.Run("authentication and permissions", func(t *testing.T) {
		srv.put(t, "auth/name", "secured")
		srv.put(t, "auth-secret/name", "secret")
		srv.etcdctl(t, "user", "add", "root:root-password")
		srv.etcdctl(t, "user", "grant-role", "root", "root")
		srv.etcdctl(t, "user", "add", "reader:reader-password")
		srv.etcdctl(t, "role", "add", "reader")
		srv.etcdctl(t, "role", "grant-permission", "--prefix=true", "reader", "read", "auth/")
		srv.etcdctl(t, "user", "grant-role", "reader", "reader")
		srv.etcdctl(t, "auth", "enable")
		t.Cleanup(func() { srv.etcdctl(t, "--user=root:root-password", "auth", "disable") })

		parse := func(prefix string, opts ...etcd.Option) (*etcdConfig, error) {
			driver, err := etcd.New(prefix, append([]etcd.Option{etcd.WithEndpoints(srv.endpoints...)}, opts...)...)
			require.NoError(t, err)
			conf, _, err := parseEtcd(t, driver)
			return conf, err
		}

		conf, err := parse("auth", etcd.WithAuth("reader", "reader-password"))
		require.NoError(t, err)
		assert.Equal(t, "secured", conf.Name)

		for name, opts := range map[string][]etcd.Option{
			"without user":       nil,
			"wrong password":     {etcd.WithAuth("reader", "wrong")},
			"without permission": {etcd.WithAuth("reader", "reader-password")},
		} {
			prefix := "auth"
			if name == "without permission" {
				prefix = "auth-secret"
			}
			_, err = parse(prefix, opts...)
			assert.ErrorIs(t, err, tinyconf.ErrAccessDenied, name)
		}
	})
}

func TestEtcd_Cluster(t *testing.T) {
	srv := startEtcd(t, tcetcd.WithNodes("etcd-1", "etcd-2", "etcd-3"))
	require.Len(t, srv.endpoints, 3)
	srv.put(t, "cluster/name", "first")

	driver, err := etcd.New("cluster", etcd.WithEndpoints(srv.endpoints...))
	require.NoError(t, err)
	conf, manager, err := parseEtcd(t, driver)
	require.NoError(t, err)
	assert.Equal(t, "first", conf.Name)
	configs := subscribe[etcdConfig](t, manager)

	// the node that has answered is stopped, the cluster keeps the quorum
	timeout := 10 * time.Second
	require.NoError(t, srv.container.Stop(context.Background(), &timeout))

	// the change is made by another node, the driver continues watching with the next endpoint
	putByGateway(t, srv.endpoints[1], "cluster/name", "second")
	for {
		if c := next(t, configs); c.Name == "second" {
			break
		}
	}
	// the driver reads from the next endpoint
	require.NoError(t, manager.Parse(conf))
	assert.Equal(t, "second", conf.Name)
}

// putByGateway puts the value by the gRPC gateway of the etcd node.
func putByGateway(t *testing.T, endpoint, key, value string) {
	t.Helper()
	body, err := json.Marshal(map[string][]byte{"key": []byte(key), "value": []byte(value)})
	require.NoError(t, err)
	resp, err := http.Post(endpoint+"/v3/kv/put", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
