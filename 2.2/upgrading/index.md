# Upgrading to v2

v2 changes the module path and a few methods. The steps for the code of v1:

1. Change the imports from `github.com/insei/tinyconf/...` to `github.com/insei/tinyconf/v2/...`:

   ```sh
   go get github.com/insei/tinyconf/v2@latest
   ```

1. `Parse` takes the context: `Parse(conf)` is `Parse(context.Background(), conf)`. The context stops the requests of the [remote drivers](https://insei.github.io/tinyconf/2.2/features/manager/#parse).

1. The zap adapter is the separate module, zap is not a dependency of the core anymore: `logger.NewZapLogger(l)` is `zaplogger.New(l)` from `github.com/insei/tinyconf/logger/zaplogger`, see [Logging](https://insei.github.io/tinyconf/2.2/features/logging/index.md).

1. Fields of an [embedded struct](https://insei.github.io/tinyconf/2.2/features/structs/#embedded-structs) without the tag are on the level of the embedding struct, like in `encoding/json`. Check the keys of such fields in the files and the environment variables.

1. `Parse` returns the errors: a value that can't be converted to the type of the field, a file that can't be read, a field with `required:"true"` without a value. v1 only logged them and returned `nil`. All fields are parsed anyway, the errors are returned as one error, see [Errors](https://insei.github.io/tinyconf/2.2/features/manager/#errors).
