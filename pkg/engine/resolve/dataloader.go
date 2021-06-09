package resolve

import (
	"fmt"
	"strings"
	"sync"

	"github.com/buger/jsonparser"

	"github.com/jensneuse/graphql-go-tools/pkg/fastbuffer"
)

type Dataloader struct {
}

func (d *Dataloader) Load(ctx *Context, fetch *BatchFetch, objectPath []string) (err error) {

	return
}

func newResponseNode(rootPath string, value []byte) *responseNode {
	return &responseNode{
		key:      rootPath,
		value:    value,
		children: make(map[string]*responseNode),
		mux:      &sync.Mutex{},
	}
}

type responseNode struct {
	key         string
	value       []byte
	parent      *responseNode
	nextSibling *responseNode
	children    map[string]*responseNode

	mux *sync.Mutex
}

type selectionOptions struct {
	objectPath []string
	isArray bool
	arrayPath []string
}

func (r *responseNode) fetch(ctx *Context, fetch *BatchFetch, selectionObj selectionOptions) (response []byte, err error) {
	currentPath := string(ctx.path())

	val, ok := r.get(currentPath)
	if ok { // the batch has already been resolved
		return val.value, nil
	}

	// it's required to build and fetch batch
	firstParent := r.getClosestParent(currentPath)

	var parents []*responseNode
	parents = append(parents, firstParent)
	nextParent := firstParent

	for {
		if nextParent.nextSibling == nil {
			break
		}

		parents = append(parents, nextParent.nextSibling)
		nextParent = nextParent.nextSibling
	}

	inputs := make([][]byte, 0, len(parents))

	buf := fastbuffer.New()

	for i := range parents {
		val, _,_, err := jsonparser.Get(parents[i].value, selectionObj.objectPath...)
		if err != nil {
			return nil, err
		}


		if err := fetch.Fetch.InputTemplate.Render(ctx, val, buf); err != nil {
			return nil, err
		}

		inputs = append(inputs, buf.Bytes())
		buf.Reset()
	}

	batchInput, err := fetch.PrepareBatch(inputs...)
	if err != nil {
		return nil, err
	}



	return nil, nil
}

func (r *responseNode) add(other *responseNode) (*responseNode, bool) {
	parentNode := r.getClosestParent(other.key)

	parentNode.mux.Lock()

	if _, ok := parentNode.children[other.key]; ok {
		fmt.Printf("Node %q has already existed\n", other.key)
	}

	parentNode.children[other.key] = other
	other.parent = parentNode

	parentNode.mux.Unlock()

	return other, true
}

func (r *responseNode) getClosestParent(key string) *responseNode {
	parent := r
	parts := strings.Split(key, "/")
	possibleParentPath := parts[0 : len(parts)-1]

	for i := range possibleParentPath {
		if i == 0 {
			continue
		}

		node, ok := r.get(strings.Join(possibleParentPath[0:i], "/"))
		if !ok {
			return parent
		}

		parent = node
	}

	return parent
}

func (r *responseNode) get(key string) (*responseNode, bool) {
	if key == r.key {
		return r, true
	}

	if !strings.HasPrefix(key, r.key) {
		return nil, false
	}

	for childKey, childVal := range r.children {
		if childKey == key {
			return childVal, true
		}

		if childVal.hasChild(key) {
			return childVal.get(key)
		}
	}

	return nil, false
}

func (r *responseNode) hasChild(childKey string) bool {
	return strings.HasPrefix(childKey, r.key)
}

func (r *responseNode) hookCtx(ctx *Context) HookContext {
	return HookContext{
		CurrentPath: ctx.path(),
	}
}

//func (r *responseNode) first(key string) (*responseNode, bool) {
//	childrenForKey, ok := r.children[key]
//	if !ok {
//		return nil, false
//	}
//
//	if len(childrenForKey) == 0 {
//		return nil, false
//	}
//
//	return childrenForKey[0], true
//}
