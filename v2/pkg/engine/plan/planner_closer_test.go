package plan

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestCloser(t *testing.T) {

	definition := `schema {query:Query} type Query { me: String! }`
	operation := `{me}`

	def := unsafeparser.ParseGraphqlDocumentString(definition)
	op := unsafeparser.ParseGraphqlDocumentString(operation)
	err := asttransform.MergeDefinitionWithBaseSchema(&def)
	if err != nil {
		t.Fatal(err)
	}
	norm := astnormalization.NewNormalizer(true, true)
	report := &operationreport.Report{}
	norm.NormalizeOperation(&op, &def, report)
	valid := astvalidation.DefaultOperationValidator()
	valid.Validate(&op, &def, report)

	ctx, cancel := context.WithCancel(context.Background())
	closedSignal := make(chan struct{})

	factory := &FakeFactory{
		signalClosed: closedSignal,
	}

	cfg := Configuration{
		DefaultFlushIntervalMillis: 500,
		DataSources: []DataSourceConfiguration{
			{
				RootNodes: []TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"me"},
					},
				},
				ChildNodes: nil,
				Factory:    factory,
				Custom:     nil,
			},
		},
		Fields: nil,
	}

	p := NewPlanner(ctx, cfg)
	plan := p.Plan(&op, &def, "", report)
	assert.NotNil(t, plan)

	cancel()     // terminate all stateful sources
	<-ctx.Done() // stateful source closed from closer
	<-closedSignal
	// test terminates only if stateful source closed
}

type StatefulSource struct {
	signalClosed chan struct{}
}

func (s *StatefulSource) Start(ctx context.Context) {
	<-ctx.Done()
	close(s.signalClosed)
}

type FakeFactory struct {
	signalClosed chan struct{}
}

func (f *FakeFactory) Planner(ctx context.Context) DataSourcePlanner {
	source := &StatefulSource{
		signalClosed: f.signalClosed,
	}
	go source.Start(ctx)
	return &FakePlanner{
		source: source,
	}
}

type FakePlanner struct {
	source *StatefulSource
}

func (f *FakePlanner) EnterDocument(operation, definition *ast.Document) {

}

func (f *FakePlanner) Register(visitor *Visitor, _ DataSourceConfiguration, _ DataSourcePlannerConfiguration) error {
	visitor.Walker.RegisterEnterDocumentVisitor(f)
	return nil
}

func (f *FakePlanner) ConfigureFetch() FetchConfiguration {
	return FetchConfiguration{
		DataSource: &FakeDataSource{
			source: f.source,
		},
	}
}

func (f *FakePlanner) ConfigureSubscription() SubscriptionConfiguration {
	return SubscriptionConfiguration{}
}

func (f *FakePlanner) DataSourcePlanningBehavior() DataSourcePlanningBehavior {
	return DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      false,
		OverrideFieldPathFromAlias: false,
	}
}

func (f *FakePlanner) DownstreamResponseFieldAlias(downstreamFieldRef int) (alias string, exists bool) {
	return
}

type FakeDataSource struct {
	source *StatefulSource
}

func (f *FakeDataSource) Load(ctx context.Context, input []byte, w io.Writer) (err error) {
	return
}
