package resolve

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/planbytecode"
)

func (r *Resolver) ResolveGraphQLResponseBytecode(ctx *Context, response *GraphQLResponse, program *planbytecode.Program, data []byte, writer io.Writer) (*GraphQLResolveInfo, error) {
	resp := &GraphQLResolveInfo{}

	start := time.Now()
	<-r.maxConcurrency
	resp.ResolveAcquireWaitTime = time.Since(start)
	defer func() {
		r.maxConcurrency <- struct{}{}
	}()

	t := newTools(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields, r.subgraphRequestSingleFlight, nil)

	err := t.resolvable.Init(ctx, data, response.Info.OperationType)
	if err != nil {
		return nil, err
	}

	if !ctx.ExecutionOptions.SkipLoader {
		loaded := false
		if bytecodeDirectResponseRuntimeReady(ctx, program) {
			var rendered bool
			rendered, err = t.loader.LoadAndRenderGraphQLResponseBytecodeDirect(ctx, response, program, t.resolvable, writer)
			if err != nil {
				return nil, err
			}
			if rendered {
				ctx.ActualListSizes = t.resolvable.actualListSizes
				return resp, nil
			}
			loaded = true
		}
		if !loaded {
			err = t.loader.LoadGraphQLResponseDataBytecode(ctx, response, program, t.resolvable)
			if err != nil {
				return nil, err
			}
		}
	}

	rendered, err := t.resolvable.RenderGraphQLResponseBytecode(program, writer)
	if err != nil {
		return nil, err
	}
	if rendered {
		ctx.ActualListSizes = t.resolvable.actualListSizes
		return resp, nil
	}

	err = t.resolvable.Resolve(ctx.ctx, response.Data, response.Fetches, writer)
	if err != nil {
		return nil, err
	}

	ctx.ActualListSizes = t.resolvable.actualListSizes

	return resp, err
}

func (r *Resolver) ArenaResolveGraphQLResponseBytecode(ctx *Context, response *GraphQLResponse, program *planbytecode.Program, writer io.Writer) (*GraphQLResolveInfo, error) {
	resp := &GraphQLResolveInfo{}

	inflight, err := r.inboundRequestSingleFlight.GetOrCreate(ctx, response)
	if err != nil {
		return nil, err
	}

	if inflight != nil && inflight.Data != nil {
		resp.ResolveDeduplicated = true
		if ctx.SetDeduplicationData != nil && inflight.SharedData != nil {
			ctx.SetDeduplicationData(ctx.ctx, inflight.SharedData)
		}
		_, err = writer.Write(inflight.Data)
		return resp, err
	}

	start := time.Now()
	<-r.maxConcurrency
	resp.ResolveAcquireWaitTime = time.Since(start)
	defer func() {
		r.maxConcurrency <- struct{}{}
	}()

	resolveArena := r.resolveArenaPool.Acquire(ctx.Request.ID)
	t := newTools(r.options, r.allowedErrorExtensionFields, r.allowedErrorFields, r.subgraphRequestSingleFlight, resolveArena.Arena)

	err = t.resolvable.Init(ctx, nil, response.Info.OperationType)
	if err != nil {
		r.inboundRequestSingleFlight.FinishErr(inflight, err)
		r.resolveArenaPool.Release(resolveArena)
		return nil, err
	}

	var responseArena *arena.PoolItem
	var buf *arena.Buffer
	if !ctx.ExecutionOptions.SkipLoader {
		loaded := false
		if bytecodeDirectResponseRuntimeReady(ctx, program) {
			responseArena = r.responseBufferPool.Acquire(ctx.Request.ID)
			buf = arena.NewArenaBuffer(responseArena.Arena)
			var rendered bool
			rendered, err = t.loader.LoadAndRenderGraphQLResponseBytecodeDirect(ctx, response, program, t.resolvable, buf)
			if err != nil {
				r.inboundRequestSingleFlight.FinishErr(inflight, err)
				r.resolveArenaPool.Release(resolveArena)
				r.responseBufferPool.Release(responseArena)
				return nil, err
			}
			if rendered {
				ctx.ActualListSizes = t.resolvable.actualListSizes
				r.resolveArenaPool.Release(resolveArena)
				_, err = writer.Write(buf.Bytes())
				if inflight != nil && ctx.GetDeduplicationData != nil {
					inflight.SharedData = ctx.GetDeduplicationData(ctx.ctx)
				}
				r.inboundRequestSingleFlight.FinishOk(inflight, buf.Bytes())
				r.responseBufferPool.Release(responseArena)
				return resp, err
			}
			loaded = true
		}
		if !loaded {
			err = t.loader.LoadGraphQLResponseDataBytecode(ctx, response, program, t.resolvable)
			if err != nil {
				r.inboundRequestSingleFlight.FinishErr(inflight, err)
				r.resolveArenaPool.Release(resolveArena)
				if responseArena != nil {
					r.responseBufferPool.Release(responseArena)
				}
				return nil, err
			}
		}
	}

	if responseArena == nil {
		responseArena = r.responseBufferPool.Acquire(ctx.Request.ID)
		buf = arena.NewArenaBuffer(responseArena.Arena)
	}
	rendered, err := t.resolvable.RenderGraphQLResponseBytecode(program, buf)
	if err != nil {
		r.inboundRequestSingleFlight.FinishErr(inflight, err)
		r.resolveArenaPool.Release(resolveArena)
		r.responseBufferPool.Release(responseArena)
		return nil, err
	}
	if rendered {
		ctx.ActualListSizes = t.resolvable.actualListSizes
		r.resolveArenaPool.Release(resolveArena)
		_, err = writer.Write(buf.Bytes())
		if inflight != nil && ctx.GetDeduplicationData != nil {
			inflight.SharedData = ctx.GetDeduplicationData(ctx.ctx)
		}
		r.inboundRequestSingleFlight.FinishOk(inflight, buf.Bytes())
		r.responseBufferPool.Release(responseArena)
		return resp, err
	}

	err = t.resolvable.Resolve(ctx.ctx, response.Data, response.Fetches, buf)
	if err != nil {
		r.inboundRequestSingleFlight.FinishErr(inflight, err)
		r.resolveArenaPool.Release(resolveArena)
		r.responseBufferPool.Release(responseArena)
		return nil, err
	}
	ctx.ActualListSizes = t.resolvable.actualListSizes

	r.resolveArenaPool.Release(resolveArena)
	_, err = writer.Write(buf.Bytes())
	if inflight != nil && ctx.GetDeduplicationData != nil {
		inflight.SharedData = ctx.GetDeduplicationData(ctx.ctx)
	}
	r.inboundRequestSingleFlight.FinishOk(inflight, buf.Bytes())
	r.responseBufferPool.Release(responseArena)
	return resp, err
}

func (r *Resolvable) RenderGraphQLResponseBytecode(program *planbytecode.Program, writer io.Writer) (bool, error) {
	if !bytecodeResponseRuntimeReady(r, program) {
		return false, nil
	}
	pc := bytecodeResponseStart(program.Ops)
	if pc == -1 {
		return false, nil
	}

	renderer := bytecodeResponseRenderer{
		program: program,
	}
	buf := r.marshalBuf[:0]
	if cap(buf) < 1024 {
		buf = make([]byte, 0, 1024)
	}
	buf = append(buf, `{"data":`...)
	var ok bool
	buf, pc, ok = renderer.renderValue(buf, pc, r.data)
	if !ok {
		r.marshalBuf = buf[:0]
		return false, nil
	}
	if pc >= len(program.Ops) || program.Ops[pc].Code != planbytecode.OpEmitResponse {
		r.marshalBuf = buf[:0]
		return false, nil
	}
	for _, size := range renderer.listSizes {
		r.actualListSizes[size.path] += size.size
	}
	buf = append(buf, '}')
	_, err := writer.Write(buf)
	if err != nil {
		r.marshalBuf = buf[:0]
		return false, err
	}
	r.marshalBuf = buf[:0]
	return true, nil
}

func (l *Loader) LoadGraphQLResponseDataBytecode(ctx *Context, response *GraphQLResponse, program *planbytecode.Program, resolvable *Resolvable) error {
	if program == nil {
		return fmt.Errorf("load bytecode response data: nil program")
	}
	l.resolvable = resolvable
	l.ctx = ctx
	l.info = response.Info
	l.taintedObjs = make(taintedObjects)
	return l.resolveBytecode(program)
}

func (l *Loader) LoadAndRenderGraphQLResponseBytecodeDirect(ctx *Context, response *GraphQLResponse, program *planbytecode.Program, resolvable *Resolvable, writer io.Writer) (bool, error) {
	if program == nil {
		return false, fmt.Errorf("load direct bytecode response data: nil program")
	}
	l.resolvable = resolvable
	l.ctx = ctx
	l.info = response.Info
	l.taintedObjs = make(taintedObjects)

	var resultScratch [4]bytecodeRawFetchResult
	results, merged, err := l.collectBytecodeRawFetches(program, resultScratch[:0])
	if err != nil {
		return false, err
	}
	defer func() {
		for i := range results {
			if results[i].res != nil {
				batchEntityToolPool.Put(results[i].res.tools)
			}
		}
	}()

	rendered, err := l.renderDirectResponseBytecode(program, results, writer)
	if err != nil {
		return false, err
	}
	if rendered {
		l.finishRawFetchResults(results)
		return true, nil
	}

	if merged {
		l.finishRawFetchResults(results)
		return false, nil
	}

	err = l.mergeRawFetchResults(results)
	if err != nil {
		return false, err
	}
	return false, nil
}

func (l *Loader) resolveBytecode(program *planbytecode.Program) error {
	_, err := l.executeBytecodeSpan(program, 0, len(program.Ops))
	return err
}

type bytecodeRawFetchResult struct {
	item  *FetchItem
	res   *result
	items []*astjson.Value
}

func (l *Loader) collectBytecodeRawFetches(program *planbytecode.Program, results []bytecodeRawFetchResult) ([]bytecodeRawFetchResult, bool, error) {
	if cap(results) < len(program.Fetches) {
		results = make([]bytecodeRawFetchResult, 0, len(program.Fetches))
	}
	results = results[:0]
	mergeDuringCollect := bytecodeProgramNeedsMergedParentData(program)
	_, err := l.executeBytecodeRawSpan(program, 0, len(program.Ops), &results, mergeDuringCollect)
	if err != nil {
		return nil, mergeDuringCollect, err
	}
	return results, mergeDuringCollect, nil
}

func (l *Loader) executeBytecodeRawSpan(program *planbytecode.Program, pc int, end int, results *[]bytecodeRawFetchResult, mergeDuringCollect bool) (int, error) {
	for pc < end {
		op := program.Ops[pc]
		if bytecodeResponseOpcode(op.Code) {
			return pc, nil
		}
		switch op.Code {
		case planbytecode.OpNop, planbytecode.OpPasteAtPointer:
			pc++
		case planbytecode.OpFetchSubgraph:
			raw, err := l.loadBytecodeRawFetch(program, op.A)
			if err != nil {
				return pc, errors.WithStack(err)
			}
			*results = append(*results, raw)
			if mergeDuringCollect {
				if err := l.mergeBytecodeRawFetchResult(raw); err != nil {
					return pc, errors.WithStack(err)
				}
			}
			pc = skipBytecodePaste(program.Ops, pc+1)
		case planbytecode.OpEnterSequence:
			leavePC, err := bytecodeGroupLeavePC(program, pc, planbytecode.OpLeaveSequence)
			if err != nil {
				return pc, err
			}
			_, err = l.executeBytecodeRawSpan(program, pc+1, leavePC, results, mergeDuringCollect)
			if err != nil {
				return pc, err
			}
			pc = leavePC + 1
		case planbytecode.OpEnterParallel:
			leavePC, err := bytecodeGroupLeavePC(program, pc, planbytecode.OpLeaveParallel)
			if err != nil {
				return pc, err
			}
			err = l.executeBytecodeRawParallel(program, pc+1, leavePC, int(op.A), results, mergeDuringCollect)
			if err != nil {
				return pc, err
			}
			pc = leavePC + 1
		case planbytecode.OpLeaveSequence, planbytecode.OpLeaveParallel:
			return pc, nil
		default:
			return pc, fmt.Errorf("execute raw bytecode: unsupported opcode %s at pc %d", op.Code, pc)
		}
	}
	return pc, nil
}

func (l *Loader) executeBytecodeRawParallel(program *planbytecode.Program, pc int, leavePC int, children int, out *[]bytecodeRawFetchResult, mergeDuringCollect bool) error {
	if children == 0 {
		return nil
	}
	if !bytecodeParallelDirectFetches(program.Ops, pc, leavePC, children) {
		_, err := l.executeBytecodeRawSpan(program, pc, leavePC, out, mergeDuringCollect)
		return err
	}

	var resultScratch [4]bytecodeRawFetchResult
	results := resultScratch[:0]
	if children <= len(resultScratch) {
		results = resultScratch[:children]
	} else {
		results = make([]bytecodeRawFetchResult, children)
	}
	var g errgroup.Group
	cursor := pc
	for i := 0; i < children; i++ {
		i := i
		fetchRef := program.Ops[cursor].A
		cursor = skipBytecodePaste(program.Ops, cursor+1)
		g.Go(func() error {
			raw, err := l.loadBytecodeRawFetch(program, fetchRef)
			if err != nil {
				return err
			}
			results[i] = raw
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return errors.WithStack(err)
	}
	*out = append(*out, results...)
	if mergeDuringCollect {
		for i := range results {
			if err := l.mergeBytecodeRawFetchResult(results[i]); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	return nil
}

func (l *Loader) loadBytecodeRawFetch(program *planbytecode.Program, fetchRef uint32) (bytecodeRawFetchResult, error) {
	item, err := bytecodeFetchItem(program, fetchRef)
	if err != nil {
		return bytecodeRawFetchResult{}, err
	}
	items := l.selectItemsForPath(item.FetchPath)
	res := &result{}
	err = l.loadFetch(l.ctx.ctx, item.Fetch, item, items, res)
	if err != nil {
		return bytecodeRawFetchResult{}, err
	}
	return bytecodeRawFetchResult{
		item:  item,
		res:   res,
		items: items,
	}, nil
}

func (l *Loader) renderDirectResponseBytecode(program *planbytecode.Program, results []bytecodeRawFetchResult, writer io.Writer) (bool, error) {
	if !program.DirectResponseReady() {
		return false, nil
	}
	var sourceScratch [3]bytecodeValueSource
	selected := sourceScratch[:0]
	if len(results) > len(sourceScratch) {
		selected = make([]bytecodeValueSource, 0, len(results))
	}
	selected, ok := l.directValueSources(results, selected)
	if !ok {
		return false, nil
	}

	buf := l.resolvable.marshalBuf[:0]
	if cap(buf) < 1024 {
		buf = make([]byte, 0, 1024)
	}
	buf = append(buf, `{"data":{`...)
	buf, ok = l.renderDirectFields(program, buf, selected, selected, program.DirectResponse.Fields)
	if !ok {
		l.resolvable.marshalBuf = buf[:0]
		return false, nil
	}
	buf = append(buf, "}}"...)
	_, err := writer.Write(buf)
	if err != nil {
		l.resolvable.marshalBuf = buf[:0]
		return false, err
	}
	l.resolvable.marshalBuf = buf[:0]
	return true, nil
}

type bytecodeValueSource struct {
	src    []byte
	value  planbytecode.ByteRange
	prefix []string
	target *astjson.Value
}

func (l *Loader) directValueSources(results []bytecodeRawFetchResult, selected []bytecodeValueSource) ([]bytecodeValueSource, bool) {
	selected = selected[:0]
	for i := range results {
		data, ok := l.selectedDataRange(results[i])
		if !ok {
			return nil, false
		}
		if results[i].item != nil && len(results[i].item.FetchPath) != 0 {
			parentSources, ok := l.parentDataDirectSources(results[i], data)
			if !ok {
				return nil, false
			}
			selected = append(selected, parentSources...)
			continue
		}
		selected = append(selected, data)
	}
	return selected, true
}

func (l *Loader) selectedDataRange(raw bytecodeRawFetchResult) (bytecodeValueSource, bool) {
	res := raw.res
	if res == nil ||
		res.err != nil ||
		res.authorizationRejected ||
		res.rateLimitRejected ||
		res.fetchSkipped ||
		len(res.out) == 0 ||
		res.postProcessing.SelectResponseDataPath == nil ||
		l.allowCustomExtensionProperties {
		return bytecodeValueSource{}, false
	}
	if res.postProcessing.SelectResponseErrorsPath != nil {
		errorRange, status := planbytecode.FindValueRangeStatus(res.out, res.postProcessing.SelectResponseErrorsPath)
		if status == planbytecode.ValueRangeUnsupported {
			return bytecodeValueSource{}, false
		}
		if status == planbytecode.ValueRangeFound &&
			!planbytecode.ValueRangeIsNull(res.out, errorRange) &&
			!planbytecode.ValueRangeIsEmptyArray(res.out, errorRange) {
			return bytecodeValueSource{}, false
		}
	}
	dataRange, status := planbytecode.FindValueRangeStatus(res.out, res.postProcessing.SelectResponseDataPath)
	if status != planbytecode.ValueRangeFound || planbytecode.ValueRangeIsNull(res.out, dataRange) {
		return bytecodeValueSource{}, false
	}
	return bytecodeValueSource{
		src:    res.out,
		value:  dataRange,
		prefix: bytecodeDirectSourcePrefix(raw),
		target: bytecodeDirectSourceTarget(raw),
	}, true
}

func (l *Loader) parentDataDirectSources(raw bytecodeRawFetchResult, data bytecodeValueSource) ([]bytecodeValueSource, bool) {
	if raw.res == nil || raw.res.nestedMergeItems != nil || raw.item == nil {
		return nil, false
	}
	switch raw.item.Fetch.(type) {
	case *EntityFetch:
		if len(raw.items) != 1 {
			return nil, false
		}
		target := bytecodeMergePathTarget(raw.items[0], raw.res.postProcessing.MergePath)
		if target == nil {
			return nil, false
		}
		data.prefix = nil
		data.target = target
		return []bytecodeValueSource{data}, true
	case *BatchEntityFetch:
		return l.batchEntityDirectSources(raw, data)
	default:
		return nil, false
	}
}

func (l *Loader) batchEntityDirectSources(raw bytecodeRawFetchResult, data bytecodeValueSource) ([]bytecodeValueSource, bool) {
	var rangeScratch [32]planbytecode.ByteRange
	ranges, status := planbytecode.ScanArrayValueRanges(data.src, data.value, rangeScratch[:0])
	if status != planbytecode.ValueRangeFound {
		return nil, false
	}
	out := make([]bytecodeValueSource, 0, len(raw.items))
	if raw.res.batchStats != nil {
		if len(raw.res.batchStats) != len(ranges) {
			return nil, false
		}
		for responseIndex := range raw.res.batchStats {
			for _, target := range raw.res.batchStats[responseIndex] {
				target = bytecodeMergePathTarget(target, raw.res.postProcessing.MergePath)
				if target == nil {
					return nil, false
				}
				out = append(out, bytecodeValueSource{
					src:    data.src,
					value:  ranges[responseIndex],
					target: target,
				})
			}
		}
		return out, true
	}
	if len(raw.items) != len(ranges) {
		return nil, false
	}
	for i := range raw.items {
		target := bytecodeMergePathTarget(raw.items[i], raw.res.postProcessing.MergePath)
		if target == nil {
			return nil, false
		}
		out = append(out, bytecodeValueSource{
			src:    data.src,
			value:  ranges[i],
			target: target,
		})
	}
	return out, true
}

func (l *Loader) renderDirectFields(program *planbytecode.Program, buf []byte, sources []bytecodeValueSource, allSources []bytecodeValueSource, fields []planbytecode.DirectField) ([]byte, bool) {
	for i := range fields {
		if int(fields[i].NameRef) >= len(program.Strings) {
			return buf, false
		}
		if i != 0 {
			buf = append(buf, ',')
		}
		var ok bool
		buf, ok = bytecodeAppendQuotedProgramString(buf, program, fields[i].NameRef)
		if !ok {
			return buf, false
		}
		buf = append(buf, ':')
		buf, ok = l.renderDirectValue(program, buf, sources, allSources, fields[i])
		if !ok {
			return buf, false
		}
	}
	return buf, true
}

func bytecodeAppendQuotedProgramString(buf []byte, program *planbytecode.Program, ref uint32) ([]byte, bool) {
	if program == nil || int(ref) >= len(program.Strings) {
		return buf, false
	}
	if int(ref) < len(program.QuotedStrings) && program.QuotedStrings[ref] != "" {
		return append(buf, program.QuotedStrings[ref]...), true
	}
	return strconv.AppendQuote(buf, program.Strings[ref]), true
}

func (l *Loader) renderDirectValue(program *planbytecode.Program, buf []byte, sources []bytecodeValueSource, allSources []bytecodeValueSource, field planbytecode.DirectField) ([]byte, bool) {
	if planbytecode.DirectFieldIsLiteral(field.Flags) {
		switch NodeKind(planbytecode.DirectFieldKind(field.Flags)) {
		case NodeKindNull:
			return append(buf, "null"...), true
		default:
			if int(field.LiteralRef) >= len(program.Strings) {
				return buf, false
			}
			return bytecodeAppendQuotedProgramString(buf, program, field.LiteralRef)
		}
	}

	kind := NodeKind(planbytecode.DirectFieldKind(field.Flags))
	switch kind {
	case NodeKindObject:
		return l.renderDirectObject(program, buf, sources, allSources, field)
	case NodeKindArray:
		return l.renderDirectArray(program, buf, sources, allSources, field)
	default:
		source, ok := l.resolveSingleDirectSource(program, sources, field.PathRef, field.Flags)
		if !ok {
			if planbytecode.DirectFieldIsNullable(field.Flags) {
				return append(buf, "null"...), true
			}
			return buf, false
		}
		return append(buf, source.src[source.value.Start:source.value.End]...), true
	}
}

func (l *Loader) renderDirectObject(program *planbytecode.Program, buf []byte, sources []bytecodeValueSource, allSources []bytecodeValueSource, field planbytecode.DirectField) ([]byte, bool) {
	var objectSources []bytecodeValueSource
	if len(sources) == 1 {
		source, nullFound, found, ok := l.resolveDirectSource(program, sources[0], field.PathRef, field.Flags)
		if !ok {
			return buf, false
		}
		if !found {
			if nullFound || planbytecode.DirectFieldIsNullable(field.Flags) {
				return append(buf, "null"...), true
			}
			return buf, false
		}
		var sourceScratch [1]bytecodeValueSource
		objectSources = sourceScratch[:1]
		objectSources[0] = source
	} else {
		var nullFound bool
		var ok bool
		var sourceScratch [2]bytecodeValueSource
		objectSources, nullFound, ok = l.resolveDirectSources(program, sources, field.PathRef, field.Flags, sourceScratch[:0])
		if !ok {
			return buf, false
		}
		if len(objectSources) == 0 {
			if nullFound || planbytecode.DirectFieldIsNullable(field.Flags) {
				return append(buf, "null"...), true
			}
			return buf, false
		}
	}
	for i := range objectSources {
		if !bytecodeValidateObjectValue(objectSources[i].src[objectSources[i].value.Start:objectSources[i].value.End]) {
			return buf, false
		}
	}
	objectSources = appendDirectTargetOverlays(objectSources, allSources)
	buf = append(buf, '{')
	var rendered bool
	buf, rendered = l.renderDirectFields(program, buf, objectSources, allSources, field.Children)
	if !rendered {
		return buf, false
	}
	buf = append(buf, '}')
	return buf, true
}

func (l *Loader) renderDirectArray(program *planbytecode.Program, buf []byte, sources []bytecodeValueSource, allSources []bytecodeValueSource, field planbytecode.DirectField) ([]byte, bool) {
	var arraySources []bytecodeValueSource
	if len(sources) == 1 {
		source, nullFound, found, ok := l.resolveDirectSource(program, sources[0], field.PathRef, field.Flags)
		if !ok {
			return buf, false
		}
		if !found {
			if nullFound || planbytecode.DirectFieldIsNullable(field.Flags) {
				return append(buf, "null"...), true
			}
			return buf, false
		}
		return l.renderDirectSingleArraySource(program, buf, source, allSources, field)
	} else {
		var nullFound bool
		var ok bool
		var sourceScratch [2]bytecodeValueSource
		arraySources, nullFound, ok = l.resolveDirectSources(program, sources, field.PathRef, field.Flags, sourceScratch[:0])
		if !ok {
			return buf, false
		}
		if len(arraySources) == 0 {
			if nullFound || planbytecode.DirectFieldIsNullable(field.Flags) {
				return append(buf, "null"...), true
			}
			return buf, false
		}
	}

	var elementRangeScratch [2][]planbytecode.ByteRange
	elementRanges := elementRangeScratch[:0]
	if len(arraySources) <= len(elementRangeScratch) {
		elementRanges = elementRangeScratch[:len(arraySources)]
	} else {
		elementRanges = make([][]planbytecode.ByteRange, len(arraySources))
	}
	var elementCount int
	var rangeScratch [2][4]planbytecode.ByteRange
	for i := range arraySources {
		var scanScratch []planbytecode.ByteRange
		if i < len(rangeScratch) {
			scanScratch = rangeScratch[i][:0]
		}
		ranges, status := planbytecode.ScanArrayValueRanges(arraySources[i].src, arraySources[i].value, scanScratch)
		if status != planbytecode.ValueRangeFound {
			return buf, false
		}
		if i == 0 {
			elementCount = len(ranges)
		} else if len(ranges) != elementCount {
			return buf, false
		}
		elementRanges[i] = ranges
	}

	buf = append(buf, '[')
	for elementIndex := 0; elementIndex < elementCount; elementIndex++ {
		if elementIndex != 0 {
			buf = append(buf, ',')
		}
		if len(field.Children) == 0 {
			if len(arraySources) != 1 {
				return buf, false
			}
			element := bytecodeValueSource{
				src:    arraySources[0].src,
				value:  elementRanges[0][elementIndex],
				target: bytecodeArrayElementTarget(arraySources[0].target, elementIndex),
			}
			if !bytecodeValidateDirectValue(element.src[element.value.Start:element.value.End], field.ItemFlags) {
				return buf, false
			}
			buf = append(buf, element.src[element.value.Start:element.value.End]...)
			continue
		}
		var sourceScratch [2]bytecodeValueSource
		elementSources := sourceScratch[:0]
		if len(arraySources) <= len(sourceScratch) {
			elementSources = sourceScratch[:len(arraySources)]
		} else {
			elementSources = make([]bytecodeValueSource, len(arraySources))
		}
		for sourceIndex := range arraySources {
			elementSources[sourceIndex] = bytecodeValueSource{
				src:   arraySources[sourceIndex].src,
				value: elementRanges[sourceIndex][elementIndex],
				target: bytecodeArrayElementTarget(
					arraySources[sourceIndex].target,
					elementIndex,
				),
			}
			if !bytecodeValidateObjectValue(elementSources[sourceIndex].src[elementSources[sourceIndex].value.Start:elementSources[sourceIndex].value.End]) {
				return buf, false
			}
		}
		elementSources = appendDirectTargetOverlays(elementSources, allSources)
		buf = append(buf, '{')
		var rendered bool
		buf, rendered = l.renderDirectFields(program, buf, elementSources, allSources, field.Children)
		if !rendered {
			return buf, false
		}
		buf = append(buf, '}')
	}
	buf = append(buf, ']')
	return buf, true
}

func (l *Loader) renderDirectSingleArraySource(program *planbytecode.Program, buf []byte, source bytecodeValueSource, allSources []bytecodeValueSource, field planbytecode.DirectField) ([]byte, bool) {
	var rangeScratch [32]planbytecode.ByteRange
	ranges, status := planbytecode.ScanArrayValueRanges(source.src, source.value, rangeScratch[:0])
	if status != planbytecode.ValueRangeFound {
		return buf, false
	}

	buf = append(buf, '[')
	for elementIndex := range ranges {
		if elementIndex != 0 {
			buf = append(buf, ',')
		}
		element := bytecodeValueSource{
			src:    source.src,
			value:  ranges[elementIndex],
			target: bytecodeArrayElementTarget(source.target, elementIndex),
		}
		if len(field.Children) == 0 {
			if !bytecodeValidateDirectValue(element.src[element.value.Start:element.value.End], field.ItemFlags) {
				return buf, false
			}
			buf = append(buf, element.src[element.value.Start:element.value.End]...)
			continue
		}
		if !bytecodeValidateObjectValue(element.src[element.value.Start:element.value.End]) {
			return buf, false
		}

		var sourceScratch [1]bytecodeValueSource
		elementSources := sourceScratch[:1]
		elementSources[0] = element
		elementSources = appendDirectTargetOverlays(elementSources, allSources)

		buf = append(buf, '{')
		var rendered bool
		buf, rendered = l.renderDirectFields(program, buf, elementSources, allSources, field.Children)
		if !rendered {
			return buf, false
		}
		buf = append(buf, '}')
	}
	buf = append(buf, ']')
	return buf, true
}

func (l *Loader) resolveDirectSource(program *planbytecode.Program, source bytecodeValueSource, pathRef uint32, flags uint32) (bytecodeValueSource, bool, bool, bool) {
	if int(pathRef) >= len(program.Paths) {
		return bytecodeValueSource{}, false, false, false
	}
	src, path, ok := bytecodeSourceSearch(source, program.Paths[pathRef])
	if !ok {
		return bytecodeValueSource{}, false, false, true
	}
	valueRange, status := bytecodeFindValueRange(src, path)
	if status == planbytecode.ValueRangeUnsupported {
		return bytecodeValueSource{}, false, false, false
	}
	if status != planbytecode.ValueRangeFound {
		return bytecodeValueSource{}, false, false, true
	}
	if planbytecode.ValueRangeIsNull(src, valueRange) {
		if !planbytecode.DirectFieldIsNullable(flags) {
			return bytecodeValueSource{}, true, false, false
		}
		return bytecodeValueSource{}, true, false, true
	}
	return bytecodeValueSource{
		src:    src,
		value:  valueRange,
		target: bytecodeSourceChildTarget(source, path),
	}, false, true, true
}

func (l *Loader) resolveSingleDirectSource(program *planbytecode.Program, sources []bytecodeValueSource, pathRef uint32, flags uint32) (bytecodeValueSource, bool) {
	if int(pathRef) >= len(program.Paths) {
		return bytecodeValueSource{}, false
	}
	var out bytecodeValueSource
	var found bool
	var nullFound bool
	for i := range sources {
		src, path, ok := bytecodeSourceSearch(sources[i], program.Paths[pathRef])
		if !ok {
			continue
		}
		valueRange, status := bytecodeFindValueRange(src, path)
		if status == planbytecode.ValueRangeUnsupported {
			return bytecodeValueSource{}, false
		}
		if status != planbytecode.ValueRangeFound {
			continue
		}
		if planbytecode.ValueRangeIsNull(src, valueRange) {
			nullFound = true
			continue
		}
		if found {
			return bytecodeValueSource{}, false
		}
		out = bytecodeValueSource{src: src, value: valueRange}
		out.target = bytecodeSourceChildTarget(sources[i], path)
		found = true
	}
	if nullFound && found {
		return bytecodeValueSource{}, false
	}
	if nullFound {
		return bytecodeValueSource{}, false
	}
	if !found {
		return bytecodeValueSource{}, false
	}
	if !bytecodeValidateDirectValue(out.src[out.value.Start:out.value.End], flags) {
		return bytecodeValueSource{}, false
	}
	return out, true
}

func (l *Loader) resolveDirectSources(program *planbytecode.Program, sources []bytecodeValueSource, pathRef uint32, flags uint32, out []bytecodeValueSource) ([]bytecodeValueSource, bool, bool) {
	if int(pathRef) >= len(program.Paths) {
		return nil, false, false
	}
	out = out[:0]
	var nullFound bool
	for i := range sources {
		src, path, ok := bytecodeSourceSearch(sources[i], program.Paths[pathRef])
		if !ok {
			continue
		}
		valueRange, status := bytecodeFindValueRange(src, path)
		if status == planbytecode.ValueRangeUnsupported {
			return nil, false, false
		}
		if status != planbytecode.ValueRangeFound {
			continue
		}
		if planbytecode.ValueRangeIsNull(src, valueRange) {
			nullFound = true
			continue
		}
		out = append(out, bytecodeValueSource{
			src:    src,
			value:  valueRange,
			target: bytecodeSourceChildTarget(sources[i], path),
		})
	}
	if nullFound && len(out) != 0 {
		return nil, false, false
	}
	if nullFound && !planbytecode.DirectFieldIsNullable(flags) {
		return nil, false, false
	}
	return out, nullFound, true
}

func bytecodeSourceSearch(source bytecodeValueSource, path []string) ([]byte, []string, bool) {
	src := source.src[source.value.Start:source.value.End]
	if len(source.prefix) == 0 {
		return src, path, true
	}
	if len(path) < len(source.prefix) {
		return nil, nil, false
	}
	for i := range source.prefix {
		if path[i] != source.prefix[i] {
			return nil, nil, false
		}
	}
	return src, path[len(source.prefix):], true
}

func bytecodeFindValueRange(src []byte, path []string) (planbytecode.ByteRange, planbytecode.ValueRangeStatus) {
	if len(path) == 0 {
		return planbytecode.FindValueRangeStatus(src, nil)
	}
	return planbytecode.FindValueRangeStatus(src, path)
}

func bytecodeDirectSourceTarget(raw bytecodeRawFetchResult) *astjson.Value {
	if len(raw.items) != 1 {
		return nil
	}
	if raw.res == nil {
		return raw.items[0]
	}
	return bytecodeMergePathTarget(raw.items[0], raw.res.postProcessing.MergePath)
}

func bytecodeMergePathTarget(target *astjson.Value, mergePath []string) *astjson.Value {
	if target == nil {
		return nil
	}
	if len(mergePath) == 0 {
		return target
	}
	return target.Get(mergePath...)
}

func bytecodeSourceChildTarget(source bytecodeValueSource, path []string) *astjson.Value {
	if source.target == nil {
		return nil
	}
	if len(path) == 0 {
		return source.target
	}
	return source.target.Get(path...)
}

func bytecodeArrayElementTarget(target *astjson.Value, index int) *astjson.Value {
	if target == nil || target.Type() != astjson.TypeArray {
		return nil
	}
	values := target.GetArray()
	if index < 0 || index >= len(values) {
		return nil
	}
	return values[index]
}

func appendDirectTargetOverlays(current []bytecodeValueSource, all []bytecodeValueSource) []bytecodeValueSource {
	for i := range all {
		if all[i].target == nil || bytecodeDirectSourceInSet(current, all[i]) {
			continue
		}
		for j := range current {
			if current[j].target == all[i].target {
				current = append(current, all[i])
				break
			}
		}
	}
	return current
}

func bytecodeDirectSourceInSet(sources []bytecodeValueSource, candidate bytecodeValueSource) bool {
	for i := range sources {
		if sources[i].src == nil && candidate.src == nil {
			if sources[i].target == candidate.target &&
				sources[i].value == candidate.value &&
				slicesEqual(sources[i].prefix, candidate.prefix) {
				return true
			}
			continue
		}
		if len(sources[i].src) != 0 &&
			len(candidate.src) != 0 &&
			&sources[i].src[0] == &candidate.src[0] &&
			sources[i].target == candidate.target &&
			sources[i].value == candidate.value &&
			slicesEqual(sources[i].prefix, candidate.prefix) {
			return true
		}
	}
	return false
}

func slicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func bytecodeValidateDirectValue(src []byte, flags uint32) bool {
	i := bytecodeSkipJSONWS(src, 0)
	if i >= len(src) {
		return false
	}
	if bytecodeValueStartsWith(src[i:], "null") {
		return planbytecode.DirectFieldIsNullable(flags)
	}
	switch NodeKind(planbytecode.DirectFieldKind(flags)) {
	case NodeKindString:
		return src[i] == '"'
	case NodeKindBoolean:
		return bytecodeValueStartsWith(src[i:], "true") || bytecodeValueStartsWith(src[i:], "false")
	case NodeKindInteger, NodeKindFloat, NodeKindBigInt:
		return src[i] == '-' || (src[i] >= '0' && src[i] <= '9')
	case NodeKindScalar:
		return true
	default:
		return false
	}
}

func bytecodeValidateObjectValue(src []byte) bool {
	i := bytecodeSkipJSONWS(src, 0)
	return i < len(src) && src[i] == '{'
}

func bytecodeValueStartsWith(src []byte, value string) bool {
	if len(src) < len(value) {
		return false
	}
	for i := range value {
		if src[i] != value[i] {
			return false
		}
	}
	return true
}

func bytecodeSkipJSONWS(src []byte, i int) int {
	for i < len(src) {
		switch src[i] {
		case ' ', '\n', '\r', '\t':
			i++
		default:
			return i
		}
	}
	return i
}

func bytecodeDirectSourcePrefix(raw bytecodeRawFetchResult) []string {
	var prefix []string
	if raw.item != nil && len(raw.item.FetchPath) != 0 {
		prefix = bytecodeFetchPathPrefix(raw.item.FetchPath)
	}
	if raw.res != nil && len(raw.res.postProcessing.MergePath) != 0 {
		prefix = append(prefix, raw.res.postProcessing.MergePath...)
	}
	return prefix
}

func bytecodeFetchPathPrefix(path []FetchItemPathElement) []string {
	var prefix []string
	for i := range path {
		prefix = append(prefix, path[i].Path...)
	}
	return prefix
}

func (l *Loader) mergeRawFetchResults(results []bytecodeRawFetchResult) error {
	for i := range results {
		if err := l.mergeBytecodeRawFetchResult(results[i]); err != nil {
			return errors.WithStack(err)
		}
		l.finishRawFetchResult(results[i])
	}
	return nil
}

func (l *Loader) mergeBytecodeRawFetchResult(raw bytecodeRawFetchResult) error {
	if raw.res.nestedMergeItems != nil {
		for j := range raw.res.nestedMergeItems {
			err := l.mergeResultBytecode(raw.item, raw.res.nestedMergeItems[j], raw.items[j:j+1])
			if err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}
	return l.mergeResultBytecode(raw.item, raw.res, raw.items)
}

func (l *Loader) finishRawFetchResults(results []bytecodeRawFetchResult) {
	for i := range results {
		l.finishRawFetchResult(results[i])
	}
}

func (l *Loader) finishRawFetchResult(raw bytecodeRawFetchResult) {
	if raw.res == nil {
		return
	}
	if raw.res.nestedMergeItems != nil {
		for j := range raw.res.nestedMergeItems {
			l.callOnFinished(raw.res.nestedMergeItems[j])
		}
		return
	}
	l.callOnFinished(raw.res)
}

func bytecodeProgramNeedsMergedParentData(program *planbytecode.Program) bool {
	if program == nil {
		return false
	}
	for i := range program.Fetches {
		item, ok := program.Fetches[i].Item.(*FetchItem)
		if ok && item != nil && len(item.FetchPath) != 0 {
			return true
		}
	}
	return false
}

func bytecodeDirectResponseRuntimeReady(ctx *Context, program *planbytecode.Program) bool {
	if !program.DirectResponseReady() {
		return false
	}
	if bytecodeProgramNeedsMergedParentData(program) {
		return false
	}
	if ctx == nil {
		return false
	}
	if ctx.fieldRenderer != nil ||
		ctx.authorizer != nil ||
		len(ctx.RenameTypeNames) != 0 ||
		ctx.ExecutionOptions.IncludeQueryPlanInResponse ||
		(ctx.TracingOptions.Enable && ctx.TracingOptions.IncludeTraceOutputInResponseExtensions) ||
		(ctx.RateLimitOptions.Enable && ctx.RateLimitOptions.IncludeStatsInResponseExtension) {
		return false
	}
	return true
}

func bytecodeResponseRuntimeReady(r *Resolvable, program *planbytecode.Program) bool {
	if r == nil || r.ctx == nil || !program.FastPathReady() {
		return false
	}
	if r.ctx.ExecutionOptions.SkipLoader ||
		r.ctx.fieldRenderer != nil ||
		r.ctx.authorizer != nil ||
		len(r.ctx.RenameTypeNames) != 0 ||
		r.ctx.ExecutionOptions.IncludeQueryPlanInResponse ||
		(r.ctx.TracingOptions.Enable && r.ctx.TracingOptions.IncludeTraceOutputInResponseExtensions) ||
		(r.ctx.RateLimitOptions.Enable && r.ctx.RateLimitOptions.IncludeStatsInResponseExtension) ||
		r.options.ApolloCompatibilityTruncateFloatValues ||
		r.hasErrors() ||
		r.valueCompletion != nil ||
		len(r.subgraphExtensions) != 0 {
		return false
	}
	return true
}

type bytecodeResponseRenderer struct {
	program   *planbytecode.Program
	listSizes []bytecodeListSize
	fieldPath [32]string
	depth     int
}

type bytecodeListSize struct {
	path string
	size int
}

func (r *bytecodeResponseRenderer) renderValue(buf []byte, pc int, parent *astjson.Value) ([]byte, int, bool) {
	if pc >= len(r.program.Ops) {
		return buf, pc, false
	}

	switch r.program.Ops[pc].Code {
	case planbytecode.OpEnterObject:
		return r.renderObject(buf, pc, parent)
	case planbytecode.OpEnterArray:
		return r.renderArray(buf, pc, parent)
	case planbytecode.OpProjectField:
		return r.renderProjectedValue(buf, r.program.Ops[pc], parent, pc+1)
	case planbytecode.OpEmitLiteral:
		return r.renderLiteral(buf, r.program.Ops[pc], pc+1)
	default:
		return buf, pc, false
	}
}

func (r *bytecodeResponseRenderer) renderObject(buf []byte, pc int, parent *astjson.Value) ([]byte, int, bool) {
	if parent == nil {
		return buf, pc, false
	}
	op := r.program.Ops[pc]
	path, ok := r.pathRef(op.A)
	if !ok {
		return buf, pc, false
	}
	if op.B == 0 && len(path) == 0 {
		return append(buf, "{}"...), r.skipValue(pc), true
	}
	value := parent.Get(path...)
	if astjson.ValueIsNull(value) {
		if op.C != 0 {
			return append(buf, "null"...), r.skipValue(pc), true
		}
		return buf, pc, false
	}
	if value.Type() != astjson.TypeObject {
		return buf, pc, false
	}

	buf = append(buf, '{')
	childPC := pc + 1
	for fieldIndex := uint32(0); fieldIndex < op.B; fieldIndex++ {
		if childPC >= len(r.program.Ops) || r.program.Ops[childPC].Code != planbytecode.OpProjectField {
			return buf, childPC, false
		}
		fieldOp := r.program.Ops[childPC]
		name, ok := r.string(fieldOp.A)
		if !ok {
			return buf, childPC, false
		}
		if fieldIndex != 0 {
			buf = append(buf, ',')
		}
		buf, ok = bytecodeAppendQuotedProgramString(buf, r.program, fieldOp.A)
		if !ok {
			return buf, childPC, false
		}
		buf = append(buf, ':')

		if !r.pushField(name) {
			return buf, childPC, false
		}
		nextPC := childPC + 1
		if r.projectHasNestedValue(fieldOp, nextPC) {
			var rendered bool
			buf, childPC, rendered = r.renderValue(buf, nextPC, value)
			if !rendered {
				r.popField()
				return buf, childPC, false
			}
		} else {
			var rendered bool
			buf, childPC, rendered = r.renderProjectedValue(buf, fieldOp, value, nextPC)
			if !rendered {
				r.popField()
				return buf, childPC, false
			}
		}
		r.popField()
	}
	if childPC >= len(r.program.Ops) || r.program.Ops[childPC].Code != planbytecode.OpLeaveObject {
		return buf, childPC, false
	}
	buf = append(buf, '}')
	return buf, childPC + 1, true
}

func (r *bytecodeResponseRenderer) renderArray(buf []byte, pc int, parent *astjson.Value) ([]byte, int, bool) {
	if parent == nil {
		return buf, pc, false
	}
	op := r.program.Ops[pc]
	path, ok := r.pathRef(op.A)
	if !ok {
		return buf, pc, false
	}
	leavePC := r.skipValue(pc) - 1
	if pc+1 == leavePC {
		return append(buf, "[]"...), leavePC + 1, true
	}
	value := parent.Get(path...)
	if astjson.ValueIsNull(value) {
		if op.C != 0 {
			return append(buf, "null"...), r.skipValue(pc), true
		}
		return buf, pc, false
	}
	if value.Type() != astjson.TypeArray {
		return buf, pc, false
	}
	values := value.GetArray()
	if key := strings.Join(r.fieldPath[:r.depth], "."); key != "" {
		r.listSizes = append(r.listSizes, bytecodeListSize{path: key, size: len(values)})
	}

	itemPC := pc + 1
	if itemPC >= leavePC {
		return buf, pc, false
	}

	buf = append(buf, '[')
	for i, item := range values {
		if i != 0 {
			buf = append(buf, ',')
		}
		var rendered bool
		var nextPC int
		buf, nextPC, rendered = r.renderValue(buf, itemPC, item)
		if !rendered || nextPC != leavePC {
			return buf, nextPC, false
		}
	}
	buf = append(buf, ']')
	return buf, leavePC + 1, true
}

func (r *bytecodeResponseRenderer) renderProjectedValue(buf []byte, op planbytecode.Op, parent *astjson.Value, nextPC int) ([]byte, int, bool) {
	if parent == nil {
		return buf, nextPC, false
	}
	path, ok := r.pathRef(op.B)
	if !ok {
		return buf, nextPC, false
	}
	value := parent.Get(path...)
	flags := op.C
	if astjson.ValueIsNull(value) {
		if bytecodeNodeNullable(flags) {
			return append(buf, "null"...), nextPC, true
		}
		return buf, nextPC, false
	}
	if !bytecodeValidateASTJSONValue(value, flags) {
		return buf, nextPC, false
	}
	return value.MarshalTo(buf), nextPC, true
}

func (r *bytecodeResponseRenderer) renderLiteral(buf []byte, op planbytecode.Op, nextPC int) ([]byte, int, bool) {
	switch NodeKind(op.C) {
	case NodeKindNull:
		return append(buf, "null"...), nextPC, true
	case NodeKindStaticString:
		var ok bool
		buf, ok = bytecodeAppendQuotedProgramString(buf, r.program, op.A)
		if !ok {
			return buf, nextPC, false
		}
		return buf, nextPC, true
	default:
		return buf, nextPC, false
	}
}

func (r *bytecodeResponseRenderer) projectHasNestedValue(fieldOp planbytecode.Op, nextPC int) bool {
	if nextPC >= len(r.program.Ops) {
		return false
	}
	next := r.program.Ops[nextPC].Code
	switch NodeKind(bytecodeNodeKind(fieldOp.C)) {
	case NodeKindObject, NodeKindEmptyObject:
		return next == planbytecode.OpEnterObject
	case NodeKindArray, NodeKindEmptyArray:
		return next == planbytecode.OpEnterArray
	case NodeKindStaticString:
		return next == planbytecode.OpEmitLiteral
	default:
		return false
	}
}

func (r *bytecodeResponseRenderer) skipValue(pc int) int {
	if pc >= len(r.program.Ops) {
		return pc
	}
	switch r.program.Ops[pc].Code {
	case planbytecode.OpEnterObject:
		cursor := pc + 1
		for fieldIndex := uint32(0); fieldIndex < r.program.Ops[pc].B && cursor < len(r.program.Ops); fieldIndex++ {
			fieldOp := r.program.Ops[cursor]
			if fieldOp.Code != planbytecode.OpProjectField {
				return cursor
			}
			cursor++
			if r.projectHasNestedValue(fieldOp, cursor) {
				cursor = r.skipValue(cursor)
			}
		}
		if cursor < len(r.program.Ops) && r.program.Ops[cursor].Code == planbytecode.OpLeaveObject {
			return cursor + 1
		}
		return cursor
	case planbytecode.OpEnterArray:
		cursor := r.skipValue(pc + 1)
		if cursor < len(r.program.Ops) && r.program.Ops[cursor].Code == planbytecode.OpLeaveArray {
			return cursor + 1
		}
		return cursor
	default:
		return pc + 1
	}
}

func (r *bytecodeResponseRenderer) string(ref uint32) (string, bool) {
	if int(ref) >= len(r.program.Strings) {
		return "", false
	}
	return r.program.Strings[ref], true
}

func (r *bytecodeResponseRenderer) pushField(name string) bool {
	if r.depth >= len(r.fieldPath) {
		return false
	}
	r.fieldPath[r.depth] = name
	r.depth++
	return true
}

func (r *bytecodeResponseRenderer) popField() {
	r.depth--
	r.fieldPath[r.depth] = ""
}

func (r *bytecodeResponseRenderer) pathRef(ref uint32) ([]string, bool) {
	if int(ref) >= len(r.program.Paths) {
		return nil, false
	}
	return r.program.Paths[ref], true
}

func bytecodeResponseStart(ops []planbytecode.Op) int {
	for i := range ops {
		switch ops[i].Code {
		case planbytecode.OpEnterObject:
			return i
		case planbytecode.OpEmitResponse:
			return -1
		}
	}
	return -1
}

func bytecodeValidateASTJSONValue(value *astjson.Value, flags uint32) bool {
	switch NodeKind(bytecodeNodeKind(flags)) {
	case NodeKindNull:
		return astjson.ValueIsNull(value)
	case NodeKindString:
		return value.Type() == astjson.TypeString
	case NodeKindBoolean:
		return value.Type() == astjson.TypeTrue || value.Type() == astjson.TypeFalse
	case NodeKindInteger, NodeKindFloat, NodeKindBigInt:
		return value.Type() == astjson.TypeNumber
	case NodeKindScalar:
		return true
	default:
		return false
	}
}

func bytecodeNodeKind(flags uint32) uint32 {
	return flags & 0x0000ffff
}

func bytecodeNodeNullable(flags uint32) bool {
	return flags&(1<<16) != 0
}

func (l *Loader) executeBytecodeSpan(program *planbytecode.Program, pc int, end int) (int, error) {
	for pc < end {
		op := program.Ops[pc]
		if bytecodeResponseOpcode(op.Code) {
			return pc, nil
		}
		switch op.Code {
		case planbytecode.OpNop, planbytecode.OpPasteAtPointer:
			pc++
		case planbytecode.OpFetchSubgraph:
			item, err := bytecodeFetchItem(program, op.A)
			if err != nil {
				return pc, err
			}
			if err := l.resolveSingleBytecode(item); err != nil {
				return pc, errors.WithStack(err)
			}
			pc = skipBytecodePaste(program.Ops, pc+1)
		case planbytecode.OpEnterSequence:
			leavePC, err := bytecodeGroupLeavePC(program, pc, planbytecode.OpLeaveSequence)
			if err != nil {
				return pc, err
			}
			_, err = l.executeBytecodeSpan(program, pc+1, leavePC)
			if err != nil {
				return pc, err
			}
			pc = leavePC + 1
		case planbytecode.OpEnterParallel:
			leavePC, err := bytecodeGroupLeavePC(program, pc, planbytecode.OpLeaveParallel)
			if err != nil {
				return pc, err
			}
			if err := l.executeBytecodeParallel(program, pc+1, leavePC, int(op.A)); err != nil {
				return pc, err
			}
			pc = leavePC + 1
		case planbytecode.OpLeaveSequence, planbytecode.OpLeaveParallel:
			return pc, nil
		default:
			return pc, fmt.Errorf("execute bytecode: unsupported opcode %s at pc %d", op.Code, pc)
		}
	}
	return pc, nil
}

func (l *Loader) executeBytecodeParallel(program *planbytecode.Program, pc int, leavePC int, children int) error {
	if children == 0 {
		return nil
	}

	if !bytecodeParallelDirectFetches(program.Ops, pc, leavePC, children) {
		_, err := l.executeBytecodeSpan(program, pc, leavePC)
		if err != nil {
			return err
		}
		return nil
	}

	results := make([]*result, children)
	defer func() {
		for i := range results {
			if results[i] != nil {
				batchEntityToolPool.Put(results[i].tools)
			}
		}
	}()

	itemsItems := make([][]*astjson.Value, children)
	g, gCtx := errgroup.WithContext(l.ctx.ctx)
	cursor := pc
	for i := 0; i < children; i++ {
		i := i
		item, err := bytecodeFetchItem(program, program.Ops[cursor].A)
		if err != nil {
			return err
		}
		results[i] = &result{}
		itemsItems[i] = l.selectItemsForPath(item.FetchPath)
		f := item.Fetch
		items := itemsItems[i]
		res := results[i]
		cursor = skipBytecodePaste(program.Ops, cursor+1)
		g.Go(func() error {
			return l.loadFetch(gCtx, f, item, items, res)
		})
	}

	err := g.Wait()
	if err != nil {
		return errors.WithStack(err)
	}

	cursor = pc
	for i := range results {
		item, err := bytecodeFetchItem(program, program.Ops[cursor].A)
		if err != nil {
			return err
		}
		cursor = skipBytecodePaste(program.Ops, cursor+1)
		if results[i].nestedMergeItems != nil {
			for j := range results[i].nestedMergeItems {
				err = l.mergeResultBytecode(item, results[i].nestedMergeItems[j], itemsItems[i][j:j+1])
				l.callOnFinished(results[i].nestedMergeItems[j])
				if err != nil {
					return errors.WithStack(err)
				}
			}
			continue
		}
		err = l.mergeResultBytecode(item, results[i], itemsItems[i])
		l.callOnFinished(results[i])
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func (l *Loader) resolveSingleBytecode(item *FetchItem) error {
	if item == nil {
		return nil
	}
	items := l.selectItemsForPath(item.FetchPath)

	switch f := item.Fetch.(type) {
	case *SingleFetch:
		res := &result{}
		err := l.loadSingleFetch(l.ctx.ctx, f, item, items, res)
		if err != nil {
			return err
		}
		err = l.mergeResultBytecode(item, res, items)
		l.callOnFinished(res)
		return err
	case *BatchEntityFetch:
		res := &result{}
		defer batchEntityToolPool.Put(res.tools)
		err := l.loadBatchEntityFetch(l.ctx.ctx, item, f, items, res)
		if err != nil {
			return errors.WithStack(err)
		}
		err = l.mergeResultBytecode(item, res, items)
		l.callOnFinished(res)
		return err
	case *EntityFetch:
		res := &result{}
		err := l.loadEntityFetch(l.ctx.ctx, item, f, items, res)
		if err != nil {
			return errors.WithStack(err)
		}
		err = l.mergeResultBytecode(item, res, items)
		l.callOnFinished(res)
		return err
	default:
		return nil
	}
}

func (l *Loader) mergeResultBytecode(fetchItem *FetchItem, res *result, items []*astjson.Value) error {
	err := l.mergeResultDataRange(fetchItem, res, items)
	if err == nil {
		return nil
	}
	if err == errBytecodeFastMergeUnsupported {
		return l.mergeResult(fetchItem, res, items)
	}
	return err
}

var errBytecodeFastMergeUnsupported = fmt.Errorf("bytecode fast merge unsupported")

func (l *Loader) mergeResultDataRange(fetchItem *FetchItem, res *result, items []*astjson.Value) error {
	if res.err != nil ||
		res.authorizationRejected ||
		res.rateLimitRejected ||
		res.fetchSkipped ||
		len(res.out) == 0 ||
		l.allowCustomExtensionProperties ||
		res.postProcessing.SelectResponseDataPath == nil {
		return errBytecodeFastMergeUnsupported
	}

	if res.postProcessing.SelectResponseErrorsPath != nil {
		errorRange, status := planbytecode.FindValueRangeStatus(res.out, res.postProcessing.SelectResponseErrorsPath)
		if status == planbytecode.ValueRangeUnsupported {
			return errBytecodeFastMergeUnsupported
		}
		if status == planbytecode.ValueRangeFound &&
			!planbytecode.ValueRangeIsNull(res.out, errorRange) &&
			!planbytecode.ValueRangeIsEmptyArray(res.out, errorRange) {
			return errBytecodeFastMergeUnsupported
		}
	}

	dataRange, status := planbytecode.FindValueRangeStatus(res.out, res.postProcessing.SelectResponseDataPath)
	if status != planbytecode.ValueRangeFound || planbytecode.ValueRangeIsNull(res.out, dataRange) {
		return errBytecodeFastMergeUnsupported
	}

	responseData, err := astjson.ParseBytesWithArena(l.jsonArena, res.out[dataRange.Start:dataRange.End])
	if err != nil {
		return errBytecodeFastMergeUnsupported
	}

	return l.mergeResponseDataNoErrors(fetchItem, res, items, responseData)
}

func (l *Loader) mergeResponseDataNoErrors(fetchItem *FetchItem, res *result, items []*astjson.Value, responseData *astjson.Value) error {
	if len(items) == 0 {
		if responseData.Type() != astjson.TypeObject {
			return l.renderErrorsFailedToFetch(fetchItem, res, invalidGraphQLResponseShape)
		}
		l.resolvable.data = responseData
		return nil
	}
	if len(items) == 1 && res.batchStats == nil {
		var err error
		items[0], _, err = astjson.MergeValuesWithPath(l.jsonArena, items[0], responseData, res.postProcessing.MergePath...)
		if err != nil {
			return errors.WithStack(ErrMergeResult{
				Subgraph: res.ds.Name,
				Reason:   err,
				Path:     fetchItem.ResponsePath,
			})
		}
		return nil
	}

	batch := responseData.GetArray()
	if batch == nil {
		return l.renderErrorsFailedToFetch(fetchItem, res, invalidGraphQLResponseShape)
	}

	if res.batchStats != nil {
		if len(res.batchStats) != len(batch) {
			return l.renderErrorsFailedToFetch(fetchItem, res, fmt.Sprintf(invalidBatchItemCount, len(res.batchStats), len(batch)))
		}
		for batchIndex, targets := range res.batchStats {
			src := batch[batchIndex]
			for _, target := range targets {
				_, _, err := astjson.MergeValuesWithPath(l.jsonArena, target, src, res.postProcessing.MergePath...)
				if err != nil {
					return errors.WithStack(ErrMergeResult{
						Subgraph: res.ds.Name,
						Reason:   err,
						Path:     fetchItem.ResponsePath,
					})
				}
			}
		}
		return nil
	}

	if batchCount, itemCount := len(batch), len(items); batchCount != itemCount {
		return l.renderErrorsFailedToFetch(fetchItem, res, fmt.Sprintf(invalidBatchItemCount, itemCount, batchCount))
	}
	for i := range items {
		var err error
		items[i], _, err = astjson.MergeValuesWithPath(l.jsonArena, items[i], batch[i], res.postProcessing.MergePath...)
		if err != nil {
			return errors.WithStack(ErrMergeResult{
				Subgraph: res.ds.Name,
				Reason:   err,
				Path:     fetchItem.ResponsePath,
			})
		}
	}
	return nil
}

func bytecodeFetchItem(program *planbytecode.Program, ref uint32) (*FetchItem, error) {
	if int(ref) >= len(program.Fetches) {
		return nil, fmt.Errorf("bytecode fetch ref %d out of range", ref)
	}
	item, ok := program.Fetches[ref].Item.(*FetchItem)
	if !ok || item == nil {
		return nil, fmt.Errorf("bytecode fetch ref %d does not contain *resolve.FetchItem", ref)
	}
	if item.Fetch == nil {
		return nil, fmt.Errorf("bytecode fetch ref %d has nil fetch", ref)
	}
	return item, nil
}

func skipBytecodePaste(ops []planbytecode.Op, pc int) int {
	if pc < len(ops) && ops[pc].Code == planbytecode.OpPasteAtPointer {
		return pc + 1
	}
	return pc
}

func bytecodeGroupLeavePC(program *planbytecode.Program, pc int, leave planbytecode.Opcode) (int, error) {
	if pc >= len(program.Ops) {
		return pc, fmt.Errorf("bytecode group pc %d out of range", pc)
	}
	if cached := int(program.Ops[pc].B); cached > pc && cached < len(program.Ops) && program.Ops[cached].Code == leave {
		return cached, nil
	}
	return scanBytecodeGroupLeave(program.Ops, pc, leave)
}

func scanBytecodeGroupLeave(ops []planbytecode.Op, pc int, leave planbytecode.Opcode) (int, error) {
	depth := 1
	for cursor := pc + 1; cursor < len(ops); cursor++ {
		switch ops[cursor].Code {
		case planbytecode.OpEnterSequence, planbytecode.OpEnterParallel:
			depth++
		case planbytecode.OpLeaveSequence, planbytecode.OpLeaveParallel:
			depth--
			if depth == 0 {
				if ops[cursor].Code != leave {
					return cursor, fmt.Errorf("bytecode group starting at pc %d closed with %s, expected %s", pc, ops[cursor].Code, leave)
				}
				return cursor, nil
			}
		}
	}
	return pc, fmt.Errorf("bytecode group starting at pc %d missing %s", pc, leave)
}

func skipBytecodeChild(ops []planbytecode.Op, pc int) (int, error) {
	if pc >= len(ops) {
		return pc, fmt.Errorf("skip bytecode child: pc %d out of range", pc)
	}
	switch ops[pc].Code {
	case planbytecode.OpFetchSubgraph:
		return skipBytecodePaste(ops, pc+1), nil
	case planbytecode.OpEnterSequence:
		return skipBytecodeGroup(ops, pc+1, planbytecode.OpEnterSequence, planbytecode.OpLeaveSequence)
	case planbytecode.OpEnterParallel:
		return skipBytecodeGroup(ops, pc+1, planbytecode.OpEnterParallel, planbytecode.OpLeaveParallel)
	default:
		return pc + 1, nil
	}
}

func skipBytecodeGroup(ops []planbytecode.Op, pc int, enter planbytecode.Opcode, leave planbytecode.Opcode) (int, error) {
	depth := 1
	for pc < len(ops) {
		switch ops[pc].Code {
		case enter:
			depth++
		case leave:
			depth--
			if depth == 0 {
				return pc + 1, nil
			}
		}
		pc++
	}
	return pc, fmt.Errorf("skip bytecode group: missing %s", leave)
}

func bytecodeParallelDirectFetches(ops []planbytecode.Op, pc int, leavePC int, children int) bool {
	cursor := pc
	for i := 0; i < children; i++ {
		if cursor >= leavePC || cursor >= len(ops) || ops[cursor].Code != planbytecode.OpFetchSubgraph {
			return false
		}
		cursor = skipBytecodePaste(ops, cursor+1)
	}
	return cursor == leavePC
}

func bytecodeResponseOpcode(code planbytecode.Opcode) bool {
	switch code {
	case planbytecode.OpEnterObject,
		planbytecode.OpLeaveObject,
		planbytecode.OpEnterArray,
		planbytecode.OpLeaveArray,
		planbytecode.OpProjectField,
		planbytecode.OpEmitLiteral,
		planbytecode.OpEmitResponse:
		return true
	default:
		return false
	}
}
