package grpc_datasource

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/buger/jsonparser"
	"github.com/fullstorydev/grpcurl"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type Source struct {
	descriptorSource     grpcurl.DescriptorSource
	transportCredentials credentials.TransportCredentials
	target               string
}

func (s *Source) Load(ctx context.Context, input []byte, w io.Writer) (err error) {
	pkgName, service, method, body, header, target := RpcCallParams(input)

	dialCtx, err := grpc.DialContext(ctx, string(target),
		grpc.WithTransportCredentials(s.transportCredentials), grpc.WithBlock())
	if err != nil {
		return err
	}
	defer func() { _ = dialCtx.Close() }()

	methodName := RpcMethodFullName(pkgName, service, method)
	headers, err := RpcHeaders(header)
	if err != nil {
		return err
	}

	h := &handler{w: w}

	err = grpcurl.InvokeRPC(context.Background(), s.descriptorSource, dialCtx, methodName, headers, h, func(m proto.Message) error {
		return jsonpb.Unmarshal(bytes.NewReader(body), m)
	})
	if err != nil {
		return err
	}

	if h.err != nil {
		return h.err
	}

	return nil
}

func RpcHeaders(header []byte) (out []string, err error) {
	out = make([]string, 0, 2)
	err = jsonparser.ObjectEach(header, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		_, err := jsonparser.ArrayEach(value, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
			if err != nil {
				return
			}
			if len(value) == 0 {
				return
			}
			out = append(out, fmt.Sprintf("%s:%s", string(key), string(value)))
		})
		return err
	})
	if err != nil {
		return nil, err
	}

	return
}
