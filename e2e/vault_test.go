package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/moby/moby/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	tcvault "github.com/testcontainers/testcontainers-go/modules/vault"

	"github.com/insei/tinyconf/v2"
	"github.com/insei/tinyconf/v2/drivers/vault"
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
		tcvault.WithInitCommand("secrets enable -version=1 -path=kv1 kv"),
		// Vault calls the fake API of Kubernetes in the test for the Kubernetes auth method
		testcontainers.WithHostConfigModifier(func(hc *container.HostConfig) {
			hc.ExtraHosts = append(hc.ExtraHosts, "host.docker.internal:host-gateway")
		}))
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

// fakeKubernetes is the TokenReview API of Kubernetes for the Kubernetes auth method of Vault:
// the valid tokens are the tokens of the service account default/app.
type fakeKubernetes struct {
	*httptest.Server
	address string

	mu      sync.Mutex
	valid   map[string]bool
	reviews []string
}

func startFakeKubernetes(t *testing.T) *fakeKubernetes {
	t.Helper()
	k := &fakeKubernetes{valid: map[string]bool{}}
	k.Server = httptest.NewUnstartedServer(http.HandlerFunc(k.handle))
	// Vault in the container calls it by the address of the host
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	require.NoError(t, err)
	k.Listener = listener
	k.Start()
	t.Cleanup(k.Close)
	k.address = fmt.Sprintf("http://host.docker.internal:%d", listener.Addr().(*net.TCPAddr).Port)
	return k
}

func (k *fakeKubernetes) handle(w http.ResponseWriter, r *http.Request) {
	var review struct {
		Spec struct {
			Token     string   `json:"token"`
			Audiences []string `json:"audiences"`
		} `json:"spec"`
	}
	_ = json.NewDecoder(r.Body).Decode(&review)
	k.mu.Lock()
	k.reviews = append(k.reviews, review.Spec.Token)
	status := map[string]any{"authenticated": false}
	if k.valid[review.Spec.Token] {
		status = map[string]any{
			"authenticated": true,
			"user": map[string]any{
				"username": "system:serviceaccount:default:app",
				"uid":      "uid-app",
				"groups":   []string{"system:serviceaccounts", "system:serviceaccounts:default"},
			},
			"audiences": review.Spec.Audiences,
		}
	}
	k.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "authentication.k8s.io/v1", "kind": "TokenReview", "status": status})
}

// token issues the new token of the service account default/app, the previous tokens stay valid until revoke.
func (k *fakeKubernetes) token(t *testing.T, id string) string {
	t.Helper()
	encode := func(v any) string {
		data, err := json.Marshal(v)
		require.NoError(t, err)
		return base64.RawURLEncoding.EncodeToString(data)
	}
	token := encode(map[string]any{"alg": "RS256", "typ": "JWT"}) + "." + encode(map[string]any{
		"iss": "kubernetes/serviceaccount",
		"sub": "system:serviceaccount:default:app",
		"jti": id,
		"kubernetes.io/serviceaccount/namespace":            "default",
		"kubernetes.io/serviceaccount/service-account.name": "app",
		"kubernetes.io/serviceaccount/service-account.uid":  "uid-app",
	}) + "." + base64.RawURLEncoding.EncodeToString([]byte("signature"))
	k.mu.Lock()
	defer k.mu.Unlock()
	k.valid[token] = true
	return token
}

func (k *fakeKubernetes) revoke(token string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.valid, token)
}

func (k *fakeKubernetes) reviewed() []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]string{}, k.reviews...)
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
		return conf, manager, manager.Parse(context.Background(), conf)
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

	t.Run("approle", func(t *testing.T) {
		srv.put(t, "approle/config", map[string]any{"name": "approle"})
		srv.request(t, http.MethodPost, "sys/auth/approle", map[string]any{"type": "approle"})
		srv.request(t, http.MethodPut, "sys/policies/acl/read-approle", map[string]any{
			"policy": `path "secret/data/approle/*" { capabilities = ["read"] }`,
		})
		srv.request(t, http.MethodPost, "auth/approle/role/app", map[string]any{"token_policies": []string{"read-approle"}, "token_ttl": "1h"})
		var roleID, secretID struct {
			Data struct {
				RoleID   string `json:"role_id"`
				SecretID string `json:"secret_id"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(srv.request(t, http.MethodGet, "auth/approle/role/app/role-id", nil), &roleID))
		require.NoError(t, json.Unmarshal(srv.request(t, http.MethodPost, "auth/approle/role/app/secret-id", nil), &secretID))

		driver, err := vault.New("approle/config", vault.WithAddress(srv.address), vault.WithAppRole(roleID.Data.RoleID, secretID.Data.SecretID))
		require.NoError(t, err)
		conf, _, err := parse(t, driver)
		require.NoError(t, err)
		assert.Equal(t, "approle", conf.Name)

		driver, err = vault.New("approle/config", vault.WithAddress(srv.address), vault.WithAppRole(roleID.Data.RoleID, "wrong"))
		require.NoError(t, err)
		_, _, err = parse(t, driver)
		assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)
	})

	t.Run("kubernetes", func(t *testing.T) {
		k8s := startFakeKubernetes(t)
		srv.put(t, "kubernetes/config", map[string]any{"name": "kubernetes"})
		srv.request(t, http.MethodPost, "sys/auth/k8s", map[string]any{"type": "kubernetes"})
		// without the CA and the reviewer token Vault reviews the token of the login by itself
		srv.request(t, http.MethodPost, "auth/k8s/config", map[string]any{"kubernetes_host": k8s.address, "disable_local_ca_jwt": true})
		srv.request(t, http.MethodPut, "sys/policies/acl/read-kubernetes", map[string]any{
			"policy": `path "secret/data/kubernetes/*" { capabilities = ["read"] }`,
		})
		// the short lease makes the driver login again in the test
		srv.request(t, http.MethodPost, "auth/k8s/role/app", map[string]any{
			"bound_service_account_names":      []string{"app"},
			"bound_service_account_namespaces": []string{"default"},
			"token_policies":                   []string{"read-kubernetes"},
			"token_ttl":                        "3s",
		})
		first := k8s.token(t, "first")
		tokenFile := filepath.Join(t.TempDir(), "token")
		require.NoError(t, os.WriteFile(tokenFile, []byte(first), 0o600))

		driver, err := vault.New("kubernetes/config", vault.WithAddress(srv.address), vault.WithKubernetes("app", tokenFile), vault.WithAuthMount("k8s"))
		require.NoError(t, err)
		conf, manager, err := parse(t, driver)
		require.NoError(t, err)
		assert.Equal(t, "kubernetes", conf.Name)
		assert.Equal(t, []string{first}, k8s.reviewed())

		// Kubernetes rotates the token of the service account, the old one is not valid anymore
		second := k8s.token(t, "second")
		k8s.revoke(first)
		require.NoError(t, os.WriteFile(tokenFile, []byte(second), 0o600))
		// the token of Vault is requested again after 2/3 of its lease
		time.Sleep(2500 * time.Millisecond)
		require.NoError(t, manager.Parse(context.Background(), &vaultConfig{}))
		assert.Equal(t, []string{first, second}, k8s.reviewed())

		driver, err = vault.New("kubernetes/config", vault.WithAddress(srv.address), vault.WithKubernetes("other", tokenFile), vault.WithAuthMount("k8s"))
		require.NoError(t, err)
		_, _, err = parse(t, driver)
		assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)
	})
}
