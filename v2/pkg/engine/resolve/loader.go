package resolve

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"io"
	"net/http/httptrace"
	"slices"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"golang.org/x/sync/errgroup"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

const (
	IntrospectionSchemaTypeDataSourceID     = "introspection__schema&__type"
	IntrospectionTypeFieldsDataSourceID     = "introspection__type__fields"
	IntrospectionTypeEnumValuesDataSourceID = "introspection__type__enumValues"
)

type LoaderHooks interface {
	// OnLoad is called before the fetch is executed
	OnLoad(ctx context.Context, dataSourceID string) context.Context
	// OnFinished is called after the fetch has been executed and the response has been processed and merged
	OnFinished(ctx context.Context, statusCode int, dataSourceID string, err error)
}

func IsIntrospectionDataSource(dataSourceID string) bool {
	return dataSourceID == IntrospectionSchemaTypeDataSourceID || dataSourceID == IntrospectionTypeFieldsDataSourceID || dataSourceID == IntrospectionTypeEnumValuesDataSourceID
}

type Loader struct {
	data       *astjson.JSON
	dataRoot   int
	errorsRoot int
	ctx        *Context
	path       []string
	info       *GraphQLResponseInfo

	propagateSubgraphErrors      bool
	propagateSubgraphStatusCodes bool
	subgraphErrorPropagationMode SubgraphErrorPropagationMode
	rewriteSubgraphErrorPaths    bool
	omitSubgraphErrorLocations   bool
	omitSubgraphErrorExtensions  bool
}

func (l *Loader) Free() {
	l.info = nil
	l.ctx = nil
	l.data = nil
	l.dataRoot = -1
	l.errorsRoot = -1
	l.path = l.path[:0]
}

func (l *Loader) LoadGraphQLResponseData(ctx *Context, response *GraphQLResponse, resolvable *Resolvable) (err error) {
	l.data = resolvable.storage
	l.dataRoot = resolvable.dataRoot
	l.errorsRoot = resolvable.errorsRoot
	l.ctx = ctx
	l.info = response.Info

	// fallback to data mostly for tests
	fetchTree := response.FetchTree
	if response.FetchTree == nil {
		fetchTree = response.Data
	}

	return l.walkNode(fetchTree, []int{resolvable.dataRoot})
}

func (l *Loader) walkNode(node Node, items []int) error {
	switch n := node.(type) {
	case *Object:
		return l.walkObject(n, items)
	case *Array:
		return l.walkArray(n, items)
	default:
		return nil
	}
}

func (l *Loader) pushPath(path []string) {
	l.path = append(l.path, path...)
}

func (l *Loader) popPath(path []string) {
	l.path = l.path[:len(l.path)-len(path)]
}

func (l *Loader) pushArrayPath() {
	l.path = append(l.path, "@")
}

func (l *Loader) popArrayPath() {
	l.path = l.path[:len(l.path)-1]
}

func (l *Loader) renderPath() string {
	builder := strings.Builder{}
	if l.info != nil {
		switch l.info.OperationType {
		case ast.OperationTypeQuery:
			builder.WriteString("query")
		case ast.OperationTypeMutation:
			builder.WriteString("mutation")
		case ast.OperationTypeSubscription:
			builder.WriteString("subscription")
		case ast.OperationTypeUnknown:
		}
	}
	if len(l.path) == 0 {
		return builder.String()
	}
	for i := range l.path {
		builder.WriteByte('.')
		builder.WriteString(l.path[i])
	}
	return builder.String()
}

func (l *Loader) walkObject(object *Object, parentItems []int) (err error) {
	l.pushPath(object.Path)
	defer l.popPath(object.Path)
	objectItems := l.selectNodeItems(parentItems, object.Path)
	if object.Fetch != nil {
		err = l.resolveAndMergeFetch(object.Fetch, objectItems)
		if err != nil {
			return err
		}
	}
	for i := range object.Fields {
		err = l.walkNode(object.Fields[i].Value, objectItems)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (l *Loader) walkArray(array *Array, parentItems []int) error {
	l.pushPath(array.Path)
	l.pushArrayPath()
	nodeItems := l.selectNodeItems(parentItems, array.Path)
	err := l.walkNode(array.Item, nodeItems)
	l.popArrayPath()
	l.popPath(array.Path)
	return err
}

func (l *Loader) selectNodeItems(parentItems []int, path []string) (items []int) {
	if parentItems == nil {
		return nil
	}
	if len(path) == 0 {
		return parentItems
	}
	if len(parentItems) == 1 {
		field := l.data.Get(parentItems[0], path)
		if field == -1 {
			return nil
		}
		if l.data.Nodes[field].Kind == astjson.NodeKindArray {
			return l.data.Nodes[field].ArrayValues
		}
		return []int{field}
	}
	items = make([]int, 0, len(parentItems))
	for _, parent := range parentItems {
		field := l.data.Get(parent, path)
		if field == -1 {
			continue
		}
		if l.data.Nodes[field].Kind == astjson.NodeKindArray {
			items = append(items, l.data.Nodes[field].ArrayValues...)
		} else {
			items = append(items, field)
		}
	}
	return
}

func (l *Loader) itemsData(items []int, out io.Writer) error {
	if len(items) == 0 {
		return nil
	}
	if len(items) == 1 {
		return l.data.PrintNode(l.data.Nodes[items[0]], out)
	}
	return l.data.PrintNode(astjson.Node{
		Kind:        astjson.NodeKindArray,
		ArrayValues: items,
	}, out)
}

func (l *Loader) resolveAndMergeFetch(fetch Fetch, items []int) error {
	switch f := fetch.(type) {
	case *SingleFetch:
		res := &result{
			out: pool.BytesBuffer.Get(),
		}

		err := l.loadSingleFetch(l.ctx.ctx, f, items, res)
		if err != nil {
			return err
		}
		err = l.mergeResult(res, items)
		if l.ctx.LoaderHooks != nil && res.loaderHookContext != nil {
			l.ctx.LoaderHooks.OnFinished(res.loaderHookContext, res.statusCode, res.subgraphName, goerrors.Join(res.err, l.ctx.subgraphErrors))
		}
		if err != nil {
			return err
		}
		return nil
	case *SerialFetch:
		if l.ctx.TracingOptions.Enable {
			f.Trace = &DataSourceLoadTrace{
				Path: l.renderPath(),
			}
		}
		for i := range f.Fetches {
			err := l.resolveAndMergeFetch(f.Fetches[i], items)
			if err != nil {
				return errors.WithStack(err)
			}
		}
	case *ParallelFetch:
		if l.ctx.TracingOptions.Enable {
			f.Trace = &DataSourceLoadTrace{
				Path: l.renderPath(),
			}
		}
		results := make([]*result, len(f.Fetches))
		g, ctx := errgroup.WithContext(l.ctx.ctx)
		for i := range f.Fetches {
			i := i
			results[i] = &result{}
			g.Go(func() error {
				return l.loadFetch(ctx, f.Fetches[i], items, results[i])
			})
		}
		err := g.Wait()
		if err != nil {
			return errors.WithStack(err)
		}
		for i := range results {
			if results[i].nestedMergeItems != nil {
				for j := range results[i].nestedMergeItems {
					err = l.mergeResult(results[i].nestedMergeItems[j], items[j:j+1])
					if l.ctx.LoaderHooks != nil && results[i].nestedMergeItems[j].loaderHookContext != nil {
						l.ctx.LoaderHooks.OnFinished(results[i].nestedMergeItems[j].loaderHookContext, results[i].nestedMergeItems[j].statusCode, results[i].nestedMergeItems[j].subgraphName, goerrors.Join(results[i].nestedMergeItems[j].err, l.ctx.subgraphErrors))
					}
					if err != nil {
						return errors.WithStack(err)
					}
				}
			} else {
				err = l.mergeResult(results[i], items)
				if l.ctx.LoaderHooks != nil && results[i].loaderHookContext != nil {
					l.ctx.LoaderHooks.OnFinished(results[i].loaderHookContext, results[i].statusCode, results[i].subgraphName, goerrors.Join(results[i].err, l.ctx.subgraphErrors))
				}
				if err != nil {
					return errors.WithStack(err)
				}
			}
		}
	case *ParallelListItemFetch:
		if l.ctx.TracingOptions.Enable {
			f.Trace = &DataSourceLoadTrace{
				Path: l.renderPath(),
			}
		}
		results := make([]*result, len(items))
		g, ctx := errgroup.WithContext(l.ctx.ctx)
		for i := range items {
			i := i
			results[i] = &result{
				out: pool.BytesBuffer.Get(),
			}
			g.Go(func() error {
				return l.loadFetch(ctx, f.Fetch, items[i:i+1], results[i])
			})
		}
		err := g.Wait()
		if err != nil {
			return errors.WithStack(err)
		}
		for i := range results {
			err = l.mergeResult(results[i], items[i:i+1])
			if l.ctx.LoaderHooks != nil && results[i].loaderHookContext != nil {
				l.ctx.LoaderHooks.OnFinished(results[i].loaderHookContext, results[i].statusCode, results[i].subgraphName, goerrors.Join(results[i].err, l.ctx.subgraphErrors))
			}
			if err != nil {
				return errors.WithStack(err)
			}
		}
	case *EntityFetch:
		res := &result{
			out: pool.BytesBuffer.Get(),
		}
		err := l.loadEntityFetch(l.ctx.ctx, f, items, res)
		if err != nil {
			return errors.WithStack(err)
		}
		err = l.mergeResult(res, items)
		if l.ctx.LoaderHooks != nil && res.loaderHookContext != nil {
			l.ctx.LoaderHooks.OnFinished(res.loaderHookContext, res.statusCode, res.subgraphName, goerrors.Join(res.err, l.ctx.subgraphErrors))
		}
		return err
	case *BatchEntityFetch:
		res := &result{
			out: pool.BytesBuffer.Get(),
		}
		err := l.loadBatchEntityFetch(l.ctx.ctx, f, items, res)
		if err != nil {
			return errors.WithStack(err)
		}
		err = l.mergeResult(res, items)
		if l.ctx.LoaderHooks != nil && res.loaderHookContext != nil {
			l.ctx.LoaderHooks.OnFinished(res.loaderHookContext, res.statusCode, res.subgraphName, goerrors.Join(res.err, l.ctx.subgraphErrors))
		}
		return err
	}
	return nil
}

func (l *Loader) loadFetch(ctx context.Context, fetch Fetch, items []int, res *result) error {
	switch f := fetch.(type) {
	case *SingleFetch:
		res.out = pool.BytesBuffer.Get()
		return l.loadSingleFetch(ctx, f, items, res)
	case *SerialFetch:
		return fmt.Errorf("serial fetch must not be nested")
	case *ParallelFetch:
		return fmt.Errorf("parallel fetch must not be nested")
	case *ParallelListItemFetch:
		results := make([]*result, len(items))
		if l.ctx.TracingOptions.Enable {
			f.Traces = make([]*SingleFetch, len(items))
		}
		g, ctx := errgroup.WithContext(l.ctx.ctx)
		for i := range items {
			i := i
			results[i] = &result{
				out: pool.BytesBuffer.Get(),
			}
			if l.ctx.TracingOptions.Enable {
				f.Traces[i] = new(SingleFetch)
				*f.Traces[i] = *f.Fetch
				g.Go(func() error {
					return l.loadFetch(ctx, f.Traces[i], items[i:i+1], results[i])
				})
				continue
			}
			g.Go(func() error {
				return l.loadFetch(ctx, f.Fetch, items[i:i+1], results[i])
			})
		}
		err := g.Wait()
		if err != nil {
			return errors.WithStack(err)
		}
		res.nestedMergeItems = results
		return nil
	case *EntityFetch:
		res.out = pool.BytesBuffer.Get()
		return l.loadEntityFetch(ctx, f, items, res)
	case *BatchEntityFetch:
		res.out = pool.BytesBuffer.Get()
		return l.loadBatchEntityFetch(ctx, f, items, res)
	}
	return nil
}

func (l *Loader) mergeResult(res *result, items []int) error {
	defer pool.BytesBuffer.Put(res.out)
	if res.err != nil {
		return l.renderErrorsFailedToFetch(res, failedToFetchNoReason)
	}
	if res.authorizationRejected {
		err := l.renderAuthorizationRejectedErrors(res)
		if err != nil {
			return err
		}
		for _, item := range items {
			l.data.Nodes = append(l.data.Nodes, astjson.Node{
				Kind: astjson.NodeKindNullSkipError,
			})
			ref := len(l.data.Nodes) - 1
			l.data.MergeNodesWithPath(item, ref, res.postProcessing.MergePath)
		}
		return nil
	}
	if res.rateLimitRejected {
		err := l.renderRateLimitRejectedErrors(res)
		if err != nil {
			return err
		}
		for _, item := range items {
			l.data.Nodes = append(l.data.Nodes, astjson.Node{
				Kind: astjson.NodeKindNullSkipError,
			})
			ref := len(l.data.Nodes) - 1
			l.data.MergeNodesWithPath(item, ref, res.postProcessing.MergePath)
		}
		return nil
	}
	if res.fetchSkipped {
		return nil
	}
	if res.out.Len() == 0 {
		return l.renderErrorsFailedToFetch(res, emptyGraphQLResponse)
	}

	node, err := l.data.AppendAnyJSONBytes(res.out.Bytes())
	if err != nil {
		return l.renderErrorsFailedToFetch(res, invalidGraphQLResponse)
	}

	hasErrors := false

	// We check if the subgraph response has errors
	if res.postProcessing.SelectResponseErrorsPath != nil {
		ref := l.data.Get(node, res.postProcessing.SelectResponseErrorsPath)
		if ref != -1 {
			hasErrors = l.data.NodeIsDefined(ref) && len(l.data.Nodes[ref].ArrayValues) > 0
			// Look for errors in the response and merge them into the errors array
			err = l.mergeErrors(res, ref)
			if err != nil {
				return errors.WithStack(err)
			}
		}
	}

	// We also check if any data is there to processed
	if res.postProcessing.SelectResponseDataPath != nil {
		node = l.data.Get(node, res.postProcessing.SelectResponseDataPath)
		// Check if the not set or null
		if !l.data.NodeIsDefined(node) {
			// If we didn't get any data nor errors, we return an error because the response is invalid
			// Returning an error here also avoids the need to walk over it later.
			if !hasErrors {
				return l.renderErrorsFailedToFetch(res, invalidGraphQLResponseShape)
			}
			// no data
			return nil
		}

		// If the data is set, it must be an object according to GraphQL over HTTP spec
		if l.data.Nodes[l.data.RootNode].Kind != astjson.NodeKindObject {
			return l.renderErrorsFailedToFetch(res, invalidGraphQLResponseShape)
		}
	}

	withPostProcessing := res.postProcessing.ResponseTemplate != nil
	if withPostProcessing && len(items) <= 1 {
		postProcessed := pool.BytesBuffer.Get()
		defer pool.BytesBuffer.Put(postProcessed)
		res.out.Reset()
		err = l.data.PrintNode(l.data.Nodes[node], res.out)
		if err != nil {
			return errors.WithStack(err)
		}
		err = res.postProcessing.ResponseTemplate.Render(l.ctx, res.out.Bytes(), postProcessed)
		if err != nil {
			return errors.WithStack(err)
		}
		node, err = l.data.AppendObject(postProcessed.Bytes())
		if err != nil {
			return errors.WithStack(err)
		}
	}
	if len(items) == 0 {
		l.data.RootNode = node
		return nil
	}
	if len(items) == 1 && res.batchStats == nil {
		l.data.MergeNodesWithPath(items[0], node, res.postProcessing.MergePath)
		return nil
	}
	if res.batchStats != nil {
		var (
			postProcessed *bytes.Buffer
			rendered      *bytes.Buffer
		)
		if withPostProcessing {
			postProcessed = pool.BytesBuffer.Get()
			defer pool.BytesBuffer.Put(postProcessed)
			rendered = pool.BytesBuffer.Get()
			defer pool.BytesBuffer.Put(rendered)
			for i, stats := range res.batchStats {
				postProcessed.Reset()
				rendered.Reset()
				_, _ = rendered.Write(lBrack)
				addComma := false
				for _, item := range stats {
					if addComma {
						_, _ = rendered.Write(comma)
					}
					if item == -1 {
						_, _ = rendered.Write(null)
						addComma = true
						continue
					}
					err = l.data.PrintNode(l.data.Nodes[l.data.Nodes[node].ArrayValues[item]], rendered)
					if err != nil {
						return errors.WithStack(err)
					}
					addComma = true
				}
				_, _ = rendered.Write(rBrack)
				err = res.postProcessing.ResponseTemplate.Render(l.ctx, rendered.Bytes(), postProcessed)
				if err != nil {
					return errors.WithStack(err)
				}
				nodeProcessed, err := l.data.AppendObject(postProcessed.Bytes())
				if err != nil {
					return errors.WithStack(err)
				}
				l.data.MergeNodesWithPath(items[i], nodeProcessed, res.postProcessing.MergePath)
			}
		} else {
			for i, stats := range res.batchStats {
				for _, item := range stats {
					if item == -1 {
						continue
					}
					l.data.MergeNodesWithPath(items[i], l.data.Nodes[node].ArrayValues[item], res.postProcessing.MergePath)
				}
			}
		}
	} else {
		for i, item := range items {
			l.data.MergeNodesWithPath(item, l.data.Nodes[node].ArrayValues[i], res.postProcessing.MergePath)
		}
	}
	return nil
}

type result struct {
	postProcessing   PostProcessingConfiguration
	out              *bytes.Buffer
	batchStats       [][]int
	fetchSkipped     bool
	nestedMergeItems []*result

	statusCode   int
	err          error
	subgraphName string

	authorizationRejected        bool
	authorizationRejectedReasons []string

	rateLimitRejected       bool
	rateLimitRejectedReason string

	// loaderHookContext used to share data between the OnLoad and OnFinished hooks
	// Only set when the OnLoad is called
	loaderHookContext context.Context
}

func (r *result) init(postProcessing PostProcessingConfiguration, info *FetchInfo) {
	r.postProcessing = postProcessing
	if info != nil {
		r.subgraphName = info.DataSourceID
	}
}

var (
	errorsInvalidInputHeader = []byte(`{"errors":[{"message":"could not render fetch input","path":[`)
	errorsInvalidInputFooter = []byte(`]}]}`)
)

func (l *Loader) renderErrorsInvalidInput(out *bytes.Buffer) error {
	_, _ = out.Write(errorsInvalidInputHeader)
	for i := range l.path {
		if i != 0 {
			_, _ = out.Write(comma)
		}
		_, _ = out.Write(quote)
		_, _ = out.WriteString(l.path[i])
		_, _ = out.Write(quote)
	}
	_, _ = out.Write(errorsInvalidInputFooter)
	return nil
}

func (l *Loader) mergeErrors(res *result, ref int) error {
	if l.errorsRoot == -1 {
		l.data.Nodes = append(l.data.Nodes, astjson.Node{
			Kind: astjson.NodeKindArray,
		})
		l.errorsRoot = len(l.data.Nodes) - 1
	}

	path := l.renderPath()

	responseErrorsBuf := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(responseErrorsBuf)

	// print them into the buffer to be able to parse them
	err := l.data.PrintNode(l.data.Nodes[ref], responseErrorsBuf)
	if err != nil {
		return err
	}

	// Serialize subgraph errors from the response
	// and append them to the subgraph downsteam errors
	if len(l.data.Nodes[ref].ArrayValues) > 0 {
		graphqlErrors := make([]GraphQLError, 0, len(l.data.Nodes[ref].ArrayValues))
		err = json.Unmarshal(responseErrorsBuf.Bytes(), &graphqlErrors)
		if err != nil {
			return errors.WithStack(err)
		}

		subgraphError := NewSubgraphError(res.subgraphName, path, failedToFetchNoReason, res.statusCode)

		for _, gqlError := range graphqlErrors {
			gErr := gqlError
			subgraphError.AppendDownstreamError(&gErr)
		}

		l.ctx.appendSubgraphError(goerrors.Join(res.err, subgraphError))
	}

	l.optionallyOmitErrorExtensions(ref)
	l.optionallyOmitErrorLocations(ref)
	l.optionallyRewriteErrorPaths(ref)

	if l.subgraphErrorPropagationMode == SubgraphErrorPropagationModePassThrough {
		l.data.MergeArrays(l.errorsRoot, ref)
		return nil
	}

	errorObject, err := l.data.AppendObject([]byte(l.renderSubgraphBaseError(res.subgraphName, path, failedToFetchNoReason)))
	if err != nil {
		return errors.WithStack(err)
	}

	if !l.propagateSubgraphErrors {
		l.data.Nodes[l.errorsRoot].ArrayValues = append(l.data.Nodes[l.errorsRoot].ArrayValues, errorObject)
		return nil
	}

	extensions := l.data.Get(errorObject, []string{"extensions"})
	if extensions == -1 {
		extensions, _ = l.data.AppendObject([]byte(`{}`))
		_ = l.data.SetObjectField(errorObject, extensions, "extensions")
	}
	_ = l.data.SetObjectField(extensions, ref, "errors")
	l.setSubgraphStatusCode(errorObject, res.statusCode)
	l.data.Nodes[l.errorsRoot].ArrayValues = append(l.data.Nodes[l.errorsRoot].ArrayValues, errorObject)

	return nil
}

func (l *Loader) optionallyOmitErrorExtensions(ref int) {
	if !l.omitSubgraphErrorExtensions {
		return
	}
WithNextError:
	for _, i := range l.data.Nodes[ref].ArrayValues {
		if l.data.Nodes[i].Kind != astjson.NodeKindObject {
			continue
		}
		fields := l.data.Nodes[i].ObjectFields
		for j, k := range fields {
			key := l.data.ObjectFieldKey(k)
			if !bytes.Equal(key, literalExtensions) {
				continue
			}
			l.data.Nodes[i].ObjectFields = append(fields[:j], fields[j+1:]...)
			continue WithNextError
		}
	}
}

func (l *Loader) optionallyOmitErrorLocations(ref int) {
	if !l.omitSubgraphErrorLocations {
		return
	}
WithNextError:
	for _, i := range l.data.Nodes[ref].ArrayValues {
		if l.data.Nodes[i].Kind != astjson.NodeKindObject {
			continue
		}
		fields := l.data.Nodes[i].ObjectFields
		for j, k := range fields {
			key := l.data.ObjectFieldKey(k)
			if !bytes.Equal(key, literalLocations) {
				continue
			}
			l.data.Nodes[i].ObjectFields = append(fields[:j], fields[j+1:]...)
			continue WithNextError
		}
	}
}

func (l *Loader) optionallyRewriteErrorPaths(ref int) {
	if !l.rewriteSubgraphErrorPaths {
		return
	}
	pathPrefix := make([]int, len(l.path))
	for i := range l.path {
		str := l.data.AppendString(l.path[i])
		pathPrefix[i] = str
	}
	// remove the trailing @ in case we're in an array as it looks weird in the path
	// errors, like fetches, are attached to objects, not arrays
	if len(l.path) != 0 && l.path[len(l.path)-1] == "@" {
		pathPrefix = pathPrefix[:len(pathPrefix)-1]
	}
WithNextError:
	for _, i := range l.data.Nodes[ref].ArrayValues {
		if l.data.Nodes[i].Kind != astjson.NodeKindObject {
			continue
		}
		fields := l.data.Nodes[i].ObjectFields
		for _, j := range fields {
			key := l.data.ObjectFieldKey(j)
			if !bytes.Equal(key, literalPath) {
				continue
			}
			value := l.data.ObjectFieldValue(j)
			if l.data.Nodes[value].Kind != astjson.NodeKindArray {
				continue
			}
			if len(l.data.Nodes[value].ArrayValues) == 0 {
				continue WithNextError
			}
			v := l.data.Nodes[value].ArrayValues[0]
			if l.data.Nodes[v].Kind != astjson.NodeKindString {
				continue WithNextError
			}
			elem := l.data.Nodes[v].ValueBytes(l.data)
			if !bytes.Equal(elem, literalUnderscoreEntities) {
				continue WithNextError
			}
			l.data.Nodes[value].ArrayValues = append(pathPrefix, l.data.Nodes[value].ArrayValues[1:]...)
		}
	}
}

func (l *Loader) setSubgraphStatusCode(errorObjectRef, statusCode int) {
	if !l.propagateSubgraphStatusCodes {
		return
	}
	if statusCode == 0 {
		return
	}
	ref := l.data.AppendInt(statusCode)
	if ref == -1 {
		return
	}
	extensions := l.data.Get(errorObjectRef, []string{"extensions"})
	if extensions == -1 {
		extensions, _ = l.data.AppendObject([]byte(`{}`))
		_ = l.data.SetObjectField(errorObjectRef, extensions, "extensions")
	}
	_ = l.data.SetObjectField(extensions, ref, "statusCode")
}

const (
	failedToFetchNoReason       = ""
	emptyGraphQLResponse        = "empty response"
	invalidGraphQLResponse      = "invalid JSON"
	invalidGraphQLResponseShape = "no data or errors in response"
)

func (l *Loader) renderErrorsFailedToFetch(res *result, reason string) error {
	path := l.renderPath()
	l.ctx.appendSubgraphError(goerrors.Join(res.err, NewSubgraphError(res.subgraphName, path, reason, res.statusCode)))
	errorObject, err := l.data.AppendObject([]byte(l.renderSubgraphBaseError(res.subgraphName, path, reason)))
	if err != nil {
		return errors.WithStack(err)
	}
	l.setSubgraphStatusCode(errorObject, res.statusCode)
	l.data.Nodes[l.errorsRoot].ArrayValues = append(l.data.Nodes[l.errorsRoot].ArrayValues, errorObject)
	return nil
}

func (l *Loader) renderSubgraphBaseError(subgraphName, path, reason string) string {
	if subgraphName == "" {
		if reason == "" {
			return fmt.Sprintf(`{"message":"Failed to fetch from Subgraph at Path '%s'."}`, path)
		}
		return fmt.Sprintf(`{"message":"Failed to fetch from Subgraph at Path '%s', Reason: %s."}`, path, reason)
	}
	if reason == "" {
		return fmt.Sprintf(`{"message":"Failed to fetch from Subgraph '%s' at Path '%s'."}`, subgraphName, path)
	}
	return fmt.Sprintf(`{"message":"Failed to fetch from Subgraph '%s' at Path '%s', Reason: %s."}`, subgraphName, path, reason)
}

func (l *Loader) renderAuthorizationRejectedErrors(res *result) error {
	path := l.renderPath()
	for i := range res.authorizationRejectedReasons {
		l.ctx.appendSubgraphError(goerrors.Join(res.err, NewSubgraphError(res.subgraphName, path, res.authorizationRejectedReasons[i], res.statusCode)))
	}
	if res.subgraphName == "" {
		for _, reason := range res.authorizationRejectedReasons {
			if reason == "" {
				errorObject, err := l.data.AppendObject([]byte(fmt.Sprintf(`{"message":"Unauthorized Subgraph request at Path '%s'."}`, path)))
				if err != nil {
					return errors.WithStack(err)
				}
				l.data.Nodes[l.errorsRoot].ArrayValues = append(l.data.Nodes[l.errorsRoot].ArrayValues, errorObject)
			} else {
				errorObject, err := l.data.AppendObject([]byte(fmt.Sprintf(`{"message":"Unauthorized Subgraph request at Path '%s', Reason: %s."}`, path, reason)))
				if err != nil {
					return errors.WithStack(err)
				}
				l.data.Nodes[l.errorsRoot].ArrayValues = append(l.data.Nodes[l.errorsRoot].ArrayValues, errorObject)
			}
		}
	} else {
		for _, reason := range res.authorizationRejectedReasons {
			if reason == "" {
				errorObject, err := l.data.AppendObject([]byte(fmt.Sprintf(`{"message":"Unauthorized request to Subgraph '%s' at Path '%s'."}`, res.subgraphName, path)))
				if err != nil {
					return errors.WithStack(err)
				}
				l.data.Nodes[l.errorsRoot].ArrayValues = append(l.data.Nodes[l.errorsRoot].ArrayValues, errorObject)
			} else {
				errorObject, err := l.data.AppendObject([]byte(fmt.Sprintf(`{"message":"Unauthorized request to Subgraph '%s' at Path '%s', Reason: %s."}`, res.subgraphName, path, reason)))
				if err != nil {
					return errors.WithStack(err)
				}
				l.data.Nodes[l.errorsRoot].ArrayValues = append(l.data.Nodes[l.errorsRoot].ArrayValues, errorObject)
			}
		}
	}
	return nil
}

func (l *Loader) renderRateLimitRejectedErrors(res *result) error {
	path := l.renderPath()
	l.ctx.appendSubgraphError(goerrors.Join(res.err, NewRateLimitError(res.subgraphName, path, res.rateLimitRejectedReason)))

	if res.subgraphName == "" {
		if res.rateLimitRejectedReason == "" {
			errorObject, err := l.data.AppendObject([]byte(fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph request at Path '%s'."}`, path)))
			if err != nil {
				return errors.WithStack(err)
			}
			l.data.Nodes[l.errorsRoot].ArrayValues = append(l.data.Nodes[l.errorsRoot].ArrayValues, errorObject)
		} else {
			errorObject, err := l.data.AppendObject([]byte(fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph request at Path '%s', Reason: %s."}`, path, res.rateLimitRejectedReason)))
			if err != nil {
				return errors.WithStack(err)
			}
			l.data.Nodes[l.errorsRoot].ArrayValues = append(l.data.Nodes[l.errorsRoot].ArrayValues, errorObject)
		}
	} else {
		if res.rateLimitRejectedReason == "" {
			errorObject, err := l.data.AppendObject([]byte(fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph '%s' at Path '%s'."}`, res.subgraphName, path)))
			if err != nil {
				return errors.WithStack(err)
			}
			l.data.Nodes[l.errorsRoot].ArrayValues = append(l.data.Nodes[l.errorsRoot].ArrayValues, errorObject)
		} else {
			errorObject, err := l.data.AppendObject([]byte(fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph '%s' at Path '%s', Reason: %s."}`, res.subgraphName, path, res.rateLimitRejectedReason)))
			if err != nil {
				return errors.WithStack(err)
			}
			l.data.Nodes[l.errorsRoot].ArrayValues = append(l.data.Nodes[l.errorsRoot].ArrayValues, errorObject)
		}
	}
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

func (l *Loader) loadSingleFetch(ctx context.Context, fetch *SingleFetch, items []int, res *result) error {
	res.init(fetch.PostProcessing, fetch.Info)
	input := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(input)
	preparedInput := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(preparedInput)
	err := l.itemsData(items, input)
	if err != nil {
		return errors.WithStack(err)
	}
	if l.ctx.TracingOptions.Enable {
		fetch.Trace = &DataSourceLoadTrace{}
		if !l.ctx.TracingOptions.ExcludeRawInputData {
			inputCopy := make([]byte, input.Len())
			copy(inputCopy, input.Bytes())
			fetch.Trace.RawInputData = inputCopy
		}
	}
	err = fetch.InputTemplate.Render(l.ctx, input.Bytes(), preparedInput)
	if err != nil {
		return l.renderErrorsInvalidInput(res.out)
	}
	fetchInput := preparedInput.Bytes()
	allowed, err := l.validatePreFetch(fetchInput, fetch.Info, res)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
	l.executeSourceLoad(ctx, fetch.DataSource, fetchInput, res, fetch.Trace)
	return nil
}

func (l *Loader) loadEntityFetch(ctx context.Context, fetch *EntityFetch, items []int, res *result) error {
	res.init(fetch.PostProcessing, fetch.Info)
	itemData := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(itemData)
	preparedInput := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(preparedInput)
	item := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(item)
	err := l.itemsData(items, itemData)
	if err != nil {
		return errors.WithStack(err)
	}

	if l.ctx.TracingOptions.Enable {
		fetch.Trace = &DataSourceLoadTrace{}
		if !l.ctx.TracingOptions.ExcludeRawInputData {
			itemDataCopy := make([]byte, itemData.Len())
			copy(itemDataCopy, itemData.Bytes())
			fetch.Trace.RawInputData = itemDataCopy
		}
	}

	var undefinedVariables []string

	err = fetch.Input.Header.RenderAndCollectUndefinedVariables(l.ctx, nil, preparedInput, &undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}

	err = fetch.Input.Item.Render(l.ctx, itemData.Bytes(), item)
	if err != nil {
		if fetch.Input.SkipErrItem {
			err = nil // nolint:ineffassign
			// skip fetch on render item error
			if l.ctx.TracingOptions.Enable {
				fetch.Trace.LoadSkipped = true
			}
			return nil
		}
		return errors.WithStack(err)
	}
	renderedItem := item.Bytes()
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
	_, _ = item.WriteTo(preparedInput)
	err = fetch.Input.Footer.RenderAndCollectUndefinedVariables(l.ctx, nil, preparedInput, &undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}

	err = SetInputUndefinedVariables(preparedInput, undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}
	fetchInput := preparedInput.Bytes()

	if l.ctx.TracingOptions.Enable && res.fetchSkipped {
		l.setTracingInput(fetchInput, fetch.Trace)
		return nil
	}

	allowed, err := l.validatePreFetch(fetchInput, fetch.Info, res)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
	l.executeSourceLoad(ctx, fetch.DataSource, fetchInput, res, fetch.Trace)
	return nil
}

func (l *Loader) loadBatchEntityFetch(ctx context.Context, fetch *BatchEntityFetch, items []int, res *result) error {
	res.init(fetch.PostProcessing, fetch.Info)

	if l.ctx.TracingOptions.Enable {
		fetch.Trace = &DataSourceLoadTrace{}
		if !l.ctx.TracingOptions.ExcludeRawInputData {
			buf := &bytes.Buffer{}
			err := l.itemsData(items, buf)
			if err != nil {
				return errors.WithStack(err)
			}
			fetch.Trace.RawInputData = buf.Bytes()
		}
	}

	preparedInput := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(preparedInput)

	var undefinedVariables []string

	err := fetch.Input.Header.RenderAndCollectUndefinedVariables(l.ctx, nil, preparedInput, &undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}
	res.batchStats = make([][]int, len(items))
	itemHashes := make([]uint64, 0, len(items)*len(fetch.Input.Items))
	batchItemIndex := 0
	addSeparator := false

	keyGen := pool.Hash64.Get()
	defer pool.Hash64.Put(keyGen)

	itemData := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(itemData)

	itemInput := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(itemInput)

WithNextItem:
	for i, item := range items {
		itemData.Reset()
		err = l.data.PrintNode(l.data.Nodes[item], itemData)
		if err != nil {
			return errors.WithStack(err)
		}
		for j := range fetch.Input.Items {
			itemInput.Reset()
			err = fetch.Input.Items[j].Render(l.ctx, itemData.Bytes(), itemInput)
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
			if fetch.Input.SkipNullItems && itemInput.Len() == 4 && bytes.Equal(itemInput.Bytes(), null) {
				res.batchStats[i] = append(res.batchStats[i], -1)
				continue
			}
			if fetch.Input.SkipEmptyObjectItems && itemInput.Len() == 2 && bytes.Equal(itemInput.Bytes(), emptyObject) {
				res.batchStats[i] = append(res.batchStats[i], -1)
				continue
			}

			keyGen.Reset()
			_, _ = keyGen.Write(itemInput.Bytes())
			itemHash := keyGen.Sum64()
			for k := range itemHashes {
				if itemHashes[k] == itemHash {
					res.batchStats[i] = append(res.batchStats[i], k)
					continue WithNextItem
				}
			}
			itemHashes = append(itemHashes, itemHash)
			if addSeparator {
				err = fetch.Input.Separator.Render(l.ctx, nil, preparedInput)
				if err != nil {
					return errors.WithStack(err)
				}
			}
			_, _ = itemInput.WriteTo(preparedInput)
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

	err = fetch.Input.Footer.RenderAndCollectUndefinedVariables(l.ctx, nil, preparedInput, &undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}

	err = SetInputUndefinedVariables(preparedInput, undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}
	fetchInput := preparedInput.Bytes()

	if l.ctx.TracingOptions.Enable && res.fetchSkipped {
		l.setTracingInput(fetchInput, fetch.Trace)
		return nil
	}

	allowed, err := l.validatePreFetch(fetchInput, fetch.Info, res)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
	l.executeSourceLoad(ctx, fetch.DataSource, fetchInput, res, fetch.Trace)
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

func (l *Loader) setTracingInput(input []byte, trace *DataSourceLoadTrace) {
	trace.Path = l.renderPath()
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

func (l *Loader) executeSourceLoad(ctx context.Context, source DataSource, input []byte, res *result, trace *DataSourceLoadTrace) {
	if l.ctx.Extensions != nil {
		input, res.err = jsonparser.Set(input, l.ctx.Extensions, "body", "extensions")
		if res.err != nil {
			res.err = errors.WithStack(res.err)
			return
		}
	}
	if l.ctx.TracingOptions.Enable {
		ctx = setSingleFlightStats(ctx, &SingleFlightStats{})
		trace.Path = l.renderPath()
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
		res.loaderHookContext = l.ctx.LoaderHooks.OnLoad(ctx, res.subgraphName)

		// Prevent that the context is destroyed when the loader hook return an empty context
		if res.loaderHookContext != nil {
			res.err = l.loadByContext(res.loaderHookContext, source, input, res)
		} else {
			res.err = l.loadByContext(ctx, source, input, res)
		}

	} else {
		res.err = l.loadByContext(ctx, source, input, res)
	}

	res.statusCode = responseContext.StatusCode

	l.ctx.Stats.NumberOfFetches.Inc()
	l.ctx.Stats.CombinedResponseSize.Add(int64(res.out.Len()))

	if l.ctx.TracingOptions.Enable {
		stats := GetSingleFlightStats(ctx)
		if stats != nil {
			trace.SingleFlightUsed = stats.SingleFlightUsed
			trace.SingleFlightSharedResponse = stats.SingleFlightSharedResponse
		}
		if !l.ctx.TracingOptions.ExcludeOutput && res.out.Len() > 0 {
			if l.ctx.TracingOptions.EnablePredictableDebugTimings {
				dataCopy := make([]byte, res.out.Len())
				copy(dataCopy, res.out.Bytes())
				trace.Output = jsonparser.Delete(dataCopy, "extensions", "trace", "response", "headers", "Date")
			} else {
				trace.Output = make([]byte, res.out.Len())
				copy(trace.Output, res.out.Bytes())
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
