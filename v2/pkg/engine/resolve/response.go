package resolve

import (
	"fmt"
	"io"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type GraphQLSubscription struct {
	Trigger  GraphQLSubscriptionTrigger
	Response *GraphQLResponse
	Filter   *SubscriptionFilter
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

type GraphQLDeferResponse struct {
	Response *GraphQLResponse
	Defers   []*DeferFetchGroup

	// DeferDescriptors lists every @defer fragment in the operation, keyed by ID.
	// Used to render `pending` entries in the initial response and to look up the
	// path / label of a defer at envelope-render time.
	DeferDescriptors map[int]DeferDescriptor

	// DeferTree is the execution tree built from DeferDescriptors during post-processing.
	// Nil until the buildDeferTree post-processor runs.
	DeferTree *DeferTreeNode
}

// DeferDescriptor describes a single @defer fragment for the incremental-delivery envelope.
type DeferDescriptor struct {
	ID       int      // Valid IDs start with 1.
	ParentID int      // ParentID is the id of the enclosing @defer (0 for top-level).
	Label    string   // Label is the user-supplied label (empty when none);
	Path     []string // Path is the response path of the fragment (where it was mounted in the operation);
}

func (r *GraphQLDeferResponse) QueryPlanString() string {
	indent := func(s string) string {
		return strings.ReplaceAll(s, "\n", "\n    ")
	}

	primary := indent(r.Response.Fetches.QueryPlan().PrettyPrint())
	var secondary []string

	for _, g := range r.Defers {
		secondary = append(secondary, strings.ReplaceAll(g.Fetches.QueryPlan().PrettyPrint(), "\n", "\n    "))
	}

	return fmt.Sprintf(`
QueryPlan {
  Primary {
	%s
  }
  Deferred [
    %s
  ]
}
`, primary, strings.Join(secondary, "\n"))
}

type DeferFetchGroup struct {
	DeferID int
	Fetches *FetchTreeNode
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

type DeferResponseWriter interface {
	ResponseWriter
	Flush() error
	Complete()
}

type SubscriptionResponseWriter interface {
	ResponseWriter
	Flush() error
	Complete()
	Heartbeat() error
	Error(data []byte)
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
