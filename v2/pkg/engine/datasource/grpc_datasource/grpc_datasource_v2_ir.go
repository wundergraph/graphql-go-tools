package grpcdatasource

import (
	"buf.build/go/hyperpb"
	protoref "google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type v2Program struct {
	stages           []v2Stage
	nativeOperation  bool
	requiresFallback bool
	fallbackReasons  []string
}

type v2Stage struct {
	fetches []v2Fetch
}

type v2Fetch struct {
	id             int
	kind           CallKind
	dependencies   []int
	serviceName    string
	methodName     string
	responsePath   ast.Path
	request        *v2RequestProgram
	response       *v2ResponseProgram
	native         bool
	fallbackReason string
}

type v2RequestProgram struct {
	message *v2MessageRuntime
	fields  []v2RequestFieldProgram
	context *v2ContextProgram
	wire    *v2WirePlan
}

type v2RequestFieldProgram struct {
	runtime     *v2FieldRuntime
	jsonPath    string
	staticValue string
	enumName    string
	optional    bool
	repeated    bool
	child       *v2RequestProgram
}

type v2ResponseProgram struct {
	message   *v2MessageRuntime
	fields    []v2ResponseFieldProgram
	oneOfType OneOfType
	fragments map[string]*v2ResponseProgram
}

type v2ResponseFieldProgram struct {
	runtime     *v2FieldRuntime
	name        string
	staticValue string
	enumName    string
	repeated    bool
	child       *v2ResponseProgram
	scalarType  DataType
}

type v2NativeResponse struct {
	kind         CallKind
	responsePath ast.Path
	output       protoref.Message
	shared       *hyperpb.Shared
	skip         bool
}

type v2ContextProgram struct {
	runtime *v2FieldRuntime
	message *v2MessageRuntime
	fields  []v2ContextFieldProgram
}

type v2ContextFieldProgram struct {
	runtime *v2FieldRuntime
	path    v2ResolvePathProgram
}

type v2ResolvePathProgram struct {
	steps []v2ResolvePathStep
}

type v2ResolvePathStep struct {
	runtime *v2FieldRuntime
}
