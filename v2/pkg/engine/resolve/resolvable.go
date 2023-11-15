package resolve

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/tidwall/gjson"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astjson"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/pool"
)

type Resolvable struct {
	storage         *astjson.JSON
	dataRoot        int
	errorsRoot      int
	variablesRoot   int
	print           bool
	out             io.Writer
	printErr        error
	path            []astjson.PathElement
	depth           int
	operationType   ast.OperationType
	renameTypeNames []RenameTypeName
	enableTracing   bool
}

func NewResolvable() *Resolvable {
	return &Resolvable{
		storage: &astjson.JSON{
			Nodes: make([]astjson.Node, 0, 4096),
		},
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
}

func (r *Resolvable) Init(ctx *Context, initialData []byte, operationType ast.OperationType) (err error) {
	r.operationType = operationType
	r.renameTypeNames = ctx.RenameTypeNames
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
	r.operationType = ast.OperationTypeSubscription
	r.renameTypeNames = ctx.RenameTypeNames
	if len(ctx.Variables) != 0 {
		r.variablesRoot, err = r.storage.AppendObject(ctx.Variables)
	}
	switch {
	case postProcessing.SelectResponseErrorsPath == nil && postProcessing.SelectResponseDataPath == nil:
		r.dataRoot, r.errorsRoot, err = r.storage.InitResolvable(initialData)
		if err != nil {
			return
		}
	case postProcessing.SelectResponseErrorsPath == nil && postProcessing.SelectResponseDataPath != nil:
		r.dataRoot, r.errorsRoot, err = r.storage.InitResolvable(nil)
		if err != nil {
			return
		}
		raw, err := r.storage.AppendObject(initialData)
		if err != nil {
			return err
		}
		data := r.storage.Get(raw, postProcessing.SelectResponseDataPath)
		if !r.storage.NodeIsDefined(data) {
			return nil
		}
		r.storage.MergeNodes(r.dataRoot, data)
	case postProcessing.SelectResponseErrorsPath != nil && postProcessing.SelectResponseDataPath == nil:
		r.dataRoot, r.errorsRoot, err = r.storage.InitResolvable(nil)
		if err != nil {
			return
		}
		raw, err := r.storage.AppendObject(initialData)
		if err != nil {
			return err
		}
		errors := r.storage.Get(raw, postProcessing.SelectResponseErrorsPath)
		if !r.storage.NodeIsDefined(errors) {
			return nil
		}
		r.storage.MergeArrays(r.errorsRoot, errors)
	case postProcessing.SelectResponseErrorsPath != nil && postProcessing.SelectResponseDataPath != nil:
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
			r.storage.MergeNodes(r.dataRoot, data)
		}
		errors := r.storage.Get(raw, postProcessing.SelectResponseErrorsPath)
		if r.storage.NodeIsDefined(errors) {
			r.storage.MergeArrays(r.errorsRoot, errors)
		}
	}
	return
}

func (r *Resolvable) Resolve(root *Object, out io.Writer) error {
	r.out = out
	r.print = false
	r.printErr = nil
	err := r.walkObject(root, r.dataRoot)
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
			r.printExtensions(root)
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

func (r *Resolvable) printExtensions(root *Object) {
	r.printBytes(quote)
	r.printBytes(literalExtensions)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printBytes(lBrace)

	if r.enableTracing {
		r.printTrace(root)
	}

	r.printBytes(rBrace)
}

func (r *Resolvable) printTrace(root *Object) {
	trace := GetTrace(root)

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
	return r.enableTracing
}

func (r *Resolvable) hasErrors() bool {
	if r.errorsRoot == -1 {
		return false
	}
	return len(r.storage.Nodes[r.errorsRoot].ArrayValues) > 0
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
	r.pushNodePathElement(obj.Path)
	isRoot := r.depth < 2
	defer r.popNodePathElement(obj.Path)
	ref = r.storage.Get(ref, obj.Path)
	if !r.storage.NodeIsDefined(ref) {
		if obj.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(obj.Path)
		return r.err()
	}
	if r.storage.Nodes[ref].Kind == astjson.NodeKindNull {
		return r.walkNull()
	}
	if r.storage.Nodes[ref].Kind != astjson.NodeKindObject {
		r.addTypeMismatchError("Object cannot represent non-object value.", obj.Path)
		return r.err()
	}
	if r.print && !isRoot {
		r.printBytes(lBrace)
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
		r.addNonNullableFieldError(arr.Path)
		return r.err()
	}
	if r.storage.Nodes[ref].Kind != astjson.NodeKindArray {
		r.addTypeMismatchError("Array cannot represent non-array value.", arr.Path)
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
	}
	return false
}

func (r *Resolvable) walkString(s *String, ref int) bool {
	ref = r.storage.Get(ref, s.Path)
	if !r.storage.NodeIsDefined(ref) {
		if s.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(s.Path)
		return r.err()
	}
	if r.storage.Nodes[ref].Kind != astjson.NodeKindString {
		value := string(r.storage.Nodes[ref].ValueBytes(r.storage))
		r.addTypeMismatchError(fmt.Sprintf("String cannot represent non-string value: \\\"%s\\\"", value), s.Path)
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
	ref = r.storage.Get(ref, b.Path)
	if !r.storage.NodeIsDefined(ref) {
		if b.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(b.Path)
		return r.err()
	}
	if r.storage.Nodes[ref].Kind != astjson.NodeKindBoolean {
		value := string(r.storage.Nodes[ref].ValueBytes(r.storage))
		r.addTypeMismatchError(fmt.Sprintf("Bool cannot represent non-boolean value: \\\"%s\\\"", value), b.Path)
		return r.err()
	}
	if r.print {
		r.printNode(ref)
	}
	return false
}

func (r *Resolvable) walkInteger(i *Integer, ref int) bool {
	ref = r.storage.Get(ref, i.Path)
	if !r.storage.NodeIsDefined(ref) {
		if i.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(i.Path)
		return r.err()
	}
	if r.storage.Nodes[ref].Kind != astjson.NodeKindNumber {
		value := string(r.storage.Nodes[ref].ValueBytes(r.storage))
		r.addTypeMismatchError(fmt.Sprintf("Int cannot represent non-integer value: \\\"%s\\\"", value), i.Path)
		return r.err()
	}
	if r.print {
		r.printNode(ref)
	}
	return false
}

func (r *Resolvable) walkFloat(f *Float, ref int) bool {
	ref = r.storage.Get(ref, f.Path)
	if !r.storage.NodeIsDefined(ref) {
		if f.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(f.Path)
		return r.err()
	}
	if r.storage.Nodes[ref].Kind != astjson.NodeKindNumber {
		value := string(r.storage.Nodes[ref].ValueBytes(r.storage))
		r.addTypeMismatchError(fmt.Sprintf("Float cannot represent non-float value: \\\"%s\\\"", value), f.Path)
		return r.err()
	}
	if r.print {
		r.printNode(ref)
	}
	return false
}

func (r *Resolvable) walkBigInt(b *BigInt, ref int) bool {
	ref = r.storage.Get(ref, b.Path)
	if !r.storage.NodeIsDefined(ref) {
		if b.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(b.Path)
		return r.err()
	}
	if r.print {
		r.printNode(ref)
	}
	return false
}

func (r *Resolvable) walkScalar(s *Scalar, ref int) bool {
	ref = r.storage.Get(ref, s.Path)
	if !r.storage.NodeIsDefined(ref) {
		if s.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(s.Path)
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
	ref = r.storage.Get(ref, c.Path)
	if !r.storage.NodeIsDefined(ref) {
		if c.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(c.Path)
		return r.err()
	}
	value := r.storage.Nodes[ref].ValueBytes(r.storage)
	resolved, err := c.Resolve(value)
	if err != nil {
		r.addUnableToResolveError(err.Error(), c.Path)
		return r.err()
	}
	if r.print {
		r.printBytes(resolved)
	}
	return false
}

func (r *Resolvable) addNonNullableFieldError(fieldPath []string) {
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

func (r *Resolvable) addTypeMismatchError(message string, fieldPath []string) {
	r.pushNodePathElement(fieldPath)
	ref := r.storage.AppendErrorWithMessage(message, r.path)
	r.storage.Nodes[r.errorsRoot].ArrayValues = append(r.storage.Nodes[r.errorsRoot].ArrayValues, ref)
	r.popNodePathElement(fieldPath)
}

func (r *Resolvable) addUnableToResolveError(message string, fieldPath []string) {
	r.pushNodePathElement(fieldPath)
	ref := r.storage.AppendErrorWithMessage(message, r.path)
	r.storage.Nodes[r.errorsRoot].ArrayValues = append(r.storage.Nodes[r.errorsRoot].ArrayValues, ref)
	r.popNodePathElement(fieldPath)
}
