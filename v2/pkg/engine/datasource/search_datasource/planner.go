package search_datasource

import (
	"log"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// Planner implements plan.DataSourcePlanner for the search datasource.
type Planner struct {
	id               int
	config           Configuration
	visitor          *plan.Visitor
	dataSourceConfig plan.DataSourceConfiguration[Configuration]
	factory          *Factory

	// State collected during visitor walk
	searchFieldName string
	fieldRef        int
	presentArgs     map[string]bool // tracks which arguments are present on the field
}

func (p *Planner) SetID(id int) { p.id = id }
func (p *Planner) ID() int      { return p.id }

func (p *Planner) Register(visitor *plan.Visitor, configuration plan.DataSourceConfiguration[Configuration], _ plan.DataSourcePlannerConfiguration) error {
	p.visitor = visitor
	p.dataSourceConfig = configuration
	p.config = Configuration(configuration.CustomConfiguration())

	visitor.Walker.RegisterEnterFieldVisitor(p)
	return nil
}

func (p *Planner) EnterField(ref int) {
	fieldName := p.visitor.Operation.FieldNameString(ref)
	if fieldName != p.config.SearchField {
		return
	}
	p.searchFieldName = fieldName
	p.fieldRef = ref

	// Collect which arguments are present on this field in the operation.
	p.presentArgs = make(map[string]bool)
	for _, argName := range []string{"query", "search", "filter", "sort", "geoSort", "fuzziness", "limit", "offset", "facets", "first", "after", "last", "before", "prefix"} {
		if _, ok := p.visitor.Operation.FieldArgument(ref, []byte(argName)); ok {
			p.presentArgs[argName] = true
		}
	}
}

func (p *Planner) DownstreamResponseFieldAlias(_ int) (alias string, exists bool) {
	return
}

func (p *Planner) ConfigureFetch() resolve.FetchConfiguration {
	source, err := p.factory.CreateSourceForConfig(p.config)
	if err != nil {
		log.Printf("search_datasource: failed to create source: %v", err)
	}

	input := p.buildFetchInput()

	return resolve.FetchConfiguration{
		Input:      input,
		DataSource: source,
		PostProcessing: resolve.PostProcessingConfiguration{
			SelectResponseDataPath: []string{"data"},
		},
	}
}

func (p *Planner) ConfigureSubscription() plan.SubscriptionConfiguration {
	return plan.SubscriptionConfiguration{}
}

func (p *Planner) hasArg(name string) bool {
	return p.presentArgs != nil && p.presentArgs[name]
}

func (p *Planner) buildFetchInput() string {
	// Use {{.arguments.X}} template syntax. The plan.Visitor's resolveInputTemplates
	// method resolves these to proper ContextVariables at plan time.
	input := `{"search_field":"` + p.config.SearchField + `"`

	if p.config.IsSuggest {
		input += `,"is_suggest":true`
	}

	if p.hasArg("prefix") {
		input += `,"prefix":{{.arguments.prefix}}`
	}

	if p.config.HasVectorSearch && p.hasArg("search") {
		input += `,"search":{{.arguments.search}}`
	} else if p.hasArg("query") {
		input += `,"query":{{.arguments.query}}`
	}

	// Only include optional arguments that are actually present in the operation.
	if p.hasArg("filter") {
		input += `,"filter":{{.arguments.filter}}`
	}
	if p.hasArg("sort") {
		input += `,"sort":{{.arguments.sort}}`
	}
	if p.hasArg("limit") {
		input += `,"limit":{{.arguments.limit}}`
	}
	if p.hasArg("offset") {
		input += `,"offset":{{.arguments.offset}}`
	}
	if p.hasArg("geoSort") {
		input += `,"geoSort":{{.arguments.geoSort}}`
	}
	if p.hasArg("fuzziness") {
		input += `,"fuzziness":{{.arguments.fuzziness}}`
	}
	if p.hasArg("facets") {
		input += `,"facets":{{.arguments.facets}}`
	}
	if p.hasArg("first") {
		input += `,"first":{{.arguments.first}}`
	}
	if p.hasArg("after") {
		input += `,"after":{{.arguments.after}}`
	}
	if p.hasArg("last") {
		input += `,"last":{{.arguments.last}}`
	}
	if p.hasArg("before") {
		input += `,"before":{{.arguments.before}}`
	}

	input += `}`
	return input
}
