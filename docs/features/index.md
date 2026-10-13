# Features

Reference of tinyconf capabilities grouped by area.

## Configuration

| Page | What's inside |
|---|---|
| [Manager](manager.md) | `New`, `WithDriver`, `WithLogger`, `Register`, `Parse`, priority of drivers, errors, concurrency |
| [Struct tags](tags.md) | Tags of drivers and common tags: `doc`, `hidden`, `required`, `separator`, `kvseparator` |
| [Supported types](types.md) | Scalars, durations, time, slices, arrays, maps, structs, pointers — the same for all drivers |
| [Nested structs and sub configs](structs.md) | Grouping fields into structs, parsing a part of the config |
| [Required fields](required.md) | `required:"true"` tag and `ErrRequired` |

## Operations

| Page | What's inside |
|---|---|
| [Documentation generation](gendoc.md) | `GenDoc`: a config template in the format of the driver |
| [Value sources](sources.md) | `Sources`: which driver has set the value of every field |
| [Reloading](reloading.md) | `Watch`: parsing the config again after changes of the files |
| [Logging](logging.md) | What the manager logs, bundled loggers, custom logger |
