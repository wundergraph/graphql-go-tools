module github.com/wundergraph/graphql-go-tools

go 1.18

require (
	github.com/99designs/gqlgen v0.13.1-0.20210728041543-7e38dd46943c
	github.com/buger/jsonparser v1.1.1
	github.com/cespare/xxhash/v2 v2.1.2
	github.com/dave/jennifer v1.4.0
	github.com/davecgh/go-spew v1.1.1
	github.com/eclipse/paho.mqtt.golang v1.2.0
	github.com/evanphx/json-patch/v5 v5.1.0
	github.com/go-test/deep v1.0.4
	github.com/gobwas/ws v1.0.4
	github.com/golang/mock v1.4.1
	github.com/google/go-cmp v0.5.6
	github.com/gorilla/websocket v1.4.2
	github.com/hashicorp/golang-lru v0.5.4
	github.com/iancoleman/strcase v0.0.0-20191112232945-16388991a334
	github.com/jensneuse/abstractlogger v0.0.4
	github.com/jensneuse/byte-template v0.0.0-20200214152254-4f3cf06e5c68
	github.com/jensneuse/diffview v1.0.0
	github.com/jensneuse/pipeline v0.0.0-20200117120358-9fb4de085cd6
	github.com/mitchellh/go-homedir v1.1.0
	github.com/nats-io/nats.go v1.14.0
	github.com/qri-io/jsonschema v0.2.1
	github.com/sebdah/goldie v0.0.0-20180424091453-8784dd1ab561
	github.com/spf13/cobra v0.0.5
	github.com/spf13/viper v1.3.2
	github.com/stretchr/testify v1.7.0
	github.com/tidwall/gjson v1.11.0
	github.com/tidwall/sjson v1.0.4
	github.com/vektah/gqlparser/v2 v2.2.0
	github.com/wundergraph/graphql-go-tools/examples/chat v0.0.0-20220521142629-9fe3016fb1a7
	github.com/wundergraph/graphql-go-tools/examples/federation v0.0.0-20220521142629-9fe3016fb1a7
	go.uber.org/atomic v1.9.0
	go.uber.org/zap v1.18.1
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
	gopkg.in/yaml.v2 v2.2.8
	nhooyr.io/websocket v1.8.7
)

require (
	github.com/Masterminds/goutils v1.1.0 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Masterminds/sprig v2.22.0+incompatible // indirect
	github.com/agnivade/levenshtein v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/gobwas/httphead v0.0.0-20180130184737-2c6c146eadee // indirect
	github.com/gobwas/pool v0.2.0 // indirect
	github.com/golang/protobuf v1.5.0 // indirect
	github.com/google/uuid v1.1.1 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/huandu/xstrings v1.2.1 // indirect
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/klauspost/compress v1.14.4 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/logrusorgru/aurora v0.0.0-20200102142835-e9ef32dff381 // indirect
	github.com/magiconair/properties v1.8.0 // indirect
	github.com/mattn/go-colorable v0.1.4 // indirect
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/mitchellh/copystructure v1.0.0 // indirect
	github.com/mitchellh/mapstructure v1.4.1 // indirect
	github.com/mitchellh/reflectwalk v1.0.0 // indirect
	github.com/nats-io/nats-server/v2 v2.8.2 // indirect
	github.com/nats-io/nkeys v0.3.0 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/pelletier/go-toml v1.6.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/qri-io/jsonpointer v0.1.1 // indirect
	github.com/sirupsen/logrus v1.4.2 // indirect
	github.com/spf13/afero v1.6.0 // indirect
	github.com/spf13/cast v1.3.0 // indirect
	github.com/spf13/jwalterweatherman v1.0.0 // indirect
	github.com/spf13/pflag v1.0.3 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	golang.org/x/crypto v0.0.0-20220315160706-3147a52a75dd // indirect
	golang.org/x/net v0.0.0-20211112202133-69e39bad7dc2 // indirect
	golang.org/x/sys v0.0.0-20220111092808-5a964db01320 // indirect
	golang.org/x/text v0.3.6 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)

replace github.com/wundergraph/graphql-go-tools/examples/federation => ./examples/federation

replace github.com/wundergraph/graphql-go-tools/examples/chat => ./examples/chat
