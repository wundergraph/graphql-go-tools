module github.com/jensneuse/graphql-go-tools

go 1.15

require (
	github.com/99designs/gqlgen v0.13.1-0.20210728041543-7e38dd46943c
	github.com/OneOfOne/xxhash v1.2.8
	github.com/agnivade/levenshtein v1.1.1 // indirect
	github.com/buger/jsonparser v1.1.1
	github.com/cespare/xxhash v1.1.0
	github.com/dave/jennifer v1.4.0
	github.com/davecgh/go-spew v1.1.1
	github.com/eclipse/paho.mqtt.golang v1.2.0
	github.com/evanphx/json-patch/v5 v5.1.0
	github.com/go-test/deep v1.0.4
	github.com/gobuffalo/packr v1.30.1
	github.com/gobwas/ws v1.0.4
	github.com/golang/mock v1.4.1
	github.com/gorilla/websocket v1.4.2
	github.com/hashicorp/golang-lru v0.5.4
	github.com/iancoleman/strcase v0.0.0-20191112232945-16388991a334
	github.com/jensneuse/abstractlogger v0.0.4
	github.com/jensneuse/byte-template v0.0.0-20200214152254-4f3cf06e5c68
	github.com/jensneuse/diffview v1.0.0
	github.com/jensneuse/graphql-go-tools/examples/chat v0.0.0-20210714083836-7bf4457dc2b2
	github.com/jensneuse/graphql-go-tools/examples/federation v0.0.0-20210714083836-7bf4457dc2b2
	github.com/jensneuse/pipeline v0.0.0-20200117120358-9fb4de085cd6
	github.com/klauspost/compress v1.13.1 // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/mitchellh/mapstructure v1.4.1 // indirect
	github.com/nats-io/nats.go v1.11.1-0.20210623165838-4b75fc59ae30
	github.com/sebdah/goldie v0.0.0-20180424091453-8784dd1ab561
	github.com/spf13/afero v1.6.0 // indirect
	github.com/spf13/cobra v0.0.5
	github.com/spf13/viper v1.3.2
	github.com/stretchr/testify v1.7.0
	github.com/tidwall/gjson v1.8.1
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/tidwall/sjson v1.0.4
	github.com/vektah/gqlparser/v2 v2.2.0
	go.uber.org/atomic v1.9.0
	go.uber.org/zap v1.18.1
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
	nhooyr.io/websocket v1.8.7
)

replace github.com/jensneuse/graphql-go-tools/examples/federation => ./examples/federation

replace github.com/jensneuse/graphql-go-tools/examples/chat => ./examples/chat
