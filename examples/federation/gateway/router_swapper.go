package main


import (
	"net/http"
	"sync"
)

func NewRouterSwapper() *RouterSwapper {
	return &RouterSwapper{
		router: http.NewServeMux(),
		mu:     &sync.Mutex{},
	}
}

type RouterSwapper struct {
	router *http.ServeMux
	mu     *sync.Mutex
	cb     []func(router *http.ServeMux)
}

func (rs *RouterSwapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rs.mu.Lock()
	router := rs.router
	rs.mu.Unlock()

	router.ServeHTTP(w, r)
}

func (rs *RouterSwapper) Swap() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	router := http.NewServeMux()
	rs.router = router
	for _, cb := range rs.cb {
		cb(router)
	}
}

func (rs *RouterSwapper) Register(cb func(router *http.ServeMux)) {
	rs.cb = append(rs.cb, cb)
}
