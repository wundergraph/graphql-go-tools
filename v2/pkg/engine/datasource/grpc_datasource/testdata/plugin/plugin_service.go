package main

import (
	"context"

	"github.com/hashicorp/go-plugin"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource/testdata"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource/testdata/productv1"
	"google.golang.org/grpc"
)

var handshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "GRPC_DATASOURCE_PLUGIN",
	MagicCookieValue: "Foobar",
}

type GRPCDataSourcePlugin struct {
	plugin.Plugin
}

func (p *GRPCDataSourcePlugin) GRPCServer(broker *plugin.GRPCBroker, server *grpc.Server) error {
	productv1.RegisterProductServiceServer(server, &testdata.MockService{})
	return nil
}

func (p *GRPCDataSourcePlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return nil, nil
}

func main() {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: handshakeConfig,
		GRPCServer:      plugin.DefaultGRPCServer,
		Plugins: map[string]plugin.Plugin{
			"grpc_datasource": &GRPCDataSourcePlugin{},
		},
	})
}
