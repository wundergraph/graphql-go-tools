package execution

import (
	"bytes"
	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash"
)

const (
	ObjectKind NodeKind = iota + 1
	FieldKind
	ListKind
	ValueKind
)

type NodeKind int

type Node interface {
	Kind() NodeKind
}

type Context struct {
	Variables Variables
}

type Variables map[uint64][]byte

type Argument interface {
	ArgName() []byte
}

type ContextVariableArgument struct {
	Name         []byte
	VariableName []byte
}

func (c *ContextVariableArgument) ArgName() []byte {
	return c.Name
}

type ObjectVariableArgument struct {
	Name []byte
	Path []string
}

func (o *ObjectVariableArgument) ArgName() []byte {
	return o.Name
}

type StaticVariableArgument struct {
	Name  []byte
	Value []byte
}

func (s *StaticVariableArgument) ArgName() []byte {
	return s.Name
}

type Object struct {
	Fields []Field
	Path   []string
}

func (*Object) Kind() NodeKind {
	return ObjectKind
}

type BooleanCondition interface {
	Evaluate(ctx Context, data []byte) bool
}

type Field struct {
	Name    []byte
	Value   Node
	Resolve *Resolve
	Skip    BooleanCondition
}

type IfEqual struct {
	Left, Right Argument
}

func (i *IfEqual) Evaluate(ctx Context, data []byte) bool {
	var left []byte
	var right []byte

	switch value := i.Left.(type) {
	case *ContextVariableArgument:
		left = ctx.Variables[xxhash.Sum64(value.VariableName)]
	case *ObjectVariableArgument:
		left, _, _, _ = jsonparser.Get(data, value.Path...)
	case *StaticVariableArgument:
		left = value.Value
	}

	switch value := i.Right.(type) {
	case *ContextVariableArgument:
		right = ctx.Variables[xxhash.Sum64(value.VariableName)]
	case *ObjectVariableArgument:
		right, _, _, _ = jsonparser.Get(data, value.Path...)
	case *StaticVariableArgument:
		right = value.Value
	}

	return bytes.Equal(left, right)
}

type IfNotEqual struct {
	Left, Right Argument
}

func (i *IfNotEqual) Evaluate(ctx Context, data []byte) bool {
	equal := IfEqual{
		Left:  i.Left,
		Right: i.Right,
	}
	return !equal.Evaluate(ctx, data)
}

func (*Field) Kind() NodeKind {
	return FieldKind
}

type Value struct {
	Path []string
}

func (*Value) Kind() NodeKind {
	return ValueKind
}

type List struct {
	Path  []string
	Value Node
}

func (*List) Kind() NodeKind {
	return ListKind
}

type Resolve struct {
	Args     []Argument
	Resolver Resolver
}

type Resolver interface {
	Resolve(ctx Context, args []Argument) []byte
}

type TypeResolver struct {
}

var userType = []byte(`{
			  "__type": {
				"name": "User",
				"fields": [
				  {
					"name": "id",
					"type": { "name": "String" }
				  },
				  {
					"name": "name",
					"type": { "name": "String" }
				  },
				  {
					"name": "birthday",
					"type": { "name": "Date" }
				  }
				]
			  }
			}`)

var userData = []byte(`
		{
			"data":	{
				"user":	{
					"id":1,
					"name":"Jens",
					"birthday":"08.02.1988"
				}
			}
		}`)

var friendsData = []byte(`[
   {
      "id":2,
      "name":"Yaara",
      "birthday":"1990 I guess? ;-)"
   },
   {
      "id":3,
      "name":"Ahmet",
      "birthday":"1980"
   }]`)

var petsData = []byte(`{
   "data":{
      "userPets":[{
            "__typename":"Dog",
            "name":"Paw",
            "nickname":"Pawie",
            "woof":"Woof! Woof!"
         },
         {
            "__typename":"Cat",
            "name":"Mietz",
            "nickname":"Mietzie",
            "meow":"Meow meow!"
         }]}
}`)

func (t *TypeResolver) Resolve(ctx Context, args []Argument) []byte {
	return userType
}

type GraphQLResolver struct {
	Upstream string
	URL      string
	Query    []byte
}

func (g *GraphQLResolver) Resolve(ctx Context, args []Argument) []byte {

	if bytes.Equal(g.Query, []byte("query q1($id: String!){user{id name birthday}}")) {
		return userData
	}

	if bytes.Equal(g.Query, []byte("query q1($id: String!){userPets(id: $id){	__typename name nickname... on Dog {woof} ... on Cat {meow}}}")) {
		return petsData
	}

	return []byte("query mismatch")
}

type RESTResolver struct {
	Upstream string
	URL      string
}

func (r *RESTResolver) Resolve(ctx Context, args []Argument) []byte {

	if r.URL == "/user/:id/friends" {
		return friendsData
	}

	return nil
}
