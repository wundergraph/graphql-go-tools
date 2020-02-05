module github.com/jensneuse/graphql-go-tools

go 1.12

require (
	github.com/buger/jsonparser v0.0.0-20181115193947-bf1c66bbce23
	github.com/cespare/xxhash v1.1.0
	github.com/davecgh/go-spew v1.1.1
	github.com/eclipse/paho.mqtt.golang v1.2.0
	github.com/go-test/deep v1.0.4
	github.com/gobuffalo/packr v1.30.1
	github.com/gobwas/httphead v0.0.0-20180130184737-2c6c146eadee // indirect
	github.com/gobwas/pool v0.2.0 // indirect
	github.com/gobwas/ws v1.0.2
	github.com/jensneuse/abstractlogger v0.0.4
	github.com/jensneuse/diffview v1.0.0
	github.com/jensneuse/pipeline v0.0.0-20200117120358-9fb4de085cd6
	github.com/nats-io/nats-server/v2 v2.1.2 // indirect
	github.com/nats-io/nats.go v1.9.1
	github.com/sebdah/goldie v0.0.0-20180424091453-8784dd1ab561
	github.com/tidwall/gjson v1.3.5
	github.com/tidwall/sjson v1.0.4
	github.com/valyala/fasttemplate v1.1.0
	github.com/wasmerio/go-ext-wasm v0.0.0-20191213132134-adcef605ea8e
	go.uber.org/atomic v1.5.1 // indirect
	go.uber.org/multierr v1.4.0 // indirect
	golang.org/x/lint v0.0.0-20191125180803-fdd1cda4f05f // indirect
	golang.org/x/sys v0.0.0-20200113162924-86b910548bc1 // indirect
	golang.org/x/tools v0.0.0-20200115044656-831fdb1e1868 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/yaml.v2 v2.2.4 // indirect
)

replace github.com/tidwall/gjson => github.com/jensneuse/gjson v1.3.6-0.20200106141904-7ea619137b22
