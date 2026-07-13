package postprocess

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type PlanVisitor interface {
	PlanObjectVisitor
	PlanArrayVisitor
}

type PlanObjectVisitor interface {
	PlanEnterObjectVisitor
	PlanLeaveObjectVisitor
}

type PlanEnterObjectVisitor interface {
	EnterObject(object *resolve.Object)
}

type PlanLeaveObjectVisitor interface {
	LeaveObject(object *resolve.Object)
}

type PlanArrayVisitor interface {
	PlanEnterArrayVisitor
	PlanLeaveArrayVisitor
}

type PlanEnterArrayVisitor interface {
	EnterArray(array *resolve.Array)
}

type PlanLeaveArrayVisitor interface {
	LeaveArray(array *resolve.Array)
}

type PlanFieldVisitor interface {
	PlanEnterFieldVisitor
	PlanLeaveFieldVisitor
}

type PlanEnterFieldVisitor interface {
	EnterField(field *resolve.Field)
}

type PlanLeaveFieldVisitor interface {
	LeaveField(field *resolve.Field)
}

type PlanWalker struct {
	info *resolve.GraphQLResponseInfo

	CurrentFields  []*resolve.Field
	CurrentObjects []*resolve.Object
	path           []string

	objectVisitor PlanObjectVisitor
	arrayVisitor  PlanArrayVisitor
	fieldVisitor  PlanFieldVisitor

	skip bool
}

func (e *PlanWalker) SetSkip(skip bool) {
	e.skip = skip
}

func (e *PlanWalker) pushField(field *resolve.Field) {
	e.CurrentFields = append(e.CurrentFields, field)
}

func (e *PlanWalker) popField() {
	e.CurrentFields = e.CurrentFields[:len(e.CurrentFields)-1]
}

func (e *PlanWalker) pushObject(object *resolve.Object) {
	e.CurrentObjects = append(e.CurrentObjects, object)
}

func (e *PlanWalker) popObject() {
	e.CurrentObjects = e.CurrentObjects[:len(e.CurrentObjects)-1]
}

func (e *PlanWalker) pushPath(path []string) {
	e.path = append(e.path, path...)
}

func (e *PlanWalker) popPath(path []string) {
	e.path = e.path[:len(e.path)-len(path)]
}

func (e *PlanWalker) pushArrayPath() {
	e.path = append(e.path, "@")
}

func (e *PlanWalker) popArrayPath() {
	e.path = e.path[:len(e.path)-1]
}

func (e *PlanWalker) Walk(data *resolve.Object, info *resolve.GraphQLResponseInfo) {
	e.info = info
	e.walkNode(data)
}

func (e *PlanWalker) walkNode(node resolve.Node) {
	switch n := node.(type) {
	case *resolve.Object:
		e.walkObject(n)
	case *resolve.Array:
		e.walkArray(n)
	}
}

func (e *PlanWalker) walkField(field *resolve.Field) {
	e.pushPath(field.Value.NodePath())
	defer e.popPath(field.Value.NodePath())

	e.onEnterField(field)
	defer e.onLeaveField(field)

	e.pushField(field)
	defer e.popField()

	if e.skip {
		return
	}

	e.walkNode(field.Value)
}

func (e *PlanWalker) onEnterField(field *resolve.Field) {
	if e.fieldVisitor != nil {
		e.fieldVisitor.EnterField(field)
	}
}

func (e *PlanWalker) onLeaveField(field *resolve.Field) {
	if e.fieldVisitor != nil {
		e.fieldVisitor.LeaveField(field)
	}
}

func (e *PlanWalker) walkObject(object *resolve.Object) {
	e.objectVisitor.EnterObject(object)
	defer e.objectVisitor.LeaveObject(object)

	e.pushObject(object)
	defer e.popObject()

	for i := range object.Fields {
		e.walkField(object.Fields[i])
	}
}

func (e *PlanWalker) onEnterArray(array *resolve.Array) {
	if e.arrayVisitor != nil {
		e.arrayVisitor.EnterArray(array)
	}
}

func (e *PlanWalker) onLeaveArray(array *resolve.Array) {
	if e.arrayVisitor != nil {
		e.arrayVisitor.LeaveArray(array)
	}
}

func (e *PlanWalker) walkArray(array *resolve.Array) {
	e.pushArrayPath()
	defer e.popArrayPath()

	e.onEnterArray(array)
	defer e.onLeaveArray(array)

	e.walkNode(array.Item)
}
