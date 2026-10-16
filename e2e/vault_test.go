package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcvault "github.com/testcontainers/testcontainers-go/modules/vault"

	"github.com/insei/tinyconf"
	"github.com/insei/tinyconf/drivers/vault"
)

const (
	vaultImage = "hashicorp/vault:1.18"
	vaultRoot  = "root"
)

// vaultServer is a Vault dev server in Docker.
type vaultServer struct {
	container *tcvault.VaultContainer
	address   string
}

func startVault(t *testing.T) *vaultServer {
	t.Helper()
	ctx := context.Background()
	c, err := tcvault.Run(ctx, vaultImage, tcvault.WithToken(vaultRoot),
		// KV version 1 for the tests of the old secrets engine, the dev server has KV version 2 at secret/
		tcvault.WithInitCommand("secrets enable -version=1 -path=kv1 kv"))
	testcontainers.CleanupContainer(t, c)
	require.NoError(t, err)
	address, err := c.HttpHostAddress(ctx)
	require.NoError(t, err)
	return &vaultServer{container: c, address: address}
}

// request calls the HTTP API of Vault with the root token and returns the body of the response.
func (s *vaultServer) request(t *testing.T, method, path string, body any) []byte {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		reader = strings.NewReader(string(data))
	}
	req, err := http.NewRequest(method, s.address+"/v1/"+path, reader)
	require.NoError(t, err)
	req.Header.Set("X-Vault-Token", vaultRoot)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Less(t, resp.StatusCode, 300, "%s %s: %s", method, path, data)
	return data
}

// put writes the secret to the KV secrets engine version 2 at secret/.
func (s *vaultServer) put(t *testing.T, path string, data map[string]any) {
	s.request(t, http.MethodPost, "secret/data/"+path, map[string]any{"data": data})
}

// createToken creates the token that can read the secret.
func (s *vaultServer) createToken(t *testing.T, path string) string {
	t.Helper()
	name := "read-" + strings.ReplaceAll(path, "/", "-")
	s.request(t, http.MethodPut, "sys/policies/acl/"+name, map[string]any{
		"policy": fmt.Sprintf(`path "secret/data/%s" { capabilities = ["read"] }`, path),
	})
	var resp struct {
		Auth struct {
			ClientToken string `json:"client_token"`
		} `json:"auth"`
	}
	require.NoError(t, json.Unmarshal(s.request(t, http.MethodPost, "auth/token/create", map[string]any{"policies": []string{name}}), &resp))
	return resp.Auth.ClientToken
}

type vaultDatabase struct {
	Host     string `vault:"host"`
	Port     int    `vault:"port"`
	Password string `vault:"password" hidden:"true"`
}

type vaultConfig struct {
	Name     string            `vault:"name"`
	Database vaultDatabase     `vault:"database"`
	Hosts    []string          `vault:"hosts"`
	Labels   map[string]string `vault:"labels"`
	Timeout  time.Duration     `vault:"timeout"`
}

func TestVault(t *testing.T) {
	srv := startVault(t)
	options := func(opts ...vault.Option) []vault.Option {
		return append([]vault.Option{vault.WithAddress(srv.address), vault.WithToken(vaultRoot)}, opts...)
	}
	parse := func(t *testing.T, driver tinyconf.Driver) (*vaultConfig, *tinyconf.Manager, error) {
		manager := newManager(t, driver)
		conf := &vaultConfig{}
		require.NoError(t, manager.Register(conf))
		return conf, manager, manager.Parse(conf)
	}

	t.Run("secrets merged in the order of the paths", func(t *testing.T) {
		srv.put(t, "merge/config", map[string]any{
			"name":     "app",
			"database": map[string]any{"host": "db", "port": 5432},
			"hosts":    []string{"a", "b"},
			"labels":   map[string]string{"env": "prod"},
			"timeout":  "1m30s",
		})
		srv.put(t, "merge/secrets", map[string]any{"database": map[string]any{"password": "secret"}})

		driver, err := vault.New("merge/config", options(vault.WithPaths("merge/secrets"))...)
		require.NoError(t, err)
		conf, manager, err := parse(t, driver)
		require.NoError(t, err)
		assert.Equal(t, vaultConfig{
			Name:     "app",
			Database: vaultDatabase{Host: "db", Port: 5432, Password: "secret"},
			Hosts:    []string{"a", "b"},
			Labels:   map[string]string{"env": "prod"},
			Timeout:  90 * time.Second,
		}, *conf)
		assert.Contains(t, manager.Sources(conf), tinyconf.FieldSource{Field: "Database.Password", Driver: "vault", Source: "database.password"})
	})

	t.Run("kv version 1", func(t *testing.T) {
		srv.request(t, http.MethodPost, "kv1/app", map[string]any{"name": "old engine"})
		driver, err := vault.New("app", options(vault.WithMount("kv1"), vault.WithKVv1())...)
		require.NoError(t, err)
		conf, _, err := parse(t, driver)
		require.NoError(t, err)
		assert.Equal(t, "old engine", conf.Name)
	})

	t.Run("gendoc command puts the values to vault", func(t *testing.T) {
		driver, err := vault.New("gendoc/config", options()...)
		require.NoError(t, err)
		manager := newManager(t, driver)
		conf := &vaultConfig{
			Name:     "it's my app",
			Database: vaultDatabase{Host: "db", Port: 5432},
			Hosts:    []string{"a b", "c"},
			Labels:   map[string]string{"env": "prod"},
			Timeout:  time.Minute,
		}
		require.NoError(t, manager.Register(conf))
		script := "export VAULT_ADDR=http://127.0.0.1:8200 VAULT_TOKEN=" + vaultRoot + "\n" + manager.GenDoc("vault")

		// the command is run by the Vault CLI in the container
		code, output, err := srv.container.Exec(context.Background(), []string{"sh", "-c", script})
		require.NoError(t, err)
		out, _ := io.ReadAll(output)
		require.Equal(t, 0, code, string(out))

		parsed, _, err := parse(t, driver)
		require.NoError(t, err)
		assert.Equal(t, *conf, *parsed)
	})

	t.Run("permissions", func(t *testing.T) {
		srv.put(t, "acl/allowed", map[string]any{"name": "allowed"})
		srv.put(t, "acl/denied", map[string]any{"name": "denied"})
		token := srv.createToken(t, "acl/allowed")

		driver, err := vault.New("acl/allowed", vault.WithAddress(srv.address), vault.WithToken(token))
		require.NoError(t, err)
		conf, _, err := parse(t, driver)
		require.NoError(t, err)
		assert.Equal(t, "allowed", conf.Name)

		driver, err = vault.New("acl/denied", vault.WithAddress(srv.address), vault.WithToken(token))
		require.NoError(t, err)
		_, _, err = parse(t, driver)
		assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)
		assert.ErrorContains(t, err, "permission denied")
	})

	t.Run("absent secret", func(t *testing.T) {
		driver, err := vault.New("absent", options()...)
		require.NoError(t, err)
		_, _, err = parse(t, driver)
		assert.ErrorContains(t, err, "secret absent is not found")

		driver, err = vault.New("absent", options(vault.WithOptional())...)
		require.NoError(t, err)
		_, _, err = parse(t, driver)
		assert.NoError(t, err)
	})

	t.Run("reload", func(t *testing.T) {
		srv.put(t, "reload/config", map[string]any{"name": "first", "database": map[string]any{"port": 5432}})
		driver, err := vault.New("reload/config", options(vault.WithWatchInterval(100*time.Millisecond))...)
		require.NoError(t, err)
		_, manager, err := parse(t, driver)
		require.NoError(t, err)
		configs := subscribe[vaultConfig](t, manager)
		databases := subscribe[vaultDatabase](t, manager)

		srv.put(t, "reload/config", map[string]any{"name": "second", "database": map[string]any{"port": 5432}})
		assert.Equal(t, "second", next(t, configs).Name)
		noReload(t, databases)

		srv.put(t, "reload/config", map[string]any{"name": "second", "database": map[string]any{"port": 6432}})
		assert.Equal(t, 6432, next(t, databases).Port)
	})
}
