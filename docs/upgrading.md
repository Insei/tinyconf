# Upgrading to v2

v2 changes the module path and a few methods. The steps for the code of v1:

1. Change the imports from `github.com/insei/tinyconf/...` to `github.com/insei/tinyconf/v2/...`:

    ```sh
    go get github.com/insei/tinyconf/v2@latest
    ```

2. `Parse` takes the context: `Parse(conf)` is `Parse(context.Background(), conf)`. The context stops the requests of the [remote drivers](features/manager.md#parse).

3. The zap adapter is the separate module, zap is not a dependency of the core anymore: `logger.NewZapLogger(l)` is `zaplogger.New(l)` from `github.com/insei/tinyconf/logger/zaplogger`, see [Logging](features/logging.md).

4. Fields of an [embedded struct](features/structs.md#embedded-structs) without the tag are on the level of the embedding struct, like in `encoding/json`. Check the keys of such fields in the files and the environment variables.
