package plan

import (
	"github.com/jensneuse/abstractlogger"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type Configuration struct {
	Logger                     abstractlogger.Logger
	DefaultFlushIntervalMillis int64
	DataSources                []DataSource
	Fields                     FieldConfigurations
	Types                      TypeConfigurations
	// DisableResolveFieldPositions should be set to true for testing purposes
	// This setting removes position information from all fields
	// In production, this should be set to false so that error messages are easier to understand
	DisableResolveFieldPositions bool
	CustomResolveMap             map[string]resolve.CustomResolve

	// Debug - configure debug options
	Debug DebugConfiguration
	// IncludeInfo will add additional information to the plan,
	// e.g. the origin of a field, possible types, etc.
	// This information is required to compute the schema usage info from a plan
	IncludeInfo bool
}

type DebugConfiguration struct {
	PrintOperationTransformations bool
	PrintOperationEnableASTRefs   bool
	PrintPlanningPaths            bool
	PrintQueryPlans               bool

	PrintNodeSuggestions bool
	NodeSuggestion       NodeSuggestionDebugConfiguration

	ConfigurationVisitor bool
	PlanningVisitor      bool
	DatasourceVisitor    bool
}

type NodeSuggestionDebugConfiguration struct {
	SelectionReasons  bool
	FilterNotSelected bool
}

type TypeConfigurations []TypeConfiguration

func (t *TypeConfigurations) RenameTypeNameOnMatchStr(typeName string) string {
	for i := range *t {
		if (*t)[i].TypeName == typeName {
			return (*t)[i].RenameTo
		}
	}
	return typeName
}

func (t *TypeConfigurations) RenameTypeNameOnMatchBytes(typeName []byte) []byte {
	str := string(typeName)
	for i := range *t {
		if (*t)[i].TypeName == str {
			return []byte((*t)[i].RenameTo)
		}
	}
	return typeName
}

type TypeConfiguration struct {
	TypeName string
	// RenameTo modifies the TypeName
	// so that a downstream Operation can contain a different TypeName than the upstream Schema
	// e.g. if the downstream Operation contains { ... on Human_api { height } }
	// the upstream Operation can be rewritten to { ... on Human { height }}
	// by setting RenameTo to Human
	// This way, Types can be suffixed / renamed in downstream Schemas while keeping the contract with the upstream ok
	RenameTo string
}

type FieldConfigurations []FieldConfiguration

func (f FieldConfigurations) ForTypeField(typeName, fieldName string) *FieldConfiguration {
	for i := range f {
		if f[i].TypeName == typeName && f[i].FieldName == fieldName {
			return &f[i]
		}
	}
	return nil
}

type FieldConfiguration struct {
	TypeName  string
	FieldName string
	// DisableDefaultMapping - instructs planner whether to use path mapping coming from Path field
	DisableDefaultMapping bool
	// Path - represents a json path to lookup for a field value in response json
	Path      []string
	Arguments ArgumentsConfigurations
	// UnescapeResponseJson set to true will allow fields (String,List,Object)
	// to be resolved from an escaped JSON string
	// e.g. {"response":"{\"foo\":\"bar\"}"} will be returned as {"foo":"bar"} when path is "response"
	// This way, it is possible to resolve a JSON string as part of the response without extra String encoding of the JSON
	UnescapeResponseJson bool
	// HasAuthorizationRule needs to be set to true if the Authorizer should be called for this field
	HasAuthorizationRule bool

	SubscriptionFilterCondition *SubscriptionFilterCondition
}

type SubscriptionFilterCondition struct {
	And []SubscriptionFilterCondition
	Or  []SubscriptionFilterCondition
	Not *SubscriptionFilterCondition
	In  *SubscriptionFieldCondition
}

type SubscriptionFieldCondition struct {
	FieldPath []string
	Values    []string
}

type ArgumentsConfigurations []ArgumentConfiguration

func (a ArgumentsConfigurations) ForName(argName string) *ArgumentConfiguration {
	for i := range a {
		if a[i].Name == argName {
			return &a[i]
		}
	}
	return nil
}

// SourceType is used to determine the source of an argument
type SourceType string

const (
	ObjectFieldSource   SourceType = "object_field"
	FieldArgumentSource SourceType = "field_argument"
)

// ArgumentRenderConfig is used to determine how an argument should be rendered
type ArgumentRenderConfig string

const (
	RenderArgumentDefault        ArgumentRenderConfig = ""
	RenderArgumentAsArrayCSV     ArgumentRenderConfig = "render_argument_as_array_csv"
	RenderArgumentAsGraphQLValue ArgumentRenderConfig = "render_argument_as_graphql_value"
	RenderArgumentAsJSONValue    ArgumentRenderConfig = "render_argument_as_json_value"
)

type ArgumentConfiguration struct {
	Name         string
	SourceType   SourceType
	SourcePath   []string
	RenderConfig ArgumentRenderConfig
	RenameTypeTo string
}
