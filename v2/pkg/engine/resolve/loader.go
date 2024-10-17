package resolve

import (
	"bytes"
	"context"
	"crypto/tls"
	goerrors "errors"
	"fmt"
	"io"
	"net/http/httptrace"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash/v2"
	"github.com/goccy/go-json"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/wundergraph/astjson"
	"golang.org/x/sync/errgroup"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
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
	OnFinished(ctx context.Context, statusCode int, ds DataSourceInfo, err error)
}

func IsIntrospectionDataSource(dataSourceID string) bool {
	return dataSourceID == IntrospectionSchemaTypeDataSourceID || dataSourceID == IntrospectionTypeFieldsDataSourceID || dataSourceID == IntrospectionTypeEnumValuesDataSourceID
}

var (
	loaderBufPool = sync.Pool{}
)

func acquireLoaderBuf() *bytes.Buffer {
	v := loaderBufPool.Get()
	if v == nil {
		return bytes.NewBuffer(make([]byte, 0, 1024))
	}
	return v.(*bytes.Buffer)
}

func releaseLoaderBuf(buf *bytes.Buffer) {
	buf.Reset()
	loaderBufPool.Put(buf)
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
	allowedErrorExtensionFields       map[string]struct{}
	defaultErrorExtensionCode         string
	allowedSubgraphErrorFields        map[string]struct{}
}

func (l *Loader) Free() {
	l.info = nil
	l.ctx = nil
	l.resolvable = nil
}

func (l *Loader) LoadGraphQLResponseData(ctx *Context, response *GraphQLResponse, resolvable *Resolvable) (err error) {
	l.resolvable = resolvable
	l.ctx = ctx
	l.info = response.Info
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
		g.Go(func() error {
			return l.loadFetch(ctx, nodes[i].Item.Fetch, nodes[i].Item, itemsItems[i], results[i])
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
					l.ctx.LoaderHooks.OnFinished(results[i].nestedMergeItems[j].loaderHookContext, results[i].nestedMergeItems[j].statusCode, results[i].nestedMergeItems[j].ds, goerrors.Join(results[i].nestedMergeItems[j].err, l.ctx.subgraphErrors))
				}
				if err != nil {
					return errors.WithStack(err)
				}
			}
		} else {
			err = l.mergeResult(nodes[i].Item, results[i], itemsItems[i])
			if l.ctx.LoaderHooks != nil && results[i].loaderHookContext != nil {
				l.ctx.LoaderHooks.OnFinished(results[i].loaderHookContext, results[i].statusCode, results[i].ds, goerrors.Join(results[i].err, l.ctx.subgraphErrors))
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
			out: acquireLoaderBuf(),
		}
		err := l.loadSingleFetch(l.ctx.ctx, f, item, items, res)
		if err != nil {
			return err
		}
		err = l.mergeResult(item, res, items)
		if l.ctx.LoaderHooks != nil && res.loaderHookContext != nil {
			l.ctx.LoaderHooks.OnFinished(res.loaderHookContext, res.statusCode, res.ds, goerrors.Join(res.err, l.ctx.subgraphErrors))
		}
		return err
	case *BatchEntityFetch:
		res := &result{
			out: acquireLoaderBuf(),
		}
		err := l.loadBatchEntityFetch(l.ctx.ctx, item, f, items, res)
		if err != nil {
			return errors.WithStack(err)
		}
		err = l.mergeResult(item, res, items)
		if l.ctx.LoaderHooks != nil && res.loaderHookContext != nil {
			l.ctx.LoaderHooks.OnFinished(res.loaderHookContext, res.statusCode, res.ds, goerrors.Join(res.err, l.ctx.subgraphErrors))
		}
		return err
	case *EntityFetch:
		res := &result{
			out: acquireLoaderBuf(),
		}
		err := l.loadEntityFetch(l.ctx.ctx, item, f, items, res)
		if err != nil {
			return errors.WithStack(err)
		}
		err = l.mergeResult(item, res, items)
		if l.ctx.LoaderHooks != nil && res.loaderHookContext != nil {
			l.ctx.LoaderHooks.OnFinished(res.loaderHookContext, res.statusCode, res.ds, goerrors.Join(res.err, l.ctx.subgraphErrors))
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
				out: acquireLoaderBuf(),
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
			if l.ctx.LoaderHooks != nil && results[i].loaderHookContext != nil {
				l.ctx.LoaderHooks.OnFinished(results[i].loaderHookContext, results[i].statusCode, results[i].ds, goerrors.Join(results[i].err, l.ctx.subgraphErrors))
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
	if len(path) == 0 {
		return []*astjson.Value{l.resolvable.data}
	}
	items := []*astjson.Value{l.resolvable.data}
	for i := range path {
		if len(items) == 0 {
			break
		}
		if path[i].Kind == FetchItemPathElementKindObject {
			items = l.selectObjectItems(items, path[i].Path)
		}
		if path[i].Kind == FetchItemPathElementKindArray {
			items = l.selectArrayItems(items, path[i].Path)
		}
	}
	return items
}

func (l *Loader) selectObjectItems(items []*astjson.Value, path []string) []*astjson.Value {
	if len(items) == 0 {
		return nil
	}
	if len(path) == 0 {
		return items
	}
	if len(items) == 1 {
		field := items[0].Get(path...)
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
		field := item.Get(path...)
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

func (l *Loader) selectArrayItems(items []*astjson.Value, path []string) []*astjson.Value {
	if len(items) == 0 {
		return nil
	}
	if len(path) == 0 {
		return items
	}
	if len(items) == 1 {
		field := items[0].Get(path...)
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
		field := item.Get(path...)
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

func (l *Loader) selectNodeItems(parentItems []*astjson.Value, path []string) (items []*astjson.Value) {
	if parentItems == nil {
		return nil
	}
	if len(path) == 0 {
		return parentItems
	}
	if len(parentItems) == 1 {
		field := parentItems[0].Get(path...)
		if field == nil {
			return nil
		}
		if field.Type() == astjson.TypeArray {
			return field.GetArray()
		}
		return []*astjson.Value{field}
	}
	items = make([]*astjson.Value, 0, len(parentItems))
	for _, parent := range parentItems {
		field := parent.Get(path...)
		if field == nil {
			continue
		}
		if field.Type() == astjson.TypeArray {
			items = append(items, field.GetArray()...)
			continue
		}
		items = append(items, field)
	}
	return
}

func (l *Loader) itemsData(items []*astjson.Value, out io.Writer) {
	if len(items) == 0 {
		return
	}
	if len(items) == 1 {
		data := items[0].MarshalTo(nil)
		_, _ = out.Write(data)
		return
	}
	_, _ = out.Write(lBrack)
	var data []byte
	for i, item := range items {
		if i != 0 {
			_, _ = out.Write(comma)
		}
		data = item.MarshalTo(data[:0])
		_, _ = out.Write(data)
	}
	_, _ = out.Write(rBrack)
}

func (l *Loader) loadFetch(ctx context.Context, fetch Fetch, fetchItem *FetchItem, items []*astjson.Value, res *result) error {
	switch f := fetch.(type) {
	case *SingleFetch:
		res.out = acquireLoaderBuf()
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
				out: acquireLoaderBuf(),
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
		res.out = acquireLoaderBuf()
		return l.loadEntityFetch(ctx, fetchItem, f, items, res)
	case *BatchEntityFetch:
		res.out = acquireLoaderBuf()
		return l.loadBatchEntityFetch(ctx, fetchItem, f, items, res)
	}
	return nil
}

func (l *Loader) mergeResult(fetchItem *FetchItem, res *result, items []*astjson.Value) error {
	defer releaseLoaderBuf(res.out)
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
	value, err := l.resolvable.parseJSON(res.out.Bytes())
	if err != nil {
		return l.renderErrorsFailedToFetch(fetchItem, res, invalidGraphQLResponse)
	}

	hasErrors := false

	// We check if the subgraph response has errors
	if res.postProcessing.SelectResponseErrorsPath != nil {
		errorsValue := value.Get(res.postProcessing.SelectResponseErrorsPath...)
		if astjson.ValueIsNonNull(errorsValue) {
			errorObjects := errorsValue.GetArray()
			hasErrors = len(errorObjects) > 0
			// If errors field are present in response, but the errors array is empty, we don't consider it as an error
			// Note: it is not compliant to graphql spec
			if hasErrors {
				// Look for errors in the response and merge them into the errors array
				err = l.mergeErrors(res, fetchItem, errorsValue, errorObjects)
				if err != nil {
					return errors.WithStack(err)
				}
			}
		}
	}

	// We also check if any data is there to processed
	if res.postProcessing.SelectResponseDataPath != nil {
		value = value.Get(res.postProcessing.SelectResponseDataPath...)
		// Check if the not set or null
		if astjson.ValueIsNull(value) {
			// If we didn't get any data nor errors, we return an error because the response is invalid
			// Returning an error here also avoids the need to walk over it later.
			if !hasErrors && !l.resolvable.options.ApolloCompatibilitySuppressFetchErrors {
				return l.renderErrorsFailedToFetch(fetchItem, res, invalidGraphQLResponseShape)
			}
			// no data
			return nil
		}
	}

	withPostProcessing := res.postProcessing.ResponseTemplate != nil
	if withPostProcessing && len(items) <= 1 {
		postProcessed := &bytes.Buffer{}
		valueJSON := value.MarshalTo(nil)
		err = res.postProcessing.ResponseTemplate.Render(l.ctx, valueJSON, postProcessed)
		if err != nil {
			return errors.WithStack(err)
		}
		value, err = l.resolvable.parseJSON(postProcessed.Bytes())
		if err != nil {
			return errors.WithStack(err)
		}
	}
	if len(items) == 0 {
		// If the data is set, it must be an object according to GraphQL over HTTP spec
		if value.Type() != astjson.TypeObject {
			return l.renderErrorsFailedToFetch(fetchItem, res, invalidGraphQLResponseShape)
		}
		l.resolvable.data = value
		return nil
	}
	if len(items) == 1 && res.batchStats == nil {
		astjson.MergeValuesWithPath(items[0], value, res.postProcessing.MergePath...)
		return nil
	}
	batch := value.GetArray()
	if batch == nil {
		return l.renderErrorsFailedToFetch(fetchItem, res, invalidGraphQLResponseShape)
	}
	if res.batchStats != nil {
		var (
			postProcessed *bytes.Buffer
			rendered      *bytes.Buffer
			itemBuffer    = make([]byte, 0, 1024)
		)
		if withPostProcessing {
			postProcessed = &bytes.Buffer{}
			rendered = &bytes.Buffer{}
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
					itemBuffer = batch[item].MarshalTo(itemBuffer[:0])
					_, _ = rendered.Write(itemBuffer)
					addComma = true
				}
				_, _ = rendered.Write(rBrack)
				err = res.postProcessing.ResponseTemplate.Render(l.ctx, rendered.Bytes(), postProcessed)
				if err != nil {
					return errors.WithStack(err)
				}
				nodeProcessed := astjson.MustParseBytes(postProcessed.Bytes())
				astjson.MergeValuesWithPath(items[i], nodeProcessed, res.postProcessing.MergePath...)
			}
		} else {
			for i, stats := range res.batchStats {
				for _, item := range stats {
					if item == -1 {
						continue
					}
					astjson.MergeValuesWithPath(items[i], batch[item], res.postProcessing.MergePath...)
				}
			}
		}
	} else {
		for i, item := range items {
			astjson.MergeValuesWithPath(item, batch[i], res.postProcessing.MergePath...)
		}
	}
	return nil
}

type DataSourceInfo struct {
	ID   string
	Name string
}

type result struct {
	postProcessing   PostProcessingConfiguration
	out              *bytes.Buffer
	batchStats       [][]int
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
	// Only set when the OnLoad is called
	loaderHookContext context.Context
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

	l.ctx.appendSubgraphError(goerrors.Join(res.err, subgraphError))

	return nil
}

func (l *Loader) mergeErrors(res *result, fetchItem *FetchItem, value *astjson.Value, values []*astjson.Value) error {
	l.optionallyOmitErrorLocations(values)
	l.optionallyRewriteErrorPaths(fetchItem, values)
	l.optionallyAllowCustomExtensionProperties(values)
	l.optionallyEnsureExtensionErrorCode(values)

	if l.subgraphErrorPropagationMode == SubgraphErrorPropagationModePassThrough {
		// Attach datasource information to all errors when we don't wrap them
		l.optionallyAttachServiceNameToErrorExtension(values, res.ds.Name)
		l.setSubgraphStatusCode(values, res.statusCode)

		// Allow to delete extensions entirely
		l.optionallyOmitErrorExtensions(values)

		l.optionallyOmitErrorFields(values)

		if len(values) > 0 {
			// Append the subgraph errors to the response payload
			if err := l.appendSubgraphError(res, fetchItem, value, values); err != nil {
				return err
			}
		}

		// If the error propagation mode is pass-through, we append the errors to the root array
		astjson.MergeValues(l.resolvable.errors, value)
		return nil
	}

	if len(values) > 0 {
		// Append the subgraph errors to the response payload
		if err := l.appendSubgraphError(res, fetchItem, value, values); err != nil {
			return err
		}
	}

	// Wrap mode (default)

	errorObject := astjson.MustParse(l.renderSubgraphBaseError(res.ds, fetchItem.ResponsePath, failedToFetchNoReason))
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

	astjson.AppendToArray(l.resolvable.errors, errorObject)

	return nil
}

// optionallyAllowCustomExtensionProperties removes all properties from the extensions object that are not in the allowedProperties map
// If no properties are left, the extensions object is removed
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

// optionallyEnsureExtensionErrorCode ensures that all values have an error code in the extensions object
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

// optionallyAttachServiceNameToErrorExtension attaches the service name to the extensions object of all values
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

// optionallyOmitErrorExtensions removes the extensions object from all values
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

// optionallyOmitErrorFields removes all fields from the subgraph error which are not whitelisted. We do not remove message.
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

// optionallyOmitErrorLocations removes the locations object from all values
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

// optionallyRewriteErrorPaths rewrites the path field of all values
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
					newPath = append(newPath, unsafebytes.BytesToString(pathItems[j].GetStringBytes()))
				}
				newPathJSON, _ := json.Marshal(newPath)
				value.Set("path", astjson.MustParseBytes(newPathJSON))
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
			extensions.Set("statusCode", astjson.MustParse(strconv.Itoa(statusCode)))
		} else {
			value.Set("extensions", astjson.MustParse(`{"statusCode":`+strconv.Itoa(statusCode)+`}`))
		}
	}
}

const (
	failedToFetchNoReason       = ""
	emptyGraphQLResponse        = "empty response"
	invalidGraphQLResponse      = "invalid JSON"
	invalidGraphQLResponseShape = "no data or errors in response"
)

func (l *Loader) renderAtPathErrorPart(path string) string {
	if path == "" {
		return ""
	}
	return fmt.Sprintf(` at Path '%s'`, path)
}

func (l *Loader) renderErrorsFailedToFetch(fetchItem *FetchItem, res *result, reason string) error {
	l.ctx.appendSubgraphError(goerrors.Join(res.err, NewSubgraphError(res.ds, fetchItem.ResponsePath, reason, res.statusCode)))
	errorObject, err := astjson.Parse(l.renderSubgraphBaseError(res.ds, fetchItem.ResponsePath, reason))
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
		l.ctx.appendSubgraphError(goerrors.Join(res.err, NewSubgraphError(res.ds, fetchItem.ResponsePath, res.authorizationRejectedReasons[i], res.statusCode)))
	}
	pathPart := l.renderAtPathErrorPart(fetchItem.ResponsePath)
	if res.ds.Name == "" {
		for _, reason := range res.authorizationRejectedReasons {
			if reason == "" {
				errorObject := astjson.MustParse(fmt.Sprintf(`{"message":"Unauthorized Subgraph request%s."}`, pathPart))
				astjson.AppendToArray(l.resolvable.errors, errorObject)
			} else {
				errorObject := astjson.MustParse(fmt.Sprintf(`{"message":"Unauthorized Subgraph request%s, Reason: %s."}`, pathPart, reason))
				astjson.AppendToArray(l.resolvable.errors, errorObject)
			}
		}
	} else {
		for _, reason := range res.authorizationRejectedReasons {
			if reason == "" {
				errorObject := astjson.MustParse(fmt.Sprintf(`{"message":"Unauthorized request to Subgraph '%s'%s."}`, res.ds.Name, pathPart))
				astjson.AppendToArray(l.resolvable.errors, errorObject)
			} else {
				errorObject := astjson.MustParse(fmt.Sprintf(`{"message":"Unauthorized request to Subgraph '%s'%s, Reason: %s."}`, res.ds.Name, pathPart, reason))
				astjson.AppendToArray(l.resolvable.errors, errorObject)
			}
		}
	}
	return nil
}

func (l *Loader) renderRateLimitRejectedErrors(fetchItem *FetchItem, res *result) error {
	l.ctx.appendSubgraphError(goerrors.Join(res.err, NewRateLimitError(res.ds.Name, fetchItem.ResponsePath, res.rateLimitRejectedReason)))
	pathPart := l.renderAtPathErrorPart(fetchItem.ResponsePath)
	if res.ds.Name == "" {
		if res.rateLimitRejectedReason == "" {
			errorObject := astjson.MustParse(fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph request%s."}`, pathPart))
			astjson.AppendToArray(l.resolvable.errors, errorObject)
		} else {
			errorObject := astjson.MustParse(fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph request%s, Reason: %s."}`, pathPart, res.rateLimitRejectedReason))
			astjson.AppendToArray(l.resolvable.errors, errorObject)
		}
	} else {
		if res.rateLimitRejectedReason == "" {
			errorObject := astjson.MustParse(fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph '%s'%s."}`, res.ds.Name, pathPart))
			astjson.AppendToArray(l.resolvable.errors, errorObject)
		} else {
			errorObject := astjson.MustParse(fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph '%s'%s, Reason: %s."}`, res.ds.Name, pathPart, res.rateLimitRejectedReason))
			astjson.AppendToArray(l.resolvable.errors, errorObject)
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

var (
	singleFetchPool = sync.Pool{
		New: func() any {
			return &singleFetchBuffer{
				input:         &bytes.Buffer{},
				preparedInput: &bytes.Buffer{},
			}
		},
	}
)

type singleFetchBuffer struct {
	input         *bytes.Buffer
	preparedInput *bytes.Buffer
}

func acquireSingleFetchBuffer() *singleFetchBuffer {
	return singleFetchPool.Get().(*singleFetchBuffer)
}

func releaseSingleFetchBuffer(buf *singleFetchBuffer) {
	buf.input.Reset()
	buf.preparedInput.Reset()
	singleFetchPool.Put(buf)
}

func (l *Loader) loadSingleFetch(ctx context.Context, fetch *SingleFetch, fetchItem *FetchItem, items []*astjson.Value, res *result) error {
	res.init(fetch.PostProcessing, fetch.Info)
	buf := acquireSingleFetchBuffer()
	defer releaseSingleFetchBuffer(buf)
	l.itemsData(items, buf.input)
	if l.ctx.TracingOptions.Enable {
		fetch.Trace = &DataSourceLoadTrace{}
		if !l.ctx.TracingOptions.ExcludeRawInputData {
			fetch.Trace.RawInputData, _ = l.compactJSON(buf.input.Bytes())
		}
	}
	err := fetch.InputTemplate.Render(l.ctx, buf.input.Bytes(), buf.preparedInput)
	if err != nil {
		return l.renderErrorsInvalidInput(fetchItem, res.out)
	}
	fetchInput := buf.preparedInput.Bytes()
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
				itemData:      &bytes.Buffer{},
				preparedInput: &bytes.Buffer{},
			}
		},
	}
)

type entityFetchBuffer struct {
	item          *bytes.Buffer
	itemData      *bytes.Buffer
	preparedInput *bytes.Buffer
}

func acquireEntityFetchBuffer() *entityFetchBuffer {
	return entityFetchPool.Get().(*entityFetchBuffer)
}

func releaseEntityFetchBuffer(buf *entityFetchBuffer) {
	buf.item.Reset()
	buf.itemData.Reset()
	buf.preparedInput.Reset()
	entityFetchPool.Put(buf)
}

func (l *Loader) loadEntityFetch(ctx context.Context, fetchItem *FetchItem, fetch *EntityFetch, items []*astjson.Value, res *result) error {
	res.init(fetch.PostProcessing, fetch.Info)
	buf := acquireEntityFetchBuffer()
	defer releaseEntityFetchBuffer(buf)
	l.itemsData(items, buf.itemData)

	if l.ctx.TracingOptions.Enable {
		fetch.Trace = &DataSourceLoadTrace{}
		if !l.ctx.TracingOptions.ExcludeRawInputData {
			fetch.Trace.RawInputData, _ = l.compactJSON(buf.itemData.Bytes())
		}
	}

	var undefinedVariables []string

	err := fetch.Input.Header.RenderAndCollectUndefinedVariables(l.ctx, nil, buf.preparedInput, &undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}

	err = fetch.Input.Item.Render(l.ctx, buf.itemData.Bytes(), buf.item)
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
		if !l.ctx.TracingOptions.ExcludeRawInputData {
			buf := &bytes.Buffer{}
			l.itemsData(items, buf)
			fetch.Trace.RawInputData, _ = l.compactJSON(buf.Bytes())
		}
	}

	var undefinedVariables []string

	err := fetch.Input.Header.RenderAndCollectUndefinedVariables(l.ctx, nil, buf.preparedInput, &undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}
	res.batchStats = make([][]int, len(items))
	itemHashes := make([]uint64, 0, len(items)*len(fetch.Input.Items))
	batchItemIndex := 0
	addSeparator := false
	itemData := make([]byte, 0, 1024)

WithNextItem:
	for i, item := range items {
		itemData = item.MarshalTo(itemData[:0])
		for j := range fetch.Input.Items {
			buf.itemInput.Reset()
			err = fetch.Input.Items[j].Render(l.ctx, itemData, buf.itemInput)
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
	v, err := astjson.ParseBytes(out)
	if err != nil {
		return nil, err
	}
	astjson.DeduplicateObjectKeysRecursively(v)
	return v.MarshalTo(nil), nil
}
