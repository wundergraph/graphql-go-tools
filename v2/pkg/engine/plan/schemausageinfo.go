package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type SchemaUsageInfo struct {
	OperationType ast.OperationType
	TypeFields    []TypeFieldUsageInfo
}

type TypeFieldUsageInfo struct {
	FieldName string
	TypeNames []string
	Path      []string
	Source    TypeFieldSource
}

type TypeFieldSource struct {
	// IDs is a list of data source IDs that can be used to resolve the field
	IDs []string
}

func GetSchemaUsageInfo(plan Plan) SchemaUsageInfo {
	visitor := planVisitor{}
	switch p := plan.(type) {
	case *SynchronousResponsePlan:
		if p.Response.Info != nil {
			visitor.usage.OperationType = p.Response.Info.OperationType
		}
		visitor.visitNode(p.Response.Data, nil)
	case *SubscriptionResponsePlan:
		if p.Response.Response.Info != nil {
			visitor.usage.OperationType = p.Response.Response.Info.OperationType
		}
		visitor.visitNode(p.Response.Response.Data, nil)
	}
	return visitor.usage
}

type planVisitor struct {
	usage SchemaUsageInfo
}

func (p *planVisitor) visitNode(node resolve.Node, path []string) {
	switch t := node.(type) {
	case *resolve.Object:
		for _, field := range t.Fields {
			if field.Info != nil {
				p.usage.TypeFields = append(p.usage.TypeFields, TypeFieldUsageInfo{
					FieldName: field.Info.Name,
					TypeNames: field.Info.ParentTypeNames,
					Path:      append(path, field.Info.Name),
					Source: TypeFieldSource{
						IDs: field.Info.Source.IDs,
					},
				})
			}
			p.visitNode(field.Value, append(path, t.Path...))
		}
	case *resolve.Array:
		p.visitNode(t.Item, append(path, t.Path...))
	}
}
