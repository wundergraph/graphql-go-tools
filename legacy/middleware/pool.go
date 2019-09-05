package middleware

type InvokerPool struct {
	index    chan int
	invokers []*Invoker
}

func NewInvokerPool(size int, middleWares ...GraphqlMiddleware) *InvokerPool {
	pool := &InvokerPool{}
	pool.index = make(chan int, size)
	pool.invokers = make([]*Invoker, size)
	for i := 0; i < size; i++ {
		pool.index <- i
		pool.invokers[i] = NewInvoker(middleWares...)
	}

	return pool
}

func (i *InvokerPool) Get() (index int, invoker *Invoker) {
	index = <-i.index
	invoker = i.invokers[index]
	return
}

func (i *InvokerPool) Free(index int) {
	i.index <- index
}
