package resolve

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"
)

// ResponseErrorHandler runs immediately before each execution error is serialized,
// including errors in deferred payloads and subscription events. Use errors.AsType
// to access the mutable *GraphQLError or optional SubgraphResponseError metadata.
// The GraphQLError is detached from the engine's response and telemetry data.
//
// Returning the input error (or nil) keeps that error, including mutations made to
// it. Returning an error containing a *GraphQLError replaces the complete error
// object; any other error replaces only the message. The returned error's Error()
// supplies the message, so wrapping an error also changes its response message.
// A result that cannot be encoded as JSON becomes an "Internal server error".
//
// Implementations must be concurrency-safe and must not retain the input or
// modify subgraph metadata. No hook-related conversion takes place when nil.
type ResponseErrorHandler func(ctx context.Context, err error) error

// SubgraphResponseError identifies an execution error associated with a subgraph.
// Its underlying *GraphQLError is accessible through errors.AsType. FetchPath is
// the engine's fetch path, distinct from GraphQLError.Path in the client response.
// ResponseCode is zero when the HTTP status is unavailable.
//
// DownstreamErrors contains the original errors before path rewriting, extension
// filtering, and default code insertion: all errors in wrapped mode, or the single
// corresponding error in pass-through mode. Transport failures have no downstream
// errors. All metadata and downstream errors must be treated as read-only.
type SubgraphResponseError interface {
	error
	SubgraphID() string
	SubgraphName() string
	FetchPath() string
	Reason() string
	ResponseCode() int
	DownstreamErrors() []*GraphQLError
}

type subgraphResponseError struct {
	err    *GraphQLError
	source *SubgraphError
}

func (e *subgraphResponseError) Error() string                     { return e.err.Error() }
func (e *subgraphResponseError) Unwrap() error                     { return e.err }
func (e *subgraphResponseError) SubgraphID() string                { return e.source.DataSourceInfo.ID }
func (e *subgraphResponseError) SubgraphName() string              { return e.source.DataSourceInfo.Name }
func (e *subgraphResponseError) FetchPath() string                 { return e.source.Path }
func (e *subgraphResponseError) Reason() string                    { return e.source.Reason }
func (e *subgraphResponseError) ResponseCode() int                 { return e.source.ResponseCode }
func (e *subgraphResponseError) DownstreamErrors() []*GraphQLError { return e.source.DownstreamErrors }

var _ SubgraphResponseError = (*subgraphResponseError)(nil)

// responseError keeps an outgoing error and its optional subgraph source together
// as errors move from fetch loaders into the response, including deferred groups
// and subscription events. The value may be arena-owned; source is heap-owned.
type responseError struct {
	value  *astjson.Value
	source *SubgraphError
}

type responseErrors []responseError

func (e responseErrors) marshalTo(dst []byte) []byte {
	dst = append(dst, '[')
	for i := range e {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = e[i].value.MarshalTo(dst)
	}
	return append(dst, ']')
}

// With a hook, only individual error objects are decoded, never the response data.
// Handler results are serialized directly, without storing heap values in an arena.
func (r *Resolvable) printResponseErrors() {
	if r.printErr != nil {
		return
	}
	if r.ctx == nil || r.ctx.ResponseErrorHandler == nil {
		r.marshalBuf = r.errors.marshalTo(r.marshalBuf[:0])
		r.printBytes(r.marshalBuf)
		return
	}
	r.printBytes(lBrack)
	for i, entry := range r.errors {
		if i > 0 {
			r.printBytes(comma)
		}
		r.printResponseError(entry)
	}
	r.printBytes(rBrack)
}

func (r *Resolvable) printResponseError(entry responseError) {
	if r.printErr != nil {
		return
	}
	var original GraphQLError
	r.marshalBuf = entry.value.MarshalTo(r.marshalBuf[:0])
	if err := json.Unmarshal(r.marshalBuf, &original); err != nil {
		r.printBytes([]byte(`{"message":"Internal server error"}`))
		return
	}
	var input error = &original
	if entry.source != nil {
		input = &subgraphResponseError{err: &original, source: entry.source}
	}
	rewritten := r.ctx.ResponseErrorHandler(r.ctx.Context(), input)
	if rewritten == nil {
		rewritten = input
	}
	output, ok := errors.AsType[*GraphQLError](rewritten)
	if !ok {
		output = &original
	} else if output == nil {
		// A typed nil GraphQLError cannot supply a response message.
		r.printBytes([]byte(`{"message":"Internal server error"}`))
		return
	}
	copy := *output
	copy.Message = rewritten.Error()
	encoded, err := json.Marshal(copy)
	if err != nil {
		// A failed redaction must not fall back to potentially sensitive details.
		r.printBytes([]byte(`{"message":"Internal server error"}`))
		return
	}
	r.printBytes(encoded)
}

func (l *Loader) appendSubgraphResponseError(value *astjson.Value, res *result, path, reason string) {
	entry := responseError{value: value}
	if l.ctx.ResponseErrorHandler != nil {
		entry.source = NewSubgraphError(res.ds, path, reason, res.statusCode)
	}
	l.errors = append(l.errors, entry)
}

func (r *Resolvable) appendError(message string) *astjson.Value {
	value := fastjsonext.CreateErrorObjectWithPath(r.astjsonArena, message, r.path)
	r.errors = append(r.errors, responseError{value: value})
	return value
}

// Subscription event errors bypass the fetch loader. Preserve their source before
// combining them with errors from the event's follow-up fetches.
func (r *Resolvable) setSubscriptionErrorSources(trigger GraphQLSubscriptionTrigger) {
	if r.ctx.ResponseErrorHandler == nil || r.errors == nil {
		return
	}
	var rawErrors []*GraphQLError
	if err := json.Unmarshal(r.errors.marshalTo(nil), &rawErrors); err != nil {
		return
	}
	for i := range r.errors {
		source := NewSubgraphError(DataSourceInfo{ID: trigger.SourceID, Name: trigger.SourceName}, "subscription", "", 0)
		source.DownstreamErrors = rawErrors[i : i+1]
		r.errors[i].source = source
	}
}
