package resolve

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
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

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astjson"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/pool"
)

type Loader struct {
	data         *astjson.JSON
	dataRoot     int
	errorsRoot   int
	ctx          *Context
	path         []string
	traceOptions RequestTraceOptions
	info         *GraphQLResponseInfo
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
	l.traceOptions = resolvable.requestTraceOptions
	l.ctx = ctx
	l.info = response.Info
	return l.walkNode(response.Data, []int{resolvable.dataRoot})
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
			return errors.WithStack(err)
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
			return errors.WithStack(err)
		}
		return l.mergeResult(res, items)
	case *SerialFetch:
		if l.traceOptions.Enable {
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
		if l.traceOptions.Enable {
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
					if err != nil {
						return errors.WithStack(err)
					}
				}
			} else {
				err = l.mergeResult(results[i], items)
				if err != nil {
					return errors.WithStack(err)
				}
			}
		}
	case *ParallelListItemFetch:
		if l.traceOptions.Enable {
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
		return l.mergeResult(res, items)
	case *BatchEntityFetch:
		res := &result{
			out: pool.BytesBuffer.Get(),
		}
		err := l.loadBatchEntityFetch(l.ctx.ctx, f, items, res)
		if err != nil {
			return errors.WithStack(err)
		}
		return l.mergeResult(res, items)
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
		if l.traceOptions.Enable {
			f.Traces = make([]*SingleFetch, len(items))
		}
		g, ctx := errgroup.WithContext(l.ctx.ctx)
		for i := range items {
			i := i
			results[i] = &result{
				out: pool.BytesBuffer.Get(),
			}
			if l.traceOptions.Enable {
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

func (l *Loader) mergeErrors(ref int) {
	if ref == -1 {
		return
	}
	if l.errorsRoot == -1 {
		l.errorsRoot = ref
		return
	}
	l.data.MergeArrays(l.errorsRoot, ref)
}

func (l *Loader) mergeResult(res *result, items []int) error {
	defer pool.BytesBuffer.Put(res.out)
	if res.err != nil {
		return l.renderErrorsFailedToFetch(res)
	}
	if res.authorizationRejected {
		err := l.renderAuthorizationRejectedErrors(res)
		if err != nil {
			return errors.WithStack(err)
		}
		before := l.data.DebugPrintNode(l.dataRoot)
		for _, item := range items {
			l.data.Nodes = append(l.data.Nodes, astjson.Node{
				Kind: astjson.NodeKindNullSkipError,
			})
			ref := len(l.data.Nodes) - 1
			l.data.MergeNodesWithPath(item, ref, res.postProcessing.MergePath)
		}
		after := l.data.DebugPrintNode(l.dataRoot)
		_, _ = before, after
		return nil
	}
	if res.fetchSkipped {
		return nil
	}
	if res.out.Len() == 0 {
		return nil
	}
	node, err := l.data.AppendAnyJSONBytes(res.out.Bytes())
	if err != nil {
		return errors.WithStack(err)
	}
	if res.postProcessing.SelectResponseErrorsPath != nil {
		ref := l.data.Get(node, res.postProcessing.SelectResponseErrorsPath)
		l.mergeErrors(ref)
	}
	if res.postProcessing.SelectResponseDataPath != nil {
		node = l.data.Get(node, res.postProcessing.SelectResponseDataPath)
		if !l.data.NodeIsDefined(node) {
			// no data
			return nil
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

	err          error
	subgraphName string

	authorizationRejected        bool
	authorizationRejectedReasons []string
}

func (r *result) init(postProcessing PostProcessingConfiguration, info *FetchInfo) {
	r.postProcessing = postProcessing
	if info != nil {
		r.subgraphName = info.DataSourceID
	}
}

var (
	errorsInvalidInputHeader = []byte(`{"errors":[{"message":"invalid input","path":[`)
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

func (l *Loader) renderErrorsFailedToFetch(res *result) error {
	path := l.renderPath()
	l.ctx.appendSubgraphError(errors.Wrap(res.err, fmt.Sprintf("failed to fetch from subgraph '%s' at path '%s'", res.subgraphName, path)))
	if res.subgraphName == "" {
		errorObject, err := l.data.AppendObject([]byte(fmt.Sprintf(`{"message":"Failed to fetch from Subgraph at path '%s'."}`, path)))
		if err != nil {
			return errors.WithStack(err)
		}
		l.data.Nodes[l.errorsRoot].ArrayValues = append(l.data.Nodes[l.errorsRoot].ArrayValues, errorObject)
	} else {
		errorObject, err := l.data.AppendObject([]byte(fmt.Sprintf(`{"message":"Failed to fetch from Subgraph '%s' at path '%s'."}`, res.subgraphName, path)))
		if err != nil {
			return errors.WithStack(err)
		}
		l.data.Nodes[l.errorsRoot].ArrayValues = append(l.data.Nodes[l.errorsRoot].ArrayValues, errorObject)
	}
	return nil
}

func (l *Loader) renderAuthorizationRejectedErrors(res *result) error {
	path := l.renderPath()
	for i := range res.authorizationRejectedReasons {
		l.ctx.appendSubgraphError(errors.Wrap(res.err, fmt.Sprintf("Authorization rejected for subgraph '%s' at path '%s'. Reason: %s", res.subgraphName, path, res.authorizationRejectedReasons[i])))
	}
	if res.subgraphName == "" {
		for _, reason := range res.authorizationRejectedReasons {
			if reason == "" {
				errorObject, err := l.data.AppendObject([]byte(fmt.Sprintf(`{"message":"Unauthorized Subgraph request at path '%s'."}`, path)))
				if err != nil {
					return errors.WithStack(err)
				}
				l.data.Nodes[l.errorsRoot].ArrayValues = append(l.data.Nodes[l.errorsRoot].ArrayValues, errorObject)
			} else {
				errorObject, err := l.data.AppendObject([]byte(fmt.Sprintf(`{"message":"Unauthorized Subgraph request at path '%s'. Reason: %s"}`, path, reason)))
				if err != nil {
					return errors.WithStack(err)
				}
				l.data.Nodes[l.errorsRoot].ArrayValues = append(l.data.Nodes[l.errorsRoot].ArrayValues, errorObject)
			}
		}
	} else {
		for _, reason := range res.authorizationRejectedReasons {
			if reason == "" {
				errorObject, err := l.data.AppendObject([]byte(fmt.Sprintf(`{"message":"Unauthorized request to Subgraph '%s' at path '%s'."}`, res.subgraphName, path)))
				if err != nil {
					return errors.WithStack(err)
				}
				l.data.Nodes[l.errorsRoot].ArrayValues = append(l.data.Nodes[l.errorsRoot].ArrayValues, errorObject)
			} else {
				errorObject, err := l.data.AppendObject([]byte(fmt.Sprintf(`{"message":"Unauthorized request to Subgraph '%s' at path '%s'. Reason: %s"}`, res.subgraphName, path, reason)))
				if err != nil {
					return errors.WithStack(err)
				}
				l.data.Nodes[l.errorsRoot].ArrayValues = append(l.data.Nodes[l.errorsRoot].ArrayValues, errorObject)
			}
		}
	}
	return nil
}

func (l *Loader) isFetchAuthorized(input []byte, info *FetchInfo, res *result) (authorized bool, err error) {
	if info == nil {
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
			return false, errors.WithStack(err)
		}
		if reject != nil {
			authorized = false
			res.authorizationRejected = true
			res.authorizationRejectedReasons = append(res.authorizationRejectedReasons, reject.Reason)
		}
	}
	return authorized, nil
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
	if l.traceOptions.Enable {
		fetch.Trace = &DataSourceLoadTrace{}
		if !l.traceOptions.ExcludeRawInputData {
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
	authorized, err := l.isFetchAuthorized(fetchInput, fetch.Info, res)
	if err != nil {
		return errors.WithStack(err)
	}
	if !authorized {
		return nil
	}
	res.err = l.executeSourceLoad(ctx, fetch.DataSource, fetchInput, res.out, fetch.Trace)
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

	if l.traceOptions.Enable {
		fetch.Trace = &DataSourceLoadTrace{}
		if !l.traceOptions.ExcludeRawInputData {
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
			if l.traceOptions.Enable {
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
		if l.traceOptions.Enable {
			fetch.Trace.LoadSkipped = true
		}
		return nil
	}
	if bytes.Equal(renderedItem, emptyObject) {
		// skip fetch if item is empty
		res.fetchSkipped = true
		if l.traceOptions.Enable {
			fetch.Trace.LoadSkipped = true
		}
		return nil
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
	authorized, err := l.isFetchAuthorized(fetchInput, fetch.Info, res)
	if err != nil {
		return errors.WithStack(err)
	}
	if !authorized {
		return nil
	}
	res.err = l.executeSourceLoad(ctx, fetch.DataSource, fetchInput, res.out, fetch.Trace)
	return nil
}

func (l *Loader) loadBatchEntityFetch(ctx context.Context, fetch *BatchEntityFetch, items []int, res *result) error {
	res.init(fetch.PostProcessing, fetch.Info)

	if l.traceOptions.Enable {
		fetch.Trace = &DataSourceLoadTrace{}
		if !l.traceOptions.ExcludeRawInputData {
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
				if l.traceOptions.Enable {
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
		if l.traceOptions.Enable {
			fetch.Trace.LoadSkipped = true
		}
		return nil
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
	authorized, err := l.isFetchAuthorized(fetchInput, fetch.Info, res)
	if err != nil {
		return errors.WithStack(err)
	}
	if !authorized {
		return nil
	}
	res.err = l.executeSourceLoad(ctx, fetch.DataSource, fetchInput, res.out, fetch.Trace)
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

func (l *Loader) executeSourceLoad(ctx context.Context, source DataSource, input []byte, out *bytes.Buffer, trace *DataSourceLoadTrace) (err error) {
	if l.ctx.Extensions != nil {
		input, err = jsonparser.Set(input, l.ctx.Extensions, "body", "extensions")
		if err != nil {
			return errors.WithStack(err)
		}
	}
	if l.traceOptions.Enable {
		ctx = setSingleFlightStats(ctx, &SingleFlightStats{})
		trace.Path = l.renderPath()
		if !l.traceOptions.ExcludeInput {
			trace.Input = make([]byte, len(input))
			copy(trace.Input, input) // copy input explicitly, omit __trace__ field
			redactedInput, err := redactHeaders(trace.Input)
			if err != nil {
				return err
			}
			trace.Input = redactedInput
		}
		if gjson.ValidBytes(input) {
			inputCopy := make([]byte, len(input))
			copy(inputCopy, input)
			input, _ = jsonparser.Set(inputCopy, []byte("true"), "__trace__")
		}
		if !l.traceOptions.ExcludeLoadStats {
			trace.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
			trace.DurationSinceStartPretty = time.Duration(trace.DurationSinceStartNano).String()
			trace.LoadStats = &LoadStats{}
			clientTrace := &httptrace.ClientTrace{
				GetConn: func(hostPort string) {
					trace.LoadStats.GetConn.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.GetConn.DurationSinceStartPretty = time.Duration(trace.LoadStats.GetConn.DurationSinceStartNano).String()
					if !l.traceOptions.EnablePredictableDebugTimings {
						trace.LoadStats.GetConn.HostPort = hostPort
					}
				},
				GotConn: func(info httptrace.GotConnInfo) {
					trace.LoadStats.GotConn.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.GotConn.DurationSinceStartPretty = time.Duration(trace.LoadStats.GotConn.DurationSinceStartNano).String()
					if !l.traceOptions.EnablePredictableDebugTimings {
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
					if !l.traceOptions.EnablePredictableDebugTimings {
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
					if !l.traceOptions.EnablePredictableDebugTimings {
						trace.LoadStats.ConnectStart.Network = network
						trace.LoadStats.ConnectStart.Addr = addr
					}
				},
				ConnectDone: func(network, addr string, err error) {
					trace.LoadStats.ConnectDone.DurationSinceStartNano = GetDurationNanoSinceTraceStart(ctx)
					trace.LoadStats.ConnectDone.DurationSinceStartPretty = time.Duration(trace.LoadStats.ConnectDone.DurationSinceStartNano).String()
					if !l.traceOptions.EnablePredictableDebugTimings {
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
	err = source.Load(ctx, input, out)
	if l.traceOptions.Enable {
		stats := GetSingleFlightStats(ctx)
		if stats != nil {
			trace.SingleFlightUsed = stats.SingleFlightUsed
			trace.SingleFlightSharedResponse = stats.SingleFlightSharedResponse
		}
		if !l.traceOptions.ExcludeOutput && out.Len() > 0 {
			if l.traceOptions.EnablePredictableDebugTimings {
				dataCopy := make([]byte, out.Len())
				copy(dataCopy, out.Bytes())
				trace.Output = jsonparser.Delete(dataCopy, "extensions", "trace", "response", "headers", "Date")
			} else {
				trace.Output = make([]byte, out.Len())
				copy(trace.Output, out.Bytes())
			}
		}
		if !l.traceOptions.ExcludeLoadStats {
			if l.traceOptions.EnablePredictableDebugTimings {
				trace.DurationLoadNano = 1
			} else {
				trace.DurationLoadNano = GetDurationNanoSinceTraceStart(ctx) - trace.DurationSinceStartNano
			}
			trace.DurationLoadPretty = time.Duration(trace.DurationLoadNano).String()
		}
	}
	if err != nil {
		if l.traceOptions.Enable {
			trace.LoadError = err.Error()
		}
		return errors.WithStack(err)
	}
	l.ctx.Stats.NumberOfFetches.Inc()
	l.ctx.Stats.CombinedResponseSize.Add(int64(out.Len()))
	return nil
}
