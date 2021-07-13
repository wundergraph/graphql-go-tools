package plan

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astnormalization"
	"github.com/jensneuse/graphql-go-tools/pkg/asttransform"
	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
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
	closer := make(chan struct{})
	closeSignal := make(chan struct{})

	factory := &FakeFactory{
		signalClosed: closeSignal,
	}

	cfg := Configuration{
		DefaultFlushInterval: 500,
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
	p := NewPlanner(cfg, closer)
	plan := p.Plan(&op, &def, "", report)
	assert.NotNil(t, plan)

	close(closer) // terminate all stateful sources
	<-closeSignal // stateful source closed from closer
	// test terminates only if stateful source closed
}

type StatefulSource struct {
	signalClosed chan struct{}
}

func (s *StatefulSource) Start(closer <-chan struct{}){
	<-closer
	close(s.signalClosed)
}

type FakeFactory struct {
	signalClosed chan struct{}
}

func (f *FakeFactory) Planner(closer <-chan struct{}) DataSourcePlanner {
	source := &StatefulSource{
		signalClosed: f.signalClosed,
	}
	go source.Start(closer)
	return &FakePlanner{
		source: source,
	}
}

type FakePlanner struct {
	source *StatefulSource
}

func (f *FakePlanner) EnterDocument(operation, definition *ast.Document) {

}

func (f *FakePlanner) Register(visitor *Visitor, customConfiguration json.RawMessage, isNested bool) error {
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

func (f *FakeDataSource) Load(ctx context.Context, input []byte, bufPair *resolve.BufPair) (err error) {
	return
}

func (f *FakeDataSource) UniqueIdentifier() []byte {
	return []byte("fake_datasource")
}
