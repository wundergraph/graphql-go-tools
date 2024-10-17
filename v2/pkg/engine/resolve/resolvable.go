package resolve

import (
	"bytes"
	"context"
	goerrors "errors"
	"fmt"
	"io"

	"github.com/cespare/xxhash/v2"
	"github.com/goccy/go-json"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

const (
	InvalidGraphqlErrorCode = "INVALID_GRAPHQL"
)

type Resolvable struct {
	options ResolvableOptions

	data                 *astjson.Value
	errors               *astjson.Value
	variables            *astjson.Value
	valueCompletion      *astjson.Value
	skipAddingNullErrors bool

	astjsonArena *astjson.Arena
	parsers      []*astjson.Parser

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

	enclosingTypeName string
}

type ResolvableOptions struct {
	ApolloCompatibilityValueCompletionInExtensions bool
	ApolloCompatibilityTruncateFloatValues         bool
	ApolloCompatibilitySuppressFetchErrors         bool
}

func NewResolvable(options ResolvableOptions) *Resolvable {
	return &Resolvable{
		options:            options,
		xxh:                xxhash.New(),
		authorizationAllow: make(map[uint64]struct{}),
		authorizationDeny:  make(map[uint64]string),
		astjsonArena:       &astjson.Arena{},
	}
}

var (
	parsers = &astjson.ParserPool{}
)

func (r *Resolvable) parseJSON(data []byte) (*astjson.Value, error) {
	parser := parsers.Get()
	r.parsers = append(r.parsers, parser)
	return parser.ParseBytes(data)
}

func (r *Resolvable) Reset(maxRecyclableParserSize int) {
	for i := range r.parsers {
		parsers.PutIfSizeLessThan(r.parsers[i], maxRecyclableParserSize)
		r.parsers[i] = nil
	}
	r.parsers = r.parsers[:0]
	r.typeNames = r.typeNames[:0]
	r.wroteErrors = false
	r.wroteData = false
	r.data = nil
	r.errors = nil
	r.valueCompletion = nil
	r.variables = nil
	r.depth = 0
	r.print = false
	r.out = nil
	r.printErr = nil
	r.path = r.path[:0]
	r.operationType = ast.OperationTypeUnknown
	r.renameTypeNames = r.renameTypeNames[:0]
	r.authorizationError = nil
	r.astjsonArena.Reset()
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
	r.data = r.astjsonArena.NewObject()
	r.errors = r.astjsonArena.NewArray()
	if len(ctx.Variables) != 0 {
		r.variables, err = astjson.ParseBytes(ctx.Variables)
		if err != nil {
			return err
		}
	}
	if initialData != nil {
		initialValue, err := astjson.ParseBytes(initialData)
		if err != nil {
			return err
		}
		r.data, _ = astjson.MergeValues(r.data, initialValue)
	}
	return
}

func (r *Resolvable) InitSubscription(ctx *Context, initialData []byte, postProcessing PostProcessingConfiguration) (err error) {
	r.ctx = ctx
	r.operationType = ast.OperationTypeSubscription
	r.renameTypeNames = ctx.RenameTypeNames
	if len(ctx.Variables) != 0 {
		r.variables = astjson.MustParseBytes(ctx.Variables)
	}
	if initialData != nil {
		initialValue, err := astjson.ParseBytes(initialData)
		if err != nil {
			return err
		}
		if postProcessing.SelectResponseDataPath == nil {
			r.data, _ = astjson.MergeValuesWithPath(r.data, initialValue, postProcessing.MergePath...)
		} else {
			selectedInitialValue := initialValue.Get(postProcessing.SelectResponseDataPath...)
			if selectedInitialValue != nil {
				r.data, _ = astjson.MergeValuesWithPath(r.data, selectedInitialValue, postProcessing.MergePath...)
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
		r.data = r.astjsonArena.NewObject()
	}
	if r.errors == nil {
		r.errors = r.astjsonArena.NewArray()
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

	if r.valueCompletion != nil {
		if writeComma {
			r.printBytes(comma)
		}
		writeComma = true //nolint:all // should we add another print func, we should not forget to write a comma
		err := r.printValueCompletionExtension()
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

func (r *Resolvable) printValueCompletionExtension() error {
	r.printBytes(quote)
	r.printBytes(literalValueCompletion)
	r.printBytes(quote)
	r.printBytes(colon)
	r.printNode(r.valueCompletion)
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
	if r.valueCompletion != nil {
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

func (r *Resolvable) printNode(value *astjson.Value) {
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

func (r *Resolvable) walkNode(node Node, value, parent *astjson.Value) bool {
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

func (r *Resolvable) walkObject(obj *Object, parent *astjson.Value) bool {
	r.enclosingTypeName = obj.TypeName
	value := parent.Get(obj.Path...)
	if value == nil || value.Type() == astjson.TypeNull {
		if obj.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(obj.Path, parent)
		return r.err()
	}
	r.pushNodePathElement(obj.Path)
	isRoot := r.depth < 2
	defer r.popNodePathElement(obj.Path)
	if value.Type() != astjson.TypeObject {
		r.addError("Object cannot represent non-object value.", obj.Path)
		return r.err()
	}

	typeName := value.GetStringBytes("__typename")
	if typeName != nil && len(obj.PossibleTypes) > 0 {
		// when we have a typename field present in a json object, we need to check if the type is valid

		if _, ok := obj.PossibleTypes[string(typeName)]; !ok {
			if !r.print {
				// during prewalk we need to add an error when the typename do not match a possible type
				if r.options.ApolloCompatibilityValueCompletionInExtensions {
					r.addValueCompletion(fmt.Sprintf("Invalid __typename found for object at %s.", r.pathLastElementDescription(obj.TypeName)), InvalidGraphqlErrorCode)
				} else {
					r.addErrorWithCode(fmt.Sprintf("Subgraph '%s' returned invalid value '%s' for __typename field.", obj.SourceName, string(typeName)), InvalidGraphqlErrorCode)
				}

				// if object is not nullable at prewalk we need to return an error
				// to immediately stop the resolving of the current object and buble up null
				if !obj.Nullable {
					return r.err()
				}

				// if object is nullable we can just set it to null
				// so return no error here
				return false
			} else {
				// at print walk we will render the object to null if it was nullable
				// in case it is not nullable - we already reported an error and won't walk this object again
				return r.walkNull()
			}
		}
	}

	if r.print && !isRoot {
		r.printBytes(lBrace)
		r.ctx.Stats.ResolvedObjects++
	}
	addComma := false

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
						astjson.SetNull(value, path...)
					}
				} else if obj.Nullable && len(obj.Path) > 0 {
					// if the field value is not nullable, but the object is nullable
					// we can just set the whole object to null
					astjson.SetNull(parent, obj.Path...)
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
		err := r.walkNode(obj.Fields[i].Value, value, parent)
		if err {
			if obj.Nullable {
				if len(obj.Path) > 0 {
					astjson.SetNull(parent, obj.Path...)
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

func (r *Resolvable) authorizeField(value *astjson.Value, field *Field) (skipField bool) {
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
	dataSourceName := field.Info.Source.Names[0]
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
		r.addRejectFieldError(result.Reason, DataSourceInfo{
			ID:   dataSourceID,
			Name: dataSourceName,
		}, field)
		return true
	}
	return false
}

func (r *Resolvable) authorize(value *astjson.Value, dataSourceID string, coordinate GraphCoordinate) (result *AuthorizationDeny, err error) {
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

func (r *Resolvable) addRejectFieldError(reason string, ds DataSourceInfo, field *Field) {
	nodePath := field.Value.NodePath()
	r.pushNodePathElement(nodePath)
	fieldPath := r.renderFieldPath()

	var errorMessage string
	if reason == "" {
		errorMessage = fmt.Sprintf("Unauthorized to load field '%s'.", fieldPath)
	} else {
		errorMessage = fmt.Sprintf("Unauthorized to load field '%s', Reason: %s.", fieldPath, reason)
	}
	r.ctx.appendSubgraphError(goerrors.Join(errors.New(errorMessage),
		NewSubgraphError(ds, fieldPath, reason, 0)))
	fastjsonext.AppendErrorToArray(r.astjsonArena, r.errors, errorMessage, r.path)
	r.popNodePathElement(nodePath)
}

func (r *Resolvable) objectFieldTypeName(v *astjson.Value, field *Field) string {
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
	return variable.Type() == astjson.TypeTrue
}

func (r *Resolvable) excludeField(includeVariableName string) bool {
	variable := r.variables.Get(includeVariableName)
	if variable == nil {
		return true
	}
	return variable.Type() == astjson.TypeFalse
}

func (r *Resolvable) walkArray(arr *Array, value *astjson.Value) bool {
	parent := value
	value = value.Get(arr.Path...)
	if astjson.ValueIsNull(value) {
		if arr.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(arr.Path, parent)
		return r.err()
	}
	r.pushNodePathElement(arr.Path)
	defer r.popNodePathElement(arr.Path)
	if value.Type() != astjson.TypeArray {
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
		err := r.walkNode(arr.Item, arrayValue, parent)
		r.popArrayPathElement()
		if err {
			if arr.Item.NodeKind() == NodeKindObject && arr.Item.NodeNullable() {
				value.SetArrayItem(i, astjson.NullValue)
				continue
			}
			if arr.Nullable {
				astjson.SetNull(parent, arr.Path...)
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

func (r *Resolvable) walkString(s *String, value *astjson.Value) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	parent := value
	value = value.Get(s.Path...)
	if astjson.ValueIsNull(value) {
		if s.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(s.Path, parent)
		return r.err()
	}
	if value.Type() != astjson.TypeString {
		r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
		r.addError(fmt.Sprintf("String cannot represent non-string value: \"%s\"", string(r.marshalBuf)), s.Path)
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

func (r *Resolvable) walkBoolean(b *Boolean, value *astjson.Value) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	parent := value
	value = value.Get(b.Path...)
	if astjson.ValueIsNull(value) {
		if b.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(b.Path, parent)
		return r.err()
	}
	if value.Type() != astjson.TypeTrue && value.Type() != astjson.TypeFalse {
		r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
		r.addError(fmt.Sprintf("Bool cannot represent non-boolean value: \"%s\"", string(r.marshalBuf)), b.Path)
		return r.err()
	}
	if r.print {
		r.printNode(value)
	}
	return false
}

func (r *Resolvable) walkInteger(i *Integer, value *astjson.Value) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	parent := value
	value = value.Get(i.Path...)
	if astjson.ValueIsNull(value) {
		if i.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(i.Path, parent)
		return r.err()
	}
	if value.Type() != astjson.TypeNumber {
		r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
		r.addError(fmt.Sprintf("Int cannot represent non-integer value: \"%s\"", string(r.marshalBuf)), i.Path)
		return r.err()
	}
	if r.print {
		r.printNode(value)
	}
	return false
}

func (r *Resolvable) walkFloat(f *Float, value *astjson.Value) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	parent := value
	value = value.Get(f.Path...)
	if astjson.ValueIsNull(value) {
		if f.Nullable {
			return r.walkNull()
		}
		r.addNonNullableFieldError(f.Path, parent)
		return r.err()
	}
	if !r.print {
		if value.Type() != astjson.TypeNumber {
			r.marshalBuf = value.MarshalTo(r.marshalBuf[:0])
			r.addError(fmt.Sprintf("Float cannot represent non-float value: \"%s\"", string(r.marshalBuf)), f.Path)
			return r.err()
		}
	}
	if r.print {
		if r.options.ApolloCompatibilityTruncateFloatValues {
			floatValue := value.GetFloat64()
			if floatValue == float64(int64(floatValue)) {
				_, _ = fmt.Fprintf(r.out, "%d", int64(floatValue))
				return false
			}
		}
		r.printNode(value)
	}
	return false
}

func (r *Resolvable) walkBigInt(b *BigInt, value *astjson.Value) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	parent := value
	value = value.Get(b.Path...)
	if astjson.ValueIsNull(value) {
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

func (r *Resolvable) walkScalar(s *Scalar, value *astjson.Value) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	parent := value
	value = value.Get(s.Path...)
	if astjson.ValueIsNull(value) {
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

func (r *Resolvable) walkCustom(c *CustomNode, value *astjson.Value) bool {
	if r.print {
		r.ctx.Stats.ResolvedLeafs++
	}
	parent := value
	value = value.Get(c.Path...)
	if astjson.ValueIsNull(value) {
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

func (r *Resolvable) addNonNullableFieldError(fieldPath []string, parent *astjson.Value) {
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
	if r.options.ApolloCompatibilityValueCompletionInExtensions {
		r.addValueCompletion(r.renderApolloCompatibleNonNullableErrorMessage(), InvalidGraphqlErrorCode)
	} else {
		errorMessage := fmt.Sprintf("Cannot return null for non-nullable field '%s'.", r.renderFieldPath())
		fastjsonext.AppendErrorToArray(r.astjsonArena, r.errors, errorMessage, r.path)
	}
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

func (r *Resolvable) renderApolloCompatibleNonNullableErrorMessage() string {
	pathLength := len(r.path)
	if pathLength < 1 {
		return "invalid path"
	}
	lastPathItem := r.path[pathLength-1]
	if lastPathItem.Name != "" {
		return fmt.Sprintf("Cannot return null for non-nullable field %s.", r.renderFieldCoordinates())
	}
	// If the item has no name, it's a GraphQL list element. A list must be returned by a field.
	if pathLength < 2 {
		return "invalid path"
	}
	return fmt.Sprintf("Cannot return null for non-nullable array element of type %s at index %d.", r.enclosingTypeName, lastPathItem.Idx)
}

func (r *Resolvable) renderFieldCoordinates() string {
	buf := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(buf)
	pathLength := len(r.path)
	switch pathLength {
	case 0:
		return "invalid path"
	case 1:
		switch r.operationType {
		case ast.OperationTypeQuery:
			_, _ = buf.WriteString("Query.")
		case ast.OperationTypeMutation:
			_, _ = buf.WriteString("Mutation.")
		case ast.OperationTypeSubscription:
			_, _ = buf.WriteString("Subscription.")
		default:
			return "invalid path"
		}
		_, _ = buf.WriteString(r.path[0].Name)
	default:
		_, _ = buf.WriteString(r.enclosingTypeName)
		_, _ = buf.WriteString(".")
		_, _ = buf.WriteString(r.path[pathLength-1].Name)
	}
	return buf.String()
}

func (r *Resolvable) addError(message string, fieldPath []string) {
	r.pushNodePathElement(fieldPath)
	fastjsonext.AppendErrorToArray(r.astjsonArena, r.errors, message, r.path)
	r.popNodePathElement(fieldPath)
}

func (r *Resolvable) addErrorWithCode(message, code string) {
	fastjsonext.AppendErrorWithExtensionsCodeToArray(r.astjsonArena, r.errors, message, code, r.path)
}

func (r *Resolvable) addValueCompletion(message, code string) {
	if r.valueCompletion == nil {
		r.valueCompletion = r.astjsonArena.NewArray()
	}
	fastjsonext.AppendErrorWithExtensionsCodeToArray(r.astjsonArena, r.valueCompletion, message, code, r.path)
}

func (r *Resolvable) pathLastElementDescription(typeName string) string {
	if len(r.path) <= 1 {
		switch r.operationType {
		case ast.OperationTypeQuery:
			typeName = "Query"
		case ast.OperationTypeMutation:
			typeName = "Mutation"
		case ast.OperationTypeSubscription:
			typeName = "Subscription"
		}

		if len(r.path) == 0 {
			return typeName
		}
	}
	elem := r.path[len(r.path)-1]
	if elem.Name != "" {
		return fmt.Sprintf("field %s.%s", typeName, elem.Name)
	}
	return fmt.Sprintf("array element of type %s at index %d", typeName, elem.Idx)
}
