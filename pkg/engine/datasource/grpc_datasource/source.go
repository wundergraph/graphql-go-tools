package grpc_datasource

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/buger/jsonparser"
	"github.com/fullstorydev/grpcurl"
	"github.com/golang/protobuf/jsonpb" //lint:ignore SA1019 old package is required cause grpcurl uses it in interfaces
	"github.com/golang/protobuf/proto"  //lint:ignore SA1019 old package is required cause grpcurl uses it in interfaces
	"github.com/jhump/protoreflect/desc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/wundergraph/graphql-go-tools/pkg/engine/datasource/httpclient"
)

type Source struct {
	config           EndpointConfiguration
	descriptorSource grpcurl.DescriptorSource
	connection       *grpc.ClientConn
}

func (s *Source) Load(ctx context.Context, input []byte, w io.Writer) (err error) {
	_, _, body, header, _ := httpclient.RequestInputParams(input)

	methodName := s.config.RpcMethodFullName()
	headers, err := RpcHeaders(header)
	if err != nil {
		return err
	}

	h := &handler{
		w:    w,
		body: body,
	}

	err = grpcurl.InvokeRPC(ctx, s.descriptorSource, s.connection, methodName, headers, h, h.supplyRequest)
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

type handler struct {
	w              io.Writer
	err            error
	body           []byte
	isBodySupplied bool
}

func (h *handler) supplyRequest(m proto.Message) error {
	if h.isBodySupplied {
		return io.EOF
	}

	err := jsonpb.Unmarshal(bytes.NewReader(h.body), m)
	if err != nil {
		return err
	}

	h.isBodySupplied = true
	return nil
}

func (h *handler) OnReceiveResponse(msg proto.Message) {
	jsm := jsonpb.Marshaler{}
	h.err = jsm.Marshal(h.w, msg)
}

func (h *handler) OnResolveMethod(md *desc.MethodDescriptor)             {}
func (h *handler) OnSendHeaders(md metadata.MD)                          {}
func (h *handler) OnReceiveHeaders(md metadata.MD)                       {}
func (h *handler) OnReceiveTrailers(stat *status.Status, md metadata.MD) {}
