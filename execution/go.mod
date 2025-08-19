module github.com/wundergraph/graphql-go-tools/execution

go 1.23.0

require (
	github.com/99designs/gqlgen v0.17.45
	github.com/gobwas/ws v1.4.0
	github.com/golang/mock v1.6.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.1
	github.com/hashicorp/go-plugin v1.6.3
	github.com/hashicorp/golang-lru v0.5.4
	github.com/jensneuse/abstractlogger v0.0.4
	github.com/sebdah/goldie/v2 v2.7.1
	github.com/stretchr/testify v1.10.0
	github.com/vektah/gqlparser/v2 v2.5.14
	github.com/wundergraph/astjson v0.0.0-20250106123708-be463c97e083
	github.com/wundergraph/cosmo/composition-go v0.0.0-20241020204711-78f240a77c99
	github.com/wundergraph/cosmo/router v0.0.0-20240729154441-b20b00e892c6
	github.com/wundergraph/graphql-go-tools/v2 v2.0.0-rc.186
	go.uber.org/atomic v1.11.0
	google.golang.org/grpc v1.71.0
	google.golang.org/protobuf v1.36.4
)

require (
	github.com/agnivade/levenshtein v1.1.1 // indirect
	github.com/bufbuild/protocompile v0.14.1 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dlclark/regexp2 v1.11.0 // indirect
	github.com/dop251/goja v0.0.0-20230906160731-9410bcaa81d2 // indirect
	github.com/fatih/color v1.16.0 // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/pprof v0.0.0-20230207041349-798e818bf904 // indirect
	github.com/hashicorp/go-hclog v0.14.1 // indirect
	github.com/hashicorp/yamux v0.1.1 // indirect
	github.com/jensneuse/byte-template v0.0.0-20200214152254-4f3cf06e5c68 // indirect
	github.com/kingledion/go-tools v0.6.0 // indirect
	github.com/logrusorgru/aurora/v3 v3.0.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/oklog/run v1.0.0 // indirect
	github.com/phf/go-queue v0.0.0-20170504031614-9abe38d0371d // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/r3labs/sse/v2 v2.8.1 // indirect
	github.com/sergi/go-diff v1.3.1 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/tidwall/gjson v1.17.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.26.0 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sync v0.12.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250115164207-1a7da9e5054f // indirect
	gopkg.in/cenkalti/backoff.v1 v1.1.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	rogchap.com/v8go v0.9.0 // indirect
)

// cosmo/router dependency uses indirect dependency of gqlgen of version v0.17.39
// code in this workspace uses v0.17.22
// this is a workaround to make sure that the correct version is used
// as we cannot pin the specific version in go mod
replace github.com/99designs/gqlgen => github.com/99designs/gqlgen v0.17.22
