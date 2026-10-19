package vault

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/insei/tinyconf/v2"
)

func TestDriver_AppRole(t *testing.T) {
	vault := newFakeVault(map[string]map[string]any{"app": {"name": "app"}})
	defer vault.Close()
	vault.login = "approle"
	vault.loginBody = map[string]any{"role_id": "role", "secret_id": "secret"}
	vault.lease = 3600

	driver, err := New("app", WithAddress(vault.URL), WithToken("ignored"), WithAppRole("role", "secret"), WithNamespace("team"))
	assert.NoError(t, err)
	conf, manager, err := parse(t, driver)
	assert.NoError(t, err)
	assert.Equal(t, "app", conf.Name)
	assert.Equal(t, "login-1", vault.lastRequest().Header.Get("X-Vault-Token"))
	assert.Equal(t, "team", vault.requests[0].Header.Get("X-Vault-Namespace"))

	t.Run("token is used until 2/3 of the lease", func(t *testing.T) {
		assert.NoError(t, manager.Parse(context.Background(), &testConfig{}))
		assert.Equal(t, 1, vault.loginCount())
		c := driver.(*vaultDriver).client
		assert.WithinDuration(t, time.Now().Add(40*time.Minute), c.renewAt, time.Minute)

		c.renewAt = time.Now().Add(-time.Second)
		assert.NoError(t, manager.Parse(context.Background(), &testConfig{}))
		assert.Equal(t, 2, vault.loginCount())
		assert.Equal(t, "login-2", vault.lastRequest().Header.Get("X-Vault-Token"))
	})

	t.Run("revoked token is requested again", func(t *testing.T) {
		vault.mu.Lock()
		vault.token = "other"
		vault.mu.Unlock()
		assert.NoError(t, manager.Parse(context.Background(), &testConfig{}))
		assert.Equal(t, 3, vault.loginCount())
	})

	t.Run("denied secret is read once more with the new token", func(t *testing.T) {
		vault.mu.Lock()
		vault.denied["app"] = true
		vault.mu.Unlock()
		err := manager.Parse(context.Background(), &testConfig{})
		assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)
		// Parse reads the version of the secret and the secret, every read logins once more
		assert.Equal(t, 5, vault.loginCount())
	})
}

func TestDriver_AppRoleWithoutLease(t *testing.T) {
	vault := newFakeVault(map[string]map[string]any{"app": {"name": "app"}})
	defer vault.Close()
	vault.login = "auth/approle-team"
	vault.loginBody = map[string]any{"role_id": "role", "secret_id": "secret"}

	driver, err := New("app", WithAddress(vault.URL), WithAuthMount("/auth/approle-team/"), WithAppRole("role", "secret"))
	assert.NoError(t, err)
	_, manager, err := parse(t, driver)
	assert.NoError(t, err)
	assert.True(t, driver.(*vaultDriver).client.renewAt.IsZero())
	assert.NoError(t, manager.Parse(context.Background(), &testConfig{}))
	assert.Equal(t, 1, vault.loginCount())
}

func TestDriver_Kubernetes(t *testing.T) {
	vault := newFakeVault(map[string]map[string]any{"app": {"name": "app"}})
	defer vault.Close()
	vault.login = "k8s"
	vault.loginBody = map[string]any{"role": "app", "jwt": "first"}
	tokenFile := filepath.Join(t.TempDir(), "token")
	assert.NoError(t, os.WriteFile(tokenFile, []byte("first\n"), 0o600))

	driver, err := New("app", WithAddress(vault.URL), WithKubernetes("app", tokenFile), WithAuthMount("k8s"))
	assert.NoError(t, err)
	conf, manager, err := parse(t, driver)
	assert.NoError(t, err)
	assert.Equal(t, "app", conf.Name)

	// the rotated token of the service account is read before the next login
	vault.mu.Lock()
	vault.loginBody = map[string]any{"role": "app", "jwt": "second"}
	vault.mu.Unlock()
	assert.NoError(t, os.WriteFile(tokenFile, []byte("second"), 0o600))
	driver.(*vaultDriver).client.renewAt = time.Now().Add(-time.Second)
	assert.NoError(t, manager.Parse(context.Background(), &testConfig{}))
	assert.Equal(t, 2, vault.loginCount())

	assert.NoError(t, os.Remove(tokenFile))
	driver.(*vaultDriver).client.renewAt = time.Now().Add(-time.Second)
	assert.ErrorContains(t, manager.Parse(context.Background(), &testConfig{}), "failed to read the token of the service account")

	o := options{}
	WithKubernetes("app", "")(&o)
	c := newClient(o)
	assert.Equal(t, "kubernetes", c.login.mount)
	_, err = c.login.data()
	assert.ErrorContains(t, err, KubernetesTokenFile)
}

func TestDriver_LoginErrors(t *testing.T) {
	vault := newFakeVault(map[string]map[string]any{"app": {"name": "app"}})
	defer vault.Close()
	vault.login = "approle"
	vault.loginBody = map[string]any{"role_id": "role", "secret_id": "secret"}
	parseWith := func(t *testing.T, opts ...Option) error {
		driver, err := New("app", opts...)
		assert.NoError(t, err)
		_, _, err = parse(t, driver)
		return err
	}

	t.Run("wrong credentials", func(t *testing.T) {
		err := parseWith(t, WithAddress(vault.URL), WithAppRole("role", "wrong"))
		assert.ErrorIs(t, err, tinyconf.ErrAccessDenied)
		assert.ErrorContains(t, err, "failed to login by auth/approle: invalid role or secret ID")
		assert.Equal(t, 0, vault.loginCount())
	})

	t.Run("server error", func(t *testing.T) {
		vault.loginResponse = func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"errors":["Vault is sealed"]}`))
		}
		defer func() { vault.loginResponse = nil }()
		err := parseWith(t, WithAddress(vault.URL), WithAppRole("role", "secret"))
		assert.ErrorContains(t, err, "failed to login by auth/approle: 503 Service Unavailable: Vault is sealed")
		assert.NotErrorIs(t, err, tinyconf.ErrAccessDenied)
	})

	t.Run("no token in the response", func(t *testing.T) {
		vault.loginResponse = func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"auth":null}`)) }
		defer func() { vault.loginResponse = nil }()
		assert.ErrorContains(t, parseWith(t, WithAddress(vault.URL), WithAppRole("role", "secret")), "no token in the response")
	})

	t.Run("unavailable", func(t *testing.T) {
		closed := newFakeVault(nil)
		closed.Close()
		assert.ErrorContains(t, parseWith(t, WithAddress(closed.URL), WithAppRole("role", "secret")), "failed to login by auth/approle")
	})

	t.Run("wrong address", func(t *testing.T) {
		assert.Error(t, parseWith(t, WithAddress("http://[::1"), WithAppRole("role", "secret")))
	})
}
