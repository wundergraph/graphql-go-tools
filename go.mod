module github.com/jensneuse/graphql-go-tools

go 1.12

require (
	github.com/buger/jsonparser v0.0.0-20181115193947-bf1c66bbce23
	github.com/cespare/xxhash v1.1.0
	github.com/dave/jennifer v1.4.0
	github.com/davecgh/go-spew v1.1.1
	github.com/eclipse/paho.mqtt.golang v1.2.0
	github.com/evanphx/json-patch/v5 v5.1.0
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/go-test/deep v1.0.4
	github.com/gobuffalo/packr v1.30.1
	github.com/gobwas/httphead v0.0.0-20180130184737-2c6c146eadee // indirect
	github.com/gobwas/pool v0.2.0 // indirect
	github.com/gobwas/ws v1.0.2
	github.com/golang/mock v1.4.1
	github.com/gorilla/websocket v1.4.2
	github.com/iancoleman/strcase v0.0.0-20191112232945-16388991a334
	github.com/jensneuse/abstractlogger v0.0.4
	github.com/jensneuse/byte-template v0.0.0-20200214152254-4f3cf06e5c68
	github.com/jensneuse/diffview v1.0.0
	github.com/jensneuse/pipeline v0.0.0-20200117120358-9fb4de085cd6
	github.com/klauspost/compress v1.13.1 // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/mitchellh/mapstructure v1.2.2 // indirect
	github.com/nats-io/nats-server/v2 v2.1.2 // indirect
	github.com/nats-io/nats.go v1.9.1
	github.com/pelletier/go-toml v1.6.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/sebdah/goldie v0.0.0-20180424091453-8784dd1ab561
	github.com/spf13/cobra v0.0.5
	github.com/spf13/viper v1.3.2
	github.com/stretchr/testify v1.5.1
	github.com/tidwall/gjson v1.3.5
	github.com/tidwall/sjson v1.0.4
	github.com/valyala/fasthttp v1.12.0
	go.uber.org/atomic v1.5.1
	go.uber.org/multierr v1.4.0 // indirect
	golang.org/x/lint v0.0.0-20191125180803-fdd1cda4f05f // indirect
	golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e
	golang.org/x/sys v0.0.0-20201101102859-da207088b7d1 // indirect
	golang.org/x/text v0.3.2 // indirect
	golang.org/x/tools v0.0.0-20200414032229-332987a829c3 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/yaml.v2 v2.2.8 // indirect
	nhooyr.io/websocket v1.8.7 // indirect
)

replace github.com/tidwall/gjson => github.com/jensneuse/gjson v1.3.6-0.20200106141904-7ea619137b22

replace github.com/jensneuse/graphql-go-tools/examples/chat => ./examples/chat
