module github.com/RachidChabane/spec-proof/alignment

go 1.26.4

require (
	github.com/RachidChabane/spec-proof/format v0.0.0
	github.com/odvcencio/gotreesitter v0.55.1
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3
	gopkg.in/yaml.v3 v3.0.1
	pgregory.net/rapid v1.3.0
)

tool gotest.tools/gotestsum

replace github.com/RachidChabane/spec-proof/format => ../format
