package grpcdatasource

import (
	"slices"

	"github.com/tidwall/gjson"
	protoref "google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// matchesMemberType mirrors V1's isAllowedForTypename: for repeated-message
// request fields that carry a MemberTypes constraint (entity-lookup keys),
// only elements whose __typename is in the set are accepted. Empty
// memberTypes means "no constraint".
func (f *v2RequestFieldProgram) matchesMemberType(element gjson.Result) bool {
	if len(f.memberTypes) == 0 {
		return true
	}
	typeName := element.Get("__typename")
	if !typeName.Exists() {
		return true
	}
	return slices.Contains(f.memberTypes, typeName.String())
}

// IR (intermediate representation) for the V2 gRPC datasource.
//
// Design pattern adopted from the parallel codex implementation in
// `hollow-playroom`: pure data types, compiled once at NewDataSourceV2,
// executed by the runtime file. Separating these concerns gives each file a
// tight responsibility and makes expansion (new call kinds, new response
// shapes) a matter of adding cases in one place.
//
// Key IR types:
//   - v2Program: the compiled whole-plan recipe.
//   - v2Stage: a batch of fetches that can run concurrently.
//   - v2Fetch: single fetch with request + response programs.
//   - v2RequestProgram / v2ResponseProgram: compiled encoders / decoders.
//   - v2ContextProgram / v2ResolvePathProgram: compiled resolve-kind helpers.

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
	methodFullName string // pre-computed "/svc/method"
	responsePath   ast.Path
	request        *v2RequestProgram
	response       *v2ResponseProgram
	native         bool
	fallbackReason string
}

type v2RequestProgram struct {
	message *v2MessageRuntime
	fields  []v2RequestFieldProgram
	context *v2ContextProgram // non-nil for resolve-kind requests
}

type v2RequestFieldProgram struct {
	runtime     *v2FieldRuntime
	jsonPath    string
	staticValue string
	enumName    string
	optional    bool
	repeated    bool
	// memberTypes carries the allowed __typename values for repeated-message
	// fields that act as entity-lookup keys. Empty means "accept all elements"
	// (non-federation paths). Populated from RPCMessage.MemberTypes at compile
	// time so the runtime only does a cheap slice scan per element.
	memberTypes []string
	child       *v2RequestProgram
}

type v2ResponseProgram struct {
	message *v2MessageRuntime
	fields  []v2ResponseFieldProgram
}

type v2ResponseFieldProgram struct {
	runtime     *v2FieldRuntime
	name        string // GraphQL alias-or-path — emitted as the JSON key
	staticValue string
	enumName    string
	repeated    bool
	child       *v2ResponseProgram
	scalarType  DataType
}

// v2NativeResponse carries the output of a single fetch's Invoke so the
// post-invoke merge phase can attach it into the response frame.
type v2NativeResponse struct {
	kind         CallKind
	responsePath ast.Path
	output       protoref.Message
	skip         bool
}

// v2ContextProgram builds a resolve-kind request's "context" list from the
// output of the prior fetch. Each field pulls a value via a compiled path
// walker.
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
