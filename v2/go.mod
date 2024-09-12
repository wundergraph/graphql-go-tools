module github.com/wundergraph/graphql-go-tools/v2

go 1.21

require (
	github.com/99designs/gqlgen v0.17.45
	github.com/alitto/pond v1.8.3
	github.com/buger/jsonparser v1.1.1
	github.com/cespare/xxhash/v2 v2.2.0
	github.com/davecgh/go-spew v1.1.1
	github.com/goccy/go-json v0.10.2
	github.com/golang/mock v1.6.0
	github.com/google/go-cmp v0.6.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.1
	github.com/jensneuse/abstractlogger v0.0.4
	github.com/jensneuse/byte-template v0.0.0-20200214152254-4f3cf06e5c68
	github.com/jensneuse/diffview v1.0.0
	github.com/kingledion/go-tools v0.6.0
	github.com/kylelemons/godebug v1.1.0
	github.com/pkg/errors v0.9.1
	github.com/r3labs/sse/v2 v2.8.1
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.1
	github.com/sebdah/goldie/v2 v2.5.3
	github.com/stretchr/testify v1.9.0
	github.com/tidwall/gjson v1.17.0
	github.com/tidwall/sjson v1.2.5
	github.com/vektah/gqlparser/v2 v2.5.11
	github.com/wundergraph/astjson v0.0.0-20240910140849-bb15f94bd362
	go.uber.org/atomic v1.11.0
	go.uber.org/zap v1.26.0
	golang.org/x/sync v0.7.0
	gonum.org/v1/gonum v0.14.0
	gopkg.in/yaml.v2 v2.4.0
	nhooyr.io/websocket v1.8.11
)

require (
	github.com/agnivade/levenshtein v1.1.1 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/phf/go-queue v0.0.0-20170504031614-9abe38d0371d // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.12.0 // indirect
	github.com/sergi/go-diff v1.3.1 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/sosodev/duration v1.2.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	go.uber.org/goleak v1.3.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/net v0.26.0 // indirect
	golang.org/x/sys v0.21.0 // indirect
	gopkg.in/cenkalti/backoff.v1 v1.1.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

//replace github.com/wundergraph/astjson => ../../astjson
