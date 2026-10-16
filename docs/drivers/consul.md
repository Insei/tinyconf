# consul

The consul package reads the config from [Consul KV](https://developer.hashicorp.com/consul/docs/dynamic-app-config/kv) in two ways:

| Way | Where the config is | Tags |
|---|---|---|
| [key per field](#key-per-field) | every value in its own key: `app/http/port` | `consul` |
| [document](#document) | YAML, JSON or TOML document in one key: `app/config` | of the format: `yaml`, `json`, `toml` |

Both ways report changes for [reloading](../features/reloading.md) as soon as they happen: the driver uses [blocking queries](https://developer.hashicorp.com/consul/api-docs/features/blocking) of Consul, not polling. The package talks to the HTTP API of Consul directly and adds no dependencies.

## Key per field

```go
import "github.com/insei/tinyconf/drivers/consul"

consulDriver, err := consul.New("app")
```

The key of the field is the prefix and the path by the `consul` tags, dots are replaced with slashes:

```go
type Config struct {
    HTTP struct {
        Host string `consul:"host"`
        Port int    `consul:"port"`
    } `consul:"http"`
    Hosts []string `consul:"hosts"`
}
```

```bash
consul kv put app/http/host example.com
consul kv put app/http/port 9090
consul kv put app/hosts a.example.com,b.example.com
```

Values are parsed by the rules of [string values](../features/types.md), like environment variables: slices are comma separated, structs in slices are JSON.

All keys with the prefix are read by one request once per `Parse` call. A key that is absent is not an error, the field keeps its value from the previous driver.

!!! warning "Set the tags of the parents"
    The key of the nested field is built from the `consul` tags of its parents. If the `HTTP` field has no `consul` tag, the `Host` field is read from `app/host`, not from `app/http/host`. When a value is found this way, the manager [logs a warning](../features/logging.md). [Embedded structs](../features/structs.md#embedded-structs) without the tag are the exception, their fields are on the level of the embedding struct.

## Document

`consul.Document` is a source of the config content for the [yaml](yaml.md), [json](json.md) and [toml](toml.md) drivers. The values are read by the tags of the format driver and merged with the files of the driver, as one more file:

```go
yamlDriver, err := yaml.New("config.yaml",
    yaml.WithSource(consul.Document("app/config")),
)
```

```bash
consul kv put app/config @config.yaml
```

Here `config.yaml` has the defaults, and the document from Consul overrides them. An absent key is an error of `Parse`, unless the format driver has `WithOptional()`.

## Connection

```go
consulDriver, err := consul.New("app",
    consul.WithAddress("https://consul.service:8501"),
    consul.WithToken(token),
    consul.WithDatacenter("dc2"),
    consul.WithHTTPClient(&http.Client{Transport: tlsTransport}),
)
document := consul.Document("app/config", consul.WithAddress("consul.service:8500"))
```

| Option | Default | Description |
|---|---|---|
| `WithAddress(address)` | `CONSUL_HTTP_ADDR` or `127.0.0.1:8500` | `host:port` or URL with the scheme; without the scheme `https` is used if `CONSUL_HTTP_SSL=true` |
| `WithToken(token)` | `CONSUL_HTTP_TOKEN` | ACL token |
| `WithDatacenter(dc)` | the datacenter of the agent | datacenter of the keys |
| `WithHTTPClient(client)` | `http.DefaultClient` | client with TLS settings, for example; its timeout should be greater than the wait time |
| `WithWaitTime(duration)` | 5 minutes | maximum time of one blocking query, Consul limits it to 10 minutes |

The options are the same for `New` and `Document`.

## Permissions

With ACL Consul hides the keys that the token can not read, and answers as if they are absent. The driver does not let it make the config silently incomplete:

| Case | Result |
|---|---|
| the token can not read the prefix | error of `Parse`: `403 Forbidden` |
| the token can read only some keys with the prefix | error of `Parse`: some keys are hidden by ACL |
| the token can not read the key of the document | error of `Parse`: `403 Forbidden` |
| access is denied while watching | the driver stops watching, the error is logged: retries do not fix the permissions |

All these errors wrap `tinyconf.ErrAccessDenied`: the manager [logs](../features/logging.md) them as `access denied`, and they can be checked with `errors.Is`. The token needs `read` on the prefix: `key_prefix "app/" { policy = "read" }`.

## Watching

The manager receives the changes of the keys with the prefix, or of the key of the document, right after they are written to Consul. Keys outside of the prefix do not cause reloads.

If Consul is not available, the driver keeps retrying the requests, with the interval that grows from 1 to 30 seconds, so a restart of Consul does not stop watching. After the failure the config is always reloaded: the changes made while Consul was not available are picked up, even if Consul has lost its data and started its indexes from scratch. The same is true for the start of the application: if Consul is not available at the first `Parse`, the values are loaded by the reloading as soon as Consul is available, and the subscribers are called.

## Documentation

`GenDoc("consul")` generates the commands of the Consul CLI that put the current values to Consul KV, values of `hidden:"true"` fields are empty:

```bash
# http host
consul kv put app/http/host example.com
consul kv put app/http/port 9090
consul kv put app/hosts a.example.com,b.example.com
```

After `Parse` these are the values of the current environment: a way to move the config to Consul. For the document use `GenDoc` of the format driver.
