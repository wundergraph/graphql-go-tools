package grpc_datasource

import (
	"io"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/jhump/protoreflect/desc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type handler struct {
	w    io.Writer
	err  error
	body []byte
}

func (h *handler) OnReceiveResponse(msg proto.Message) {
	jsm := jsonpb.Marshaler{}
	h.err = jsm.Marshal(h.w, msg)
}

func (h *handler) OnResolveMethod(md *desc.MethodDescriptor) {
}

func (h *handler) OnSendHeaders(md metadata.MD) {
}

func (h *handler) OnReceiveHeaders(md metadata.MD) {

}

func (h *handler) OnReceiveTrailers(stat *status.Status, md metadata.MD) {
}
