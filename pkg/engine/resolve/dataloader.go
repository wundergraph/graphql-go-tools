package resolve

import (
	"fmt"
	"sync"

	"github.com/buger/jsonparser"

	"github.com/jensneuse/graphql-go-tools/pkg/fastbuffer"
)

const initialValueID = -1

func NewDataLoader(initialValue []byte) *DataLoader { // initial value represent data from subscription
	fetches := make(map[int]*batchState)

	if initialValue != nil {
		fetches[initialValueID] = &batchState{
			nextIdx: 0,
			err:     nil,
			results: [][]byte{initialValue},
		}
	}

	return &DataLoader{
		fetches: fetches,
		mu:      &sync.Mutex{},
	}
}

type DataLoader struct {
	fetches map[int]*batchState
	mu      *sync.Mutex
}

// Load @TODO move duplicated code
func (d *DataLoader) Load(ctx *Context, fetch *SingleFetch) (response []byte, err error) {
	var fetchResult *batchState

	d.mu.Lock()
	fetchResult, ok := d.fetches[fetch.BufferId]
	if ok {
		response, err = fetchResult.next(ctx)
	} else {
		fetchResult = &batchState{}
		d.fetches[fetch.BufferId] = fetchResult
	}

	d.mu.Unlock()

	if err != nil {
		return nil, err
	}

	if response != nil {
		return response, nil
	}

	buf := fastbuffer.New()

	if fetch.BufferId == 0 { // it must be root query
		if err := fetch.InputTemplate.Render(ctx, nil, buf); err != nil {
			return nil, err
		}

		result, err := d.resolveSingleFetch(ctx, fetch, buf.Bytes())
		fetchResult.results = [][]byte{result}
		fetchResult.err = err

		return fetchResult.next(ctx)
	}

	parentResponses, ok := d.fetches[ctx.lastFetchID]
	if !ok && fetch.BufferId == 0 { // It's impossible case
		return nil, fmt.Errorf("has not got fetch for %d", ctx.lastFetchID)
	}

	args := d.selectedDataForFetch(parentResponses.results, ctx.responseElements...) // TODO rename argument

	wg := sync.WaitGroup{}
	wg.Add(len(args))

	results := make([][]byte, 0, len(args))
	resultCh := make(chan struct {
		result []byte
		err    error
		pos    int
	}, len(args))

	for i, val := range args {
		if err := fetch.InputTemplate.Render(ctx, val, buf); err != nil {
			return nil, err
		}

		go func(pos int) {
			result, err := d.resolveSingleFetch(ctx, fetch, buf.Bytes())
			resultCh <- struct {
				result []byte
				err    error
				pos    int
			}{result: result, err: err, pos: pos}

			wg.Done()
		}(i)

		buf.Reset()
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	for res := range resultCh {
		if res.err != nil {
			fetchResult.err = res.err
		}
		results[res.pos] = res.result
	}

	fetchResult.results = results

	return fetchResult.next(ctx)
}

// LoadBatch @TODO move duplicated code
func (d *DataLoader) LoadBatch(ctx *Context, batchFetch *BatchFetch) (response []byte, err error) {
	var fetchResult *batchState

	d.mu.Lock()
	fetchResult, ok := d.fetches[batchFetch.Fetch.BufferId]
	if ok {
		response, err = fetchResult.next(ctx)
	} else {
		fetchResult = &batchState{}
		d.fetches[batchFetch.Fetch.BufferId] = fetchResult
	}

	d.mu.Unlock()

	if err != nil {
		return nil, err
	}

	if response != nil {
		return response, nil
	}

	parentResponses, ok := d.fetches[ctx.lastFetchID]
	if !ok { // It must be impossible case
		return nil, fmt.Errorf("has not got fetch for %d", ctx.lastFetchID)
	}

	var inputs [][]byte

	for _, val := range d.selectedDataForFetch(parentResponses.results, ctx.responseElements...) {
		buf := fastbuffer.New()
		if err := batchFetch.Fetch.InputTemplate.Render(ctx, val, buf); err != nil {
			return nil, err
		}

		inputs = append(inputs, buf.Bytes())
	}

	fetchResult.results, fetchResult.err = d.resolveBatchFetch(ctx, batchFetch, inputs...)

	return fetchResult.next(ctx)
}

// @TODO add handling of error
func (d *DataLoader) resolveSingleFetch(ctx *Context, fetch *SingleFetch, input []byte) (result []byte, err error) {
	if ctx.beforeFetchHook != nil {
		ctx.beforeFetchHook.OnBeforeFetch(d.hookCtx(ctx), input)
	}

	// @TODO use pool of pairs
	batchBufferPair := &BufPair{
		Data:   fastbuffer.New(),
		Errors: fastbuffer.New(),
	}

	if err = fetch.DataSource.Load(ctx.Context, input, batchBufferPair); err != nil {
		return nil, err
	}

	if ctx.afterFetchHook != nil {
		if batchBufferPair.HasData() {
			ctx.afterFetchHook.OnData(d.hookCtx(ctx), batchBufferPair.Data.Bytes(), false)
		}
		if batchBufferPair.HasErrors() {
			ctx.afterFetchHook.OnError(d.hookCtx(ctx), batchBufferPair.Errors.Bytes(), false)
		}
	}

	return batchBufferPair.Data.Bytes(), nil
}

// @TODO add handling of error
func (d *DataLoader) resolveBatchFetch(ctx *Context, batchFetch *BatchFetch, inputs ...[]byte) (result [][]byte, err error) {
	batchInput, err := batchFetch.PrepareBatch(inputs...)
	if err != nil {
		return nil, err
	}

	fmt.Println("batch request", string(batchInput.Input))

	response, err := d.resolveSingleFetch(ctx, batchFetch.Fetch, batchInput.Input)

	var outPosition int
	result = make([][]byte, len(inputs))

	_, err = jsonparser.ArrayEach(response, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		inputPositions := batchInput.OutToInPositions[outPosition]

		for _, pos := range inputPositions {
			result[pos] = value
		}

		outPosition++
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (d *DataLoader) hookCtx(ctx *Context) HookContext {
	return HookContext{
		CurrentPath: ctx.path(),
	}
}

// @TODO possible performance issue, try to use loop instead of recursion
func (d *DataLoader) selectedDataForFetch(input [][]byte, path ...string) [][]byte {
	if len(path) == 0 {
		return input
	}

	current, rest := path[0], path[1:]

	if current == "@" {
		return flatMap(input, func(val []byte) [][]byte {
			var vals [][]byte
			// @TODO handle error
			_, _ = jsonparser.ArrayEach(val, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
				vals = append(vals, value)
			})

			return d.selectedDataForFetch(vals, rest...)
		})
	}

	temp := make([][]byte, 0, len(input))

	for i := range input {
		el, _, _, err := jsonparser.Get(input[i], current)
		if err != nil {
			return nil
		}
		temp = append(temp, el)
	}

	return d.selectedDataForFetch(temp, rest...)
}

type batchState struct {
	nextIdx int

	err     error
	results [][]byte
}

func (f *batchState) next(ctx *Context) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}

	res := f.results[f.nextIdx]

	f.nextIdx++

	return res, nil
}

func flatMap(input [][]byte, f func(val []byte) [][]byte) [][]byte {
	var result [][]byte

	for i := range input {
		result = append(result, f(input[i])...)
	}

	return result
}
