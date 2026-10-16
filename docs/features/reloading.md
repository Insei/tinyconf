# Reloading

`Watch` keeps the config up to date: after every change of the config sources it parses the config again and passes the result to the callback.

```go
err := config.Watch(ctx, c, func(conf any, err error) {
    // conf is a new *Config
})
```

## How it works

* `Watch` blocks until the context is done, run it in a goroutine.
* After every change it parses the config to a **new copy** and calls `onChange` with the pointer to this copy and the error of parsing.
* The config passed to `Watch` is **never changed**. Its values at the moment of the call are used as defaults for every copy, so the result is the same as `Parse` would give at the start.
* All drivers are applied to every copy, not only the changed one: the usual [priority of drivers](manager.md#priority-of-drivers) is kept.

Because the old config is never modified, it can be read from any goroutine without locks. The new config is published by replacing the pointer:

```go
var current atomic.Pointer[Config]

c := &Config{}
if err := config.Register(c); err != nil {
    panic(err)
}
if err := config.Parse(c); err != nil {
    panic(err)
}
current.Store(c)

go func() {
    err := config.Watch(ctx, c, func(conf any, err error) {
        if err != nil {
            // keep the previous config
            log.Println("config is not reloaded:", err)
            return
        }
        current.Store(conf.(*Config))
    })
    if err != nil {
        log.Println("config watching is stopped:", err)
    }
}()

// anywhere in the application
port := current.Load().HTTP.Port
```

!!! warning "Check the error"
    The copy is passed to `onChange` even if parsing has failed, with the fields that were parsed. A file saved in the middle of editing is a typical reason of the error, so do not apply the config with the error.

## What is watched

Changes are reported by drivers. The bundled [yaml](../drivers/yaml.md), [json](../drivers/json.md) and [toml](../drivers/toml.md) drivers watch their files from the OS file system: the main file and the files from `WithFiles`. The [consul](../drivers/consul.md) driver and Consul documents are notified by Consul right after the change.

| Source | Watched |
|---|---|
| files from `New` and `WithFiles` | yes, polling |
| Consul keys and documents | yes, notified by Consul |
| files from `WithFS`, data from `WithData` | no |
| environment variables, flags, tags | no |

Files are checked by polling, every 5 seconds by default. The interval is set with the `WithWatchInterval` option of the driver:

```go
yamlDriver, err := yaml.New("config.yaml", yaml.WithWatchInterval(time.Second))
```

Polling works with everything that replaces the file: editors, `kubectl` updates of ConfigMap volumes, atomic renames. A file that appears or disappears is a change too.

## Errors

| Case | Result |
|---|---|
| the config is not registered | `Watch` returns `tinyconf.ErrNotRegisteredConfig` |
| no driver can watch for changes | `Watch` returns `tinyconf.ErrNoWatchers` |
| the new config is parsed with errors | `onChange` is called with the error, `Watch` continues |
| the context is done | `Watch` returns `nil` |

## Details

* `onChange` is called from one goroutine, the next call starts after the previous one returns.
* Several changes that happen while the config is being parsed are merged into one reload.
* A [sub config](structs.md#sub-configs) can be watched the same way as the whole config.
* On the debug level only the fields with a new value are [logged](logging.md#history-of-a-field) after a reload, with the driver that has set the value. Nothing is logged if the new config has errors, changes are logged relative to the last config without errors.
* [Structs stored by pointer](structs.md#structs-stored-by-pointer) are copied for every new config too, the structs of the original config are not changed.
* What can be applied on the fly is up to the application: a log level can be changed at any time, a listening port requires restart of the server. Compare the old and the new config in `onChange` and act on the fields that matter.
