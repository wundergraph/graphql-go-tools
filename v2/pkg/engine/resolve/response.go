package resolve

import (
	"io"
	"time"

	"github.com/gobwas/ws"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// SubscriptionCacheMode determines how subscription events manage L2 cache entries.
type SubscriptionCacheMode int

const (
	// SubscriptionCacheModePopulate writes entity data to L2 on each subscription event.
	SubscriptionCacheModePopulate SubscriptionCacheMode = iota
	// SubscriptionCacheModeInvalidate deletes the L2 cache entry on each subscription event.
	SubscriptionCacheModeInvalidate
)

// SubscriptionEntityCachePopulation configures how the resolver manages L2 cache
// entries for root entities received via subscription events.
type SubscriptionEntityCachePopulation struct {
	// Mode determines whether to populate or invalidate L2 cache entries.
	Mode SubscriptionCacheMode
	// CacheKeyTemplate generates cache keys from entity @key fields.
	CacheKeyTemplate *EntityQueryCacheKeyTemplate
	// CacheName identifies which LoaderCache instance to use.
	CacheName string
	// TTL is the time-to-live for populated cache entries (only used in Populate mode).
	TTL time.Duration
	// IncludeSubgraphHeaderPrefix controls whether forwarded headers affect cache keys.
	IncludeSubgraphHeaderPrefix bool
	// DataSourceName is the subgraph name for SubgraphHeadersBuilder lookup.
	DataSourceName string
	// SubscriptionFieldName is the name of the subscription root field (e.g., "updateProductPrice").
	// Used to navigate from the subscription data root to the entity data.
	SubscriptionFieldName string
	// EntityTypeName is the entity type name (e.g., "Product") used to set __typename in cache keys.
	EntityTypeName string
}

type GraphQLSubscription struct {
	Trigger               GraphQLSubscriptionTrigger
	Response              *GraphQLResponse
	Filter                *SubscriptionFilter
	EntityCachePopulation *SubscriptionEntityCachePopulation
}

type GraphQLSubscriptionTrigger struct {
	Input          []byte
	InputTemplate  InputTemplate
	Variables      Variables
	Source         SubscriptionDataSource
	PostProcessing PostProcessingConfiguration
	QueryPlan      *QueryPlan
	SourceName     string
	SourceID       string
}

// GraphQLResponse contains an ordered tree of fetches and the response shape.
// Fields are filled in this order:
//
//  1. Planner fills RawFetches and Info fields.
//  2. PostProcessor processes RawFetches to build DataSources and Fetches.
//  3. Loader executes Fetches to collect all JSON data.
//  4. Resolver uses Data to create a final JSON shape that is returned as a response.
type GraphQLResponse struct {
	Data *Object

	RawFetches []*FetchItem
	Fetches    *FetchTreeNode

	Info        *GraphQLResponseInfo
	DataSources []DataSourceInfo
}

func (g *GraphQLResponse) SingleFlightAllowed() bool {
	if g == nil {
		return false
	}
	if g.Info == nil {
		return false
	}
	if g.Info.OperationType == ast.OperationTypeQuery {
		return true
	}
	return false
}

type GraphQLResponseInfo struct {
	OperationType ast.OperationType
}

type RenameTypeName struct {
	From, To []byte
}

type ResponseWriter interface {
	io.Writer
}

type SubscriptionCloseKind struct {
	WSCode ws.StatusCode
	Reason string
}

var (
	SubscriptionCloseKindNormal                 SubscriptionCloseKind = SubscriptionCloseKind{ws.StatusNormalClosure, "Normal closure"}
	SubscriptionCloseKindDownstreamServiceError SubscriptionCloseKind = SubscriptionCloseKind{ws.StatusGoingAway, "Downstream service error"}
	SubscriptionCloseKindGoingAway              SubscriptionCloseKind = SubscriptionCloseKind{ws.StatusGoingAway, "Going away"}
)

type SubscriptionResponseWriter interface {
	ResponseWriter
	Flush() error
	Complete()
	Heartbeat() error
	Close(kind SubscriptionCloseKind)
}

func writeFlushComplete(writer SubscriptionResponseWriter, msg []byte) error {
	_, err := writer.Write(msg)
	if err != nil {
		return err
	}
	err = writer.Flush()
	if err != nil {
		return err
	}
	writer.Complete()
	return nil
}
