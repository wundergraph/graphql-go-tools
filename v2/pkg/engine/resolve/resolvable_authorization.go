package resolve

import (
	"fmt"
	"strings"

	"github.com/cespare/xxhash/v2"
	"github.com/pkg/errors"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/errorcodes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
)

func (r *Resolvable) authorizeField(value *astjson.Value, field *Field) (skipField bool) {
	if field.Info == nil {
		return false
	}
	if !field.Info.HasAuthorizationRule {
		return false
	}
	if r.ctx.authorizer == nil && r.ctx.preFetchFieldAuthorizer == nil {
		return false
	}
	if len(field.Info.Source.IDs) == 0 {
		return false
	}
	dataSourceID := field.Info.Source.IDs[0]
	dataSourceName := field.Info.Source.Names[0]
	typeName := r.objectFieldTypeName(value, field)
	if r.ctx.preFetchFieldAuthorizer != nil {
		typeName = field.Info.ExactParentTypeName
	}
	gc := GraphCoordinate{
		TypeName:  typeName,
		FieldName: field.Info.Name,
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
	decisionID := authorizationDecisionID(dataSourceID, coordinate)
	if _, ok := r.authorizationAllow[decisionID]; ok {
		return nil, nil
	}
	if reason, ok := r.authorizationDeny[decisionID]; ok {
		return &AuthorizationDeny{Reason: reason}, nil
	}
	if r.ctx.authorizer == nil {
		// Pre-fetch field authorization without a post-fetch authorizer: the only decisions are those
		// seeded up front. A coordinate without a seeded decision is treated as authorized.
		return nil, nil
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

func authorizationDecisionID(dataSourceID string, coordinate GraphCoordinate) uint64 {
	return xxhash.Sum64String(dataSourceID + coordinate.TypeName + coordinate.FieldName)
}

func (r *Resolvable) seedAuthorizationAllow(dataSourceID string, coordinate GraphCoordinate) {
	r.authorizationAllow[authorizationDecisionID(dataSourceID, coordinate)] = struct{}{}
}

func (r *Resolvable) seedAuthorizationDeny(dataSourceID string, coordinate GraphCoordinate, reason string) {
	r.authorizationDeny[authorizationDecisionID(dataSourceID, coordinate)] = reason
}

func (r *Resolvable) authorizationDenyReason(dataSourceID string, coordinate GraphCoordinate) (string, bool) {
	reason, ok := r.authorizationDeny[authorizationDecisionID(dataSourceID, coordinate)]
	return reason, ok
}

func (r *Resolvable) appendUnauthorizedFieldErrorsForUnreachedData(root *Object, data *astjson.Value) {
	emitted := make(map[string]struct{})
	r.appendUnauthorizedFieldErrorsForUnreachedObject(root, data, nil, emitted)
}

func (r *Resolvable) appendUnauthorizedFieldErrorsForUnreachedObject(obj *Object, value *astjson.Value, path []string, emitted map[string]struct{}) {
	if obj == nil || astjson.ValueIsNull(value) || value.Type() != astjson.TypeObject {
		r.appendUnauthorizedFieldErrorsInSubtree(obj, path, emitted)
		return
	}
	for i := range obj.Fields {
		field := obj.Fields[i]
		switch node := field.Value.(type) {
		case *Object:
			fieldPath := appendAuthorizationPath(path, node.Path)
			child := value.Get(node.Path...)
			if astjson.ValueIsNull(child) || child.Type() != astjson.TypeObject {
				r.appendUnauthorizedFieldErrorsInSubtree(node, fieldPath, emitted)
				continue
			}
			r.appendUnauthorizedFieldErrorsForUnreachedObject(node, child, fieldPath, emitted)
		case *Array:
			fieldPath := appendAuthorizationPath(path, node.Path)
			child := value.Get(node.Path...)
			if astjson.ValueIsNull(child) || child.Type() != astjson.TypeArray {
				r.appendUnauthorizedFieldErrorsInSubtree(node.Item, fieldPath, emitted)
				continue
			}
			items := child.GetArray()
			if len(items) == 0 {
				r.appendUnauthorizedFieldErrorsInSubtree(node.Item, fieldPath, emitted)
				continue
			}
			for j := range items {
				r.appendUnauthorizedFieldErrorsForUnreachedNode(node.Item, items[j], fieldPath, emitted)
			}
		}
	}
}

func (r *Resolvable) appendUnauthorizedFieldErrorsForUnreachedNode(node Node, value *astjson.Value, path []string, emitted map[string]struct{}) {
	switch n := node.(type) {
	case *Object:
		r.appendUnauthorizedFieldErrorsForUnreachedObject(n, value, path, emitted)
	case *Array:
		child := value.Get(n.Path...)
		fieldPath := appendAuthorizationPath(path, n.Path)
		if astjson.ValueIsNull(child) || child.Type() != astjson.TypeArray {
			r.appendUnauthorizedFieldErrorsInSubtree(n.Item, fieldPath, emitted)
			return
		}
		items := child.GetArray()
		if len(items) == 0 {
			r.appendUnauthorizedFieldErrorsInSubtree(n.Item, fieldPath, emitted)
			return
		}
		for i := range items {
			r.appendUnauthorizedFieldErrorsForUnreachedNode(n.Item, items[i], fieldPath, emitted)
		}
	}
}

func (r *Resolvable) appendUnauthorizedFieldErrorsInSubtree(node Node, path []string, emitted map[string]struct{}) {
	switch n := node.(type) {
	case *Object:
		for i := range n.Fields {
			field := n.Fields[i]
			fieldPath := appendAuthorizationFieldPath(path, field)
			if field.Info != nil && field.Info.HasAuthorizationRule && len(field.Info.Source.IDs) > 0 {
				dataSourceID := field.Info.Source.IDs[0]
				reason, denied := r.authorizationDenyReason(dataSourceID, GraphCoordinate{
					TypeName:  field.Info.ExactParentTypeName,
					FieldName: field.Info.Name,
				})
				if denied {
					key := dataSourceID + "\x00" + field.Info.ExactParentTypeName + "\x00" + field.Info.Name + "\x00" + strings.Join(fieldPath, ".")
					if _, ok := emitted[key]; !ok {
						emitted[key] = struct{}{}
						r.addRejectFieldPathError(reason, DataSourceInfo{
							ID:   dataSourceID,
							Name: firstString(field.Info.Source.Names),
						}, fieldPath)
					}
				}
			}
			r.appendUnauthorizedFieldErrorsInSubtree(field.Value, fieldPath, emitted)
		}
	case *Array:
		r.appendUnauthorizedFieldErrorsInSubtree(n.Item, path, emitted)
	}
}

func appendAuthorizationPath(path, nodePath []string) []string {
	out := make([]string, 0, len(path)+len(nodePath))
	out = append(out, path...)
	out = append(out, nodePath...)
	return out
}

func appendAuthorizationFieldPath(path []string, field *Field) []string {
	if nodePath := field.Value.NodePath(); len(nodePath) > 0 {
		return appendAuthorizationPath(path, nodePath)
	}
	out := make([]string, 0, len(path)+1)
	out = append(out, path...)
	out = append(out, string(field.Name))
	return out
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
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
	r.ctx.appendSubgraphErrors(ds, errors.New(errorMessage),
		NewSubgraphError(ds, fieldPath, reason, 0))
	r.ensureErrorsInitialized()
	fastjsonext.AppendErrorWithExtensionsCodeToArray(r.astjsonArena, r.errors, errorMessage, errorcodes.UnauthorizedFieldOrType, r.path)
	r.popNodePathElement(nodePath)
}

func (r *Resolvable) addRejectFieldPathError(reason string, ds DataSourceInfo, fieldPath []string) {
	r.pushNodePathElement(fieldPath)
	renderedFieldPath := r.renderFieldPath()

	var errorMessage string
	if reason == "" {
		errorMessage = fmt.Sprintf("Unauthorized to load field '%s'.", renderedFieldPath)
	} else {
		errorMessage = fmt.Sprintf("Unauthorized to load field '%s', Reason: %s.", renderedFieldPath, reason)
	}
	r.ctx.appendSubgraphErrors(ds, errors.New(errorMessage),
		NewSubgraphError(ds, renderedFieldPath, reason, 0))
	r.ensureErrorsInitialized()
	fastjsonext.AppendErrorWithExtensionsCodeToArray(r.astjsonArena, r.errors, errorMessage, errorcodes.UnauthorizedFieldOrType, r.path)
	r.popNodePathElement(fieldPath)
}

func (r *Resolvable) objectFieldTypeName(v *astjson.Value, field *Field) string {
	typeName := v.GetStringBytes("__typename")
	if typeName != nil {
		return unsafebytes.BytesToString(typeName)
	}
	return field.Info.ExactParentTypeName
}
