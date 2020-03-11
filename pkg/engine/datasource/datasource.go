// Package datasource defines the interface of how to resolve data during the execution from various data sources
package datasource

import (
	"context"
)

type StaticDataSource struct {
	Data []byte
}

func (s StaticDataSource) Resolve(ctx context.Context, config, input []byte) (output []byte, err error) {
	return s.Data, nil
}
