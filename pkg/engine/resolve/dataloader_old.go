package resolve

//type Dataloader struct {
//	rootResponseNode *responseNode
//}
//
//// @TODO do not handle case when some fetch return errors and data
//
//func (d *Dataloader) Load(ctx *Context, fetch *BatchFetch, objectPath []string) (err error) {
//
//	return
//}
//
//func (d *Dataloader) LoadBatch(ctx *Context, batchFetch *BatchFetch, objectPath []string) (response []byte, err error) {
//	currentPath := string(ctx.path())
//
//	val, ok := d.rootResponseNode.get(currentPath)
//	if ok { // the batch has already been resolved
//		return val.value, nil
//	}
//
//	// it's required to build and batchFetch batch
//	firstParent := d.rootResponseNode.getClosestParent(currentPath)
//
//	var parents []*responseNode
//	parents = append(parents, firstParent)
//	nextParent := firstParent
//
//	for {
//		if nextParent.nextSibling == nil {
//			break
//		}
//
//		parents = append(parents, nextParent.nextSibling)
//		nextParent = nextParent.nextSibling
//	}
//
//	inputs := make([][]byte, 0, len(parents))
//
//	buf := fastbuffer.New()
//
//	for i := range parents {
//		val, _, _, err := jsonparser.Get(parents[i].value, selectionObj.objectPath...)
//		if err != nil {
//			return nil, err
//		}
//
//		if err := batchFetch.Fetch.InputTemplate.Render(ctx, val, buf); err != nil {
//			return nil, err
//		}
//
//		inputs = append(inputs, buf.Bytes())
//		buf.Reset()
//	}
//
//	batchResponse, err := d.resolveBatchFetch(ctx, batchFetch, inputs...)
//	if err != nil {
//		return nil, err
//	}
//
//	childSuffix := strings.TrimLeft(currentPath, firstParent.key)
//	var prevNode *responseNode
//
//	for i, parent := range parents {
//		if !selectionObj.isArray {
//			childKey := parent.key + childSuffix
//			node := newResponseNode(childKey, batchResponse[i])
//			if prevNode != nil {
//				prevNode.nextSibling = node
//			}
//			prevNode = node
//			parent.add(node)
//		}
//	}
//
//	return nil, nil
//}
//
//func (d *Dataloader) flattenChildren(parentData []byte, childrenPath string) {
//
//}
//
//func (d *Dataloader) resolveBatchFetch(ctx *Context, batchFetch *BatchFetch, inputs ...[]byte) (result [][]byte, err error) {
//	batchInput, err := batchFetch.PrepareBatch(inputs...)
//	if err != nil {
//		return nil, err
//	}
//
//	fmt.Println("batch request", string(batchInput.Input))
//
//	if ctx.beforeFetchHook != nil {
//		ctx.beforeFetchHook.OnBeforeFetch(d.hookCtx(ctx), batchInput.Input)
//	}
//
//	batchBufferPair := &BufPair{
//		Data:   fastbuffer.New(),
//		Errors: fastbuffer.New(),
//	}
//
//	if err = batchFetch.Fetch.DataSource.Load(ctx.Context, batchInput.Input, batchBufferPair); err != nil {
//		return nil, err
//	}
//
//	if ctx.afterFetchHook != nil {
//		if batchBufferPair.HasData() {
//			ctx.afterFetchHook.OnData(d.hookCtx(ctx), batchBufferPair.Data.Bytes(), false)
//		}
//		if batchBufferPair.HasErrors() {
//			ctx.afterFetchHook.OnError(d.hookCtx(ctx), batchBufferPair.Errors.Bytes(), false)
//		}
//	}
//
//	var outPosition int
//	result = make([][]byte, 0, len(inputs))
//
//	_, err = jsonparser.ArrayEach(batchBufferPair.Data.Bytes(), func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
//		inputPositions := batchInput.OutToInPositions[outPosition]
//
//		for _, pos := range inputPositions {
//			result[pos] = value
//		}
//
//		outPosition++
//	})
//	if err != nil {
//		return nil, err
//	}
//
//	return result, nil
//}
//
//func (d *Dataloader) hookCtx(ctx *Context) HookContext {
//	return HookContext{
//		CurrentPath: ctx.path(),
//	}
//}
//
//func newResponseNode(key string, value []byte) *responseNode {
//	return &responseNode{
//		key:      key,
//		value:    value,
//		children: make(map[string]*responseNode),
//		mux:      &sync.Mutex{},
//	}
//}
//
//type responseNode struct {
//	key         string
//	value       []byte
//	parent      *responseNode
//	nextSibling *responseNode
//	children    map[string]*responseNode
//
//	mux *sync.Mutex
//}
//
//func (r *responseNode) add(other *responseNode) (*responseNode, bool) {
//	parentNode := r.getClosestParent(other.key)
//
//	parentNode.mux.Lock()
//
//	if _, ok := parentNode.children[other.key]; ok {
//		fmt.Printf("Node %q has already existed\n", other.key)
//	}
//
//	parentNode.children[other.key] = other
//	other.parent = parentNode
//
//	parentNode.mux.Unlock()
//
//	return other, true
//}
//
//func (r *responseNode) getClosestParent(key string) *responseNode {
//	parent := r
//	parts := strings.Split(key, "/")
//	possibleParentPath := parts[0 : len(parts)-1]
//
//	for i := range possibleParentPath {
//		if i == 0 {
//			continue
//		}
//
//		node, ok := r.get(strings.Join(possibleParentPath[0:i], "/"))
//		if !ok {
//			return parent
//		}
//
//		parent = node
//	}
//
//	return parent
//}
//
//func (r *responseNode) get(key string) (*responseNode, bool) {
//	if key == r.key {
//		return r, true
//	}
//
//	if !strings.HasPrefix(key, r.key) {
//		return nil, false
//	}
//
//	for childKey, childVal := range r.children {
//		if childKey == key {
//			return childVal, true
//		}
//
//		if childVal.hasChild(key) {
//			return childVal.get(key)
//		}
//	}
//
//	return nil, false
//}
//
//func (r *responseNode) hasChild(childKey string) bool {
//	return strings.HasPrefix(childKey, r.key)
//}

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
