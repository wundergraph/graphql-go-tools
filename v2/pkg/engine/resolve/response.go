package resolve

import (
	"io"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
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
	return r.QueryPlan().PrettyPrint()
}

// QueryPlan returns the structured query plan for the canonical composite
// primary-and-deferred execution tree.
func (r *GraphQLDeferResponse) QueryPlan() *FetchTreeQueryPlanNode {
	return r.PlannedExecutionTree().QueryPlan()
}

type DeferFetchGroup struct {
	DeferID int
	Fetches *FetchTreeNode
}

type GraphQLResponseInfo struct {
	OperationType ast.OperationType
	// AuthorizationCoordinates lists every protected field selected by the operation,
	// deduplicated by {DataSourceID, TypeName, FieldName}. It is populated once per plan by the
	// postprocess package while building the fetch tree, and drives pre-fetch field authorization.
	AuthorizationCoordinates []AuthorizationCoordinate
}

// AuthorizationCoordinate is a protected field coordinate paired with the data source that resolves it.
type AuthorizationCoordinate struct {
	DataSourceID string
	Coordinate   GraphCoordinate
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

func writeGraphqlResponse(buf *BufPair, writer io.Writer, ignoreData bool) (err error) {
	hasErrors := buf.Errors.Len() != 0
	hasData := buf.Data.Len() != 0 && !ignoreData

	err = writeSafe(err, writer, lBrace)

	if hasErrors {
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, literalErrors)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, colon)
		err = writeSafe(err, writer, lBrack)
		err = writeSafe(err, writer, buf.Errors.Bytes())
		err = writeSafe(err, writer, rBrack)
		err = writeSafe(err, writer, comma)
	}

	err = writeSafe(err, writer, quote)
	err = writeSafe(err, writer, literalData)
	err = writeSafe(err, writer, quote)
	err = writeSafe(err, writer, colon)

	if hasData {
		_, err = writer.Write(buf.Data.Bytes())
	} else {
		err = writeSafe(err, writer, literal.NULL)
	}
	err = writeSafe(err, writer, rBrace)

	return err
}

func writeSafe(err error, writer io.Writer, data []byte) error {
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
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
