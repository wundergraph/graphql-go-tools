package resolve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
)

func TestResponseErrorHandlerRawSubgraphErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		mode SubgraphErrorPropagationMode
	}{
		{name: "wrapped", mode: SubgraphErrorPropagationModeWrapped},
		{name: "pass-through", mode: SubgraphErrorPropagationModePassThrough},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := NewContext(t.Context())
			calls := 0
			ctx.ResponseErrorHandler = func(_ context.Context, err error) error {
				value, ok := errors.AsType[*GraphQLError](err)
				require.True(t, ok)
				source, _ := errors.AsType[SubgraphResponseError](err)
				require.NotNil(t, source)
				assert.Equal(t, "products", source.SubgraphName())
				assert.Equal(t, "query", source.FetchPath())
				assert.Equal(t, 503, source.ResponseCode())
				require.NotEmpty(t, source.DownstreamErrors())
				raw := source.DownstreamErrors()[0]
				require.NotNil(t, raw)
				code, ok := raw.Extensions["errorType"].(string)
				require.True(t, ok)
				if tc.mode == SubgraphErrorPropagationModePassThrough && calls == 1 {
					assert.Equal(t, "SECOND", code)
				} else {
					assert.Equal(t, "FIRST", code)
				}
				assert.Nil(t, raw.Extensions["code"], "the hook must not see the defaulted code")
				value.Message = "redacted \"message\"\n"
				require.NotNil(t, value.Extensions)
				value.Extensions["code"] = code
				delete(value.Extensions, "errors")
				calls++
				return err
			}
			loader := &Loader{
				ctx: ctx, subgraphErrorPropagationMode: tc.mode, propagateSubgraphErrors: true,
				allowedSubgraphErrorFields: map[string]struct{}{"message": {}, "extensions": {}, "path": {}},
				defaultErrorExtensionCode:  "DEFAULT", allowedErrorExtensionFields: map[string]struct{}{"code": {}},
			}
			result := &result{ds: DataSourceInfo{Name: "products"}, statusCode: 503}
			require.NoError(t, loader.mergeErrors(result, &FetchItem{ResponsePath: "query"}, astjson.MustParseBytes([]byte(`[
				{"message":"secret1","extensions":{"errorType":"FIRST"}},
				{"message":"secret2","extensions":{"errorType":"SECOND"}}
			]`))))
			before := string(loader.errors.marshalTo(nil))
			var output bytes.Buffer
			r := &Resolvable{ctx: ctx, errors: loader.errors, out: &output}
			r.printResponseErrors()
			if tc.mode == SubgraphErrorPropagationModeWrapped {
				assert.Equal(t, 1, calls)
			} else {
				assert.Equal(t, 2, calls)
			}
			assert.NotContains(t, output.String(), "secret")
			assert.Equal(t, before, string(loader.errors.marshalTo(nil)), "rewrites must not modify engine errors")
			recorded, ok := errors.AsType[*SubgraphError](result.subgraphError)
			require.True(t, ok)
			require.NotEmpty(t, recorded.DownstreamErrors)
			assert.Equal(t, "secret1", recorded.DownstreamErrors[0].Message)
			assert.Equal(t, "DEFAULT", recorded.DownstreamErrors[0].Extensions["code"])
			assert.Nil(t, recorded.DownstreamErrors[0].Extensions["errorType"])
		})
	}
}

func TestResponseErrorHandlerFetchFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status int
	}{
		{name: "transport failure", status: 0},
		{name: "HTTP status fallback", status: 503},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := NewContext(t.Context())
			calls := 0
			ctx.ResponseErrorHandler = func(_ context.Context, err error) error {
				value, ok := errors.AsType[*GraphQLError](err)
				require.True(t, ok)
				source, ok := errors.AsType[SubgraphResponseError](err)
				require.True(t, ok)
				assert.Equal(t, "products", source.SubgraphName())
				assert.Equal(t, tc.status, source.ResponseCode())
				assert.NotEmpty(t, source.Reason())
				assert.Empty(t, source.DownstreamErrors())
				value.Message = "redacted"
				calls++
				return err
			}
			loader := &Loader{ctx: ctx}
			result := &result{ds: DataSourceInfo{Name: "products"}, statusCode: tc.status}
			if tc.status == 0 {
				require.NoError(t, loader.renderErrorsFailedToFetch(&FetchItem{ResponsePath: "query"}, result, "empty response"))
			} else {
				require.NoError(t, loader.renderErrorsStatusFallback(&FetchItem{ResponsePath: "query"}, result, tc.status))
			}
			var output bytes.Buffer
			r := &Resolvable{ctx: ctx, errors: loader.errors, out: &output}
			r.printResponseErrors()
			assert.Equal(t, 1, calls)
			assert.JSONEq(t, `[{"message":"redacted"}]`, output.String())
		})
	}
}

func TestResponseErrorHandlerLocalErrorsAndContextLifecycle(t *testing.T) {
	t.Parallel()

	ctx := NewContext(t.Context())
	ctx.ResponseErrorHandler = func(_ context.Context, err error) error {
		value, ok := errors.AsType[*GraphQLError](err)
		require.True(t, ok)
		source, _ := errors.AsType[SubgraphResponseError](err)
		assert.Nil(t, source)
		value.Message = "redacted"
		return err
	}
	clone := ctx.clone(t.Context())
	var output bytes.Buffer
	r := &Resolvable{ctx: clone, out: &output}
	r.addError("Cannot return null", []string{"field"})
	r.printResponseErrors()
	assert.JSONEq(t, `[{"message":"redacted","path":["field"]}]`, output.String())
	clone.Free()
	assert.Nil(t, clone.ResponseErrorHandler)
}

func TestResponseErrorHandlerSubscriptionSources(t *testing.T) {
	t.Parallel()

	ctx := NewContext(t.Context())
	var sources []string
	ctx.ResponseErrorHandler = func(_ context.Context, err error) error {
		value, ok := errors.AsType[*GraphQLError](err)
		require.True(t, ok)
		source, _ := errors.AsType[SubgraphResponseError](err)
		if source == nil {
			sources = append(sources, "local")
			return err
		}
		sources = append(sources, source.SubgraphName())
		if source.SubgraphName() == "events" {
			require.Len(t, source.DownstreamErrors(), 1)
			assert.Equal(t, source.DownstreamErrors()[0].Message, value.Message)
		}
		value.Message = source.SubgraphName()
		return err
	}
	r := NewResolvable(nil, ResolvableOptions{})
	postProcessing := PostProcessingConfiguration{
		SelectResponseDataPath:   []string{"data"},
		SelectResponseErrorsPath: []string{"errors"},
	}
	require.NoError(t, r.InitSubscription(ctx, []byte(`{"data":{},"errors":[{"message":"event1"},{"message":"event2"}]}`), postProcessing))
	r.setSubscriptionErrorSources(GraphQLSubscriptionTrigger{SourceName: "events"})
	loader := &Loader{ctx: ctx}
	require.NoError(t, loader.renderErrorsFailedToFetch(&FetchItem{ResponsePath: "subscription.product"}, &result{ds: DataSourceInfo{Name: "products"}}, "unavailable"))
	r.errors = append(r.errors, loader.errors...)
	r.addError("Cannot return null", []string{"field"})

	var output bytes.Buffer
	r.out = &output
	r.printResponseErrors()
	assert.Equal(t, []string{"events", "events", "products", "local"}, sources)
	assert.JSONEq(t, `[{"message":"events"},{"message":"events"},{"message":"products"},{"message":"Cannot return null","path":["field"]}]`, output.String())

	// The next event must not retain errors or sources from the previous one.
	require.NoError(t, r.InitSubscription(ctx, []byte(`{"data":{}}`), postProcessing))
	r.setSubscriptionErrorSources(GraphQLSubscriptionTrigger{SourceName: "events"})
	assert.False(t, r.hasErrors())
	r.addError("Cannot return null", []string{"other"})
	sources = nil
	output.Reset()
	r.printResponseErrors()
	assert.Equal(t, []string{"local"}, sources)
}

func TestResponseErrorHandlerDisablesResponseDeduplication(t *testing.T) {
	t.Parallel()

	ctx := NewContext(t.Context())
	ctx.ResponseErrorHandler = func(_ context.Context, err error) error { return err }
	flight := &InboundRequestSingleFlight{}
	request, err := flight.GetOrCreate(ctx, nil)
	assert.NoError(t, err)
	assert.Nil(t, request, "request-specific error rewrites must not be shared with other clients")
}

func TestResponseErrorHandlerReplacement(t *testing.T) {
	t.Parallel()

	const original = `{"message":"original","path":["field",0],"locations":[{"line":1,"column":2}],"extensions":{"code":"ORIGINAL"},"custom":9007199254740993}`
	cases := []struct {
		name    string
		rewrite func(error) error
		want    string
	}{
		{name: "unchanged", rewrite: func(err error) error { return err }, want: original},
		{name: "nil keeps input", rewrite: func(error) error { return nil }, want: original},
		{name: "message only", rewrite: func(error) error { return errors.New("redacted") }, want: `{"message":"redacted","path":["field",0],"locations":[{"line":1,"column":2}],"extensions":{"code":"ORIGINAL"},"custom":9007199254740993}`},
		{name: "wrapper", rewrite: func(err error) error { return fmt.Errorf("wrapped: %w", err) }, want: `{"message":"wrapped: original","path":["field",0],"locations":[{"line":1,"column":2}],"extensions":{"code":"ORIGINAL"},"custom":9007199254740993}`},
		{name: "replace whole error", rewrite: func(error) error {
			return &GraphQLError{Message: "redacted", Extensions: map[string]any{"code": "CUSTOM"}}
		}, want: `{"message":"redacted","extensions":{"code":"CUSTOM"}}`},
		{name: "invalid replacement", rewrite: func(error) error {
			return &GraphQLError{Message: "redacted", Extensions: map[string]any{"bad": make(chan int)}}
		}, want: `{"message":"Internal server error"}`},
		{name: "typed nil", rewrite: func(error) error { return (*GraphQLError)(nil) }, want: `{"message":"Internal server error"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := NewContext(t.Context())
			ctx.ResponseErrorHandler = func(_ context.Context, err error) error { return tc.rewrite(err) }
			var output bytes.Buffer
			value := astjson.MustParse(original)
			r := &Resolvable{ctx: ctx, out: &output, errors: responseErrors{{value: value}}}
			r.printResponseErrors()
			assert.NoError(t, r.printErr)
			assert.JSONEq(t, "["+tc.want+"]", output.String())
			assert.Equal(t, original, value.String())
			if bytes.Contains([]byte(tc.want), []byte("9007199254740993")) {
				assert.Contains(t, output.String(), "9007199254740993")
			}
		})
	}
}

func TestResponseErrorHandlerWrappedMetadata(t *testing.T) {
	t.Parallel()

	ctx := NewContext(t.Context())
	ctx.ResponseErrorHandler = func(_ context.Context, err error) error {
		wrapped := fmt.Errorf("outer: %w", err)
		source, ok := errors.AsType[SubgraphResponseError](wrapped)
		require.True(t, ok)
		assert.Equal(t, "products-id", source.SubgraphID())
		assert.Equal(t, "products", source.SubgraphName())
		assert.Equal(t, "query.products", source.FetchPath())
		assert.Equal(t, "unavailable", source.Reason())
		assert.Equal(t, 503, source.ResponseCode())
		value, ok := errors.AsType[*GraphQLError](wrapped)
		require.True(t, ok)
		assert.Equal(t, []any{"products", json.Number("2")}, value.Path)
		value.Message = "rewritten"
		return wrapped
	}
	var output bytes.Buffer
	r := &Resolvable{ctx: ctx, out: &output, errors: responseErrors{{
		value:  astjson.MustParse(`{"message":"original","path":["products",2]}`),
		source: NewSubgraphError(DataSourceInfo{ID: "products-id", Name: "products"}, "query.products", "unavailable", 503),
	}}}
	r.printResponseErrors()
	// fmt.Errorf captures the message at wrapping time, while the underlying
	// GraphQLError remains available for field changes through errors.AsType.
	assert.JSONEq(t, `[{"message":"outer: original","path":["products",2]}]`, output.String())
}

func TestResponseErrorHandlerNoHook(t *testing.T) {
	// AllocsPerRun cannot run in parallel with other tests.

	ctx := NewContext(t.Context())
	value := astjson.MustParse(`{"message":"original","custom":9007199254740993,"extensions":{"z":1,"a":2}}`)
	r := &Resolvable{ctx: ctx, out: io.Discard, errors: responseErrors{{value: value}}, marshalBuf: make([]byte, 0, 512)}
	assert.Zero(t, testing.AllocsPerRun(100, r.printResponseErrors), "no hook must not convert errors to public types")
	var output bytes.Buffer
	r.out = &output
	r.printResponseErrors()
	assert.Equal(t, "["+value.String()+"]", output.String(), "the default output must preserve field order and numeric precision")

	loader := &Loader{ctx: ctx}
	loader.appendSubgraphResponseError(value, &result{ds: DataSourceInfo{Name: "products"}}, "query", "reason")
	assert.Nil(t, loader.errors[0].source, "hook-only source metadata must not be created without a hook")
}
