package resolve

import (
	"slices"
	"strconv"

	"github.com/wundergraph/astjson"
)

// A GraphQL error identifies a field in one response, before its paths are
// rewritten for the client. For an entity response, index refers to an outgoing
// representation, which can map to several merge targets.
type contextResponseError struct {
	index int
	path  []string
}

func contextResponseErrors(item *FetchItem, res *result, data, errs *astjson.Value) (out []contextResponseError) {
	entity := res.multi != nil || item.Fetch.FetchKind() != FetchKindSingle
	for _, e := range errs.GetArray() {
		path := e.GetArray("path")
		if len(path) == 0 {
			continue
		}
		index := 0
		source := data
		if entity {
			if len(path) < 2 || string(path[0].GetStringBytes()) != res.errorPathRoot() {
				continue
			}
			var err error
			index, err = path[1].Int()
			if err != nil || index < 0 {
				continue
			}
			path = path[2:]
			if res.multi != nil || item.Fetch.FetchKind() == FetchKindEntityBatch {
				batch := data.GetArray()
				if index >= len(batch) {
					continue
				}
				source = batch[index]
			} else if index != 0 {
				continue
			}
		} else {
			path = relativeRootErrorPath(res.postProcessing.SelectResponseDataPath, path)
			if path == nil {
				continue
			}
		}
		if fields, ok := contextErrorPath(source, path, res.postProcessing.MergePath); ok {
			out = append(out, contextResponseError{index: index, path: fields})
		}
	}
	return out
}

// GraphQL error paths are relative to "data". A root fetch or subscription
// trigger can select only a subtree of that data before merging it.
func relativeRootErrorPath(dataPath []string, path []*astjson.Value) []*astjson.Value {
	if i := slices.Index(dataPath, "data"); i >= 0 {
		for _, key := range dataPath[i+1:] {
			if len(path) == 0 || string(path[0].GetStringBytes()) != key {
				return nil
			}
			path = path[1:]
		}
	}
	return path
}

func contextErrorPath(source *astjson.Value, path []*astjson.Value, mergePath []string) ([]string, bool) {
	fields := make([]string, len(mergePath), len(mergePath)+len(path))
	copy(fields, mergePath)
	for _, step := range path {
		if astjson.ValueIsNull(source) {
			// The error may already have bubbled to an entire representation.
			break
		}
		var key string
		switch step.Type() {
		case astjson.TypeString:
			key = string(step.GetStringBytes())
		case astjson.TypeNumber:
			index, err := step.Int()
			if err != nil || index < 0 {
				return nil, false
			}
			key = strconv.Itoa(index)
		default:
			return nil, false
		}
		fields = append(fields, key)
		source = source.Get(key)
	}
	return fields, true
}

// Bind the captured paths to the merged document. A source container can be
// shared with the subgraph response, merged into an existing container, or
// copied into several independent targets for a deduplicated representation.
func (l *Loader) recordContextFieldErrors(fetchID int, res *result, items []*astjson.Value, errs []contextResponseError) {
	for _, e := range errs {
		var targets []*astjson.Value
		switch {
		case res.batchStats != nil:
			if e.index < len(res.batchStats) {
				targets = res.batchStats[e.index]
			}
		case len(items) == 0:
			targets = []*astjson.Value{l.dataBuffer.Get()}
		case e.index < len(items):
			targets = items[e.index : e.index+1]
		}
		if len(e.path) == 0 {
			// An error on the representation itself has no finer location.
			l.recordContextTargetsLocked(fetchID, targets)
			continue
		}
		for _, target := range targets {
			l.dataBuffer.markContextFieldError(target, e.path)
		}
	}
}

type contextField struct {
	parent *astjson.Value
	// Empty for a failed list element. A projection of a list reads every index.
	name string
}

func (db *DataBuffer) markContextFieldError(parent *astjson.Value, path []string) {
	var enclosingLists []*astjson.Value
	for i, key := range path {
		if astjson.ValueIsNull(parent) {
			return
		}
		if parent.Type() == astjson.TypeArray {
			enclosingLists = append(enclosingLists, parent)
		} else {
			enclosingLists = enclosingLists[:0]
		}
		if i == len(path)-1 {
			if db.failedContextFields == nil {
				db.failedContextFields = make(map[contextField]struct{})
			}
			if len(enclosingLists) == 0 {
				db.failedContextFields[contextField{parent: parent, name: key}] = struct{}{}
			}
			// Propagate an element error out through consecutive list wrappers
			// so reading a scalar-list argument remains a constant-time lookup.
			for _, list := range enclosingLists {
				db.failedContextFields[contextField{parent: list}] = struct{}{}
			}
			return
		}
		parent = parent.Get(key)
	}
}

// Subscription event data arrives already merged by InitSubscription, outside
// Loader.mergeResult. The original event errors retain their GraphQL paths.
func (db *DataBuffer) recordSubscriptionContextErrors(fetches *FetchTreeNode, post PostProcessingConfiguration, errs *astjson.Value) {
	if len(errs.GetArray()) == 0 {
		return
	}
	db.fetchTreeToCheck = fetches
	if !db.hasContextWrites() {
		return
	}
	source := db.Get().Get(post.MergePath...)
	for _, e := range errs.GetArray() {
		path := relativeRootErrorPath(post.SelectResponseDataPath, e.GetArray("path"))
		if path == nil {
			continue
		}
		if fields, ok := contextErrorPath(source, path, post.MergePath); ok {
			db.markContextFieldError(db.Get(), fields)
		}
	}
}
