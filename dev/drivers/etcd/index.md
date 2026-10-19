# etcd

The etcd package reads the config from [etcd](https://etcd.io) version 3 in two ways:

| Way                             | Where the config is                                  | Tags                                  |
| ------------------------------- | ---------------------------------------------------- | ------------------------------------- |
| [key per field](#key-per-field) | every value in its own key: `app/http/port`          | `etcd`                                |
| [document](#document)           | YAML, JSON or TOML document in one key: `app/config` | of the format: `yaml`, `json`, `toml` |

Both ways report changes for [reloading](https://insei.github.io/tinyconf/dev/features/reloading/index.md) as soon as they happen: the driver watches the keys by the stream of the events of etcd, not by polling. The package talks to the [gRPC gateway](https://etcd.io/docs/v3.5/dev-guide/api_grpc_gateway/) of etcd — its JSON API — directly and adds no dependencies.

## Key per field

```go
import "github.com/insei/tinyconf/v2/drivers/etcd"

etcdDriver, err := etcd.New("app")
```

The key of the field is the prefix and the path by the `etcd` tags, dots are replaced with slashes:

```go
type Config struct {
    HTTP struct {
        Host string `etcd:"host"`
        Port int    `etcd:"port"`
    } `etcd:"http"`
    Hosts []string `etcd:"hosts"`
}
```

```bash
etcdctl put app/http/host example.com
etcdctl put app/http/port 9090
etcdctl put app/hosts a.example.com,b.example.com
```

Values are parsed by the rules of [string values](https://insei.github.io/tinyconf/dev/features/types/index.md), like environment variables: slices are comma separated, structs in slices are JSON.

All keys with the prefix are read by one request once per `Parse` call. A key that is absent is not an error, the field keeps its value from the previous driver.

Set the tags of the parents

The key of the nested field is built from the `etcd` tags of its parents. If the `HTTP` field has no `etcd` tag, the `Host` field is read from `app/host`, not from `app/http/host`. When a value is found this way, the manager [logs a warning](https://insei.github.io/tinyconf/dev/features/logging/index.md). [Embedded structs](https://insei.github.io/tinyconf/dev/features/structs/#embedded-structs) without the tag are the exception.

## Document

`etcd.Document` is a source of the config content for the [yaml](https://insei.github.io/tinyconf/dev/drivers/yaml/index.md), [json](https://insei.github.io/tinyconf/dev/drivers/json/index.md) and [toml](https://insei.github.io/tinyconf/dev/drivers/toml/index.md) drivers. The values are read by the tags of the format driver and merged with the files of the driver, as one more file:

```go
yamlDriver, err := yaml.New("config.yaml",
    yaml.WithSource(etcd.Document("app/config")),
)
```

```bash
etcdctl put app/config < config.yaml
```

An absent key is an error of `Parse`, unless the format driver has `WithOptional()`.

## Connection

```go
etcdDriver, err := etcd.New("app",
    etcd.WithEndpoints("https://etcd-1:2379", "https://etcd-2:2379", "https://etcd-3:2379"),
    etcd.WithAuth("app", password),
    etcd.WithHTTPClient(&http.Client{Transport: tlsTransport}),
)
document := etcd.Document("app/config", etcd.WithEndpoints("etcd:2379"))
```

| Option                        | Default                                                           | Description                                                         |
| ----------------------------- | ----------------------------------------------------------------- | ------------------------------------------------------------------- |
| `WithEndpoints(endpoints...)` | `ETCDCTL_ENDPOINTS` or `http://127.0.0.1:2379`                    | `host:port` or URL with the scheme; the variable is comma separated |
| `WithAuth(user, password)`    | `ETCDCTL_USER` (`user` or `user:password`) and `ETCDCTL_PASSWORD` | the user for the clusters with authentication                       |
| `WithHTTPClient(client)`      | `http.DefaultClient`                                              | the client with TLS settings and client certificates, for example   |

The options are the same for `New` and `Document`, and the variables are the same as etcdctl uses.

Requests go to the next endpoint if the current one is not available, and the endpoint that has answered is used first next time. A cluster keeps working for the application while it has the quorum.

With the user the driver gets the token of etcd and gets it again when the token is expired.

## Permissions

| Case                                           | Result                                                                             |
| ---------------------------------------------- | ---------------------------------------------------------------------------------- |
| authentication is enabled, the user is not set | error of `Parse`: `user name is empty`                                             |
| wrong password                                 | error of `Parse`: `authentication failed`                                          |
| the user can not read the prefix or the key    | error of `Parse`: `permission denied`                                              |
| access is denied while watching                | the driver stops watching, the error is logged: retries do not fix the permissions |

All these errors wrap `tinyconf.ErrAccessDenied`: the manager [logs](https://insei.github.io/tinyconf/dev/features/logging/index.md) them as `access denied`, and they can be checked with `errors.Is`. The role of the user needs `read` on the prefix: `etcdctl role grant-permission --prefix=true app read app/`.

## Watching

The manager receives the changes of the keys with the prefix, or of the key of the document, right after they are written to etcd. Keys outside of the prefix do not cause reloads.

If the stream of the events is broken, the driver watches again from the last known revision, with the interval that grows from 1 to 30 seconds, so no changes are missed. If the history of the changes is already [compacted](https://etcd.io/docs/v3.5/op-guide/maintenance/#history-compaction-v3-api-key-value-database), the config is reloaded from the current state. The same is true for the start of the application: if etcd is not available at the first `Parse`, the values are loaded by the reloading as soon as etcd is available, and the subscribers are called.

## Documentation

`GenDoc("etcd")` generates the commands of etcdctl that put the current values to etcd, values of `hidden:"true"` fields are empty:

```bash
# http host
etcdctl put -- app/http/host example.com
etcdctl put -- app/http/port 9090
etcdctl put -- app/hosts a.example.com,b.example.com
```

After `Parse` these are the values of the current environment: a way to move the config to etcd. For the document use `GenDoc` of the format driver.
