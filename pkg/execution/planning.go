package execution

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

type Argument struct {
	Name         []byte
	VariableName []byte
}

type Object struct {
	Fields []Field
	Path   []byte
}

func (*Object) Kind() NodeKind {
	return ObjectKind
}

type Field struct {
	Name    []byte
	Value   Node
	Data    []byte
	Resolve *Resolve
}

func (*Field) Kind() NodeKind {
	return FieldKind
}

type Value struct {
	Path []byte
}

func (*Value) Kind() NodeKind {
	return ValueKind
}

type List struct {
	Path  []byte
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

func (t *TypeResolver) Resolve(ctx Context, args []Argument) []byte {
	return userType
}

type GraphQLResolver struct {
	Upstream  string
	URL       string
	Query     string
	Variables []interface{}
}

func (g *GraphQLResolver) Resolve(ctx Context, args []Argument) []byte {
	return nil
}

type RESTResolver struct {
}

func (R *RESTResolver) Resolve(ctx Context, args []Argument) []byte {
	return nil
}
