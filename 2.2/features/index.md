# Features

Reference of tinyconf capabilities grouped by area.

Looking for a recipe?

The [Use cases](https://insei.github.io/tinyconf/2.2/use-cases/index.md) page shows how the features work together in typical setups: a twelve-factor service, a config file with overrides, a command line tool, a Kubernetes deployment.

## Configuration

| Page                                                                                             | What's inside                                                                                    |
| ------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------ |
| [Manager](https://insei.github.io/tinyconf/2.2/features/manager/index.md)                        | `New`, `WithDriver`, `WithLogger`, `Register`, `Parse`, priority of drivers, errors, concurrency |
| [Struct tags](https://insei.github.io/tinyconf/2.2/features/tags/index.md)                       | Tags of drivers and common tags: `doc`, `hidden`, `required`, `separator`, `kvseparator`         |
| [Supported types](https://insei.github.io/tinyconf/2.2/features/types/index.md)                  | Scalars, durations, time, slices, arrays, maps, structs, pointers — the same for all drivers     |
| [Nested structs and sub configs](https://insei.github.io/tinyconf/2.2/features/structs/index.md) | Grouping fields into structs, optional sections with pointers, parsing a part of the config      |
| [Required fields](https://insei.github.io/tinyconf/2.2/features/required/index.md)               | `required:"true"` tag and `ErrRequired`                                                          |

## Operations

| Page                                                                                      | What's inside                                                                    |
| ----------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| [Documentation generation](https://insei.github.io/tinyconf/2.2/features/gendoc/index.md) | `GenDoc`: a config template in the format of the driver                          |
| [Value sources](https://insei.github.io/tinyconf/2.2/features/sources/index.md)           | `Sources`: which driver has set the value of every field                         |
| [Reloading](https://insei.github.io/tinyconf/2.2/features/reloading/index.md)             | `Subscribe` and `Current`: configs are parsed again after changes of the sources |
| [Logging](https://insei.github.io/tinyconf/2.2/features/logging/index.md)                 | What the manager logs, bundled loggers, custom logger                            |
