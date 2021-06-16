package resolve

import (
	"fmt"
	"sync"

	"github.com/buger/jsonparser"

	"github.com/jensneuse/graphql-go-tools/pkg/fastbuffer"
)

const initialValueID = -1

type fetcher interface {
	fetch(ctx *Context, fetch *SingleFetch, preparedInput []byte, buf *BufPair) (err error)
}

type dataloaderFactory struct {
	dataloaderPool   sync.Pool
	muPool           sync.Pool
	waitGroupPool    sync.Pool
	bufPairPool      sync.Pool
	bufPairSlicePool sync.Pool
}

func (df *dataloaderFactory) getWaitGroup() *sync.WaitGroup {
	return df.waitGroupPool.Get().(*sync.WaitGroup)
}

func (df *dataloaderFactory) freeWaitGroup(wg *sync.WaitGroup) {
	df.waitGroupPool.Put(wg)
}

func (df *dataloaderFactory) getBufPairSlicePool() *[]*BufPair {
	return df.bufPairSlicePool.Get().(*[]*BufPair)
}

func (df *dataloaderFactory) freeBufPairSlice(slice *[]*BufPair) {
	for i := range *slice {
		df.freeBufPair((*slice)[i])
	}
	*slice = (*slice)[:0]
	df.bufPairSlicePool.Put(slice)
}

func (df *dataloaderFactory) getBufPair() *BufPair {
	return df.bufPairPool.Get().(*BufPair)
}

func (df *dataloaderFactory) freeBufPair(pair *BufPair) {
	pair.Data.Reset()
	pair.Errors.Reset()
	df.bufPairPool.Put(pair)
}

func (df *dataloaderFactory) getMutex() *sync.Mutex {
	return df.muPool.Get().(*sync.Mutex)
}

func (df *dataloaderFactory) freeMutex(mu *sync.Mutex) {
	df.muPool.Put(mu)
}

func newDataloaderFactory() *dataloaderFactory {
	return &dataloaderFactory{
		muPool: sync.Pool{
			New: func() interface{} {
				return &sync.Mutex{}
			},
		},
		waitGroupPool: sync.Pool{
			New: func() interface{} {
				return &sync.WaitGroup{}
			},
		},
		bufPairPool: sync.Pool{
			New: func() interface{} {
				pair := BufPair{
					Data:   fastbuffer.New(),
					Errors: fastbuffer.New(),
				}
				return &pair
			},
		},
		bufPairSlicePool: sync.Pool{
			New: func() interface{} {
				slice := make([]*BufPair, 0, 24)
				return &slice
			},
		},
		dataloaderPool: sync.Pool{
			New: func() interface{} {
				return &dataLoader{
					fetches:      make(map[int]*batchState),
					inUseBufPair: make([]*BufPair, 0, 8),
				}
			},
		},
	}
}

func (df *dataloaderFactory) newDataLoader(fetcher fetcher, initialValue []byte) *dataLoader { // initial value represent data from subscription
	dataloader := df.dataloaderPool.Get().(*dataLoader)

	if initialValue != nil {
		dataloader.fetches[initialValueID] = &batchState{
			nextIdx: 0,
			err:     nil,
			results: [][]byte{initialValue},
		}
	}

	dataloader.fetcher = fetcher
	dataloader.resourceProvider = df
	dataloader.mu = df.getMutex()

	return dataloader
}

func (df *dataloaderFactory) freeDataLoader(d *dataLoader) {
	for _, pair := range d.inUseBufPair {
		d.resourceProvider.freeBufPair(pair)
	}

	d.resourceProvider.freeMutex(d.mu)

	d.inUseBufPair = d.inUseBufPair[:0]
	d.fetches = nil
}

type dataLoader struct {
	fetches          map[int]*batchState
	mu               *sync.Mutex
	fetcher          fetcher
	resourceProvider *dataloaderFactory

	inUseBufPair []*BufPair
}

func (d *dataLoader) Load(ctx *Context, fetch *SingleFetch) (response []byte, err error) {
	var fetchResult *batchState

	fetchResult, ok := d.getFetchState(fetch.BufferId)
	if ok {
		return fetchResult.next(ctx)
	}

	fetchResult = &batchState{}

	parentResult, ok := d.getFetchState(ctx.lastFetchID)

	if !ok { // it must be root query without subscription data
		buf := d.resourceProvider.getBufPair()
		defer d.resourceProvider.freeBufPair(buf)

		if err := fetch.InputTemplate.Render(ctx, nil, buf.Data); err != nil {
			return nil, err
		}

		pair := d.getResultBufPair()
		err := d.fetcher.fetch(ctx, fetch, buf.Data.Bytes(), pair)
		fetchResult.results = [][]byte{pair.Data.Bytes()}
		fetchResult.err = err

		d.setFetchState(fetchResult, fetch.BufferId)

		return fetchResult.next(ctx)
	}

	fetchParams := d.selectedDataForFetch(parentResult.results, ctx.responseElements...)
	fetchResult.results, fetchResult.err = d.resolveSingleFetch(ctx, fetch, fetchParams)

	d.setFetchState(fetchResult, fetch.BufferId)

	return fetchResult.next(ctx)
}

func (d *dataLoader) LoadBatch(ctx *Context, batchFetch *BatchFetch) (response []byte, err error) {
	var fetchResult *batchState

	fetchResult, ok := d.getFetchState(batchFetch.Fetch.BufferId)
	if ok {
		return fetchResult.next(ctx)
	}

	fetchResult = &batchState{}

	parentResult, ok := d.getFetchState(ctx.lastFetchID)
	if !ok {
		return nil, fmt.Errorf("has not got fetch for %d", ctx.lastFetchID)
	}

	fetchParams := d.selectedDataForFetch(parentResult.results, ctx.responseElements...)
	fetchResult.results, fetchResult.err = d.resolveBatchFetch(ctx, batchFetch, fetchParams)

	d.setFetchState(fetchResult, batchFetch.Fetch.BufferId)

	return fetchResult.next(ctx)
}

func (d *dataLoader) resolveBatchFetch(ctx *Context, batchFetch *BatchFetch, fetchParams [][]byte) (result [][]byte, err error) {
	var inputs [][]byte

	bufSlice := d.resourceProvider.getBufPairSlicePool()
	defer d.resourceProvider.freeBufPairSlice(bufSlice)

	for i := range fetchParams {
		bufPair := d.resourceProvider.getBufPair()
		*bufSlice = append(*bufSlice, bufPair)
		if err := batchFetch.Fetch.InputTemplate.Render(ctx, fetchParams[i], bufPair.Data); err != nil {
			return nil, err
		}

		inputs = append(inputs, bufPair.Data.Bytes())
	}

	batchInput, err := batchFetch.PrepareBatch(inputs...)
	if err != nil {
		return nil, err
	}

	pair := d.getResultBufPair()
	if err := d.fetcher.fetch(ctx, batchFetch.Fetch, batchInput.Input, pair); err != nil {
		return nil, err
	}

	var outPosition int
	result = make([][]byte, len(inputs))

	_, err = jsonparser.ArrayEach(pair.Data.Bytes(), func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
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

func (d *dataLoader) resolveSingleFetch(ctx *Context, fetch *SingleFetch, fetchParams [][]byte) ([][]byte, error) {
	wg := d.resourceProvider.getWaitGroup()
	defer d.resourceProvider.freeWaitGroup(wg)

	wg.Add(len(fetchParams))

	results := make([][]byte, len(fetchParams))

	type fetchResult struct {
		result *BufPair
		err    error
		pos    int
	}

	resultCh := make(chan fetchResult, len(fetchParams))

	bufSlice := d.resourceProvider.getBufPairSlicePool()
	defer d.resourceProvider.freeBufPairSlice(bufSlice)

	for i, val := range fetchParams {
		bufPair := d.resourceProvider.getBufPair()
		*bufSlice = append(*bufSlice, bufPair)
		if err := fetch.InputTemplate.Render(ctx, val, bufPair.Data); err != nil {
			return nil, err
		}

		pair := d.getResultBufPair()
		pos := i

		go func() {
			err := d.fetcher.fetch(ctx, fetch, bufPair.Data.Bytes(), pair)
			resultCh <- fetchResult{result: pair, err: err, pos: pos}
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var err error

	for res := range resultCh {
		if res.err != nil {
			err = res.err
		}
		results[res.pos] = res.result.Data.Bytes()
	}

	return results, err
}

func (d *dataLoader) getFetchState(fetchID int) (batchState *batchState, ok bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	batchState, ok = d.fetches[fetchID]
	return
}

func (d *dataLoader) setFetchState(batchState *batchState, fetchID int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.fetches[fetchID] = batchState
}

func (d *dataLoader) selectedDataForFetch(input [][]byte, path ...string) [][]byte {
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

func (d *dataLoader) getResultBufPair() (pair *BufPair) {
	d.mu.Lock()
	defer d.mu.Unlock()

	pair = d.resourceProvider.bufPairPool.Get().(*BufPair)
	d.inUseBufPair = append(d.inUseBufPair, pair)

	return
}

type batchState struct {
	nextIdx int

	err     error
	results [][]byte
}

// next works correctly only with synchronous resolve strategy
// In case of asynchronous resolve strategy it's required to compute response position based on values from ctx (current path)
// But there is no reason for asynchronous resolve strategy, it's not useful, as all IO operations (fetching data) is be done by dataloader
func (b *batchState) next(ctx *Context) ([]byte, error) {
	if b.err != nil {
		return nil, b.err
	}

	res := b.results[b.nextIdx]

	b.nextIdx++

	return res, nil
}

func flatMap(input [][]byte, f func(val []byte) [][]byte) [][]byte {
	var result [][]byte

	for i := range input {
		result = append(result, f(input[i])...)
	}

	return result
}
