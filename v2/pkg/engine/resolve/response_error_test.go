package resolve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
)

func TestResponseErrorHandlerRawSubgraphErrors(t *testing.T) {
	for _, mode := range []SubgraphErrorPropagationMode{SubgraphErrorPropagationModeWrapped, SubgraphErrorPropagationModePassThrough} {
		t.Run(strconv.Itoa(int(mode)), func(t *testing.T) {
			ctx := NewContext(t.Context())
			calls := 0
			ctx.ResponseErrorHandler = func(_ context.Context, err error) error {
				value, ok := errors.AsType[*GraphQLError](err)
				require.True(t, ok)
				source, _ := errors.AsType[SubgraphResponseError](err)
				require.NotNil(t, source)
				require.Equal(t, "products", source.SubgraphName())
				require.Equal(t, "query", source.FetchPath())
				require.Equal(t, 503, source.ResponseCode())
				raw := source.DownstreamErrors()[0]
				code := raw.Extensions["errorType"].(string)
				if mode == SubgraphErrorPropagationModePassThrough && calls == 1 {
					require.Equal(t, "SECOND", code)
				} else {
					require.Equal(t, "FIRST", code)
				}
				require.Nil(t, raw.Extensions["code"], "the hook must not see the defaulted code")
				value.Message = "redacted \"message\"\n"
				value.Extensions["code"] = code
				delete(value.Extensions, "errors")
				calls++
				return err
			}
			loader := &Loader{
				ctx: ctx, subgraphErrorPropagationMode: mode, propagateSubgraphErrors: true,
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
			if mode == SubgraphErrorPropagationModeWrapped {
				require.Equal(t, 1, calls)
			} else {
				require.Equal(t, 2, calls)
			}
			require.NotContains(t, output.String(), "secret")
			require.Equal(t, before, string(loader.errors.marshalTo(nil)), "rewrites must not modify engine errors")
			recorded, ok := errors.AsType[*SubgraphError](result.subgraphError)
			require.True(t, ok)
			require.Equal(t, "secret1", recorded.DownstreamErrors[0].Message)
			require.Equal(t, "DEFAULT", recorded.DownstreamErrors[0].Extensions["code"])
			require.Nil(t, recorded.DownstreamErrors[0].Extensions["errorType"])
		})
	}
}

func TestResponseErrorHandlerFetchFailures(t *testing.T) {
	for _, status := range []int{0, 503} {
		ctx := NewContext(t.Context())
		calls := 0
		ctx.ResponseErrorHandler = func(_ context.Context, err error) error {
			value, ok := errors.AsType[*GraphQLError](err)
			require.True(t, ok)
			source, _ := errors.AsType[SubgraphResponseError](err)
			require.Equal(t, "products", source.SubgraphName())
			require.Equal(t, status, source.ResponseCode())
			require.NotEmpty(t, source.Reason())
			require.Empty(t, source.DownstreamErrors())
			value.Message = "redacted"
			calls++
			return err
		}
		loader := &Loader{ctx: ctx}
		result := &result{ds: DataSourceInfo{Name: "products"}, statusCode: status}
		if status == 0 {
			require.NoError(t, loader.renderErrorsFailedToFetch(&FetchItem{ResponsePath: "query"}, result, "empty response"))
		} else {
			require.NoError(t, loader.renderErrorsStatusFallback(&FetchItem{ResponsePath: "query"}, result, status))
		}
		var output bytes.Buffer
		r := &Resolvable{ctx: ctx, errors: loader.errors, out: &output}
		r.printResponseErrors()
		require.Equal(t, 1, calls)
		require.JSONEq(t, `[{"message":"redacted"}]`, output.String())
	}
}

func TestResponseErrorHandlerLocalErrorsAndContextLifecycle(t *testing.T) {
	ctx := NewContext(t.Context())
	ctx.ResponseErrorHandler = func(_ context.Context, err error) error {
		value, ok := errors.AsType[*GraphQLError](err)
		require.True(t, ok)
		source, _ := errors.AsType[SubgraphResponseError](err)
		require.Nil(t, source)
		value.Message = "redacted"
		return err
	}
	clone := ctx.clone(t.Context())
	var output bytes.Buffer
	r := &Resolvable{ctx: clone, out: &output}
	r.addError("Cannot return null", []string{"field"})
	r.printResponseErrors()
	require.JSONEq(t, `[{"message":"redacted","path":["field"]}]`, output.String())
	clone.Free()
	require.Nil(t, clone.ResponseErrorHandler)
}

func TestResponseErrorHandlerSubscriptionSources(t *testing.T) {
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
			require.Equal(t, source.DownstreamErrors()[0].Message, value.Message)
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
	require.Equal(t, []string{"events", "events", "products", "local"}, sources)
	require.JSONEq(t, `[{"message":"events"},{"message":"events"},{"message":"products"},{"message":"Cannot return null","path":["field"]}]`, output.String())

	// The next event must not retain errors or sources from the previous one.
	require.NoError(t, r.InitSubscription(ctx, []byte(`{"data":{}}`), postProcessing))
	r.setSubscriptionErrorSources(GraphQLSubscriptionTrigger{SourceName: "events"})
	require.False(t, r.hasErrors())
	r.addError("Cannot return null", []string{"other"})
	sources = nil
	output.Reset()
	r.printResponseErrors()
	require.Equal(t, []string{"local"}, sources)
}

func TestResponseErrorHandlerDisablesResponseDeduplication(t *testing.T) {
	ctx := NewContext(t.Context())
	ctx.ResponseErrorHandler = func(_ context.Context, err error) error { return err }
	flight := &InboundRequestSingleFlight{}
	request, err := flight.GetOrCreate(ctx, nil)
	require.NoError(t, err)
	require.Nil(t, request, "request-specific error rewrites must not be shared with other clients")
}

func TestResponseErrorHandlerReplacement(t *testing.T) {
	const original = `{"message":"original","path":["field",0],"locations":[{"line":1,"column":2}],"extensions":{"code":"ORIGINAL"},"custom":9007199254740993}`
	for _, tc := range []struct {
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := NewContext(t.Context())
			ctx.ResponseErrorHandler = func(_ context.Context, err error) error { return tc.rewrite(err) }
			var output bytes.Buffer
			value := astjson.MustParse(original)
			r := &Resolvable{ctx: ctx, out: &output, errors: responseErrors{{value: value}}}
			r.printResponseErrors()
			require.NoError(t, r.printErr)
			require.JSONEq(t, "["+tc.want+"]", output.String())
			require.Equal(t, original, value.String())
			if bytes.Contains([]byte(tc.want), []byte("9007199254740993")) {
				require.Contains(t, output.String(), "9007199254740993")
			}
		})
	}
}

func TestResponseErrorHandlerWrappedMetadata(t *testing.T) {
	ctx := NewContext(t.Context())
	ctx.ResponseErrorHandler = func(_ context.Context, err error) error {
		wrapped := fmt.Errorf("outer: %w", err)
		source, ok := errors.AsType[SubgraphResponseError](wrapped)
		require.True(t, ok)
		require.Equal(t, "products-id", source.SubgraphID())
		require.Equal(t, "products", source.SubgraphName())
		require.Equal(t, "query.products", source.FetchPath())
		require.Equal(t, "unavailable", source.Reason())
		require.Equal(t, 503, source.ResponseCode())
		value, ok := errors.AsType[*GraphQLError](wrapped)
		require.True(t, ok)
		require.Equal(t, []any{"products", json.Number("2")}, value.Path)
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
	require.JSONEq(t, `[{"message":"outer: original","path":["products",2]}]`, output.String())
}

func TestResponseErrorHandlerNoHook(t *testing.T) {
	ctx := NewContext(t.Context())
	value := astjson.MustParse(`{"message":"original","custom":9007199254740993,"extensions":{"z":1,"a":2}}`)
	r := &Resolvable{ctx: ctx, out: io.Discard, errors: responseErrors{{value: value}}, marshalBuf: make([]byte, 0, 512)}
	require.Zero(t, testing.AllocsPerRun(100, r.printResponseErrors), "no hook must not convert errors to public types")
	var output bytes.Buffer
	r.out = &output
	r.printResponseErrors()
	require.Equal(t, "["+value.String()+"]", output.String(), "the default output must preserve field order and numeric precision")

	loader := &Loader{ctx: ctx}
	loader.appendSubgraphResponseError(value, &result{ds: DataSourceInfo{Name: "products"}}, "query", "reason")
	require.Nil(t, loader.errors[0].source, "hook-only source metadata must not be created without a hook")
}
