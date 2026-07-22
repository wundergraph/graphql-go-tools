module github.com/wundergraph/graphql-go-tools/examples/federation

go 1.25.0

require (
	github.com/99designs/gqlgen v0.17.76
	github.com/gobwas/ws v1.4.0
	github.com/gorilla/websocket v1.5.1
	github.com/jensneuse/abstractlogger v0.0.4
	github.com/vektah/gqlparser/v2 v2.5.30
	github.com/wundergraph/cosmo/router v0.0.0-20260611115430-e8a965a40952
	github.com/wundergraph/graphql-go-tools/execution v1.0.1
	github.com/wundergraph/graphql-go-tools/v2 v2.4.4
	go.uber.org/atomic v1.11.0
	go.uber.org/zap v1.27.0
	google.golang.org/protobuf v1.36.11
)

require (
	connectrpc.com/connect v1.19.2 // indirect
	github.com/agnivade/levenshtein v1.2.1 // indirect
	github.com/bufbuild/protocompile v0.14.1 // indirect
	github.com/buger/jsonparser v1.1.2 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/coder/websocket v1.8.14 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/jensneuse/byte-template v0.0.0-20231025215717-69252eb3ed56 // indirect
	github.com/kingledion/go-tools v0.6.0 // indirect
	github.com/logrusorgru/aurora/v4 v4.0.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/phf/go-queue v0.0.0-20170504031614-9abe38d0371d // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/r3labs/sse/v2 v2.8.1 // indirect
	github.com/rs/xid v1.6.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/sosodev/duration v1.3.1 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/urfave/cli/v2 v2.27.7 // indirect
	github.com/wundergraph/astjson v1.1.0 // indirect
	github.com/wundergraph/go-arena v1.3.0 // indirect
	github.com/xrash/smetrics v0.0.0-20250705151800-55b8f293f342 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	golang.org/x/tools v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260401024825-9d38bb4040a9 // indirect
	google.golang.org/grpc v1.80.0 // indirect
	gopkg.in/cenkalti/backoff.v1 v1.1.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/wundergraph/graphql-go-tools/v2 => ../../v2

replace github.com/wundergraph/graphql-go-tools/execution => ../../execution

tool github.com/99designs/gqlgen
