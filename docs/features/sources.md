# Value sources

With several drivers it is not always obvious where the final value came from. `Sources` returns which driver has set every field in the last `Parse` call.

```go
if err := config.Parse(c); err != nil {
    panic(err)
}
for _, source := range config.Sources(c) {
    fmt.Printf("%s is set by %s driver from %s\n", source.Field, source.Driver, source.Source)
}
```

```text
HTTP.Host is set by env driver from HTTP_HOST
HTTP.Port is set by yaml driver from http.port
Password is set by env driver from PASSWORD
```

## FieldSource

| Field | Description |
|---|---|
| `Field` | struct path of the field in the config, `HTTP.Port` for example |
| `Driver` | name of the driver that has set the final value |
| `Source` | source of the value inside the driver |

`Source` depends on the driver:

| Driver | Source |
|---|---|
| tag | name of the tag, `initial` |
| yaml, json, toml | path by the tags, `http.port` |
| env | name of the variable, with the prefix; `HOSTS_*` for the value from [indexed keys](../drivers/env.md#indexed-keys) |
| flag | name of the flag, `-http-port` |
| consul | the key, `app/http/port` |

## Rules

* Sources are returned in the order of the config fields.
* Only the last driver that has set the value is reported, the overridden ones are not.
* Fields without values from drivers are not included: they have the value that was in the config before `Parse`.
* Sources are stored per config type, pass the same pointer type that was used in `Parse`. For a [sub config](structs.md#sub-configs) the paths are relative to it.

It is handy to log the sources once at the start of the application: the answer to "why is this value here" is in the logs. Values themselves are not included, so secrets are not leaked.
