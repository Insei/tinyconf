module example

go 1.18

replace tinyconf => ../

require (
	github.com/google/uuid v1.6.0
	tinyconf v0.0.0-00010101000000-000000000000
)

require (
	github.com/insei/cast v1.1.1 // indirect
	github.com/insei/fmap/v3 v3.0.0 // indirect
	github.com/insei/tinyconf v1.0.2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
