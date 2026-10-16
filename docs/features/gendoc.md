# Documentation generation

`GenDoc(driverName)` generates a template of the config in the format of the driver: all fields of all registered configs with their descriptions and current values.

```go
c := &Config{}
if err := config.Register(c); err != nil {
    panic(err)
}
if err := config.Parse(c); err != nil {
    panic(err)
}
fmt.Println(config.GenDoc("yaml"))
fmt.Println(config.GenDoc("env"))
```

For this config

```go
type HTTP struct {
    Host string `yaml:"host" env:"HTTP_HOST" initial:"localhost" doc:"http host"`
    Port int    `yaml:"port" env:"HTTP_PORT" initial:"8080" doc:"http port"`
}

type Config struct {
    HTTP     HTTP   `yaml:"http"`
    Password string `yaml:"password" env:"PASSWORD" hidden:"true"`
}
```

the output is

=== "yaml"

    ```yaml
    #
    #http: 
    	#http host
    	#host: localhost
    	#http port
    	#port: 8080
    #
    #password: 
    ```

=== "env"

    ```bash
    #http host
    #HTTP_HOST=localhost
    #http port
    #HTTP_PORT=8080

    #
    #PASSWORD=
    ```

* The description is taken from the `doc` tag.
* The value is the current value of the field in the registered config, so call `GenDoc` after `Parse` to get the actual values, or before it to get a clean template.
* Values of the fields with `hidden:"true"` are not shown.
* Slices and maps are shown in the format of the driver: `hosts: ["a","b"]` for yaml, `HOSTS=a,b` for env, so the value can be copied from the template as is.
* The name of the driver is the name of its tag: `yaml`, `json`, `toml`, `ini`, `properties`, `env`, `flag`, `consul`, `vault`, `etcd`. The consul, vault and etcd drivers generate the commands of their CLI, see [consul](../drivers/consul.md#documentation), [vault](../drivers/vault.md#documentation) and [etcd](../drivers/etcd.md#documentation).
* The json and toml drivers generate documents without descriptions, see [json](../drivers/json.md#documentation) and [toml](../drivers/toml.md#documentation).

A common way to use it is a command line flag of the application that prints the template and exits.
