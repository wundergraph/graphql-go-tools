module github.com/jensneuse/graphql-go-tools/examples/federation

go 1.15

require (
	github.com/99designs/gqlgen v0.13.1-0.20210728041543-7e38dd46943c
	github.com/gobwas/ws v1.0.4
	github.com/gorilla/websocket v1.4.2
	github.com/jensneuse/abstractlogger v0.0.4
	github.com/jensneuse/graphql-go-tools v1.20.2
	github.com/nats-io/nats-server/v2 v2.3.2 // indirect
	github.com/vektah/gqlparser/v2 v2.2.0
	go.uber.org/atomic v1.9.0
	go.uber.org/zap v1.18.1
)

replace github.com/jensneuse/graphql-go-tools => ../../
