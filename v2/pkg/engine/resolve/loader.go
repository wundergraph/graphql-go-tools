package resolve

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash/v2"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/sync/errgroup"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/errorcodes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
)

const (
	IntrospectionSchemaTypeDataSourceID     = "introspection__schema&__type"
	IntrospectionTypeFieldsDataSourceID     = "introspection__type__fields"
	IntrospectionTypeEnumValuesDataSourceID = "introspection__type__enumValues"
)

type LoaderHooks interface {
	// OnLoad is called before the fetch is executed
	OnLoad(ctx context.Context, ds DataSourceInfo) context.Context
	// OnFinished is called after the fetch has been executed and the response has been processed and merged
	OnFinished(ctx context.Context, ds DataSourceInfo, info *ResponseInfo)
}

type DataSourceInfo struct {
	ID   string
	Name string
}

type ResponseInfo struct {
	StatusCode int
	Err        error
	// Request is the original request that was sent to the subgraph. This should only be used for reading purposes,
	// in order to ensure there aren't memory conflicts, and the body will be nil, as it was read already.
	Request *http.Request
	// ResponseHeaders contains a clone of the headers of the response from the subgraph.
	ResponseHeaders http.Header
	// This should be private as we do not want user's to access the raw responseBody directly
	responseBody *bytes.Buffer
}

func (ri *ResponseInfo) GetResponseBody() string {
	return ri.responseBody.String()
}

func newResponseInfo(res *result, subgraphError error) *ResponseInfo {
	responseInfo := &ResponseInfo{
		StatusCode:   res.statusCode,
		Err:          goerrors.Join(res.err, subgraphError),
		responseBody: res.out,
	}
	if res.httpResponseContext != nil {
		// We're using the response.Request here, because the body will be nil (since the response was read) and won't
		// cause a memory leak.
		if res.httpResponseContext.Response != nil {
			if res.httpResponseContext.Response.Request != nil {
				responseInfo.Request = res.httpResponseContext.Response.Request
			}

			if res.httpResponseContext.Response.Header != nil {
				responseInfo.ResponseHeaders = res.httpResponseContext.Response.Header.Clone()
			}
		}

		if responseInfo.Request == nil {
			// In cases where the request errors, the response will be nil, and so we need to get the original request
			responseInfo.Request = res.httpResponseContext.Request
		}
	}

	return responseInfo
}

// batchStats represents an index map for batched items.
// It is used to ensure that the correct json values will be merged with the correct items from the batch.
//
// Example:
// [[0],[1],[0],[1]] We originally have 4 items, but we have 2 unique indexes (0 and 1).
// This means we are deduplicating 2 items by merging them from their response entity indexes.
// 0 -> 0, 1 -> 1, 2 -> 0, 3 -> 1
type batchStats [][]int

// getUniqueIndexes returns the number of unique indexes in the batchStats.
// This is used to ensure that we can provide a valid error message in case of differing array lengths.
func (b *batchStats) getUniqueIndexes() int {
	uniqueIndexes := make(map[int]struct{})
	for _, bi := range *b {
		for _, index := range bi {
			if index < 0 {
				continue
			}
			uniqueIndexes[index] = struct{}{}
		}
	}

	return len(uniqueIndexes)
}

type result struct {
	postProcessing   PostProcessingConfiguration
	out              *bytes.Buffer
	batchStats       batchStats
	fetchSkipped     bool
	nestedMergeItems []*result

	statusCode int
	err        error
	ds         DataSourceInfo

	authorizationRejected        bool
	authorizationRejectedReasons []string

	rateLimitRejected       bool
	rateLimitRejectedReason string

	// loaderHookContext used to share data between the OnLoad and OnFinished hooks
	// It should be valid even when OnLoad isn't called
	loaderHookContext context.Context

	httpResponseContext *httpclient.ResponseContext
}

func (r *result) init(postProcessing PostProcessingConfiguration, info *FetchInfo) {
	r.postProcessing = postProcessing
	if info != nil {
		r.ds = DataSourceInfo{
			ID:   info.DataSourceID,
			Name: info.DataSourceName,
		}
	}
}

func IsIntrospectionDataSource(dataSourceID string) bool {
	return dataSourceID == IntrospectionSchemaTypeDataSourceID || dataSourceID == IntrospectionTypeFieldsDataSourceID || dataSourceID == IntrospectionTypeEnumValuesDataSourceID
}

type Loader struct {
	resolvable *Resolvable
	ctx        *Context
	info       *GraphQLResponseInfo

	propagateSubgraphErrors           bool
	propagateSubgraphStatusCodes      bool
	subgraphErrorPropagationMode      SubgraphErrorPropagationMode
	rewriteSubgraphErrorPaths         bool
	omitSubgraphErrorLocations        bool
	omitSubgraphErrorExtensions       bool
	attachServiceNameToErrorExtension bool

	allowAllErrorExtensionFields bool
	allowedErrorExtensionFields  map[string]struct{}
	defaultErrorExtensionCode    string

	allowedSubgraphErrorFields map[string]struct{}

	apolloRouterCompatibilitySubrequestHTTPError bool

	propagateFetchReasons bool

	validateRequiredExternalFields bool

	// taintedEntities tracks entities fetched with errors.
	// Later fetches should ignore those entities.
	taintedEntities map[*astjson.Value]struct{}
}

func (l *Loader) Free() {
	l.info = nil
	l.ctx = nil
	l.resolvable = nil
	l.taintedEntities = nil
}

func (l *Loader) LoadGraphQLResponseData(ctx *Context, response *GraphQLResponse, resolvable *Resolvable) (err error) {
	l.resolvable = resolvable
	l.ctx = ctx
	l.info = response.Info
	l.taintedEntities = make(map[*astjson.Value]struct{})
	return l.resolveFetchNode(response.Fetches)
}

func (l *Loader) resolveFetchNode(node *FetchTreeNode) error {
	if node == nil {
		return nil
	}
	switch node.Kind {
	case FetchTreeNodeKindSingle:
		return l.resolveSingle(node.Item)
	case FetchTreeNodeKindSequence:
		return l.resolveSerial(node.ChildNodes)
	case FetchTreeNodeKindParallel:
		return l.resolveParallel(node.ChildNodes)
	default:
		return nil
	}
}

func (l *Loader) resolveParallel(nodes []*FetchTreeNode) error {
	if len(nodes) == 0 {
		return nil
	}
	results := make([]*result, len(nodes))
	itemsItems := make([][]*astjson.Value, len(nodes))
	g, ctx := errgroup.WithContext(l.ctx.ctx)
	for i := range nodes {
		i := i
		results[i] = &result{}
		itemsItems[i] = l.selectItemsForPath(nodes[i].Item.FetchPath)
		f := nodes[i].Item.Fetch
		item := nodes[i].Item
		items := itemsItems[i]
		res := results[i]
		g.Go(func() error {
			return l.loadFetch(ctx, f, item, items, res)
		})
	}
	err := g.Wait()
	if err != nil {
		return errors.WithStack(err)
	}
	for i := range results {
		if results[i].nestedMergeItems != nil {
			for j := range results[i].nestedMergeItems {
				err = l.mergeResult(nodes[i].Item, results[i].nestedMergeItems[j], itemsItems[i][j:j+1])
				if l.ctx.LoaderHooks != nil && results[i].nestedMergeItems[j].loaderHookContext != nil {
					l.ctx.LoaderHooks.OnFinished(results[i].nestedMergeItems[j].loaderHookContext,
						results[i].nestedMergeItems[j].ds,
						newResponseInfo(results[i].nestedMergeItems[j], l.ctx.subgraphErrors))
				}
				if err != nil {
					return errors.WithStack(err)
				}
			}
		} else {
			err = l.mergeResult(nodes[i].Item, results[i], itemsItems[i])
			if l.ctx.LoaderHooks != nil {
				l.ctx.LoaderHooks.OnFinished(results[i].loaderHookContext, results[i].ds, newResponseInfo(results[i], l.ctx.subgraphErrors))
			}
			if err != nil {
				return errors.WithStack(err)
			}
		}
	}
	return nil
}

func (l *Loader) resolveSerial(nodes []*FetchTreeNode) error {
	for i := range nodes {
		err := l.resolveFetchNode(nodes[i])
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (l *Loader) resolveSingle(item *FetchItem) error {
	if item == nil {
		return nil
	}
	items := l.selectItemsForPath(item.FetchPath)

	switch f := item.Fetch.(type) {
	case *SingleFetch:
		res := &result{
			out: &bytes.Buffer{},
		}
		err := l.loadSingleFetch(l.ctx.ctx, f, item, items, res)
		if err != nil {
			return err
		}
		err = l.mergeResult(item, res, items)
		if l.ctx.LoaderHooks != nil {
			l.ctx.LoaderHooks.OnFinished(res.loaderHookContext, res.ds, newResponseInfo(res, l.ctx.subgraphErrors))
		}

		return err
	case *BatchEntityFetch:
		res := &result{
			out: &bytes.Buffer{},
		}
		err := l.loadBatchEntityFetch(l.ctx.ctx, item, f, items, res)
		if err != nil {
			return errors.WithStack(err)
		}
		err = l.mergeResult(item, res, items)
		if l.ctx.LoaderHooks != nil {
			l.ctx.LoaderHooks.OnFinished(res.loaderHookContext, res.ds, newResponseInfo(res, l.ctx.subgraphErrors))
		}
		return err
	case *EntityFetch:
		res := &result{
			out: &bytes.Buffer{},
		}
		err := l.loadEntityFetch(l.ctx.ctx, item, f, items, res)
		if err != nil {
			return errors.WithStack(err)
		}
		err = l.mergeResult(item, res, items)
		if l.ctx.LoaderHooks != nil {
			l.ctx.LoaderHooks.OnFinished(res.loaderHookContext, res.ds, newResponseInfo(res, l.ctx.subgraphErrors))
		}
		return err
	case *ParallelListItemFetch:
		results := make([]*result, len(items))
		if l.ctx.TracingOptions.Enable {
			f.Traces = make([]*SingleFetch, len(items))
		}
		g, ctx := errgroup.WithContext(l.ctx.ctx)
		for i := range items {
			i := i
			results[i] = &result{
				out: &bytes.Buffer{},
			}
			if l.ctx.TracingOptions.Enable {
				f.Traces[i] = new(SingleFetch)
				*f.Traces[i] = *f.Fetch
				g.Go(func() error {
					return l.loadFetch(ctx, f.Traces[i], item, items[i:i+1], results[i])
				})
				continue
			}
			g.Go(func() error {
				return l.loadFetch(ctx, f.Fetch, item, items[i:i+1], results[i])
			})
		}
		err := g.Wait()
		if err != nil {
			return errors.WithStack(err)
		}
		for i := range results {
			err = l.mergeResult(item, results[i], items[i:i+1])
			if l.ctx.LoaderHooks != nil {
				l.ctx.LoaderHooks.OnFinished(results[i].loaderHookContext, results[i].ds, newResponseInfo(results[i], l.ctx.subgraphErrors))
			}
			if err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	default:
		return nil
	}
}

func (l *Loader) selectItemsForPath(path []FetchItemPathElement) []*astjson.Value {
	items := []*astjson.Value{l.resolvable.data}
	if len(path) == 0 {
		return l.filterNonTainted(items)
	}
	for i := range path {
		if len(items) == 0 {
			break
		}
		items = l.selectItems(items, path[i])
	}
	return l.filterNonTainted(items)
}

// filterNonTainted filters out taintedEntities from the given items list.
func (l *Loader) filterNonTainted(items []*astjson.Value) []*astjson.Value {
	if len(items) == 0 || len(l.taintedEntities) == 0 {
		return items
	}
	filtered := make([]*astjson.Value, 0, len(items))
	for _, item := range items {
		if l.isTaintedEntity(item) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

// isTaintedEntity checks if the given `item` is considered isTaintedEntity in the Loader context.
// Not only the item is being considered, but also its elements if the item is an array,
// or its values if the item is an object.
func (l *Loader) isTaintedEntity(item *astjson.Value) bool {
	_, ok := l.taintedEntities[item]
	if ok {
		return true
	}
	switch item.Type() {
	case astjson.TypeArray:
		for _, elem := range item.GetArray() {
			if l.isTaintedEntity(elem) {
				return true
			}
		}
	case astjson.TypeObject:
		obj := item.GetObject()
		found := false
		obj.Visit(func(key []byte, value *astjson.Value) {
			if l.isTaintedEntity(value) {
				found = true
			}
		})
		return found
	}
	return false
}

func (l *Loader) isItemAllowedByTypename(obj *astjson.Value, typeNames []string) bool {
	if len(typeNames) == 0 {
		return true
	}
	if obj == nil || obj.Type() != astjson.TypeObject {
		return true
	}
	__typeName := obj.GetStringBytes("__typename")
	if __typeName == nil {
		return true
	}

	__typeNameStr := string(__typeName)
	return slices.Contains(typeNames, __typeNameStr)
}

func (l *Loader) selectItems(items []*astjson.Value, element FetchItemPathElement) []*astjson.Value {
	if len(items) == 0 {
		return nil
	}
	if len(element.Path) == 0 {
		return items
	}

	if len(items) == 1 {
		if !l.isItemAllowedByTypename(items[0], element.TypeNames) {
			return nil
		}

		field := items[0].Get(element.Path...)
		if field == nil {
			return nil
		}
		if field.Type() == astjson.TypeArray {
			return field.GetArray()
		}
		return []*astjson.Value{field}
	}
	selected := make([]*astjson.Value, 0, len(items))
	for _, item := range items {
		if !l.isItemAllowedByTypename(item, element.TypeNames) {
			continue
		}
		field := item.Get(element.Path...)
		if field == nil {
			continue
		}
		if field.Type() == astjson.TypeArray {
			selected = append(selected, field.GetArray()...)
			continue
		}
		selected = append(selected, field)
	}
	return selected
}

func (l *Loader) itemsData(items []*astjson.Value) *astjson.Value {
	if len(items) == 0 {
		return astjson.NullValue
	}
	if len(items) == 1 {
		return items[0]
	}
	// previously, we used: l.resolvable.astjsonArena.NewArray()
	// however, itemsData can be called concurrently, so this might result in a race
	arr := astjson.MustParseBytes([]byte(`[]`))
	for i, item := range items {
		arr.SetArrayItem(i, item)
	}
	return arr
}

func (l *Loader) loadFetch(ctx context.Context, fetch Fetch, fetchItem *FetchItem, items []*astjson.Value, res *result) error {
	switch f := fetch.(type) {
	case *SingleFetch:
		res.out = &bytes.Buffer{}
		return l.loadSingleFetch(ctx, f, fetchItem, items, res)
	case *ParallelListItemFetch:
		results := make([]*result, len(items))
		if l.ctx.TracingOptions.Enable {
			f.Traces = make([]*SingleFetch, len(items))
		}
		g, ctx := errgroup.WithContext(l.ctx.ctx)
		for i := range items {
			i := i
			results[i] = &result{
				out: &bytes.Buffer{},
			}
			if l.ctx.TracingOptions.Enable {
				f.Traces[i] = new(SingleFetch)
				*f.Traces[i] = *f.Fetch
				g.Go(func() error {
					return l.loadFetch(ctx, f.Traces[i], fetchItem, items[i:i+1], results[i])
				})
				continue
			}
			g.Go(func() error {
				return l.loadFetch(ctx, f.Fetch, fetchItem, items[i:i+1], results[i])
			})
		}
		err := g.Wait()
		if err != nil {
			return errors.WithStack(err)
		}
		res.nestedMergeItems = results
		return nil
	case *EntityFetch:
		res.out = &bytes.Buffer{}
		return l.loadEntityFetch(ctx, fetchItem, f, items, res)
	case *BatchEntityFetch:
		res.out = &bytes.Buffer{}
		return l.loadBatchEntityFetch(ctx, fetchItem, f, items, res)
	}
	return nil
}

type ErrMergeResult struct {
	Subgraph string
	Reason   error
	Path     string
}

func (e ErrMergeResult) Error() string {
	if errors.Is(e.Reason, astjson.ErrMergeDifferingArrayLengths) {
		if e.Path == "" {
			return fmt.Sprintf("unable to merge results from subgraph %s: differing array lengths", e.Subgraph)
		}
		return fmt.Sprintf("unable to merge results from subgraph '%s' at path '%s': differing array lengths", e.Subgraph, e.Path)
	}
	if errors.Is(e.Reason, astjson.ErrMergeDifferentTypes) {
		if e.Path == "" {
			return fmt.Sprintf("unable to merge results from subgraph %s: differing types", e.Subgraph)
		}
		return fmt.Sprintf("unable to merge results from subgraph '%s' at path '%s': differing types", e.Subgraph, e.Path)
	}
	return fmt.Sprintf("unable to merge results from subgraph %s", e.Subgraph)
}

func (l *Loader) mergeResult(fetchItem *FetchItem, res *result, items []*astjson.Value) error {
	if res.err != nil {
		return l.renderErrorsFailedToFetch(fetchItem, res, failedToFetchNoReason)
	}
	if res.authorizationRejected {
		err := l.renderAuthorizationRejectedErrors(fetchItem, res)
		if err != nil {
			return err
		}
		trueValue := astjson.MustParse(`true`)
		skipErrorsPath := make([]string, len(res.postProcessing.MergePath)+1)
		copy(skipErrorsPath, res.postProcessing.MergePath)
		skipErrorsPath[len(skipErrorsPath)-1] = "__skipErrors"
		for _, item := range items {
			astjson.SetValue(item, trueValue, skipErrorsPath...)
		}
		return nil
	}
	if res.rateLimitRejected {
		err := l.renderRateLimitRejectedErrors(fetchItem, res)
		if err != nil {
			return err
		}
		trueValue := astjson.MustParse(`true`)
		skipErrorsPath := make([]string, len(res.postProcessing.MergePath)+1)
		copy(skipErrorsPath, res.postProcessing.MergePath)
		skipErrorsPath[len(skipErrorsPath)-1] = "__skipErrors"
		for _, item := range items {
			astjson.SetValue(item, trueValue, skipErrorsPath...)
		}
		return nil
	}
	if res.fetchSkipped {
		return nil
	}
	if res.out.Len() == 0 {
		return l.renderErrorsFailedToFetch(fetchItem, res, emptyGraphQLResponse)
	}

	response, err := astjson.ParseBytesWithoutCache(res.out.Bytes())
	if err != nil {
		// Fall back to status code if parsing fails and non-2XX
		if (res.statusCode > 0 && res.statusCode < 200) || res.statusCode >= 300 {
			return l.renderErrorsStatusFallback(fetchItem, res, res.statusCode)
		}
		return l.renderErrorsFailedToFetch(fetchItem, res, invalidGraphQLResponse)
	}

	var responseData *astjson.Value
	if res.postProcessing.SelectResponseDataPath != nil {
		responseData = response.Get(res.postProcessing.SelectResponseDataPath...)
	} else {
		responseData = response
	}

	hasErrors := false

	var taintedIndices []int
	// Check if the subgraph response has errors.
	if res.postProcessing.SelectResponseErrorsPath != nil {
		responseErrors := response.Get(res.postProcessing.SelectResponseErrorsPath...)
		if astjson.ValueIsNonNull(responseErrors) {
			hasErrors = len(responseErrors.GetArray()) > 0
			// If the response has the "errors" key, and its value is empty,
			// we don't consider it as an error. Note: it is not compliant with graphql spec.
			if hasErrors {
				if l.validateRequiredExternalFields && res.postProcessing.SelectResponseDataPath != nil {
					taintedIndices = l.getTaintedIndicesAndCleanErrors(fetchItem.Fetch, responseData, responseErrors)
				}
				if len(taintedIndices) > 0 {
					// Override errors with generic error about missing deps.
					err = l.renderErrorsFailedToFetch(fetchItem, res, missingRequiresDependencies)
					if err != nil {
						return errors.WithStack(err)
					}
					// The number of errors could have changed since the last check.
					hasErrors = len(responseErrors.GetArray()) > 0
				}
				if hasErrors {
					// Look for errors in the response and merge them into the "errors" array.
					err = l.mergeErrors(res, fetchItem, responseErrors)
					if err != nil {
						return errors.WithStack(err)
					}
				}
			}
		}
	}

	// Check if data needs processing.
	if res.postProcessing.SelectResponseDataPath != nil && astjson.ValueIsNull(responseData) {
		// When:
		// - No errors or data are present
		// - Status code is not within the 2XX range
		// We can fall back to a status code based error
		if !hasErrors && ((res.statusCode > 0 && res.statusCode < 200) || res.statusCode >= 300) {
			return l.renderErrorsStatusFallback(fetchItem, res, res.statusCode)
		}

		// If we didn't get any data nor errors, we return an error because the response is invalid
		// Returning an error here also avoids the need to walk over it later.
		if !hasErrors && !l.resolvable.options.ApolloCompatibilitySuppressFetchErrors {
			return l.renderErrorsFailedToFetch(fetchItem, res, invalidGraphQLResponseShape)
		}

		// we have no data but only errors
		// skip value completion
		if hasErrors && l.resolvable.options.ApolloCompatibilityValueCompletionInExtensions {
			l.resolvable.skipValueCompletion = true
		}

		// no data
		return nil
	}

	if len(items) == 0 {
		// If the data is set, it must be an object according to GraphQL over HTTP spec
		if responseData.Type() != astjson.TypeObject {
			return l.renderErrorsFailedToFetch(fetchItem, res, invalidGraphQLResponseShape)
		}
		l.resolvable.data = responseData
		return nil
	}
	if len(items) == 1 && res.batchStats == nil {
		items[0], _, err = astjson.MergeValuesWithPath(items[0], responseData, res.postProcessing.MergePath...)
		if err != nil {
			return errors.WithStack(ErrMergeResult{
				Subgraph: res.ds.Name,
				Reason:   err,
				Path:     fetchItem.ResponsePath,
			})
		}
		if slices.Contains(taintedIndices, 0) {
			l.taintedEntities[items[0]] = struct{}{}
		}
		return nil
	}
	batch := responseData.GetArray()
	if batch == nil {
		return l.renderErrorsFailedToFetch(fetchItem, res, invalidGraphQLResponseShape)
	}

	if res.batchStats != nil {
		uniqueIndexes := res.batchStats.getUniqueIndexes()
		if uniqueIndexes != len(batch) {
			return l.renderErrorsFailedToFetch(fetchItem, res, fmt.Sprintf(invalidBatchItemCount, uniqueIndexes, len(batch)))
		}

		for i, stats := range res.batchStats {
			for _, idx := range stats {
				if idx == -1 {
					continue
				}
				items[i], _, err = astjson.MergeValuesWithPath(items[i], batch[idx], res.postProcessing.MergePath...)
				if err != nil {
					return errors.WithStack(ErrMergeResult{
						Subgraph: res.ds.Name,
						Reason:   err,
						Path:     fetchItem.ResponsePath,
					})
				}
				if slices.Contains(taintedIndices, idx) {
					l.taintedEntities[items[i]] = struct{}{}
				}
			}
		}
		return nil
	}

	if batchCount, itemCount := len(batch), len(items); batchCount != itemCount {
		return l.renderErrorsFailedToFetch(fetchItem, res, fmt.Sprintf(invalidBatchItemCount, itemCount, batchCount))
	}

	for i := range items {
		items[i], _, err = astjson.MergeValuesWithPath(items[i], batch[i], res.postProcessing.MergePath...)
		if err != nil {
			return errors.WithStack(ErrMergeResult{
				Subgraph: res.ds.Name,
				Reason:   err,
				Path:     fetchItem.ResponsePath,
			})
		}
		if slices.Contains(taintedIndices, i) {
			l.taintedEntities[items[i]] = struct{}{}
		}
	}
	return nil
}

var (
	errorsInvalidInputHeader = []byte(`{"errors":[{"message":"Failed to render Fetch Input","path":[`)
	errorsInvalidInputFooter = []byte(`]}]}`)
)

func (l *Loader) renderErrorsInvalidInput(fetchItem *FetchItem, out *bytes.Buffer) error {
	elements := fetchItem.ResponsePathElements
	if len(elements) > 0 && elements[len(elements)-1] == "@" {
		elements = elements[:len(elements)-1]
	}
	if len(elements) > 0 {
		elements = elements[1:]
	}
	_, _ = out.Write(errorsInvalidInputHeader)
	for i := range elements {
		if i != 0 {
			_, _ = out.Write(comma)
		}
		_, _ = out.Write(quote)
		_, _ = out.WriteString(elements[i])
		_, _ = out.Write(quote)
	}
	_, _ = out.Write(errorsInvalidInputFooter)
	return nil
}

func (l *Loader) appendSubgraphError(res *result, fetchItem *FetchItem, value *astjson.Value, values []*astjson.Value) error {
	// print them into the buffer to be able to parse them
	errorsJSON := value.MarshalTo(nil)
	graphqlErrors := make([]GraphQLError, 0, len(values))
	err := json.Unmarshal(errorsJSON, &graphqlErrors)
	if err != nil {
		return errors.WithStack(err)
	}

	subgraphError := NewSubgraphError(res.ds, fetchItem.ResponsePath, failedToFetchNoReason, res.statusCode)

	for _, gqlError := range graphqlErrors {
		gErr := gqlError
		subgraphError.AppendDownstreamError(&gErr)
	}

	l.ctx.appendSubgraphErrors(res.err, subgraphError)

	return nil
}

// getTaintedIndicesAndCleanErrors identifies indices of malformed entities based on error paths
// in the response. It uses errors to find entities that have null value for nullable fields that
// are required for other fetches. If an error was used to find such an entity, then this error is
// removed from the errors.
//
// The high-level flow of how it is used:
//
// 1. Subgraph returns errors for specific entities in the "_entities" array;
// 2. getTaintedIndicesAndCleanErrors examines error paths like ["_entities", 1, "requiredField"];
// 3. It validates that the failed field was actually requested for @requires;
// 4. Marks entity at index 1 as "tainted";
// 5. Later fetches will ignore this tainted entity.
func (l *Loader) getTaintedIndicesAndCleanErrors(fetch Fetch, data *astjson.Value, errors *astjson.Value) (indices []int) {
	info := fetch.FetchInfo()
	if info == nil {
		return nil
	}
	// build a map to search with
	requestedForRequires := map[GraphCoordinate]struct{}{}
	for _, fr := range info.FetchReasons {
		if fr.IsRequires && fr.Nullable {
			coord := GraphCoordinate{TypeName: fr.TypeName, FieldName: fr.FieldName}
			requestedForRequires[coord] = struct{}{}
		}
	}
	if len(requestedForRequires) == 0 {
		return
	}

	errorsArray := errors.GetArray()
	for errorIdx := len(errorsArray) - 1; errorIdx >= 0; errorIdx-- {
		candidate := errorsArray[errorIdx]
		errorPath := candidate.Get("path")
		if astjson.ValueIsNull(errorPath) || errorPath.Type() != astjson.TypeArray {
			continue
		}
		pathItems := errorPath.GetArray()
		if len(pathItems) == 0 {
			continue
		}
		for i, item := range pathItems {
			if unsafebytes.BytesToString(item.GetStringBytes()) == "_entities" {
				if len(pathItems)-i <= 2 {
					break
				}
				// We have the full path to the failed item.
				// Verify that it is null and extract the enclosing typename.
				field := unsafebytes.BytesToString(pathItems[len(pathItems)-1].GetStringBytes())
				entity, index := extractEntityIndex(data, pathItems[i+1:len(pathItems)-1])
				if index == -1 || astjson.ValueIsNull(entity) || entity.Type() != astjson.TypeObject {
					break
				}

				possibleNull := entity.Get(field)
				if possibleNull == nil || possibleNull.Type() != astjson.TypeNull {
					break
				}
				typeName := unsafebytes.BytesToString(entity.GetStringBytes("__typename"))
				if typeName == "" {
					break
				}
				coord := GraphCoordinate{TypeName: typeName, FieldName: field}
				if _, ok := requestedForRequires[coord]; !ok {
					break
				}
				indices = append(indices, index)
				errors.Del(strconv.Itoa(errorIdx))
				break
			}
		}
	}
	return
}

// extractEntityIndex returns an entity and its index using the path as selectors on the response.
// Path should contain atl east the index as the first element. Other elements would lead
// to deeply nested entity.
func extractEntityIndex(response *astjson.Value, path []*astjson.Value) (*astjson.Value, int) {
	index := -1
	if len(path) == 0 {
		return nil, index
	}
	for _, el := range path {
		var key string
		switch el.Type() {
		case astjson.TypeNumber:
			parsed := el.GetInt()
			if parsed < 0 {
				return nil, index
			}
			if index == -1 {
				// index is assigned only once
				index = parsed
			}
			key = strconv.Itoa(parsed)
		case astjson.TypeString:
			key = unsafebytes.BytesToString(el.GetStringBytes())
		default:
			return nil, -1
		}
		response = response.Get(key)
		if response == nil {
			return nil, -1
		}
	}
	return response, index
}

func (l *Loader) mergeErrors(res *result, fetchItem *FetchItem, value *astjson.Value) error {
	values := value.GetArray()
	l.optionallyOmitErrorLocations(values)
	l.optionallyRewriteErrorPaths(fetchItem, values)
	l.optionallyEnsureExtensionErrorCode(values)

	if !l.allowAllErrorExtensionFields {
		l.optionallyAllowCustomExtensionProperties(values)
	}

	if l.subgraphErrorPropagationMode == SubgraphErrorPropagationModePassThrough {
		// Attach datasource information to all errors when we don't wrap them
		l.optionallyAttachServiceNameToErrorExtension(values, res.ds.Name)
		l.setSubgraphStatusCode(values, res.statusCode)

		// Allow to delete extensions entirely
		l.optionallyOmitErrorExtensions(values)

		l.optionallyOmitErrorFields(values)

		// If enabled, add the extra http status error for Apollo Router compat
		if err := l.addApolloRouterCompatibilityError(res); err != nil {
			return err
		}

		if len(values) > 0 {
			// Append the subgraph errors to the response payload
			if err := l.appendSubgraphError(res, fetchItem, value, values); err != nil {
				return err
			}
		}

		// If the error propagation mode is pass-through, we append the errors to the root array
		l.resolvable.errors.AppendArrayItems(value)
		return nil
	}

	if len(values) > 0 {
		// Append the subgraph errors to the response payload
		if err := l.appendSubgraphError(res, fetchItem, value, values); err != nil {
			return err
		}
	}

	// Wrap mode (default)
	errorObject, err := astjson.ParseWithoutCache(l.renderSubgraphBaseError(res.ds, fetchItem.ResponsePath, failedToFetchNoReason))
	if err != nil {
		return err
	}

	if l.propagateSubgraphErrors {
		// Attach all errors to the root array in the "errors" extension field
		astjson.SetValue(errorObject, value, "extensions", "errors")
	}

	v := []*astjson.Value{errorObject}

	// Only datasource information are attached to the root error in wrap mode
	l.optionallyAttachServiceNameToErrorExtension(v, res.ds.Name)
	l.setSubgraphStatusCode(v, res.statusCode)

	// Allow to delete extensions entirely
	l.optionallyOmitErrorExtensions(v)

	// If enabled, add the extra http status error for Apollo Router compat
	if err := l.addApolloRouterCompatibilityError(res); err != nil {
		return err
	}

	astjson.AppendToArray(l.resolvable.errors, errorObject)

	return nil
}

// optionallyAllowCustomExtensionProperties removes all properties from the "extensions" object
// that are not in the allowedProperties map.
// If no properties are left, the "extensions" object is removed.
func (l *Loader) optionallyAllowCustomExtensionProperties(values []*astjson.Value) {
	for _, value := range values {
		if value.Exists("extensions") {
			extensions := value.Get("extensions")
			if extensions.Type() != astjson.TypeObject {
				continue
			}
			extObj := extensions.GetObject()

			extObj.Visit(func(k []byte, v *astjson.Value) {
				kb := unsafebytes.BytesToString(k)
				if _, ok := l.allowedErrorExtensionFields[kb]; !ok {
					extensions.Del(kb)
				}
			})

			// If there are no more properties, we remove the extensions object
			if len(l.allowedErrorExtensionFields) == 0 || extObj.Len() == 0 {
				value.Del("extensions")
				continue
			}
		}
	}
}

// optionallyEnsureExtensionErrorCode ensures that all values have an error code in the "extensions" object.
func (l *Loader) optionallyEnsureExtensionErrorCode(values []*astjson.Value) {
	if l.defaultErrorExtensionCode == "" {
		return
	}

	for _, value := range values {
		if value.Exists("extensions") {
			extensions := value.Get("extensions")
			switch extensions.Type() {
			case astjson.TypeObject:
				if !extensions.Exists("code") {
					extensions.Set("code", l.resolvable.astjsonArena.NewString(l.defaultErrorExtensionCode))
				}
			case astjson.TypeNull:
				extensionsObj := l.resolvable.astjsonArena.NewObject()
				extensionsObj.Set("code", l.resolvable.astjsonArena.NewString(l.defaultErrorExtensionCode))
				value.Set("extensions", extensionsObj)
			}
		} else {
			extensionsObj := l.resolvable.astjsonArena.NewObject()
			extensionsObj.Set("code", l.resolvable.astjsonArena.NewString(l.defaultErrorExtensionCode))
			value.Set("extensions", extensionsObj)
		}
	}
}

// optionallyAttachServiceNameToErrorExtension for all values attaches the service name
// to the "extensions" object.
func (l *Loader) optionallyAttachServiceNameToErrorExtension(values []*astjson.Value, serviceName string) {
	if !l.attachServiceNameToErrorExtension {
		return
	}

	for _, value := range values {
		if value.Exists("extensions") {
			extensions := value.Get("extensions")
			switch extensions.Type() {
			case astjson.TypeObject:
				extensions.Set("serviceName", l.resolvable.astjsonArena.NewString(serviceName))
			case astjson.TypeNull:
				extensionsObj := l.resolvable.astjsonArena.NewObject()
				extensionsObj.Set("serviceName", l.resolvable.astjsonArena.NewString(serviceName))
				value.Set("extensions", extensionsObj)
			}
		} else {
			extensionsObj := l.resolvable.astjsonArena.NewObject()
			extensionsObj.Set("serviceName", l.resolvable.astjsonArena.NewString(serviceName))
			value.Set("extensions", extensionsObj)
		}
	}
}

// optionallyOmitErrorExtensions removes the "extensions" object from all values
func (l *Loader) optionallyOmitErrorExtensions(values []*astjson.Value) {
	if !l.omitSubgraphErrorExtensions {
		return
	}
	for _, value := range values {
		if value.Exists("extensions") {
			value.Del("extensions")
		}
	}
}

// optionallyOmitErrorFields removes all fields from the subgraph error that are not allowlisted.
// It does not remove the message.
func (l *Loader) optionallyOmitErrorFields(values []*astjson.Value) {
	for _, value := range values {
		if value.Type() == astjson.TypeObject {
			obj := value.GetObject()
			var keysToDelete []string
			obj.Visit(func(k []byte, v *astjson.Value) {
				key := unsafebytes.BytesToString(k)
				if _, ok := l.allowedSubgraphErrorFields[key]; !ok {
					keysToDelete = append(keysToDelete, key)
				}
			})
			for _, key := range keysToDelete {
				obj.Del(key)
			}
		}
	}
}

// optionallyOmitErrorLocations removes the "locations" object from all values.
func (l *Loader) optionallyOmitErrorLocations(values []*astjson.Value) {
	if !l.omitSubgraphErrorLocations {
		return
	}
	for _, value := range values {
		if value.Exists("locations") {
			value.Del("locations")
		}
	}
}

// optionallyRewriteErrorPaths rewrites the path field for all the values.
func (l *Loader) optionallyRewriteErrorPaths(fetchItem *FetchItem, values []*astjson.Value) {
	if !l.rewriteSubgraphErrorPaths {
		return
	}
	pathPrefix := make([]string, len(fetchItem.ResponsePathElements))
	copy(pathPrefix, fetchItem.ResponsePathElements)
	// remove the trailing @ in case we're in an array as it looks weird in the path
	// errors, like fetches, are attached to objects, not arrays
	if len(fetchItem.ResponsePathElements) != 0 && fetchItem.ResponsePathElements[len(fetchItem.ResponsePathElements)-1] == "@" {
		pathPrefix = pathPrefix[:len(pathPrefix)-1]
	}
	for _, value := range values {
		errorPath := value.Get("path")
		if astjson.ValueIsNull(errorPath) {
			continue
		}
		if errorPath.Type() != astjson.TypeArray {
			continue
		}
		pathItems := errorPath.GetArray()
		if len(pathItems) == 0 {
			continue
		}
		for i, item := range pathItems {
			if unsafebytes.BytesToString(item.GetStringBytes()) == "_entities" {
				// rewrite the path to pathPrefix + pathItems after _entities
				newPath := make([]string, 0, len(pathPrefix)+len(pathItems)-i)
				newPath = append(newPath, pathPrefix...)
				for j := i + 1; j < len(pathItems); j++ {
					// BUG: for pathItems containing integers, it will append empty strings
					newPath = append(newPath, unsafebytes.BytesToString(pathItems[j].GetStringBytes()))
				}
				newPathJSON, _ := json.Marshal(newPath)
				pathBytes, err := astjson.ParseBytesWithoutCache(newPathJSON)
				if err != nil {
					continue
				}
				value.Set("path", pathBytes)
				break
			}
		}
	}
}

func (l *Loader) setSubgraphStatusCode(values []*astjson.Value, statusCode int) {
	if !l.propagateSubgraphStatusCodes {
		return
	}

	if statusCode == 0 {
		return
	}

	for _, value := range values {
		if value.Exists("extensions") {
			extensions := value.Get("extensions")
			if extensions.Type() != astjson.TypeObject {
				continue
			}
			v, err := astjson.ParseWithoutCache(strconv.Itoa(statusCode))
			if err != nil {
				continue
			}
			extensions.Set("statusCode", v)
		} else {
			v, err := astjson.ParseWithoutCache(`{"statusCode":` + strconv.Itoa(statusCode) + `}`)
			if err != nil {
				continue
			}
			value.Set("extensions", v)
		}
	}
}

const (
	failedToFetchNoReason       = ""
	emptyGraphQLResponse        = "empty response"
	invalidGraphQLResponse      = "invalid JSON"
	invalidGraphQLResponseShape = "no data or errors in response"
	invalidBatchItemCount       = "returned entities count does not match the count of representation variables in the entities request. Expected %d, got %d"
	missingRequiresDependencies = "failed to obtain field dependencies"
)

func (l *Loader) renderAtPathErrorPart(path string) string {
	if path == "" {
		return ""
	}
	return fmt.Sprintf(` at Path '%s'`, path)
}

func (l *Loader) addApolloRouterCompatibilityError(res *result) error {
	if !l.apolloRouterCompatibilitySubrequestHTTPError || (res.statusCode < 400) {
		return nil
	}

	apolloRouterStatusErrorJSON := fmt.Sprintf(`{
			"message": "HTTP fetch failed from '%[1]s': %[3]d: %[2]s",
			"path": [],
			"extensions": {
				"code": "SUBREQUEST_HTTP_ERROR",
				"service": "%[1]s",
				"reason": "%[3]d: %[2]s",
				"http": {
					"status": %[3]d
				}
			}
		}`, res.ds.Name, http.StatusText(res.statusCode), res.statusCode)
	apolloRouterStatusError, err := astjson.ParseWithoutCache(apolloRouterStatusErrorJSON)
	if err != nil {
		return err
	}

	astjson.AppendToArray(l.resolvable.errors, apolloRouterStatusError)

	return nil
}

func (l *Loader) renderErrorsFailedToFetch(fetchItem *FetchItem, res *result, reason string) error {
	l.ctx.appendSubgraphErrors(res.err, NewSubgraphError(res.ds, fetchItem.ResponsePath, reason, res.statusCode))
	errorObject, err := astjson.ParseWithoutCache(l.renderSubgraphBaseError(res.ds, fetchItem.ResponsePath, reason))
	if err != nil {
		return err
	}
	l.setSubgraphStatusCode([]*astjson.Value{errorObject}, res.statusCode)
	astjson.AppendToArray(l.resolvable.errors, errorObject)
	return nil
}

func (l *Loader) renderErrorsStatusFallback(fetchItem *FetchItem, res *result, statusCode int) error {
	reason := fmt.Sprintf("%d", statusCode)
	if statusText := http.StatusText(statusCode); statusText != "" {
		reason += fmt.Sprintf(": %s", statusText)
	}

	l.ctx.appendSubgraphErrors(res.err, NewSubgraphError(res.ds, fetchItem.ResponsePath, reason, res.statusCode))

	errorObject, err := astjson.ParseWithoutCache(fmt.Sprintf(`{"message":"%s"}`, reason))
	if err != nil {
		return err
	}

	l.setSubgraphStatusCode([]*astjson.Value{errorObject}, res.statusCode)

	astjson.AppendToArray(l.resolvable.errors, errorObject)
	return nil
}

func (l *Loader) renderSubgraphBaseError(ds DataSourceInfo, path, reason string) string {
	pathPart := l.renderAtPathErrorPart(path)
	if ds.Name == "" {
		if reason == "" {
			return fmt.Sprintf(`{"message":"Failed to fetch from Subgraph%s."}`, pathPart)
		}
		return fmt.Sprintf(`{"message":"Failed to fetch from Subgraph%s, Reason: %s."}`, pathPart, reason)
	}
	if reason == "" {
		return fmt.Sprintf(`{"message":"Failed to fetch from Subgraph '%s'%s."}`, ds.Name, pathPart)
	}
	return fmt.Sprintf(`{"message":"Failed to fetch from Subgraph '%s'%s, Reason: %s."}`, ds.Name, pathPart, reason)
}

func (l *Loader) renderAuthorizationRejectedErrors(fetchItem *FetchItem, res *result) error {
	for i := range res.authorizationRejectedReasons {
		l.ctx.appendSubgraphErrors(res.err, NewSubgraphError(res.ds, fetchItem.ResponsePath, res.authorizationRejectedReasons[i], res.statusCode))
	}
	pathPart := l.renderAtPathErrorPart(fetchItem.ResponsePath)
	extensionErrorCode := fmt.Sprintf(`"extensions":{"code":"%s"}`, errorcodes.UnauthorizedFieldOrType)
	if res.ds.Name == "" {
		for _, reason := range res.authorizationRejectedReasons {
			if reason == "" {
				errorObject, err := astjson.ParseWithoutCache(fmt.Sprintf(`{"message":"Unauthorized Subgraph request%s.",%s}`, pathPart, extensionErrorCode))
				if err != nil {
					continue
				}
				astjson.AppendToArray(l.resolvable.errors, errorObject)
			} else {
				errorObject, err := astjson.ParseWithoutCache(fmt.Sprintf(`{"message":"Unauthorized Subgraph request%s, Reason: %s.",%s}`, pathPart, reason, extensionErrorCode))
				if err != nil {
					continue
				}
				astjson.AppendToArray(l.resolvable.errors, errorObject)
			}
		}
	} else {
		for _, reason := range res.authorizationRejectedReasons {
			if reason == "" {
				errorObject, err := astjson.ParseWithoutCache(fmt.Sprintf(`{"message":"Unauthorized request to Subgraph '%s'%s.",%s}`, res.ds.Name, pathPart, extensionErrorCode))
				if err != nil {
					continue
				}
				astjson.AppendToArray(l.resolvable.errors, errorObject)
			} else {
				errorObject, err := astjson.ParseWithoutCache(fmt.Sprintf(`{"message":"Unauthorized request to Subgraph '%s'%s, Reason: %s.",%s}`, res.ds.Name, pathPart, reason, extensionErrorCode))
				if err != nil {
					continue
				}
				astjson.AppendToArray(l.resolvable.errors, errorObject)
			}
		}
	}
	return nil
}

func (l *Loader) renderRateLimitRejectedErrors(fetchItem *FetchItem, res *result) error {
	l.ctx.appendSubgraphErrors(res.err, NewRateLimitError(res.ds.Name, fetchItem.ResponsePath, res.rateLimitRejectedReason))
	pathPart := l.renderAtPathErrorPart(fetchItem.ResponsePath)
	var (
		err         error
		errorObject *astjson.Value
	)
	if res.ds.Name == "" {
		if res.rateLimitRejectedReason == "" {
			errorObject, err = astjson.ParseWithoutCache(fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph request%s."}`, pathPart))
			if err != nil {
				return err
			}
		} else {
			errorObject, err = astjson.ParseWithoutCache(fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph request%s, Reason: %s."}`, pathPart, res.rateLimitRejectedReason))
			if err != nil {
				return err
			}
		}
	} else {
		if res.rateLimitRejectedReason == "" {
			errorObject, err = astjson.ParseWithoutCache(fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph '%s'%s."}`, res.ds.Name, pathPart))
			if err != nil {
				return err
			}
		} else {
			errorObject, err = astjson.ParseWithoutCache(fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph '%s'%s, Reason: %s."}`, res.ds.Name, pathPart, res.rateLimitRejectedReason))
			if err != nil {
				return err
			}
		}
	}
	if l.ctx.RateLimitOptions.ErrorExtensionCode.Enabled {
		extension, err := astjson.ParseWithoutCache(fmt.Sprintf(`{"code":"%s"}`, l.ctx.RateLimitOptions.ErrorExtensionCode.Code))
		if err != nil {
			return err
		}
		errorObject, _, err = astjson.MergeValuesWithPath(errorObject, extension, "extensions")
		if err != nil {
			return err
		}
	}
	astjson.AppendToArray(l.resolvable.errors, errorObject)
	return nil
}

func (l *Loader) isFetchAuthorized(input []byte, info *FetchInfo, res *result) (authorized bool, err error) {
	if info.OperationType == ast.OperationTypeQuery {
		// we only want to authorize Mutations and Subscriptions at the load level
		// Mutations can have side effects, so we don't want to send them to a subgraph if the user is not authorized
		// Subscriptions only have one single root field, so it's safe to deny the whole request if unauthorized
		// Queries can have multiple root fields, but have no side effects
		// So we don't need to deny the request if one of the root fields is unauthorized
		// Instead, we send the request to the subgraph and filter out the unauthorized fields later
		// This is done in the resolvable during the response resolution phase
		return true, nil
	}
	if l.ctx.authorizer == nil {
		return true, nil
	}
	authorized = true
	for i := range info.RootFields {
		if !info.RootFields[i].HasAuthorizationRule {
			continue
		}
		reject, err := l.ctx.authorizer.AuthorizePreFetch(l.ctx, info.DataSourceID, input, info.RootFields[i])
		if err != nil {
			return false, err
		}
		if reject != nil {
			authorized = false
			res.fetchSkipped = true
			res.authorizationRejected = true
			res.authorizationRejectedReasons = append(res.authorizationRejectedReasons, reject.Reason)
		}
	}
	return authorized, nil
}

func (l *Loader) rateLimitFetch(input []byte, info *FetchInfo, res *result) (allowed bool, err error) {
	if !l.ctx.RateLimitOptions.Enable {
		return true, nil
	}
	if l.ctx.rateLimiter == nil {
		return true, nil
	}
	result, err := l.ctx.rateLimiter.RateLimitPreFetch(l.ctx, info, input)
	if err != nil {
		return false, err
	}
	if result != nil {
		res.rateLimitRejected = true
		res.fetchSkipped = true
		res.rateLimitRejectedReason = result.Reason
		return false, nil
	}
	return true, nil
}

func (l *Loader) validatePreFetch(input []byte, info *FetchInfo, res *result) (allowed bool, err error) {
	if info == nil {
		return true, nil
	}
	allowed, err = l.isFetchAuthorized(input, info, res)
	if err != nil || !allowed {
		return
	}
	return l.rateLimitFetch(input, info, res)
}

func (l *Loader) loadSingleFetch(ctx context.Context, fetch *SingleFetch, fetchItem *FetchItem, items []*astjson.Value, res *result) error {
	res.init(fetch.PostProcessing, fetch.Info)
	buf := &bytes.Buffer{}

	inputData := l.itemsData(items)
	if l.ctx.TracingOptions.Enable {
		fetch.Trace = &DataSourceLoadTrace{}
		if !l.ctx.TracingOptions.ExcludeRawInputData && inputData != nil {
			fetch.Trace.RawInputData, _ = l.compactJSON(inputData.MarshalTo(nil))
		}
	}

	// When we don't have parent data it makes no sense to proceed with next fetches in a sequence
	// Right now, it is the case only for the introspection - because introspection uses
	// only single fetches.
	// Having null means that the previous fetch returned null as data
	if len(items) == 1 && items[0].Type() == astjson.TypeNull {
		res.fetchSkipped = true
		if l.ctx.TracingOptions.Enable {
			fetch.Trace.LoadSkipped = true
		}
		return nil
	}

	err := fetch.InputTemplate.Render(l.ctx, inputData, buf)
	if err != nil {
		return l.renderErrorsInvalidInput(fetchItem, res.out)
	}
	fetchInput := buf.Bytes()
	allowed, err := l.validatePreFetch(fetchInput, fetch.Info, res)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
	l.executeSourceLoad(ctx, fetchItem, fetch.DataSource, fetchInput, res, fetch.Trace)
	return nil
}

var (
	entityFetchPool = sync.Pool{
		New: func() any {
			return &entityFetchBuffer{
				item:          &bytes.Buffer{},
				preparedInput: &bytes.Buffer{},
			}
		},
	}
)

type entityFetchBuffer struct {
	item          *bytes.Buffer
	preparedInput *bytes.Buffer
}

func acquireEntityFetchBuffer() *entityFetchBuffer {
	return entityFetchPool.Get().(*entityFetchBuffer)
}

func releaseEntityFetchBuffer(buf *entityFetchBuffer) {
	buf.item.Reset()
	buf.preparedInput.Reset()
	entityFetchPool.Put(buf)
}

func (l *Loader) loadEntityFetch(ctx context.Context, fetchItem *FetchItem, fetch *EntityFetch, items []*astjson.Value, res *result) error {
	res.init(fetch.PostProcessing, fetch.Info)
	buf := acquireEntityFetchBuffer()
	defer releaseEntityFetchBuffer(buf)
	input := l.itemsData(items)
	if l.ctx.TracingOptions.Enable {
		fetch.Trace = &DataSourceLoadTrace{}
		if !l.ctx.TracingOptions.ExcludeRawInputData && input != nil {
			fetch.Trace.RawInputData, _ = l.compactJSON(input.MarshalTo(nil))
		}
	}

	var undefinedVariables []string

	err := fetch.Input.Header.RenderAndCollectUndefinedVariables(l.ctx, nil, buf.preparedInput, &undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}

	err = fetch.Input.Item.Render(l.ctx, input, buf.item)
	if err != nil {
		if fetch.Input.SkipErrItem {
			// skip fetch on render item error
			if l.ctx.TracingOptions.Enable {
				fetch.Trace.LoadSkipped = true
			}
			res.fetchSkipped = true
			return nil
		}
		return errors.WithStack(err)
	}
	renderedItem := buf.item.Bytes()
	if bytes.Equal(renderedItem, null) {
		// skip fetch if item is null
		res.fetchSkipped = true
		if l.ctx.TracingOptions.Enable {
			fetch.Trace.LoadSkipped = true
		} else {
			return nil
		}
	}
	if bytes.Equal(renderedItem, emptyObject) {
		// skip fetch if item is empty
		res.fetchSkipped = true
		if l.ctx.TracingOptions.Enable {
			fetch.Trace.LoadSkipped = true
		} else {
			return nil
		}
	}
	_, _ = buf.item.WriteTo(buf.preparedInput)
	err = fetch.Input.Footer.RenderAndCollectUndefinedVariables(l.ctx, nil, buf.preparedInput, &undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}

	err = SetInputUndefinedVariables(buf.preparedInput, undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}
	fetchInput := buf.preparedInput.Bytes()

	if l.ctx.TracingOptions.Enable && res.fetchSkipped {
		l.setTracingInput(fetchItem, fetchInput, fetch.Trace)
		return nil
	}

	allowed, err := l.validatePreFetch(fetchInput, fetch.Info, res)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
	l.executeSourceLoad(ctx, fetchItem, fetch.DataSource, fetchInput, res, fetch.Trace)
	return nil
}

var (
	batchEntityFetchPool = sync.Pool{}
)

type batchEntityFetchBuffer struct {
	preparedInput *bytes.Buffer
	itemInput     *bytes.Buffer
	keyGen        *xxhash.Digest
}

func acquireBatchEntityFetchBuffer() *batchEntityFetchBuffer {
	buf := batchEntityFetchPool.Get()
	if buf == nil {
		return &batchEntityFetchBuffer{
			preparedInput: &bytes.Buffer{},
			itemInput:     &bytes.Buffer{},
			keyGen:        xxhash.New(),
		}
	}
	return buf.(*batchEntityFetchBuffer)
}

func releaseBatchEntityFetchBuffer(buf *batchEntityFetchBuffer) {
	buf.preparedInput.Reset()
	buf.itemInput.Reset()
	buf.keyGen.Reset()
	batchEntityFetchPool.Put(buf)
}

func (l *Loader) loadBatchEntityFetch(ctx context.Context, fetchItem *FetchItem, fetch *BatchEntityFetch, items []*astjson.Value, res *result) error {
	res.init(fetch.PostProcessing, fetch.Info)

	buf := acquireBatchEntityFetchBuffer()
	defer releaseBatchEntityFetchBuffer(buf)

	if l.ctx.TracingOptions.Enable {
		fetch.Trace = &DataSourceLoadTrace{}
		if !l.ctx.TracingOptions.ExcludeRawInputData && len(items) != 0 {
			data := l.itemsData(items)
			if data != nil {
				fetch.Trace.RawInputData, _ = l.compactJSON(data.MarshalTo(nil))
			}
		}
	}

	var undefinedVariables []string

	err := fetch.Input.Header.RenderAndCollectUndefinedVariables(l.ctx, nil, buf.preparedInput, &undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}
	res.batchStats = make(batchStats, len(items))
	itemHashes := make([]uint64, 0, len(items))
	batchItemIndex := 0
	addSeparator := false

WithNextItem:
	for i, item := range items {
		for j := range fetch.Input.Items {
			buf.itemInput.Reset()
			err = fetch.Input.Items[j].Render(l.ctx, item, buf.itemInput)
			if err != nil {
				if fetch.Input.SkipErrItems {
					err = nil // nolint:ineffassign
					res.batchStats[i] = append(res.batchStats[i], -1)
					continue
				}
				if l.ctx.TracingOptions.Enable {
					fetch.Trace.LoadSkipped = true
				}
				return errors.WithStack(err)
			}
			if fetch.Input.SkipNullItems && buf.itemInput.Len() == 4 && bytes.Equal(buf.itemInput.Bytes(), null) {
				res.batchStats[i] = append(res.batchStats[i], -1)
				continue
			}
			if fetch.Input.SkipEmptyObjectItems && buf.itemInput.Len() == 2 && bytes.Equal(buf.itemInput.Bytes(), emptyObject) {
				res.batchStats[i] = append(res.batchStats[i], -1)
				continue
			}

			buf.keyGen.Reset()
			_, _ = buf.keyGen.Write(buf.itemInput.Bytes())
			itemHash := buf.keyGen.Sum64()
			for k := range itemHashes {
				if itemHashes[k] == itemHash {
					res.batchStats[i] = append(res.batchStats[i], k)
					continue WithNextItem
				}
			}
			itemHashes = append(itemHashes, itemHash)
			if addSeparator {
				err = fetch.Input.Separator.Render(l.ctx, nil, buf.preparedInput)
				if err != nil {
					return errors.WithStack(err)
				}
			}
			_, _ = buf.itemInput.WriteTo(buf.preparedInput)
			res.batchStats[i] = append(res.batchStats[i], batchItemIndex)
			batchItemIndex++
			addSeparator = true
		}
	}

	if len(itemHashes) == 0 {
		// all items were skipped - discard fetch
		res.fetchSkipped = true
		if l.ctx.TracingOptions.Enable {
			fetch.Trace.LoadSkipped = true
		} else {
			return nil
		}
	}

	err = fetch.Input.Footer.RenderAndCollectUndefinedVariables(l.ctx, nil, buf.preparedInput, &undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}

	err = SetInputUndefinedVariables(buf.preparedInput, undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}
	fetchInput := buf.preparedInput.Bytes()

	if l.ctx.TracingOptions.Enable && res.fetchSkipped {
		l.setTracingInput(fetchItem, fetchInput, fetch.Trace)
		return nil
	}

	allowed, err := l.validatePreFetch(fetchInput, fetch.Info, res)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
	l.executeSourceLoad(ctx, fetchItem, fetch.DataSource, fetchInput, res, fetch.Trace)
	return nil
}

func redactHeaders(rawJSON json.RawMessage) (json.RawMessage, error) {
	var obj map[string]interface{}

	sensitiveHeaders := []string{
		"authorization",
		"www-authenticate",
		"proxy-authenticate",
		"proxy-authorization",
		"cookie",
		"set-cookie",
	}

	err := json.Unmarshal(rawJSON, &obj)
	if err != nil {
		return nil, err
	}

	if headers, ok := obj["header"]; ok {
		if headerMap, isMap := headers.(map[string]interface{}); isMap {
			for key, values := range headerMap {
				if slices.Contains(sensitiveHeaders, strings.ToLower(key)) {
					headerMap[key] = []string{"****"}
				} else {
					headerMap[key] = values
				}
			}
		}
	}

	redactedJSON, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	return redactedJSON, nil
}

type disallowSingleFlightContextKey struct{}

func SingleFlightDisallowed(ctx context.Context) bool {
	return ctx.Value(disallowSingleFlightContextKey{}) != nil
}

type singleFlightStatsKey struct{}

type SingleFlightStats struct {
	SingleFlightUsed           bool
	SingleFlightSharedResponse bool
}

func GetSingleFlightStats(ctx context.Context) *SingleFlightStats {
	maybeStats := ctx.Value(singleFlightStatsKey{})
	if maybeStats == nil {
		return nil
	}
	return maybeStats.(*SingleFlightStats)
}

func setSingleFlightStats(ctx context.Context, stats *SingleFlightStats) context.Context {
	return context.WithValue(ctx, singleFlightStatsKey{}, stats)
}

func (l *Loader) setTracingInput(fetchItem *FetchItem, input []byte, trace *DataSourceLoadTrace) {
	trace.Path = fetchItem.ResponsePath
	if !l.ctx.TracingOptions.ExcludeInput {
		trace.Input = make([]byte, len(input))
		copy(trace.Input, input) // copy input explicitly, omit __trace__ field
		redactedInput, err := redactHeaders(trace.Input)
		if err != nil {
			return
		}
		trace.Input = redactedInput
	}
}

func (l *Loader) loadByContext(ctx context.Context, source DataSource, input []byte, res *result) error {
	if l.ctx.Files != nil {
		return source.LoadWithFiles(ctx, input, l.ctx.Files, res.out)
	}
	return source.Load(ctx, input, res.out)
}

func (l *Loader) executeSourceLoad(ctx context.Context, fetchItem *FetchItem, source DataSource, input []byte, res *result, trace *DataSourceLoadTrace) {
	if l.ctx.Extensions != nil {
		input, res.err = jsonparser.Set(input, l.ctx.Extensions, "body", "extensions")
		if res.err != nil {
			res.err = errors.WithStack(res.err)
			return
		}
	}
	if l.propagateFetchReasons && !IsIntrospectionDataSource(res.ds.ID) {
		info := fetchItem.Fetch.FetchInfo()
		if info != nil && len(info.PropagatedFetchReasons) > 0 {
			var encoded []byte
			encoded, res.err = json.Marshal(info.PropagatedFetchReasons)
			if res.err != nil {
				res.err = errors.WithStack(res.err)
				return
			}
			// We expect that body.extensions is an object
			input, res.err = jsonparser.Set(input, encoded, "body", "extensions", "fetch_reasons")
			if res.err != nil {
				res.err = errors.WithStack(res.err)
				return
			}
		}
	}
	if l.ctx.TracingOptions.Enable {
		ctx = setSingleFlightStats(ctx, &SingleFlightStats{})
		trace.Path = fetchItem.ResponsePath
		if !l.ctx.TracingOptions.ExcludeInput {
			trace.Input = make([]byte, len(input))
			copy(trace.Input, input) // copy input explicitly, omit __trace__ field
			redactedInput, err := redactHeaders(trace.Input)
			if err != nil {
				res.err = errors.WithStack(err)
				return
			}
			trace.Input = redactedInput
		}
		if gjson.ValidBytes(input) {
			inputCopy := make([]byte, len(input))
			copy(inputCopy, input)
			input, _ = jsonparser.Set(inputCopy, []byte("true"), "__trace__")
		}
		if !l.ctx.TracingOptions.ExcludeLoadStats {
			trace.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
			trace.DurationSinceStartPretty = time.Duration(trace.DurationSinceStartNano).String()
			trace.LoadStats = &LoadStats{}
			clientTrace := &httptrace.ClientTrace{
				GetConn: func(hostPort string) {
					trace.LoadStats.GetConn.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.GetConn.DurationSinceStartPretty = time.Duration(trace.LoadStats.GetConn.DurationSinceStartNano).String()
					if !l.ctx.TracingOptions.EnablePredictableDebugTimings {
						trace.LoadStats.GetConn.HostPort = hostPort
					}
				},
				GotConn: func(info httptrace.GotConnInfo) {
					trace.LoadStats.GotConn.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.GotConn.DurationSinceStartPretty = time.Duration(trace.LoadStats.GotConn.DurationSinceStartNano).String()
					if !l.ctx.TracingOptions.EnablePredictableDebugTimings {
						trace.LoadStats.GotConn.Reused = info.Reused
						trace.LoadStats.GotConn.WasIdle = info.WasIdle
						trace.LoadStats.GotConn.IdleTimeNano = info.IdleTime.Nanoseconds()
						trace.LoadStats.GotConn.IdleTimePretty = info.IdleTime.String()
					}
				},
				PutIdleConn: nil,
				GotFirstResponseByte: func() {
					trace.LoadStats.GotFirstResponseByte.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.GotFirstResponseByte.DurationSinceStartPretty = time.Duration(trace.LoadStats.GotFirstResponseByte.DurationSinceStartNano).String()
				},
				Got100Continue: nil,
				Got1xxResponse: nil,
				DNSStart: func(info httptrace.DNSStartInfo) {
					trace.LoadStats.DNSStart.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.DNSStart.DurationSinceStartPretty = time.Duration(trace.LoadStats.DNSStart.DurationSinceStartNano).String()
					if !l.ctx.TracingOptions.EnablePredictableDebugTimings {
						trace.LoadStats.DNSStart.Host = info.Host
					}
				},
				DNSDone: func(info httptrace.DNSDoneInfo) {
					trace.LoadStats.DNSDone.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.DNSDone.DurationSinceStartPretty = time.Duration(trace.LoadStats.DNSDone.DurationSinceStartNano).String()
				},
				ConnectStart: func(network, addr string) {
					trace.LoadStats.ConnectStart.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.ConnectStart.DurationSinceStartPretty = time.Duration(trace.LoadStats.ConnectStart.DurationSinceStartNano).String()
					if !l.ctx.TracingOptions.EnablePredictableDebugTimings {
						trace.LoadStats.ConnectStart.Network = network
						trace.LoadStats.ConnectStart.Addr = addr
					}
				},
				ConnectDone: func(network, addr string, err error) {
					trace.LoadStats.ConnectDone.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.ConnectDone.DurationSinceStartPretty = time.Duration(trace.LoadStats.ConnectDone.DurationSinceStartNano).String()
					if !l.ctx.TracingOptions.EnablePredictableDebugTimings {
						trace.LoadStats.ConnectDone.Network = network
						trace.LoadStats.ConnectDone.Addr = addr
					}
					if err != nil {
						trace.LoadStats.ConnectDone.Err = err.Error()
					}
				},
				TLSHandshakeStart: func() {
					trace.LoadStats.TLSHandshakeStart.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.TLSHandshakeStart.DurationSinceStartPretty = time.Duration(trace.LoadStats.TLSHandshakeStart.DurationSinceStartNano).String()
				},
				TLSHandshakeDone: func(state tls.ConnectionState, err error) {
					trace.LoadStats.TLSHandshakeDone.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.TLSHandshakeDone.DurationSinceStartPretty = time.Duration(trace.LoadStats.TLSHandshakeDone.DurationSinceStartNano).String()
					if err != nil {
						trace.LoadStats.TLSHandshakeDone.Err = err.Error()
					}
				},
				WroteHeaderField: nil,
				WroteHeaders: func() {
					trace.LoadStats.WroteHeaders.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.WroteHeaders.DurationSinceStartPretty = time.Duration(trace.LoadStats.WroteHeaders.DurationSinceStartNano).String()
				},
				Wait100Continue: nil,
				WroteRequest: func(info httptrace.WroteRequestInfo) {
					trace.LoadStats.WroteRequest.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.WroteRequest.DurationSinceStartPretty = time.Duration(trace.LoadStats.WroteRequest.DurationSinceStartNano).String()
					if info.Err != nil {
						trace.LoadStats.WroteRequest.Err = info.Err.Error()
					}
				},
			}
			ctx = httptrace.WithClientTrace(ctx, clientTrace)
		}
	}
	if l.info != nil && l.info.OperationType == ast.OperationTypeMutation {
		ctx = context.WithValue(ctx, disallowSingleFlightContextKey{}, true)
	}
	var responseContext *httpclient.ResponseContext
	ctx, responseContext = httpclient.InjectResponseContext(ctx)

	if l.ctx.LoaderHooks != nil {
		res.loaderHookContext = l.ctx.LoaderHooks.OnLoad(ctx, res.ds)

		// Prevent that the context is destroyed when the loader hook return an empty context
		if res.loaderHookContext != nil {
			res.err = l.loadByContext(res.loaderHookContext, source, input, res)
		} else {
			res.err = l.loadByContext(ctx, source, input, res)
			res.loaderHookContext = ctx // Set the context to the original context to ensure that OnFinished hook gets valid context
		}

	} else {
		res.err = l.loadByContext(ctx, source, input, res)
	}

	res.statusCode = responseContext.StatusCode
	res.httpResponseContext = responseContext

	if l.ctx.TracingOptions.Enable {
		stats := GetSingleFlightStats(ctx)
		if stats != nil {
			trace.SingleFlightUsed = stats.SingleFlightUsed
			trace.SingleFlightSharedResponse = stats.SingleFlightSharedResponse
		}
		if !l.ctx.TracingOptions.ExcludeOutput && res.out.Len() > 0 {
			trace.Output, _ = l.compactJSON(res.out.Bytes())
			if l.ctx.TracingOptions.EnablePredictableDebugTimings {
				trace.Output, _ = sjson.DeleteBytes(trace.Output, "extensions.trace.response.headers.Date")
			}
		}
		if !l.ctx.TracingOptions.ExcludeLoadStats {
			if l.ctx.TracingOptions.EnablePredictableDebugTimings {
				trace.DurationLoadNano = 1
			} else {
				trace.DurationLoadNano = GetDurationNanoSinceTraceStart(ctx) - trace.DurationSinceStartNano
			}
			trace.DurationLoadPretty = time.Duration(trace.DurationLoadNano).String()
		}
	}
	if res.err != nil {
		if l.ctx.TracingOptions.Enable {
			trace.LoadError = res.err.Error()
			res.err = errors.WithStack(res.err)
		}
	}
}

func (l *Loader) compactJSON(data []byte) ([]byte, error) {
	dst := bytes.NewBuffer(make([]byte, len(data))[:0])
	err := json.Compact(dst, data)
	if err != nil {
		return nil, err
	}
	out := dst.Bytes()
	v, err := astjson.ParseBytesWithoutCache(out)
	if err != nil {
		return nil, err
	}
	astjson.DeduplicateObjectKeysRecursively(v)
	return v.MarshalTo(nil), nil
}
