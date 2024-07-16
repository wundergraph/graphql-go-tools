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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash/v2"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/valyala/fastjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
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

var (
	loaderBufPool = sync.Pool{}
	loaderBufSize = atomic.NewInt32(128)
)

func acquireLoaderBuf() *bytes.Buffer {
	v := loaderBufPool.Get()
	if v == nil {
		return bytes.NewBuffer(make([]byte, 0, loaderBufSize.Load()))
	}
	return v.(*bytes.Buffer)
}

func releaseLoaderBuf(buf *bytes.Buffer) {
	loaderBufSize.Store(int32(buf.Cap()))
	buf.Reset()
	loaderBufPool.Put(buf)
}

type Loader struct {
	resolvable *Resolvable
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
	l.resolvable = nil
	l.path = l.path[:0]
}

func (l *Loader) LoadGraphQLResponseData(ctx *Context, response *GraphQLResponse, resolvable *Resolvable) (err error) {
	l.resolvable = resolvable
	l.ctx = ctx
	l.info = response.Info

	// fallback to data mostly for tests
	fetchTree := response.FetchTree
	if response.FetchTree == nil {
		fetchTree = response.Data
	}

	return l.walkNode(fetchTree, []*fastjson.Value{resolvable.data})
}

func (l *Loader) walkNode(node Node, items []*fastjson.Value) error {
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

func (l *Loader) walkObject(object *Object, parentItems []*fastjson.Value) (err error) {
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

func (l *Loader) walkArray(array *Array, parentItems []*fastjson.Value) error {
	l.pushPath(array.Path)
	l.pushArrayPath()
	nodeItems := l.selectNodeItems(parentItems, array.Path)
	err := l.walkNode(array.Item, nodeItems)
	l.popArrayPath()
	l.popPath(array.Path)
	return err
}

func (l *Loader) selectNodeItems(parentItems []*fastjson.Value, path []string) (items []*fastjson.Value) {
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
		if field.Type() == fastjson.TypeArray {
			return field.GetArray()
		}
		return []*fastjson.Value{field}
	}
	items = make([]*fastjson.Value, 0, len(parentItems))
	for _, parent := range parentItems {
		field := parent.Get(path...)
		if field == nil {
			continue
		}
		if field.Type() == fastjson.TypeArray {
			items = append(items, field.GetArray()...)
			continue
		}
		items = append(items, field)
	}
	return
}

func (l *Loader) itemsData(items []*fastjson.Value, out io.Writer) {
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
	return
}

func (l *Loader) resolveAndMergeFetch(fetch Fetch, items []*fastjson.Value) error {
	switch f := fetch.(type) {
	case *SingleFetch:
		res := &result{
			out: acquireLoaderBuf(),
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
				out: acquireLoaderBuf(),
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
			out: acquireLoaderBuf(),
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
			out: acquireLoaderBuf(),
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

func (l *Loader) loadFetch(ctx context.Context, fetch Fetch, items []*fastjson.Value, res *result) error {
	switch f := fetch.(type) {
	case *SingleFetch:
		res.out = acquireLoaderBuf()
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
				out: acquireLoaderBuf(),
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
		res.out = acquireLoaderBuf()
		return l.loadEntityFetch(ctx, f, items, res)
	case *BatchEntityFetch:
		res.out = acquireLoaderBuf()
		return l.loadBatchEntityFetch(ctx, f, items, res)
	}
	return nil
}

func (l *Loader) mergeResult(res *result, items []*fastjson.Value) error {
	defer releaseLoaderBuf(res.out)
	if res.err != nil {
		return l.renderErrorsFailedToFetch(res, failedToFetchNoReason)
	}
	if res.authorizationRejected {
		err := l.renderAuthorizationRejectedErrors(res)
		if err != nil {
			return err
		}
		trueValue := fastjson.MustParse(`true`)
		skipErrorsPath := make([]string, len(res.postProcessing.MergePath)+1)
		copy(skipErrorsPath, res.postProcessing.MergePath)
		skipErrorsPath[len(skipErrorsPath)-1] = "__skipErrors"
		for _, item := range items {
			fastjsonext.SetValue(item, trueValue, skipErrorsPath...)
		}
		return nil
	}
	if res.rateLimitRejected {
		err := l.renderRateLimitRejectedErrors(res)
		if err != nil {
			return err
		}
		trueValue := fastjson.MustParse(`true`)
		skipErrorsPath := make([]string, len(res.postProcessing.MergePath)+1)
		copy(skipErrorsPath, res.postProcessing.MergePath)
		skipErrorsPath[len(skipErrorsPath)-1] = "__skipErrors"
		for _, item := range items {
			fastjsonext.SetValue(item, trueValue, skipErrorsPath...)
		}
		return nil
	}
	if res.fetchSkipped {
		return nil
	}
	if res.out.Len() == 0 {
		return l.renderErrorsFailedToFetch(res, emptyGraphQLResponse)
	}
	l.resolvable.maxSize += res.out.Len()
	value, err := l.resolvable.parseJSON(res.out.Bytes())
	if err != nil {
		return l.renderErrorsFailedToFetch(res, invalidGraphQLResponse)
	}

	hasErrors := false

	// We check if the subgraph response has errors
	if res.postProcessing.SelectResponseErrorsPath != nil {
		errorsValue := value.Get(res.postProcessing.SelectResponseErrorsPath...)
		if fastjsonext.ValueIsNonNull(errorsValue) {
			errorObjects := errorsValue.GetArray()
			hasErrors = len(errorObjects) > 0
			// Look for errors in the response and merge them into the errors array
			err = l.mergeErrors(res, errorsValue, errorObjects)
			if err != nil {
				return errors.WithStack(err)
			}
		}
	}

	// We also check if any data is there to processed
	if res.postProcessing.SelectResponseDataPath != nil {
		value = value.Get(res.postProcessing.SelectResponseDataPath...)
		// Check if the not set or null
		if fastjsonext.ValueIsNull(value) {
			// If we didn't get any data nor errors, we return an error because the response is invalid
			// Returning an error here also avoids the need to walk over it later.
			if !hasErrors {
				return l.renderErrorsFailedToFetch(res, invalidGraphQLResponseShape)
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
		if value.Type() != fastjson.TypeObject {
			return l.renderErrorsFailedToFetch(res, invalidGraphQLResponseShape)
		}
		l.resolvable.data = value
		return nil
	}
	if len(items) == 1 && res.batchStats == nil {
		fastjsonext.MergeValuesWithPath(items[0], value, res.postProcessing.MergePath...)
		return nil
	}
	batch := value.GetArray()
	if batch == nil {
		return l.renderErrorsFailedToFetch(res, invalidGraphQLResponseShape)
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
				nodeProcessed := fastjson.MustParseBytes(postProcessed.Bytes())
				fastjsonext.MergeValuesWithPath(items[i], nodeProcessed, res.postProcessing.MergePath...)
			}
		} else {
			for i, stats := range res.batchStats {
				for _, item := range stats {
					if item == -1 {
						continue
					}
					fastjsonext.MergeValuesWithPath(items[i], batch[item], res.postProcessing.MergePath...)
				}
			}
		}
	} else {
		for i, item := range items {
			fastjsonext.MergeValuesWithPath(item, batch[i], res.postProcessing.MergePath...)
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

func (l *Loader) mergeErrors(res *result, value *fastjson.Value, values []*fastjson.Value) error {
	path := l.renderPath()

	// Serialize subgraph errors from the response
	// and append them to the subgraph downstream errors
	if len(values) > 0 {
		// print them into the buffer to be able to parse them
		errorsJSON := value.MarshalTo(nil)
		graphqlErrors := make([]GraphQLError, 0, len(values))
		err := json.Unmarshal(errorsJSON, &graphqlErrors)
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

	l.optionallyOmitErrorExtensions(values)
	l.optionallyOmitErrorLocations(values)
	l.optionallyRewriteErrorPaths(values)

	if l.subgraphErrorPropagationMode == SubgraphErrorPropagationModePassThrough {
		fastjsonext.MergeValues(l.resolvable.errors, value)
		return nil
	}

	errorObject := fastjson.MustParse(l.renderSubgraphBaseError(res.subgraphName, path, failedToFetchNoReason))
	l.setSubgraphStatusCode(errorObject, res.statusCode)
	if l.propagateSubgraphErrors {
		fastjsonext.SetValue(errorObject, value, "extensions", "errors")
	}

	fastjsonext.AppendToArray(l.resolvable.errors, errorObject)
	return nil
}

func (l *Loader) optionallyOmitErrorExtensions(values []*fastjson.Value) {
	if !l.omitSubgraphErrorExtensions {
		return
	}
	for _, value := range values {
		if value.Exists("extensions") {
			value.Del("extensions")
		}
	}
}

func (l *Loader) optionallyOmitErrorLocations(values []*fastjson.Value) {
	if !l.omitSubgraphErrorLocations {
		return
	}
	for _, value := range values {
		if value.Exists("locations") {
			value.Del("locations")
		}
	}
}

func (l *Loader) optionallyRewriteErrorPaths(values []*fastjson.Value) {
	if !l.rewriteSubgraphErrorPaths {
		return
	}
	pathPrefix := make([]string, 0, len(l.path))
	copy(pathPrefix, l.path)
	// remove the trailing @ in case we're in an array as it looks weird in the path
	// errors, like fetches, are attached to objects, not arrays
	if len(l.path) != 0 && l.path[len(l.path)-1] == "@" {
		pathPrefix = pathPrefix[:len(pathPrefix)-1]
	}
	for _, value := range values {
		errorPath := value.Get("path")
		if fastjsonext.ValueIsNull(errorPath) {
			continue
		}
		if errorPath.Type() != fastjson.TypeArray {
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
				value.Set("path", fastjson.MustParseBytes(newPathJSON))
				break
			}
		}
	}
}

func (l *Loader) setSubgraphStatusCode(errorObject *fastjson.Value, statusCode int) {
	if !l.propagateSubgraphStatusCodes {
		return
	}
	if statusCode == 0 {
		return
	}
	fastjsonext.SetValue(errorObject, fastjson.MustParse(strconv.FormatInt(int64(statusCode), 10)), "extensions", "statusCode")
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
	errorObject := fastjson.MustParse(l.renderSubgraphBaseError(res.subgraphName, path, reason))
	fastjsonext.AppendToArray(l.resolvable.errors, errorObject)
	l.setSubgraphStatusCode(errorObject, res.statusCode)
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
				errorObject := fastjson.MustParse(fmt.Sprintf(`{"message":"Unauthorized Subgraph request at Path '%s'."}`, path))
				fastjsonext.AppendToArray(l.resolvable.errors, errorObject)
			} else {
				errorObject := fastjson.MustParse(fmt.Sprintf(`{"message":"Unauthorized Subgraph request at Path '%s', Reason: %s."}`, path, reason))
				fastjsonext.AppendToArray(l.resolvable.errors, errorObject)
			}
		}
	} else {
		for _, reason := range res.authorizationRejectedReasons {
			if reason == "" {
				errorObject := fastjson.MustParse(fmt.Sprintf(`{"message":"Unauthorized request to Subgraph '%s' at Path '%s'."}`, res.subgraphName, path))
				fastjsonext.AppendToArray(l.resolvable.errors, errorObject)
			} else {
				errorObject := fastjson.MustParse(fmt.Sprintf(`{"message":"Unauthorized request to Subgraph '%s' at Path '%s', Reason: %s."}`, res.subgraphName, path, reason))
				fastjsonext.AppendToArray(l.resolvable.errors, errorObject)
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
			errorObject := fastjson.MustParse(fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph request at Path '%s'."}`, path))
			fastjsonext.AppendToArray(l.resolvable.errors, errorObject)
		} else {
			errorObject := fastjson.MustParse(fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph request at Path '%s', Reason: %s."}`, path, res.rateLimitRejectedReason))
			fastjsonext.AppendToArray(l.resolvable.errors, errorObject)
		}
	} else {
		if res.rateLimitRejectedReason == "" {
			errorObject := fastjson.MustParse(fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph '%s' at Path '%s'."}`, res.subgraphName, path))
			fastjsonext.AppendToArray(l.resolvable.errors, errorObject)
		} else {
			errorObject := fastjson.MustParse(fmt.Sprintf(`{"message":"Rate limit exceeded for Subgraph '%s' at Path '%s', Reason: %s."}`, res.subgraphName, path, res.rateLimitRejectedReason))
			fastjsonext.AppendToArray(l.resolvable.errors, errorObject)
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

func (l *Loader) loadSingleFetch(ctx context.Context, fetch *SingleFetch, items []*fastjson.Value, res *result) error {
	res.init(fetch.PostProcessing, fetch.Info)
	inputBuf := &bytes.Buffer{}
	preparedInputBuf := &bytes.Buffer{}
	l.itemsData(items, inputBuf)
	if l.ctx.TracingOptions.Enable {
		fetch.Trace = &DataSourceLoadTrace{}
		if !l.ctx.TracingOptions.ExcludeRawInputData {
			inputCopy := make([]byte, inputBuf.Len())
			copy(inputCopy, inputBuf.Bytes())
			fetch.Trace.RawInputData = inputCopy
		}
	}
	err := fetch.InputTemplate.Render(l.ctx, inputBuf.Bytes(), preparedInputBuf)
	if err != nil {
		return l.renderErrorsInvalidInput(res.out)
	}
	fetchInput := preparedInputBuf.Bytes()
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

func (l *Loader) loadEntityFetch(ctx context.Context, fetch *EntityFetch, items []*fastjson.Value, res *result) error {
	res.init(fetch.PostProcessing, fetch.Info)
	itemBuf := &bytes.Buffer{}
	itemDataBuf := &bytes.Buffer{}
	preparedInputBuf := &bytes.Buffer{}
	l.itemsData(items, itemDataBuf)

	if l.ctx.TracingOptions.Enable {
		fetch.Trace = &DataSourceLoadTrace{}
		if !l.ctx.TracingOptions.ExcludeRawInputData {
			itemDataCopy := make([]byte, itemDataBuf.Len())
			copy(itemDataCopy, itemDataBuf.Bytes())
			fetch.Trace.RawInputData = itemDataCopy
		}
	}

	var undefinedVariables []string

	err := fetch.Input.Header.RenderAndCollectUndefinedVariables(l.ctx, nil, preparedInputBuf, &undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}

	err = fetch.Input.Item.Render(l.ctx, itemDataBuf.Bytes(), itemBuf)
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
	renderedItem := itemBuf.Bytes()
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
	_, _ = itemBuf.WriteTo(preparedInputBuf)
	err = fetch.Input.Footer.RenderAndCollectUndefinedVariables(l.ctx, nil, preparedInputBuf, &undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}

	err = SetInputUndefinedVariables(preparedInputBuf, undefinedVariables)
	if err != nil {
		return errors.WithStack(err)
	}
	fetchInput := preparedInputBuf.Bytes()

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

var (
	batchEntityFetchPool         = sync.Pool{}
	batchEntityPreparedInputSize = atomic.NewInt32(32)
	batchEntityItemInputSize     = atomic.NewInt32(32)
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
			preparedInput: bytes.NewBuffer(make([]byte, 0, int(batchEntityPreparedInputSize.Load()))),
			itemInput:     bytes.NewBuffer(make([]byte, 0, int(batchEntityItemInputSize.Load()))),
			keyGen:        xxhash.New(),
		}
	}
	return buf.(*batchEntityFetchBuffer)
}

func releaseBatchEntityFetchBuffer(buf *batchEntityFetchBuffer) {
	batchEntityPreparedInputSize.Store(int32(buf.preparedInput.Cap()))
	batchEntityItemInputSize.Store(int32(buf.itemInput.Cap()))
	buf.preparedInput.Reset()
	buf.itemInput.Reset()
	buf.keyGen.Reset()
	batchEntityFetchPool.Put(buf)
}

func (l *Loader) loadBatchEntityFetch(ctx context.Context, fetch *BatchEntityFetch, items []*fastjson.Value, res *result) error {
	res.init(fetch.PostProcessing, fetch.Info)

	buf := acquireBatchEntityFetchBuffer()
	defer releaseBatchEntityFetchBuffer(buf)

	if l.ctx.TracingOptions.Enable {
		fetch.Trace = &DataSourceLoadTrace{}
		if !l.ctx.TracingOptions.ExcludeRawInputData {
			buf := &bytes.Buffer{}
			l.itemsData(items, buf)
			fetch.Trace.RawInputData = buf.Bytes()
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
