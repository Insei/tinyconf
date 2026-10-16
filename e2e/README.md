# End-to-end tests

Tests of the drivers with the real external systems: Consul and Vault, other config storages later.
The systems are started in Docker by [testcontainers](https://golang.testcontainers.org/), so only Docker is needed:

```sh
cd e2e
go test -race ./...
```

It is a separate Go module, so the dependencies of the tests (testcontainers and the Docker client) are not dependencies of tinyconf,
and its Go version does not limit the minimal Go version of tinyconf. tinyconf is used from the parent directory by `replace`.

## Adding a storage

* One file per storage: `consul_test.go`, `vault_test.go`, ...
* Start the storage by the testcontainers module if there is one, `startConsul` is an example.
  One container is shared by the subtests of the main test, every subtest uses its own keys.
* Change the data by the API of the storage, not by the driver under test.
* Cover what the unit tests with fakes can not: real responses, permissions, watching for changes, restarts.

The tests run in CI by the `e2e` job of `.github/workflows/go.yml`.
