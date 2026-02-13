package transport

import (
	"context"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/common"
)

type Transport interface {
	Subscribe(ctx context.Context, req *common.Request, opts common.Options) (results <-chan *common.Message, cancel func(), err error)
	Close() error
}
