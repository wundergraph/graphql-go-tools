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

type FetchNodeType string

const (
	FetchNodeTypeObject      FetchNodeType = "object"
	FetchNodeTypeEmptyObject FetchNodeType = "emptyObject"
	FetchNodeTypeArray       FetchNodeType = "array"
	FetchNodeTypeEmptyArray  FetchNodeType = "emptyArray"
	FetchNodeTypeNull        FetchNodeType = "null"
	FetchNodeTypeString      FetchNodeType = "string"
	FetchNodeTypeBoolean     FetchNodeType = "boolean"
	FetchNodeTypeInteger     FetchNodeType = "integer"
	FetchNodeTypeFloat       FetchNodeType = "float"
	FetchNodeTypeBigInt      FetchNodeType = "bigint"
	FetchNodeTypeCustom      FetchNodeType = "custom"
	FetchNodeTypeScalar      FetchNodeType = "scalar"
	FetchNodeTypeUnknown     FetchNodeType = "unknown"
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
	NodeType             FetchNodeType `json:"node_type,omitempty"`
	Nullable             bool          `json:"nullable,omitempty"`
	Path                 []string      `json:"path,omitempty"`
	Fields               []*TraceField `json:"fields,omitempty"`
	Items                []*TraceNode  `json:"items,omitempty"`
	UnescapeResponseJson bool          `json:"unescape_response_json,omitempty"`
	IsTypeName           bool          `json:"is_type_name,omitempty"`
}

func getTypeName(kind NodeKind) FetchNodeType {
	switch kind {
	case NodeKindObject:
		return FetchNodeTypeObject
	case NodeKindEmptyObject:
		return FetchNodeTypeEmptyObject
	case NodeKindArray:
		return FetchNodeTypeArray
	case NodeKindEmptyArray:
		return FetchNodeTypeEmptyArray
	case NodeKindNull:
		return FetchNodeTypeNull
	case NodeKindString:
		return FetchNodeTypeString
	case NodeKindBoolean:
		return FetchNodeTypeBoolean
	case NodeKindInteger:
		return FetchNodeTypeInteger
	case NodeKindFloat:
		return FetchNodeTypeFloat
	case NodeKindBigInt:
		return FetchNodeTypeBigInt
	case NodeKindCustom:
		return FetchNodeTypeCustom
	case NodeKindScalar:
		return FetchNodeTypeScalar
	default:
		return FetchNodeTypeUnknown
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
		NodeType: getTypeName(n.NodeKind()),
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
