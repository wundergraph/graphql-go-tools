package resolve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/cespare/xxhash/v2"
	"github.com/tidwall/gjson"

	"github.com/TykTechnologies/graphql-go-tools/v2/internal/pkg/unsafebytes"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astjson"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/pool"
)

type Resolvable struct {
	storage             *astjson.JSON
	dataRoot            int
	errorsRoot          int
	variablesRoot       int
	print               bool
	out                 io.Writer
	printErr            error
	path                []astjson.PathElement
	depth               int
	operationType       ast.OperationType
	renameTypeNames     []RenameTypeName
	requestTraceOptions RequestTraceOptions
	ctx                 *Context
	authorizationError  error
	xxh                 *xxhash.Digest
	authorizationAllow  map[uint64]struct{}
	authorizationDeny   map[uint64]string

	authorizationBuf          *bytes.Buffer
	authorizationBufObjectRef int
}

func NewResolvable() *Resolvable {
	return &Resolvable{
		storage:            &astjson.JSON{},
		xxh:                xxhash.New(),
		authorizationAllow: make(map[uint64]struct{}),
		authorizationDeny:  make(map[uint64]string),
	}
}

func (r *Resolvable) Reset() {
	r.storage.Reset()
	r.dataRoot = -1
	r.errorsRoot = -1
	r.variablesRoot = -1
	r.depth = 0
	r.print = false
	r.out = nil
	r.printErr = nil
	r.path = r.path[:0]
	r.operationType = ast.OperationTypeUnknown
	r.renameTypeNames = r.renameTypeNames[:0]
	r.authorizationError = nil
	r.xxh.Reset()
	r.authorizationBufObjectRef = -1
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
	r.requestTraceOptions = ctx.RequestTracingOptions
	r.dataRoot, r.errorsRoot, err = r.storage.InitResolvable(initialData)
	if err != nil {
		return
	}
	if len(ctx.Variables) != 0 {
		r.variablesRoot, err = r.storage.AppendAnyJSONBytes(ctx.Variables)
	}
	return
}

func (r *Resolvable) InitSubscription(ctx *Context, initialData []byte, postProcessing PostProcessingConfiguration) (err error) {
	r.ctx = ctx
	r.operationType = ast.OperationTypeSubscription
	r.renameTypeNames = ctx.RenameTypeNames
	if len(ctx.Variables) != 0 {
		r.variablesRoot, err = r.storage.AppendObject(ctx.Variables)
		if err != nil {
			return
		}
	}
	r.dataRoot, r.errorsRoot, err = r.storage.InitResolvable(nil)
	if err != nil {
		return
	}
	raw, err := r.storage.AppendObject(initialData)
	if err != nil {
		return err
	}
	data := r.storage.Get(raw, postProcessing.SelectResponseDataPath)
	if r.storage.NodeIsDefined(data) {
		r.storage.MergeNodesWithPath(r.dataRoot, data, postProcessing.MergePath)
	}
	errors := r.storage.Get(raw, postProcessing.SelectResponseErrorsPath)
	if r.storage.NodeIsDefined(errors) {
		r.storage.MergeArrays(r.errorsRoot, errors)
	}
	return
}

func (r *Resolvable) Resolve(ctx context.Context, root *Object, out io.Writer) error {
	r.out = out
	r.print = false
	r.printErr = nil
	r.authorizationError = nil

	// if we have errors and no data, we only print the errors and set data to null
	// in this case, we're skipping the walk because it would lead to unnecessary non-null errors
	if r.hasErrors() && !r.hasData() {
		r.printBytes(lBrace)
		r.printErrors()
		r.printBytes(quote)
		r.printBytes(literalData)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printBytes(null)
		r.printBytes(rBrace)
		return nil
	}

	err := r.walkObject(root, r.dataRoot)
	if r.authorizationError != nil {
		return r.authorizationError
	}
	r.printBytes(lBrace)
	if r.hasErrors() {
		r.printErrors()
	}

	if err {
		r.printBytes(quote)
		r.printBytes(literalData)
		r.printBytes(quote)
		r.printBytes(colon)
		r.printBytes(null)
	} else {
		r.printData(root)
		if r.hasExtensions() {
			r.printBytes(comma)
			r.printExtensions(ctx, root)
		}
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
	r.printNode(r.errorsRoot)
	r.printBytes(comma)
}

func (r *Resolvable) printData(root *Object) {
	r.printBytes(quote)
	r.printBytes(literalData)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(lBrace)
	r.print = true
	_ = r.walkObject(root, r.dataRoot)
	r.print = false
	r.printBytes(rBrace)
}

func (r *Resolvable) printExtensions(ctx context.Context, root *Object) {
	r.printBytes(quote)
	r.printBytes(literalExtensions)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(lBrace)

	if r.requestTraceOptions.IncludeTraceOutputInResponseExtensions {
		r.printTrace(ctx, root)
	}

	r.printBytes(rBrace)
}

func (r *Resolvable) printTrace(ctx context.Context, root *Object) {
	trace := GetTrace(ctx, root)

	traceData, err := json.Marshal(trace)
	if err != nil {
		return
	}

	r.printBytes(quote)
	r.printBytes(literalTrace)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(traceData)
}

func (r *Resolvable) hasExtensions() bool {
	return r.requestTraceOptions.IncludeTraceOutputInResponseExtensions
}

func (r *Resolvable) hasErrors() bool {
	return r.storage.NodeIsDefined(r.errorsRoot) &&
		len(r.storage.Nodes[r.errorsRoot].ArrayValues) > 0
}

func (r *Resolvable) hasData() bool {
	if !r.storage.NodeIsDefined(r.dataRoot) {
		return false
	}
	if r.storage.Nodes[r.dataRoot].Kind != astjson.NodeKindObject {
		return false
	}
	return len(r.storage.Nodes[r.dataRoot].ObjectFields) > 0
}

func (r *Resolvable) printBytes(b []byte) {
	if r.printErr != nil {
		return
	}
	_, r.printErr = r.out.Write(b)
}

func (r *Resolvable) printNode(ref int) {
	if r.printErr != nil {
		return
	}
	r.printErr = r.storage.PrintNode(r.storage.Nodes[ref], r.out)
}

func (r *Resolvable) pushArrayPathElement(index int) {
	r.path = append(r.path, astjson.PathElement{
		ArrayIndex: index,
	})
}

func (r *Resolvable) popArrayPathElement() {
	r.path = r.path[:len(r.path)-1]
}

func (r *Resolvable) pushNodePathElement(path []string) {
	r.depth++
	for i := range path {
		r.path = append(r.path, astjson.PathElement{
			Name: path[i],
		})
	}
}

func (r *Resolvable) popNodePathElement(path []string) {
	r.path = r.path[:len(r.path)-len(path)]
	r.depth--
}

func (r *Resolvable) walkNode(node Node, ref int) bool {
	if r.authorizationError != nil {
		return true
	}
	if r.print {
		r.ctx.Stats.ResolvedNodes++
	}
	switch n := node.(type) {
	case *Object:
		return r.walkObject(n, ref)
	case *Array:
		return r.walkArray(n, ref)
	case *Null:
		return r.walkNull()
	case *String:
		return r.walkString(n, ref)
	case *Boolean:
		return r.walkBoolean(n, ref)
	case *Integer:
		return r.walkInteger(n, ref)
	case *Float:
		return r.walkFloat(n, ref)
	case *BigInt:
		return r.walkBigInt(n, ref)
	case *Scalar:
		return r.walkScalar(n, ref)
	case *EmptyObject:
		return r.walkEmptyObject(n)
	case *EmptyArray:
		return r.walkEmptyArray(n)
	case *CustomNode:
		return r.walkCustom(n, ref)
	default:
		return false
	}
}

func (r *Resolvable) walkObject(obj *Object, ref int) bool {
	ref = r.storage.Get(ref, obj.Path)
	if !r.storage.NodeIsDefined(ref) {
		if obj.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(ref, obj.Path)
		return r.err()
	}
	r.pushNodePathElement(obj.Path)
	isRoot := r.depth < 2
	defer r.popNodePathElement(obj.Path)

	if r.storage.Nodes[ref].Kind == astjson.NodeKindNull {
		return r.walkNull()
	}
	if r.storage.Nodes[ref].Kind != astjson.NodeKindObject {
		r.addError("Object cannot represent non-object value.", obj.Path)
		return r.err()
	}
	if r.print && !isRoot {
		r.printBytes(lBrace)
		r.ctx.Stats.ResolvedObjects++
	}
	addComma := false
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
		if obj.Fields[i].OnTypeNames != nil {
			if r.skipFieldOnTypeNames(ref, obj.Fields[i]) {
				continue
			}
		}
		if !r.print {
			skip := r.authorizeField(ref, obj.Fields[i])
			if skip {
				if obj.Fields[i].Value.NodeNullable() {
					// if the field value is nullable, we can just set it to null
					// we already set an error in authorizeField
					field := r.storage.Get(ref, obj.Fields[i].Value.NodePath())
					if r.storage.NodeIsDefined(field) {
						r.storage.Nodes[field].Kind = astjson.NodeKindNull
					}
				} else if obj.Nullable {
					// if the field value is not nullable, but the object is nullable
					// we can just set the whole object to null
					r.storage.Nodes[ref].Kind = astjson.NodeKindNull
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
		err := r.walkNode(obj.Fields[i].Value, ref)
		if err {
			if obj.Nullable {
				r.storage.Nodes[ref].Kind = astjson.NodeKindNull
				return false
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

func (r *Resolvable) authorizeField(ref int, field *Field) (skipField bool) {
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
	typeName := r.objectFieldTypeName(ref, field)
	fieldName := unsafebytes.BytesToString(field.Name)
	gc := GraphCoordinate{
		TypeName:  typeName,
		FieldName: fieldName,
	}
	result, authErr := r.authorize(ref, dataSourceID, gc)
	if authErr != nil {
		r.authorizationError = authErr
		return true
	}
	if result != nil {
		r.addRejectFieldError(result.Reason, field)
		return true
	}
	return false
}

func (r *Resolvable) authorize(objectRef int, dataSourceID string, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
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
	if r.authorizationBufObjectRef != objectRef {
		if r.authorizationBuf == nil {
			r.authorizationBuf = bytes.NewBuffer(nil)
		}
		r.authorizationBuf.Reset()
		err = r.storage.PrintObjectFlat(objectRef, r.authorizationBuf)
		if err != nil {
			return nil, err
		}
		r.authorizationBufObjectRef = objectRef
	}
	result, err = r.ctx.authorizer.AuthorizeObjectField(r.ctx, dataSourceID, r.authorizationBuf.Bytes(), coordinate)
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

func (r *Resolvable) addRejectFieldError(reason string, field *Field) {
	var (
		message string
	)
	nodePath := field.Value.NodePath()
	r.pushNodePathElement(nodePath)
	fieldPath := r.renderFieldPath()
	if reason == "" {
		message = fmt.Sprintf("Unauthorized to load field '%s'.", fieldPath)
	} else {
		message = fmt.Sprintf("Unauthorized to load field '%s'. Reason: %s", fieldPath, reason)
	}
	ref := r.storage.AppendErrorWithMessage(message, r.path)
	r.storage.Nodes[r.errorsRoot].ArrayValues = append(r.storage.Nodes[r.errorsRoot].ArrayValues, ref)
	r.popNodePathElement(nodePath)
}

func (r *Resolvable) objectFieldTypeName(ref int, field *Field) string {
	typeName := r.storage.GetObjectField(ref, "__typename")
	if r.storage.NodeIsDefined(typeName) && r.storage.Nodes[typeName].Kind == astjson.NodeKindString {
		name := r.storage.Nodes[typeName].ValueBytes(r.storage)
		return unsafebytes.BytesToString(name)
	}
	return field.Info.ExactParentTypeName
}

func (r *Resolvable) skipFieldOnTypeNames(ref int, field *Field) bool {
	typeName := r.storage.GetObjectField(ref, "__typename")
	if !r.storage.NodeIsDefined(typeName) {
		return true
	}
	if r.storage.Nodes[typeName].Kind != astjson.NodeKindString {
		return true
	}
	value := r.storage.Nodes[typeName].ValueBytes(r.storage)
	for i := range field.OnTypeNames {
		if bytes.Equal(value, field.OnTypeNames[i]) {
			return false
		}
	}
	return true
}

func (r *Resolvable) skipField(skipVariableName string) bool {
	field := r.storage.GetObjectField(r.variablesRoot, skipVariableName)
	if !r.storage.NodeIsDefined(field) {
		return false
	}
	if r.storage.Nodes[field].Kind != astjson.NodeKindBoolean {
		return false
	}
	value := r.storage.Nodes[field].ValueBytes(r.storage)
	return bytes.Equal(value, literalTrue)
}

func (r *Resolvable) excludeField(includeVariableName string) bool {
	field := r.storage.GetObjectField(r.variablesRoot, includeVariableName)
	if !r.storage.NodeIsDefined(field) {
		return true
	}
	if r.storage.Nodes[field].Kind != astjson.NodeKindBoolean {
		return true
	}
	value := r.storage.Nodes[field].ValueBytes(r.storage)
	return bytes.Equal(value, literalFalse)
}

func (r *Resolvable) walkArray(arr *Array, ref int) bool {
	r.pushNodePathElement(arr.Path)
	defer r.popNodePathElement(arr.Path)
	ref = r.storage.Get(ref, arr.Path)
	if !r.storage.NodeIsDefined(ref) {
		if arr.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(ref, arr.Path)
		return r.err()
	}
	if r.storage.Nodes[ref].Kind != astjson.NodeKindArray {
		r.addError("Array cannot represent non-array value.", arr.Path)
		return r.err()
	}
	if r.print {
		r.printBytes(lBrack)
	}
	for i, value := range r.storage.Nodes[ref].ArrayValues {
		if r.print && i != 0 {
			r.printBytes(comma)
		}
		r.pushArrayPathElement(i)
		err := r.walkNode(arr.Item, value)
		r.popArrayPathElement()
		if err {
			if arr.Nullable {
				r.storage.Nodes[ref].Kind = astjson.NodeKindNull
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

func (r *Resolvable) walkString(s *String, ref int) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	ref = r.storage.Get(ref, s.Path)
	if !r.storage.NodeIsDefined(ref) {
		if s.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(ref, s.Path)
		return r.err()
	}
	if r.storage.Nodes[ref].Kind != astjson.NodeKindString {
		value := string(r.storage.Nodes[ref].ValueBytes(r.storage))
		r.addError(fmt.Sprintf("String cannot represent non-string value: \\\"%s\\\"", value), s.Path)
		return r.err()
	}
	if r.print {
		if s.IsTypeName {
			value := r.storage.Nodes[ref].ValueBytes(r.storage)
			for i := range r.renameTypeNames {
				if bytes.Equal(value, r.renameTypeNames[i].From) {
					r.printBytes(quote)
					r.printBytes(r.renameTypeNames[i].To)
					r.printBytes(quote)
					return false
				}
			}
			r.printNode(ref)
			return false
		}
		if s.UnescapeResponseJson {
			value := r.storage.Nodes[ref].ValueBytes(r.storage)
			value = bytes.ReplaceAll(value, []byte(`\"`), []byte(`"`))
			if !gjson.ValidBytes(value) {
				r.printBytes(quote)
				r.printBytes(value)
				r.printBytes(quote)
			} else {
				r.printBytes(value)
			}
		} else {
			r.printNode(ref)
		}
	}
	return false
}

func (r *Resolvable) walkBoolean(b *Boolean, ref int) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	ref = r.storage.Get(ref, b.Path)
	if !r.storage.NodeIsDefined(ref) {
		if b.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(ref, b.Path)
		return r.err()
	}
	if r.storage.Nodes[ref].Kind != astjson.NodeKindBoolean {
		value := string(r.storage.Nodes[ref].ValueBytes(r.storage))
		r.addError(fmt.Sprintf("Bool cannot represent non-boolean value: \\\"%s\\\"", value), b.Path)
		return r.err()
	}
	if r.print {
		r.printNode(ref)
	}
	return false
}

func (r *Resolvable) walkInteger(i *Integer, ref int) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	ref = r.storage.Get(ref, i.Path)
	if !r.storage.NodeIsDefined(ref) {
		if i.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(ref, i.Path)
		return r.err()
	}
	if r.storage.Nodes[ref].Kind != astjson.NodeKindNumber {
		value := string(r.storage.Nodes[ref].ValueBytes(r.storage))
		r.addError(fmt.Sprintf("Int cannot represent non-integer value: \\\"%s\\\"", value), i.Path)
		return r.err()
	}
	if r.print {
		r.printNode(ref)
	}
	return false
}

func (r *Resolvable) walkFloat(f *Float, ref int) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	ref = r.storage.Get(ref, f.Path)
	if !r.storage.NodeIsDefined(ref) {
		if f.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(ref, f.Path)
		return r.err()
	}
	if r.storage.Nodes[ref].Kind != astjson.NodeKindNumber {
		value := string(r.storage.Nodes[ref].ValueBytes(r.storage))
		r.addError(fmt.Sprintf("Float cannot represent non-float value: \\\"%s\\\"", value), f.Path)
		return r.err()
	}
	if r.print {
		r.printNode(ref)
	}
	return false
}

func (r *Resolvable) walkBigInt(b *BigInt, ref int) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	ref = r.storage.Get(ref, b.Path)
	if !r.storage.NodeIsDefined(ref) {
		if b.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(ref, b.Path)
		return r.err()
	}
	if r.print {
		r.printNode(ref)
	}
	return false
}

func (r *Resolvable) walkScalar(s *Scalar, ref int) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	ref = r.storage.Get(ref, s.Path)
	if !r.storage.NodeIsDefined(ref) {
		if s.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(ref, s.Path)
		return r.err()
	}
	if r.print {
		r.printNode(ref)
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

func (r *Resolvable) walkCustom(c *CustomNode, ref int) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	ref = r.storage.Get(ref, c.Path)
	if !r.storage.NodeIsDefined(ref) {
		if c.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(ref, c.Path)
		return r.err()
	}
	value := r.storage.Nodes[ref].ValueBytes(r.storage)
	resolved, err := c.Resolve(value)
	if err != nil {
		r.addError(err.Error(), c.Path)
		return r.err()
	}
	if r.print {
		r.printBytes(resolved)
	}
	return false
}

func (r *Resolvable) addNonNullableFieldError(fieldRef int, fieldPath []string) {
	if fieldRef != -1 && r.storage.Nodes[fieldRef].Kind == astjson.NodeKindNullSkipError {
		return
	}
	r.pushNodePathElement(fieldPath)
	ref := r.storage.AppendNonNullableFieldIsNullErr(r.renderFieldPath(), r.path)
	r.storage.Nodes[r.errorsRoot].ArrayValues = append(r.storage.Nodes[r.errorsRoot].ArrayValues, ref)
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
	ref := r.storage.AppendErrorWithMessage(message, r.path)
	r.storage.Nodes[r.errorsRoot].ArrayValues = append(r.storage.Nodes[r.errorsRoot].ArrayValues, ref)
	r.popNodePathElement(fieldPath)
}
