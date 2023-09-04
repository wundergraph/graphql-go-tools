module github.com/TykTechnologies/graphql-go-tools/examples/federation

go 1.19

require (
	github.com/99designs/gqlgen v0.17.22
	github.com/TykTechnologies/graphql-go-tools v1.20.2
	github.com/gobwas/ws v1.0.4
	github.com/gorilla/websocket v1.5.0
	github.com/jensneuse/abstractlogger v0.0.4
	github.com/vektah/gqlparser/v2 v2.5.1
	go.uber.org/atomic v1.9.0
	go.uber.org/zap v1.18.1
)

replace github.com/TykTechnologies/graphql-go-tools => ../../
