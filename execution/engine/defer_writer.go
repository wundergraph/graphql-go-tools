package engine

import "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"

// deferWriterAdapter preserves Execute's subscription-style writer API. That API
// flushes payloads without finality metadata; transports that need it for framing
// should implement resolve.DeferResponseWriter and use the resolver directly.
type deferWriterAdapter struct {
	resolve.SubscriptionResponseWriter
}

func (w deferWriterAdapter) Flush(_ bool) error {
	return w.SubscriptionResponseWriter.Flush()
}
