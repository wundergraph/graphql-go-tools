package ast

import "sync"

type documentPool struct {
	p sync.Pool
}

func newDocumentPool() *documentPool {
	return &documentPool{
		p: sync.Pool{
			New: func() interface{} {
				return newDocumentWithPreAllocation()
			},
		},
	}
}

func (p *documentPool) Put(b *Document) {
	b.Reset()
	p.p.Put(b)
}

func (p *documentPool) Get() *Document {
	return p.p.Get().(*Document)
}
