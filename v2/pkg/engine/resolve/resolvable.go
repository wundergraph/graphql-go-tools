package resolve

import (
	"bytes"
	"context"
	goerrors "errors"
	"fmt"
	"io"

	"github.com/goccy/go-json"

	"github.com/cespare/xxhash/v2"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/valyala/fastjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

type Resolvable struct {
	data                 *fastjson.Value
	errors               *fastjson.Value
	variables            *fastjson.Value
	skipAddingNullErrors bool

	parsers []*fastjson.Parser

	print              bool
	out                io.Writer
	printErr           error
	path               []fastjsonext.PathElement
	depth              int
	operationType      ast.OperationType
	renameTypeNames    []RenameTypeName
	ctx                *Context
	authorizationError error
	xxh                *xxhash.Digest
	authorizationAllow map[uint64]struct{}
	authorizationDeny  map[uint64]string

	wroteErrors bool
	wroteData   bool

	typeNames [][]byte

	marshalBuf []byte
}

func NewResolvable() *Resolvable {
	return &Resolvable{
		xxh:                xxhash.New(),
		authorizationAllow: make(map[uint64]struct{}),
		authorizationDeny:  make(map[uint64]string),
	}
}

var (
	parsers = &fastjson.ParserPool{}
)

func (r *Resolvable) parseJSON(data []byte) (*fastjson.Value, error) {
	parser := parsers.Get()
	r.parsers = append(r.parsers, parser)
	return parser.ParseBytes(data)
}

func (r *Resolvable) Reset() {
	for i := range r.parsers {
		parsers.Put(r.parsers[i])
		r.parsers[i] = nil
	}
	r.parsers = r.parsers[:0]
	r.typeNames = r.typeNames[:0]
	r.wroteErrors = false
	r.wroteData = false
	r.data = nil
	r.errors = nil
	r.variables = nil
	r.depth = 0
	r.print = false
	r.out = nil
	r.printErr = nil
	r.path = r.path[:0]
	r.operationType = ast.OperationTypeUnknown
	r.renameTypeNames = r.renameTypeNames[:0]
	r.authorizationError = nil
	r.xxh.Reset()
	for k := range r.authorizationAllow {
		delete(r.authorizationAllow, k)
	}
	for k := range r.authorizationDeny {
		delete(r.authorizationDeny, k)
	}
}

func (r *Resolvable) Init(ctx *Context, initialData []byte, operationType ast.OperationType) (err error) {
	r.ctx = ctx
	r.operationType = operationType
	r.renameTypeNames = ctx.RenameTypeNames
	r.data = fastjson.MustParse(`{}`)
	r.errors = fastjson.MustParse(`[]`)
	if len(ctx.Variables) != 0 {
		r.variables = fastjson.MustParseBytes(ctx.Variables)
	}
	if initialData != nil {
		initialValue := fastjson.MustParseBytes(initialData)
		r.data, _ = fastjsonext.MergeValues(r.data, initialValue)
	}
	return
}

func (r *Resolvable) InitSubscription(ctx *Context, initialData []byte, postProcessing PostProcessingConfiguration) (err error) {
	r.ctx = ctx
	r.operationType = ast.OperationTypeSubscription
	r.renameTypeNames = ctx.RenameTypeNames
	if len(ctx.Variables) != 0 {
		r.variables = fastjson.MustParseBytes(ctx.Variables)
	}
	if initialData != nil {
		initialValue, err := fastjson.ParseBytes(initialData)
		if err != nil {
			return err
		}
		if postProcessing.SelectResponseDataPath == nil {
			r.data, _ = fastjsonext.MergeValuesWithPath(r.data, initialValue, postProcessing.MergePath...)
		} else {
			selectedInitialValue := initialValue.Get(postProcessing.SelectResponseDataPath...)
			if selectedInitialValue != nil {
				r.data, _ = fastjsonext.MergeValuesWithPath(r.data, selectedInitialValue, postProcessing.MergePath...)
			}
		}
		if postProcessing.SelectResponseErrorsPath != nil {
			selectedInitialErrors := initialValue.Get(postProcessing.SelectResponseErrorsPath...)
			if selectedInitialErrors != nil {
				r.errors = selectedInitialErrors
			}
		}
	}
	if r.data == nil {
		r.data = fastjson.MustParse(`{}`)
	}
	if r.errors == nil {
		r.errors = fastjson.MustParse(`[]`)
	}
	return
}

func (r *Resolvable) Resolve(ctx context.Context, rootData *Object, fetchTree *FetchTreeNode, out io.Writer) error {
	r.out = out
	r.print = false
	r.printErr = nil
	r.authorizationError = nil

	if r.ctx.ExecutionOptions.SkipLoader {
		// we didn't resolve any data, so there's no point in generating errors
		// the goal is to only render extensions, e.g. to expose the query plan
		r.printBytes(lBrace)
		r.printBytes(quote)
		r.printBytes(literalData)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printBytes(null)
		if r.hasExtensions() {
			r.printBytes(comma)
			r.printErr = r.printExtensions(ctx, fetchTree)
		}
		r.printBytes(rBrace)
		return r.printErr
	}

	r.skipAddingNullErrors = r.hasErrors() && !r.hasData()

	hasErrors := r.walkObject(rootData, r.data)
	if r.authorizationError != nil {
		return r.authorizationError
	}
	r.printBytes(lBrace)
	if r.hasErrors() {
		r.printErrors()
	}

	if hasErrors {
		r.printBytes(quote)
		r.printBytes(literalData)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printBytes(null)
	} else {
		r.printData(rootData)
	}
	if r.hasExtensions() {
		r.printBytes(comma)
		r.printErr = r.printExtensions(ctx, fetchTree)
	}
	r.printBytes(rBrace)
	return r.printErr
}

func (r *Resolvable) err() bool {
	return true
}

func (r *Resolvable) printErrors() {
	r.printBytes(quote)
	r.printBytes(literalErrors)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printNode(r.errors)
	r.printBytes(comma)
	r.wroteErrors = true
}

func (r *Resolvable) printData(root *Object) {
	r.printBytes(quote)
	r.printBytes(literalData)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(lBrace)
	r.print = true
	_ = r.walkObject(root, r.data)
	r.print = false
	r.printBytes(rBrace)
	r.wroteData = true
}

func (r *Resolvable) printExtensions(ctx context.Context, fetchTree *FetchTreeNode) error {
	r.printBytes(quote)
	r.printBytes(literalExtensions)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(lBrace)

	var (
		writeComma bool
	)

	if r.ctx.authorizer != nil && r.ctx.authorizer.HasResponseExtensionData(r.ctx) {
		writeComma = true
		err := r.printAuthorizerExtension()
		if err != nil {
			return err
		}
	}

	if r.ctx.RateLimitOptions.Enable && r.ctx.RateLimitOptions.IncludeStatsInResponseExtension && r.ctx.rateLimiter != nil {
		if writeComma {
			r.printBytes(comma)
		}
		writeComma = true
		err := r.printRateLimitingExtension()
		if err != nil {
			return err
		}
	}

	if r.ctx.ExecutionOptions.IncludeQueryPlanInResponse {
		if writeComma {
			r.printBytes(comma)
		}
		writeComma = true
		err := r.printQueryPlanExtension(fetchTree)
		if err != nil {
			return err
		}
	}

	if r.ctx.TracingOptions.Enable && r.ctx.TracingOptions.IncludeTraceOutputInResponseExtensions {
		if writeComma {
			r.printBytes(comma)
		}
		writeComma = true
		err := r.printTraceExtension(ctx, fetchTree)
		if err != nil {
			return err
		}
	}

	r.printBytes(rBrace)
	return nil
}

func (r *Resolvable) printAuthorizerExtension() error {
	r.printBytes(quote)
	r.printBytes(literalAuthorization)
	r.printBytes(quote)
	r.printBytes(colon)
	return r.ctx.authorizer.RenderResponseExtension(r.ctx, r.out)
}

func (r *Resolvable) printRateLimitingExtension() error {
	r.printBytes(quote)
	r.printBytes(literalRateLimit)
	r.printBytes(quote)
	r.printBytes(colon)
	return r.ctx.rateLimiter.RenderResponseExtension(r.ctx, r.out)
}

func (r *Resolvable) printTraceExtension(ctx context.Context, fetchTree *FetchTreeNode) error {
	trace := GetTrace(ctx, fetchTree)
	content, err := json.Marshal(trace)
	if err != nil {
		return err
	}
	r.printBytes(quote)
	r.printBytes(literalTrace)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(content)
	return nil
}

func (r *Resolvable) printQueryPlanExtension(fetchTree *FetchTreeNode) error {
	queryPlan := fetchTree.QueryPlan()
	content, err := json.Marshal(queryPlan)
	if err != nil {
		return err
	}
	r.printBytes(quote)
	r.printBytes(literalQueryPlan)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(content)
	return nil
}

func (r *Resolvable) hasExtensions() bool {
	if r.ctx.authorizer != nil && r.ctx.authorizer.HasResponseExtensionData(r.ctx) {
		return true
	}
	if r.ctx.RateLimitOptions.Enable && r.ctx.RateLimitOptions.IncludeStatsInResponseExtension && r.ctx.rateLimiter != nil {
		return true
	}
	if r.ctx.TracingOptions.Enable && r.ctx.TracingOptions.IncludeTraceOutputInResponseExtensions {
		return true
	}
	if r.ctx.ExecutionOptions.IncludeQueryPlanInResponse {
		return true
	}
	return false
}

func (r *Resolvable) WroteErrorsWithoutData() bool {
	return r.wroteErrors && !r.wroteData
}

func (r *Resolvable) hasErrors() bool {
	if r.errors == nil {
		return false
	}
	values, err := r.errors.Array()
	if err != nil {
		return false
	}
	return len(values) > 0
}

func (r *Resolvable) hasData() bool {
	if r.data == nil {
		return false
	}
	obj, err := r.data.Object()
	if err != nil {
		return false
	}
	return obj.Len() > 0
}

func (r *Resolvable) printBytes(b []byte) {
	if r.printErr != nil {
		return
	}
	_, r.printErr = r.out.Write(b)
}

func (r *Resolvable) printNode(value *fastjson.Value) {
	if r.printErr != nil {
		return
	}
	r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
	_, r.printErr = r.out.Write(r.marshalBuf)
}

func (r *Resolvable) pushArrayPathElement(index int) {
	r.path = append(r.path, fastjsonext.PathElement{
		Idx: index,
	})
}

func (r *Resolvable) popArrayPathElement() {
	r.path = r.path[:len(r.path)-1]
}

func (r *Resolvable) pushNodePathElement(path []string) {
	r.depth++
	for i := range path {
		r.path = append(r.path, fastjsonext.PathElement{
			Name: path[i],
		})
	}
}

func (r *Resolvable) popNodePathElement(path []string) {
	r.path = r.path[:len(r.path)-len(path)]
	r.depth--
}

func (r *Resolvable) walkNode(node Node, value *fastjson.Value) bool {
	if r.authorizationError != nil {
		return true
	}
	if r.print {
		r.ctx.Stats.ResolvedNodes++
	}
	switch n := node.(type) {
	case *Object:
		return r.walkObject(n, value)
	case *Array:
		return r.walkArray(n, value)
	case *Null:
		return r.walkNull()
	case *String:
		return r.walkString(n, value)
	case *Boolean:
		return r.walkBoolean(n, value)
	case *Integer:
		return r.walkInteger(n, value)
	case *Float:
		return r.walkFloat(n, value)
	case *BigInt:
		return r.walkBigInt(n, value)
	case *Scalar:
		return r.walkScalar(n, value)
	case *EmptyObject:
		return r.walkEmptyObject(n)
	case *EmptyArray:
		return r.walkEmptyArray(n)
	case *CustomNode:
		return r.walkCustom(n, value)
	default:
		return false
	}
}

func (r *Resolvable) walkObject(obj *Object, parent *fastjson.Value) bool {
	value := parent.Get(obj.Path...)
	if value == nil || value.Type() == fastjson.TypeNull {
		if obj.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(obj.Path, parent)
		return r.err()
	}
	r.pushNodePathElement(obj.Path)
	isRoot := r.depth < 2
	defer r.popNodePathElement(obj.Path)
	if value.Type() != fastjson.TypeObject {
		r.addError("Object cannot represent non-object value.", obj.Path)
		return r.err()
	}
	if r.print && !isRoot {
		r.printBytes(lBrace)
		r.ctx.Stats.ResolvedObjects++
	}
	addComma := false
	typeName := value.GetStringBytes("__typename")
	r.typeNames = append(r.typeNames, typeName)
	defer func() {
		r.typeNames = r.typeNames[:len(r.typeNames)-1]
	}()
	for i := range obj.Fields {
		if obj.Fields[i].SkipDirectiveDefined {
			if r.skipField(obj.Fields[i].SkipVariableName) {
				continue
			}
		}
		if obj.Fields[i].IncludeDirectiveDefined {
			if r.excludeField(obj.Fields[i].IncludeVariableName) {
				continue
			}
		}
		if obj.Fields[i].ParentOnTypeNames != nil {
			if r.skipFieldOnParentTypeNames(obj.Fields[i]) {
				continue
			}
		}
		if obj.Fields[i].OnTypeNames != nil {
			if r.skipFieldOnTypeNames(obj.Fields[i]) {
				continue
			}
		}
		if !r.print {
			skip := r.authorizeField(value, obj.Fields[i])
			if skip {
				if obj.Fields[i].Value.NodeNullable() {
					// if the field value is nullable, we can just set it to null
					// we already set an error in authorizeField
					path := obj.Fields[i].Value.NodePath()
					field := value.Get(path...)
					if field != nil {
						fastjsonext.SetNull(value, path...)
					}
				} else if obj.Nullable && len(obj.Path) > 0 {
					// if the field value is not nullable, but the object is nullable
					// we can just set the whole object to null
					fastjsonext.SetNull(parent, obj.Path...)
					return false
				} else {
					// if the field value is not nullable and the object is not nullable
					// we return true to indicate an error
					return true
				}
				continue
			}
		}
		if r.print {
			if addComma {
				r.printBytes(comma)
			}
			r.printBytes(quote)
			r.printBytes(obj.Fields[i].Name)
			r.printBytes(quote)
			r.printBytes(colon)
		}
		err := r.walkNode(obj.Fields[i].Value, value)
		if err {
			if obj.Nullable {
				if len(obj.Path) > 0 {
					fastjsonext.SetNull(parent, obj.Path...)
					return false
				}
			}
			return err
		}
		addComma = true
	}
	if r.print && !isRoot {
		r.printBytes(rBrace)
	}
	return false
}

func (r *Resolvable) authorizeField(value *fastjson.Value, field *Field) (skipField bool) {
	if field.Info == nil {
		return false
	}
	if !field.Info.HasAuthorizationRule {
		return false
	}
	if r.ctx.authorizer == nil {
		return false
	}
	if len(field.Info.Source.IDs) == 0 {
		return false
	}
	dataSourceID := field.Info.Source.IDs[0]
	typeName := r.objectFieldTypeName(value, field)
	fieldName := unsafebytes.BytesToString(field.Name)
	gc := GraphCoordinate{
		TypeName:  typeName,
		FieldName: fieldName,
	}
	result, authErr := r.authorize(value, dataSourceID, gc)
	if authErr != nil {
		r.authorizationError = authErr
		return true
	}
	if result != nil {
		r.addRejectFieldError(result.Reason, dataSourceID, field)
		return true
	}
	return false
}

func (r *Resolvable) authorize(value *fastjson.Value, dataSourceID string, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
	r.xxh.Reset()
	_, _ = r.xxh.WriteString(dataSourceID)
	_, _ = r.xxh.WriteString(coordinate.TypeName)
	_, _ = r.xxh.WriteString(coordinate.FieldName)
	decisionID := r.xxh.Sum64()
	if _, ok := r.authorizationAllow[decisionID]; ok {
		return nil, nil
	}
	if reason, ok := r.authorizationDeny[decisionID]; ok {
		return &AuthorizationDeny{Reason: reason}, nil
	}
	r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
	result, err = r.ctx.authorizer.AuthorizeObjectField(r.ctx, dataSourceID, r.marshalBuf, coordinate)
	if err != nil {
		return nil, err
	}
	if result == nil {
		r.authorizationAllow[decisionID] = struct{}{}
	} else {
		r.authorizationDeny[decisionID] = result.Reason
	}
	return result, nil
}

func (r *Resolvable) addRejectFieldError(reason, dataSourceID string, field *Field) {
	nodePath := field.Value.NodePath()
	r.pushNodePathElement(nodePath)
	fieldPath := r.renderFieldPath()

	var errorMessage string
	if reason == "" {
		errorMessage = fmt.Sprintf("Unauthorized to load field '%s'.", fieldPath)
	} else {
		errorMessage = fmt.Sprintf("Unauthorized to load field '%s', Reason: %s.", fieldPath, reason)
	}
	r.ctx.appendSubgraphError(goerrors.Join(errors.New(errorMessage), NewSubgraphError(dataSourceID, fieldPath, reason, 0)))
	fastjsonext.AppendErrorToArray(r.errors, errorMessage, r.path)
	r.popNodePathElement(nodePath)
}

func (r *Resolvable) objectFieldTypeName(v *fastjson.Value, field *Field) string {
	typeName := v.GetStringBytes("__typename")
	if typeName != nil {
		return unsafebytes.BytesToString(typeName)
	}
	return field.Info.ExactParentTypeName
}

func (r *Resolvable) skipFieldOnParentTypeNames(field *Field) bool {
WithNext:
	for i := range field.ParentOnTypeNames {
		typeName := r.typeNames[len(r.typeNames)-1-field.ParentOnTypeNames[i].Depth]
		if typeName == nil {
			// The field has a condition but the JSON response object does not have a __typename field
			// We skip this field
			return true
		}
		for j := range field.ParentOnTypeNames[i].Names {
			if bytes.Equal(typeName, field.ParentOnTypeNames[i].Names[j]) {
				// on each layer of depth, we only need to match one of the names
				// merge_fields.go ensures that we only have on ParentOnTypeNames per depth layer
				// If we have a match, we continue WithNext condition until all layers have been checked
				continue WithNext
			}
		}
		// No match at this depth layer, we skip this field
		return true
	}
	// all layers have at least one matching typeName
	// we don't skip this field (we return false)
	return false
}

func (r *Resolvable) skipFieldOnTypeNames(field *Field) bool {
	typeName := r.typeNames[len(r.typeNames)-1]
	if typeName == nil {
		return true
	}
	for i := range field.OnTypeNames {
		if bytes.Equal(typeName, field.OnTypeNames[i]) {
			return false
		}
	}
	return true
}

func (r *Resolvable) skipField(skipVariableName string) bool {
	variable := r.variables.Get(skipVariableName)
	if variable == nil {
		return false
	}
	return variable.Type() == fastjson.TypeTrue
}

func (r *Resolvable) excludeField(includeVariableName string) bool {
	variable := r.variables.Get(includeVariableName)
	if variable == nil {
		return true
	}
	return variable.Type() == fastjson.TypeFalse
}

func (r *Resolvable) walkArray(arr *Array, value *fastjson.Value) bool {
	parent := value
	value = value.Get(arr.Path...)
	if fastjsonext.ValueIsNull(value) {
		if arr.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(arr.Path, parent)
		return r.err()
	}
	r.pushNodePathElement(arr.Path)
	defer r.popNodePathElement(arr.Path)
	if value.Type() != fastjson.TypeArray {
		r.addError("Array cannot represent non-array value.", arr.Path)
		return r.err()
	}
	if r.print {
		r.printBytes(lBrack)
	}
	values := value.GetArray()
	for i, arrayValue := range values {
		if r.print && i != 0 {
			r.printBytes(comma)
		}
		r.pushArrayPathElement(i)
		err := r.walkNode(arr.Item, arrayValue)
		r.popArrayPathElement()
		if err {
			if arr.Item.NodeKind() == NodeKindObject && arr.Item.NodeNullable() {
				value.SetArrayItem(i, fastjsonext.NullValue)
				continue
			}
			if arr.Nullable {
				fastjsonext.SetNull(parent, arr.Path...)
				return false
			}
			return err
		}
	}
	if r.print {
		r.printBytes(rBrack)
	}
	return false
}

func (r *Resolvable) walkNull() bool {
	if r.print {
		r.printBytes(null)
		r.ctx.Stats.ResolvedLeafs++
	}
	return false
}

func (r *Resolvable) walkString(s *String, value *fastjson.Value) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	parent := value
	value = value.Get(s.Path...)
	if fastjsonext.ValueIsNull(value) {
		if s.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(s.Path, parent)
		return r.err()
	}
	if value.Type() != fastjson.TypeString {
		r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
		r.addError(fmt.Sprintf("String cannot represent non-string value: \\\"%s\\\"", string(r.marshalBuf)), s.Path)
		return r.err()
	}
	if r.print {
		if s.IsTypeName {
			content := value.GetStringBytes()
			for i := range r.renameTypeNames {
				if bytes.Equal(content, r.renameTypeNames[i].From) {
					r.printBytes(quote)
					r.printBytes(r.renameTypeNames[i].To)
					r.printBytes(quote)
					return false
				}
			}
			r.printNode(value)
			return false
		}
		if s.UnescapeResponseJson {
			content := value.GetStringBytes()
			content = bytes.ReplaceAll(content, []byte(`\"`), []byte(`"`))
			if !gjson.ValidBytes(content) {
				r.printBytes(quote)
				r.printBytes(content)
				r.printBytes(quote)
			} else {
				r.printBytes(content)
			}
		} else {
			r.printNode(value)
		}
	}
	return false
}

func (r *Resolvable) walkBoolean(b *Boolean, value *fastjson.Value) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	parent := value
	value = value.Get(b.Path...)
	if fastjsonext.ValueIsNull(value) {
		if b.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(b.Path, parent)
		return r.err()
	}
	if value.Type() != fastjson.TypeTrue && value.Type() != fastjson.TypeFalse {
		r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
		r.addError(fmt.Sprintf("Bool cannot represent non-boolean value: \\\"%s\\\"", string(r.marshalBuf)), b.Path)
		return r.err()
	}
	if r.print {
		r.printNode(value)
	}
	return false
}

func (r *Resolvable) walkInteger(i *Integer, value *fastjson.Value) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	parent := value
	value = value.Get(i.Path...)
	if fastjsonext.ValueIsNull(value) {
		if i.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(i.Path, parent)
		return r.err()
	}
	if value.Type() != fastjson.TypeNumber {
		r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
		r.addError(fmt.Sprintf("Int cannot represent non-integer value: \\\"%s\\\"", string(r.marshalBuf)), i.Path)
		return r.err()
	}
	if r.print {
		r.printNode(value)
	}
	return false
}

func (r *Resolvable) walkFloat(f *Float, value *fastjson.Value) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	parent := value
	value = value.Get(f.Path...)
	if fastjsonext.ValueIsNull(value) {
		if f.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(f.Path, parent)
		return r.err()
	}
	if value.Type() != fastjson.TypeNumber {
		r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
		r.addError(fmt.Sprintf("Float cannot represent non-float value: \\\"%s\\\"", string(r.marshalBuf)), f.Path)
		return r.err()
	}
	if r.print {
		r.printNode(value)
	}
	return false
}

func (r *Resolvable) walkBigInt(b *BigInt, value *fastjson.Value) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	parent := value
	value = value.Get(b.Path...)
	if fastjsonext.ValueIsNull(value) {
		if b.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(b.Path, parent)
		return r.err()
	}
	if r.print {
		r.printNode(value)
	}
	return false
}

func (r *Resolvable) walkScalar(s *Scalar, value *fastjson.Value) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	parent := value
	value = value.Get(s.Path...)
	if fastjsonext.ValueIsNull(value) {
		if s.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(s.Path, parent)
		return r.err()
	}
	if r.print {
		r.printNode(value)
	}
	return false
}

func (r *Resolvable) walkEmptyObject(_ *EmptyObject) bool {
	if r.print {
		r.printBytes(lBrace)
		r.printBytes(rBrace)
	}
	return false
}

func (r *Resolvable) walkEmptyArray(_ *EmptyArray) bool {
	if r.print {
		r.printBytes(lBrack)
		r.printBytes(rBrack)
	}
	return false
}

func (r *Resolvable) walkCustom(c *CustomNode, value *fastjson.Value) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	parent := value
	value = value.Get(c.Path...)
	if fastjsonext.ValueIsNull(value) {
		if c.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(c.Path, parent)
		return r.err()
	}
	r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
	resolved, err := c.Resolve(r.ctx, r.marshalBuf)
	if err != nil {
		r.addError(err.Error(), c.Path)
		return r.err()
	}
	if r.print {
		r.printBytes(resolved)
	}
	return false
}

func (r *Resolvable) addNonNullableFieldError(fieldPath []string, parent *fastjson.Value) {
	if r.skipAddingNullErrors {
		return
	}
	if fieldPath != nil {
		if ancestor := parent.Get(fieldPath[:len(fieldPath)-1]...); ancestor != nil {
			if ancestor.Exists("__skipErrors") {
				return
			}
		}
	}
	r.pushNodePathElement(fieldPath)
	errorMessage := fmt.Sprintf("Cannot return null for non-nullable field '%s'.", r.renderFieldPath())
	fastjsonext.AppendErrorToArray(r.errors, errorMessage, r.path)
	r.popNodePathElement(fieldPath)
}

func (r *Resolvable) renderFieldPath() string {
	buf := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(buf)
	switch r.operationType {
	case ast.OperationTypeQuery:
		_, _ = buf.WriteString("Query")
	case ast.OperationTypeMutation:
		_, _ = buf.WriteString("Mutation")
	case ast.OperationTypeSubscription:
		_, _ = buf.WriteString("Subscription")
	}
	for i := range r.path {
		if r.path[i].Name != "" {
			_, _ = buf.WriteString(".")
			_, _ = buf.WriteString(r.path[i].Name)
		}
	}
	return buf.String()
}

func (r *Resolvable) addError(message string, fieldPath []string) {
	r.pushNodePathElement(fieldPath)
	fastjsonext.AppendErrorToArray(r.errors, message, r.path)
	r.popNodePathElement(fieldPath)
}
