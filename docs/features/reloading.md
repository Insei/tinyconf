# Reloading

The manager keeps configs up to date: after every change of the config sources it parses the configs again. The application gets the current version with `Current` and subscribes to the changes with `Subscribe`.

```go
c := &Config{}
if err := config.Register(c); err != nil {
    panic(err)
}
if err := config.Parse(c); err != nil {
    panic(err)
}

// a package subscribes to its part of the config
err := tinyconf.Subscribe(ctx, config, func(prev, next *HTTP) {
    if prev.Timeout != next.Timeout {
        server.SetTimeout(next.Timeout)
    }
})

// anywhere in the application
level := tinyconf.Current[Logging](config).Level
```

## Subscribe

```go
func Subscribe[T any](ctx context.Context, m *Manager, onChange func(prev, next *T)) error
```

* `T` is a registered config or a [sub config](structs.md#sub-configs): the type of a field of the registered config. A package subscribes to its own part and knows nothing about the whole config.
* `Subscribe` does not block, the subscription is active until `ctx` is done.
* `onChange` is called only if some fields of `T` are changed, with the previous and the new versions. A change of another part of the config does not call it.
* The configs with errors are not applied: the errors are [logged](logging.md), the subscribers are not called and keep the last good version.

## Current

```go
func Current[T any](m *Manager) *T
```

`Current` returns the current version of the config or the sub config: after the last `Parse` of the registered config or the last reload. It is `nil` before the first `Parse`, and for a sub config stored by pointer that is not created.

!!! warning "Do not change the versions"
    The versions from `Current` and `onChange` are shared by all readers, so they are read without locks. Never change them: copy the value if it should be changed.

## How it works

* Watching starts with the first subscription and stops after the context of the last one is done. One watching serves all subscribers: every driver watches its sources once for the whole application.
* After every change the registered configs with subscribers are parsed again, **from their values before drivers at the first `Parse`**, so the result is the same as `Parse` would give at the start. A value removed from the file gets its default back.
* All drivers are applied, not only the changed one: the usual [priority of drivers](manager.md#priority-of-drivers) is kept.
* The new version replaces the current one atomically, and the subscribers of the changed parts are called.
* The config passed to `Register` and `Parse` is never changed in the background, the application can read it as before.
* `Parse` called by the application updates the current version too, but does not call the subscribers: the application knows what it does.
* [`GenDoc`](gendoc.md) shows the values of the current versions.

## What is watched

Changes are reported by drivers. The bundled [yaml](../drivers/yaml.md), [json](../drivers/json.md), [toml](../drivers/toml.md), [ini](../drivers/ini.md) and [properties](../drivers/properties.md) drivers watch their files from the OS file system: the main file and the files from `WithFiles`. The [consul](../drivers/consul.md) driver and Consul documents are notified by Consul right after the change. The [vault](../drivers/vault.md) driver checks the versions of the secrets. The [etcd](../drivers/etcd.md) driver and etcd documents watch the stream of the events of etcd.

| Source | Watched |
|---|---|
| files from `New` and `WithFiles` | yes, events of the file system and polling |
| Consul keys and documents | yes, notified by Consul |
| Vault secrets | yes, polling, every 30 seconds by default |
| etcd keys and documents | yes, notified by etcd |
| files from `WithFS`, data from `WithData` | no |
| environment variables, flags, tags | no |

Files are watched by the events of the file system: inotify on Linux, kqueue on macOS and BSD, ReadDirectoryChangesW on Windows. The change is detected right after the file is written: the driver waits 100 milliseconds for the next events, so the several writes of an editor are one change.

The directories of the files are watched, not the files themselves, so everything that replaces the file works: editors, atomic renames, `kubectl` updates of ConfigMap volumes that replace symlinks. A file that appears or disappears is a change too.

The files are also checked by polling, every 5 seconds by default: it detects the changes when the events are not available — the limit of inotify watches is reached, the directory does not exist yet, the file system does not support events (some network file systems). The interval is set with the `WithWatchInterval` option of the driver:

```go
yamlDriver, err := yaml.New("config.yaml", yaml.WithWatchInterval(time.Minute))
```

## Errors

| Case | Result |
|---|---|
| the config is not registered | `Subscribe` returns `tinyconf.ErrNotRegisteredConfig`, `Current` returns `nil` |
| no driver can watch for changes | `Subscribe` returns `tinyconf.ErrNoWatchers` |
| the new config is parsed with errors | the error is logged as `failed to reload`, the current version is kept |
| a driver can not watch anymore, access is denied for example | the error is logged, other drivers continue watching |
| the handler panics | the panic is logged as `subscriber panicked`, other subscribers are called |

## Health checks

The errors of the reloads are logged, the application keeps working with the last good config. `ReloadStatus` shows this state for the health checks:

```go
status := config.ReloadStatus()
```

| Field | Description |
|---|---|
| `Watching` | there are subscriptions and the sources are watched |
| `LastReload` | the time of the last reload |
| `LastError` | the error of the last reload, `nil` if the configs are applied |
| `LastApplied` | the time of the last reload without errors |
| `StoppedDrivers` | the drivers that can not watch anymore, with the errors: access is denied, for example |

A readiness check that fails when the config is broken for a long time or a source is not watched:

```go
func configHealthy(config *tinyconf.Manager) error {
    status := config.ReloadStatus()
    if status.LastError != nil && time.Since(status.LastApplied) > 5*time.Minute {
        return fmt.Errorf("config is not reloaded since %s: %w", status.LastApplied, status.LastError)
    }
    for driver, err := range status.StoppedDrivers {
        return fmt.Errorf("%s driver does not watch for changes: %w", driver, err)
    }
    return nil
}
```

## Details

* Handlers are called one by one in the order of subscription, from one goroutine. The next reload starts after all handlers return, so a slow handler delays the next reload.
* Several changes that happen while the configs are being parsed are merged into one reload.
* On the debug level only the fields with a new value are [logged](logging.md#history-of-a-field) after a reload, with the driver that has set the value.
* What can be applied on the fly is up to the application: a log level can be changed at any time, a listening port requires restart of the server. Compare `prev` and `next` and act on the fields that matter.
