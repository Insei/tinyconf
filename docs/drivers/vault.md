# vault

The vault driver reads values from the [KV secrets engine](https://developer.hashicorp.com/vault/docs/secrets/kv) of HashiCorp Vault.

```go
import "github.com/insei/tinyconf/drivers/vault"

vaultDriver, err := vault.New("app/config")
```

The driver talks to the HTTP API of Vault directly and adds no dependencies.

## Keys

A secret in Vault is a JSON object. The `vault` tag is a key in it, tags of [nested structs](../features/structs.md) are keys of nested objects:

```go
type Config struct {
    Database struct {
        Host     string `vault:"host"`
        Password string `vault:"password" hidden:"true"`
    } `vault:"database"`
    Hosts []string `vault:"hosts"`
}
```

```bash
vault kv put -mount=secret app/config - <<'EOF'
{"database": {"host": "db.internal", "password": "secret"}, "hosts": ["a", "b"]}
EOF
```

Values of the secret can be JSON values or strings: `"port": 5432` and `"port": "5432"` are the same for an `int` field, `"timeout": "1m30s"` is a `time.Duration`, see [Supported types](../features/types.md).

!!! warning "Set the tags of the parents"
    The path of the nested field is built from the `vault` tags of its parents. If the `Database` field has no `vault` tag, the `Password` field is read from the `password` key in the root of the secret, and the manager [logs a warning](../features/logging.md). [Embedded structs](../features/structs.md#embedded-structs) without the tag are the exception.

## Several secrets

```go
vaultDriver, err := vault.New("app/config", vault.WithPaths("app/database", "shared/certificates"))
```

Secrets are merged in the order of addition, like [several files](yaml.md#several-files): values from the next secret override values from the previous one, nested objects are merged by keys. It is a common layout: the settings and the credentials in different secrets with different access policies.

## Options

```go
vaultDriver, err := vault.New("app/config",
    vault.WithAddress("https://vault.internal:8200"),
    vault.WithTokenFile("/vault/secrets/token"),
    vault.WithMount("kv"),
)
```

| Option | Default | Description |
|---|---|---|
| `WithAddress(address)` | `VAULT_ADDR` or `https://127.0.0.1:8200` | URL of Vault |
| `WithToken(token)` | `VAULT_TOKEN` | the token |
| `WithTokenFile(path)` | | the file with the token, it is read before every request |
| `WithNamespace(namespace)` | `VAULT_NAMESPACE` | the namespace of Vault Enterprise |
| `WithMount(mount)` | `secret` | the mount of the KV secrets engine |
| `WithKVv1()` | KV version 2 | read from the KV secrets engine version 1 |
| `WithHTTPClient(client)` | `http.DefaultClient` | the client with TLS settings, for example |
| `WithPaths(paths...)` | | additional secrets |
| `WithOptional()` | | secrets can be absent, by default an absent secret is an error |
| `WithWatchInterval(interval)` | 30 seconds | the interval of checking the secrets for changes |

The first argument of `New` can be an empty string, if the secrets are set by `WithPaths`.

## Authentication

The driver uses a token. Other methods — Kubernetes, AppRole, cloud IAM — are handled by [Vault Agent](https://developer.hashicorp.com/vault/docs/agent-and-proxy/agent): it logs in, renews the token and writes it to the file. Point the driver to this file:

```go
vaultDriver, err := vault.New("app/config", vault.WithTokenFile("/vault/secrets/token"))
```

The file is read before every request, so the renewed token is used without restart of the application.

## Errors

| Case | Result |
|---|---|
| the secret is absent | error of `Parse`, unless `WithOptional()` |
| the token can not read the secret | error of `Parse` that wraps `tinyconf.ErrAccessDenied`, logged as `access denied` |
| Vault is not available or sealed | error of `Parse` |

The driver is skipped in the `Parse` call with the error, see [Missing files and values](yaml.md#missing-files-and-values).

## Reloading

Vault does not notify about changes, so the driver checks the versions of the secrets every 30 seconds while there are [subscriptions](../features/reloading.md). The interval is set with `WithWatchInterval`. A secret that appears, disappears or becomes available again after an error is a change too.

For KV version 2 the version of the secret is checked, for KV version 1 — the hash of its data.

## Documentation

`GenDoc("vault")` generates the command of the Vault CLI that puts the current values to the first secret, values of `hidden:"true"` fields are empty:

```bash
vault kv put -mount=secret app/config - <<'EOF'
{
  "database": {
    "host": "db.internal",
    "password": ""
  },
  "hosts": [
    "a",
    "b"
  ]
}
EOF
```
