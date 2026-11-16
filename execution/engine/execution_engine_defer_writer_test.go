package engine

import (
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/staticdatasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type testExecutionDeferWriter struct {
	*graphql.EngineResultWriter
	payloads []string
	finals   []bool
	complete bool
}

var _ resolve.DeferResponseWriter = (*testExecutionDeferWriter)(nil)

func (w *testExecutionDeferWriter) Flush(final bool) error {
	w.payloads = append(w.payloads, w.String())
	w.finals = append(w.finals, final)
	return w.EngineResultWriter.Flush()
}

func (w *testExecutionDeferWriter) Complete() {
	w.complete = true
	w.EngineResultWriter.Complete()
}

func TestExecutionEngine_Execute_DeferWriter(t *testing.T) {
	t.Parallel()

	schema, err := graphql.NewSchemaFromString(`type Query { hello: String }`)
	require.NoError(t, err)
	ds, err := plan.NewDataSourceConfiguration[staticdatasource.Configuration](
		"static",
		&staticdatasource.Factory[staticdatasource.Configuration]{},
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{{TypeName: "Query", FieldNames: []string{"hello"}}},
		},
		staticdatasource.Configuration{Data: `{"hello":"world"}`},
	)
	require.NoError(t, err)
	configuration := NewConfiguration(schema)
	configuration.SetDataSources([]plan.DataSource{ds})
	engine, err := NewExecutionEngine(t.Context(), abstractlogger.Noop{}, configuration, resolve.ResolverOptions{MaxConcurrency: 1})
	require.NoError(t, err)

	t.Run("requires a dedicated writer before emitting data", func(t *testing.T) {
		operation := graphql.Request{Query: `{ ... @defer { hello } }`}
		writer := graphql.NewEngineResultWriter()
		err := engine.Execute(t.Context(), &operation, &writer)
		require.EqualError(t, err, "execution impossible: @defer requires WithDeferResponseWriter")
		require.Empty(t, writer.String())
	})

	t.Run("delivers finality to the dedicated writer", func(t *testing.T) {
		operation := graphql.Request{Query: `{ ... @defer { hello } }`}
		writer := graphql.NewEngineResultWriter()
		deferResult := graphql.NewEngineResultWriter()
		deferWriter := &testExecutionDeferWriter{EngineResultWriter: &deferResult}
		err := engine.Execute(t.Context(), &operation, &writer, WithDeferResponseWriter(deferWriter))
		require.NoError(t, err)
		require.Empty(t, writer.String(), "defer frames must not use the subscription writer")
		require.Equal(t, []bool{false, true}, deferWriter.finals)
		require.Len(t, deferWriter.payloads, 2)
		require.Contains(t, deferWriter.payloads[1], `"hasNext":false`)
		require.True(t, deferWriter.complete)
	})

	t.Run("does not reuse the previous execution's writer", func(t *testing.T) {
		operation := graphql.Request{Query: `{ ... @defer { hello } }`}
		writer := graphql.NewEngineResultWriter()
		err := engine.Execute(t.Context(), &operation, &writer)
		require.EqualError(t, err, "execution impossible: @defer requires WithDeferResponseWriter")
	})

	t.Run("a normalized synchronous plan needs no defer writer", func(t *testing.T) {
		operation := graphql.Request{Query: `{ ... @defer(if: false) { hello } }`}
		writer := graphql.NewEngineResultWriter()
		err := engine.Execute(t.Context(), &operation, &writer)
		require.NoError(t, err)
		require.JSONEq(t, `{"data":{"hello":"world"}}`, writer.String())
	})
}
