package resolve

import "github.com/wundergraph/astjson"

type pendingRequestScopedInjection struct {
	field RequestScopedField
	value *astjson.Value
}

func (l *Loader) tryRequestScopedInjectionAndTrace(fetch Fetch, items []*astjson.Value, res *result) bool {
	if !l.tryRequestScopedInjection(fetchCacheConfiguration(fetch), items, res) {
		return false
	}
	ensureFetchTrace(fetch).LoadSkipped = true
	if res != nil {
		res.cacheTraceRequestScopedHits = requestScopedHitCount(items)
	}
	return true
}

func (l *Loader) tryRequestScopedInjection(cache *FetchCacheConfiguration, items []*astjson.Value, res *result) bool {
	if l.ctx == nil || !l.ctx.ExecutionOptions.Caching.EnableL1Cache {
		return false
	}
	if cache == nil || len(cache.RequestScopedFields) == 0 || len(items) == 0 || len(l.requestScopedL1) == 0 {
		return false
	}

	pending := make([]pendingRequestScopedInjection, 0, len(cache.RequestScopedFields))
	for _, field := range cache.RequestScopedFields {
		if field.L1Key == "" || field.ProvidesData == nil {
			return false
		}
		cached := l.requestScopedL1[field.L1Key]
		if !astjson.ValueIsNonNull(cached) {
			return false
		}
		if !validateItemHasRequiredData(cached, field.ProvidesData) {
			return false
		}
		pending = append(pending, pendingRequestScopedInjection{
			field: field,
			value: l.structuralCopyDenormalized(cached, field.ProvidesData),
		})
	}

	for _, item := range items {
		if item == nil || item.Type() == astjson.TypeNull {
			continue
		}
		for _, injection := range pending {
			value := injection.value
			if len(items) > 1 {
				value = astjson.StructuralCopy(l.jsonArena, value)
			}
			astjson.SetValue(l.jsonArena, item, value, requestScopedInjectionPath(injection.field)...)
		}
	}
	if res != nil {
		res.fetchSkipped = true
	}
	return true
}

func (l *Loader) exportRequestScopedFields(cache *FetchCacheConfiguration, items []*astjson.Value) {
	if l.ctx == nil || !l.ctx.ExecutionOptions.Caching.EnableL1Cache {
		return
	}
	if cache == nil || len(cache.RequestScopedFields) == 0 {
		return
	}
	if l.requestScopedL1 == nil {
		l.requestScopedL1 = make(map[string]*astjson.Value, len(cache.RequestScopedFields))
	}

	sources := items
	if len(sources) == 0 && l.resolvable != nil && l.resolvable.data != nil {
		sources = []*astjson.Value{l.resolvable.data}
	}
	if len(sources) == 0 {
		return
	}

	for _, field := range cache.RequestScopedFields {
		if field.L1Key == "" || field.ProvidesData == nil {
			continue
		}
		value := firstRequestScopedFieldValue(sources, field)
		if !astjson.ValueIsNonNull(value) {
			continue
		}
		normalized := l.structuralCopyNormalized(value, field.ProvidesData)
		existing := l.requestScopedL1[field.L1Key]
		if existing == nil {
			l.requestScopedL1[field.L1Key] = normalized
			continue
		}
		working := astjson.StructuralCopy(l.jsonArena, existing)
		merged, err := astjson.MergeValues(l.jsonArena, working, normalized)
		if err != nil {
			continue
		}
		l.requestScopedL1[field.L1Key] = merged
	}
}

func firstRequestScopedFieldValue(items []*astjson.Value, field RequestScopedField) *astjson.Value {
	path := requestScopedInjectionPath(field)
	for _, item := range items {
		if item == nil || item.Type() == astjson.TypeNull {
			continue
		}
		value := item.Get(path...)
		if astjson.ValueIsNonNull(value) {
			return value
		}
	}
	return nil
}

func requestScopedInjectionPath(field RequestScopedField) []string {
	if len(field.FieldPath) != 0 {
		return field.FieldPath
	}
	if field.FieldName != "" {
		return []string{field.FieldName}
	}
	return nil
}

func validateItemHasRequiredData(value *astjson.Value, provides *Object) bool {
	if provides == nil {
		return false
	}
	return cachedValueContainsProvides(value, provides)
}

func requestScopedHitCount(items []*astjson.Value) int {
	if len(items) == 0 {
		return 1
	}
	return len(items)
}

func ensureFetchTrace(fetch Fetch) *DataSourceLoadTrace {
	switch f := fetch.(type) {
	case *SingleFetch:
		if f.Trace == nil {
			f.Trace = &DataSourceLoadTrace{}
		}
		return f.Trace
	case *EntityFetch:
		if f.Trace == nil {
			f.Trace = &DataSourceLoadTrace{}
		}
		return f.Trace
	case *BatchEntityFetch:
		if f.Trace == nil {
			f.Trace = &DataSourceLoadTrace{}
		}
		return f.Trace
	default:
		return &DataSourceLoadTrace{}
	}
}
