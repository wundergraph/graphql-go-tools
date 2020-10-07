package planv2

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type Planner struct {
	walker  *astvisitor.Walker
	visitor *visitor
}

type Configuration struct {
	DefaultFlushInterval int64
	FieldMappings        []FieldMapping
}

type FieldMapping struct {
	TypeName              string
	FieldName             string
	DisableDefaultMapping bool
	Path                  []string
}

func NewPlanner(config Configuration) *Planner {

	walker := astvisitor.NewWalker(48)
	visitor := &visitor{
		walker: &walker,
		config: config,
	}

	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterOperationVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)
	walker.RegisterSelectionSetVisitor(visitor)
	walker.RegisterEnterDirectiveVisitor(visitor)

	p := &Planner{
		walker:  &walker,
		visitor: visitor,
	}

	return p
}

func (p *Planner) Plan(operation, definition *ast.Document, operationName string, report *operationreport.Report) (plan Plan) {
	p.visitor.operationName = operationName
	p.walker.Walk(operation, definition, report)
	return p.visitor.plan
}

type visitor struct {
	operation, definition *ast.Document
	walker                *astvisitor.Walker
	config                Configuration
	plan                  Plan
	operationName         string
	objects               []*resolve.Object
	fields                []*[]resolve.Field
}

func (v *visitor) EnterDirective(ref int) {
	directiveName := v.operation.DirectiveNameString(ref)
	switch v.walker.Ancestors[len(v.walker.Ancestors)-1].Kind {
	case ast.NodeKindOperationDefinition:
		switch directiveName {
		case "flushInterval":
			if value, ok := v.operation.DirectiveArgumentValueByName(ref, literal.MILLISECONDS); ok {
				if value.Kind == ast.ValueKindInteger {
					v.plan.SetFlushInterval(v.operation.IntValueAsInt(value.Ref))
				}
			}
		}
	case ast.NodeKindField:
		switch directiveName {
		case "stream":
			initialBatchSize := 0
			if value, ok := v.operation.DirectiveArgumentValueByName(ref, literal.INITIAL_BATCH_SIZE); ok {
				if value.Kind == ast.ValueKindInteger {
					initialBatchSize = int(v.operation.IntValueAsInt(value.Ref))
				}
			}
			(*v.fields[len(v.fields)-1])[len(*v.fields[len(v.fields)-1])-1].Stream = &resolve.StreamField{
				InitialBatchSize: initialBatchSize,
			}
		case "defer":
			(*v.fields[len(v.fields)-1])[len(*v.fields[len(v.fields)-1])-1].Defer = &resolve.DeferField{}
		}
	}
}

func (v *visitor) LeaveSelectionSet(ref int) {
	v.fields = v.fields[:len(v.fields)-1]
}

func (v *visitor) EnterSelectionSet(ref int) {
	currentObject := v.objects[len(v.objects)-1]
	fieldSet := resolve.FieldSet{
		Fields: []resolve.Field{},
	}
	currentObject.FieldSets = append(currentObject.FieldSets, fieldSet)
	v.fields = append(v.fields, &currentObject.FieldSets[len(currentObject.FieldSets)-1].Fields)
}

func (v *visitor) EnterField(ref int) {
	fieldName := v.operation.FieldAliasOrNameBytes(ref)
	fieldDefinition, ok := v.walker.FieldDefinition(ref)
	if !ok {
		return
	}
	path := v.resolveFieldPath(ref)
	fieldDefinitionType := v.definition.FieldDefinitionType(fieldDefinition)
	field := resolve.Field{
		Name:  fieldName,
		Value: v.resolveFieldValue(ref, fieldDefinitionType, true, path),
	}
	*v.fields[len(v.fields)-1] = append(*v.fields[len(v.fields)-1], field)
}

func (v *visitor) LeaveField(ref int) {
	fieldDefinition, ok := v.walker.FieldDefinition(ref)
	if !ok {
		return
	}
	fieldDefinitionTypeNode := v.definition.FieldDefinitionTypeNode(fieldDefinition)
	switch fieldDefinitionTypeNode.Kind {
	case ast.NodeKindObjectTypeDefinition, ast.NodeKindInterfaceTypeDefinition:
		v.objects = v.objects[:len(v.objects)-1]
	}
}

func (v *visitor) resolveFieldValue(fieldRef, typeRef int, nullable bool, path []string) resolve.Node {
	ofType := v.definition.Types[typeRef].OfType
	switch v.definition.Types[typeRef].TypeKind {
	case ast.TypeKindNonNull:
		return v.resolveFieldValue(fieldRef, ofType, false, path)
	case ast.TypeKindList:
		listItem := v.resolveFieldValue(fieldRef, ofType, true, nil)
		return &resolve.Array{
			Nullable: nullable,
			Path:     path,
			Item:     listItem,
		}
	case ast.TypeKindNamed:
		typeName := v.definition.ResolveTypeNameString(typeRef)
		typeDefinitionNode, ok := v.definition.Index.FirstNodeByNameStr(typeName)
		if !ok {
			return &resolve.Null{}
		}
		switch typeDefinitionNode.Kind {
		case ast.NodeKindScalarTypeDefinition:
			switch typeName {
			case "String":
				return &resolve.String{
					Path:     path,
					Nullable: nullable,
				}
			case "Boolean":
				return &resolve.Boolean{
					Path:     path,
					Nullable: nullable,
				}
			case "Int":
				return &resolve.Integer{
					Path:     path,
					Nullable: nullable,
				}
			case "Float":
				return &resolve.Float{
					Path:     path,
					Nullable: nullable,
				}
			default:
				return &resolve.String{
					Path:     path,
					Nullable: nullable,
				}
			}
		case ast.NodeKindEnumTypeDefinition:
			return &resolve.String{
				Path:     path,
				Nullable: nullable,
			}
		case ast.NodeKindObjectTypeDefinition, ast.NodeKindInterfaceTypeDefinition:
			object := &resolve.Object{
				Nullable: nullable,
				Path:     path,
			}
			v.objects = append(v.objects, object)
			return object
		default:
			return &resolve.Null{}
		}
	default:
		return &resolve.Null{}
	}
}

func (v *visitor) EnterOperationDefinition(ref int) {
	operationName := v.operation.OperationDefinitionNameString(ref)
	if v.operationName != operationName {
		v.walker.SkipNode()
		return
	}

	rootObject := &resolve.Object{}

	v.objects = append(v.objects, rootObject)

	switch v.operation.OperationDefinitions[ref].OperationType {
	case ast.OperationTypeSubscription:
		v.plan = &SubscriptionResponsePlan{
			Response: resolve.GraphQLSubscription{
				Response: &resolve.GraphQLResponse{
					Data: rootObject,
				},
			},
		}
	case ast.OperationTypeQuery:
		v.plan = &SynchronousResponsePlan{
			Response: resolve.GraphQLResponse{
				Data: rootObject,
			},
		}
	case ast.OperationTypeMutation:
		v.plan = &SynchronousResponsePlan{
			Response: resolve.GraphQLResponse{
				Data: rootObject,
			},
		}
	}
}

func (v *visitor) resolveFieldPath(ref int) []string {
	typeName := v.walker.EnclosingTypeDefinition.NameString(v.definition)
	fieldName := v.operation.FieldNameString(ref)
	for i := range v.config.FieldMappings {
		if v.config.FieldMappings[i].TypeName == typeName && v.config.FieldMappings[i].FieldName == fieldName {
			if v.config.FieldMappings[i].Path != nil {
				return v.config.FieldMappings[i].Path
			}
			if v.config.FieldMappings[i].DisableDefaultMapping {
				return nil
			}
			return []string{fieldName}
		}
	}
	return []string{fieldName}
}

func (v *visitor) EnterDocument(operation, definition *ast.Document) {
	v.operation, v.definition = operation, definition
}

type Kind int

const (
	SynchronousResponseKind Kind = iota + 1
	StreamingResponseKind
	SubscriptionResponseKind
)

type Plan interface {
	PlanKind() Kind
	SetFlushInterval(interval int64)
}

type SynchronousResponsePlan struct {
	Response      resolve.GraphQLResponse
	FlushInterval int64
}

func (s *SynchronousResponsePlan) SetFlushInterval(interval int64) {
	s.FlushInterval = interval
}

func (_ *SynchronousResponsePlan) PlanKind() Kind {
	return SynchronousResponseKind
}

type StreamingResponsePlan struct {
	Response      resolve.GraphQLStreamingResponse
	FlushInterval int64
}

func (s *StreamingResponsePlan) SetFlushInterval(interval int64) {
	s.FlushInterval = interval
}

func (_ *StreamingResponsePlan) PlanKind() Kind {
	return StreamingResponseKind
}

type SubscriptionResponsePlan struct {
	Response      resolve.GraphQLSubscription
	FlushInterval int64
}

func (s *SubscriptionResponsePlan) SetFlushInterval(interval int64) {
	s.FlushInterval = interval
}

func (_ *SubscriptionResponsePlan) PlanKind() Kind {
	return SubscriptionResponseKind
}

type DataSourcePlanner interface {
	Register(walker *astvisitor.Walker)
}
