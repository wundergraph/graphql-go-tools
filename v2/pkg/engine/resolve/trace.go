package resolve

type TraceFetchType string

const (
	TraceFetchTypeSingle           TraceFetchType = "single"
	TraceFetchTypeParallel         TraceFetchType = "parallel"
	TraceFetchTypeSerial           TraceFetchType = "serial"
	TraceFetchTypeParallelListItem TraceFetchType = "parallelListItem"
	TraceFetchTypeEntity           TraceFetchType = "entity"
	TraceFetchTypeBatchEntity      TraceFetchType = "batchEntity"
)

type TraceNodeType string

const (
	TraceNodeTypeObject      TraceNodeType = "object"
	TraceNodeTypeEmptyObject TraceNodeType = "emptyObject"
	TraceNodeTypeArray       TraceNodeType = "array"
	TraceNodeTypeEmptyArray  TraceNodeType = "emptyArray"
	TraceNodeTypeNull        TraceNodeType = "null"
	TraceNodeTypeString      TraceNodeType = "string"
	TraceNodeTypeBoolean     TraceNodeType = "boolean"
	TraceNodeTypeInteger     TraceNodeType = "integer"
	TraceNodeTypeFloat       TraceNodeType = "float"
	TraceNodeTypeBigInt      TraceNodeType = "bigint"
	TraceNodeTypeCustom      TraceNodeType = "custom"
	TraceNodeTypeScalar      TraceNodeType = "scalar"
	TraceNodeTypeUnknown     TraceNodeType = "unknown"
)

type TraceFetch struct {
	Type       TraceFetchType `json:"type,omitempty"`
	Fetches    []*TraceFetch  `json:"fetches,omitempty"`
	DataSource string         `json:"datasource,omitempty"`
}

type TraceField struct {
	Name            string     `json:"name,omitempty"`
	Value           *TraceNode `json:"value,omitempty"`
	ParentTypeNames []string   `json:"parentTypeNames,omitempty"`
	NamedType       string     `json:"namedType,omitempty"`
	SourceIDs       []string   `json:"sourceIDs,omitempty"`
}

type TraceNode struct {
	Fetch                *TraceFetch   `json:"fetch,omitempty"`
	NodeType             TraceNodeType `json:"node_type,omitempty"`
	Nullable             bool          `json:"nullable,omitempty"`
	Path                 []string      `json:"path,omitempty"`
	Fields               []*TraceField `json:"fields,omitempty"`
	Items                []*TraceNode  `json:"items,omitempty"`
	UnescapeResponseJson bool          `json:"unescape_response_json,omitempty"`
	IsTypeName           bool          `json:"is_type_name,omitempty"`
}

func getNodeType(kind NodeKind) TraceNodeType {
	switch kind {
	case NodeKindObject:
		return TraceNodeTypeObject
	case NodeKindEmptyObject:
		return TraceNodeTypeEmptyObject
	case NodeKindArray:
		return TraceNodeTypeArray
	case NodeKindEmptyArray:
		return TraceNodeTypeEmptyArray
	case NodeKindNull:
		return TraceNodeTypeNull
	case NodeKindString:
		return TraceNodeTypeString
	case NodeKindBoolean:
		return TraceNodeTypeBoolean
	case NodeKindInteger:
		return TraceNodeTypeInteger
	case NodeKindFloat:
		return TraceNodeTypeFloat
	case NodeKindBigInt:
		return TraceNodeTypeBigInt
	case NodeKindCustom:
		return TraceNodeTypeCustom
	case NodeKindScalar:
		return TraceNodeTypeScalar
	default:
		return TraceNodeTypeUnknown
	}
}

func parseField(f *Field) *TraceField {
	if f == nil {
		return nil
	}

	field := &TraceField{
		Name:  string(f.Name),
		Value: parseNode(f.Value),
	}

	if f.Info == nil {
		return field
	}

	field.ParentTypeNames = f.Info.ParentTypeNames
	field.NamedType = f.Info.NamedType
	field.SourceIDs = f.Info.Source.IDs

	return field
}

func parseFetch(fetch Fetch) *TraceFetch {
	traceFetch := &TraceFetch{}

	switch f := fetch.(type) {
	case *SingleFetch:
		traceFetch.Type = TraceFetchTypeSingle

	case *ParallelFetch:
		traceFetch.Type = TraceFetchTypeParallel
		for _, subFetch := range f.Fetches {
			traceFetch.Fetches = append(traceFetch.Fetches, parseFetch(subFetch))
		}

	case *SerialFetch:
		traceFetch.Type = TraceFetchTypeSerial
		for _, subFetch := range f.Fetches {
			traceFetch.Fetches = append(traceFetch.Fetches, parseFetch(subFetch))
		}

	case *ParallelListItemFetch:
		traceFetch.Type = TraceFetchTypeParallelListItem
		traceFetch.Fetches = append(traceFetch.Fetches, parseFetch(f.Fetch))

	case *EntityFetch:
		traceFetch.Type = TraceFetchTypeEntity

	case *BatchEntityFetch:
		traceFetch.Type = TraceFetchTypeBatchEntity

	default:
		return nil
	}

	return traceFetch
}

func parseNode(n Node) *TraceNode {
	node := &TraceNode{
		NodeType: getNodeType(n.NodeKind()),
		Nullable: n.NodeKind() == NodeKindNull || n.NodePath() == nil,
		Path:     n.NodePath(),
	}

	switch v := n.(type) {
	case *Object:
		for _, field := range v.Fields {
			node.Fields = append(node.Fields, parseField(field))
		}
		node.Fetch = parseFetch(v.Fetch)

	case *Array:
		if v.Item != nil {
			node.Items = append(node.Items, parseNode(v.Item))
		} else if len(v.Items) > 0 {
			for _, item := range v.Items {
				node.Items = append(node.Items, parseNode(item))
			}
		}

	case *String:
		node.UnescapeResponseJson = v.UnescapeResponseJson
		node.IsTypeName = v.IsTypeName
	}

	return node
}

func GetTrace(root *Object) *TraceNode {
	return parseNode(root)
}
