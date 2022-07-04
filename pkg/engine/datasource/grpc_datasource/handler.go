package grpc_datasource

import (
	"fmt"
	"io"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/jhump/protoreflect/desc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type handler struct {
	method            *desc.MethodDescriptor
	methodCount       int
	reqHeaders        metadata.MD
	reqHeadersCount   int
	reqMessages       []string
	reqMessagesCount  int
	respHeaders       metadata.MD
	respHeadersCount  int
	respMessages      []string
	respTrailers      metadata.MD
	respStatus        *status.Status
	respTrailersCount int
}

func (h *handler) getRequestData() ([]byte, error) {
	// we don't use a mutex, though this method will be called from different goroutine
	// than other methods for bidi calls, because this method does not share any state
	// with the other methods.
	h.reqMessagesCount++
	if h.reqMessagesCount > len(h.reqMessages) {
		return nil, io.EOF
	}
	if h.reqMessagesCount > 1 {
		// insert delay between messages in request stream
		time.Sleep(time.Millisecond * 50)
	}
	return []byte(h.reqMessages[h.reqMessagesCount-1]), nil
}

func (h *handler) OnResolveMethod(md *desc.MethodDescriptor) {
	h.methodCount++
	h.method = md
}

func (h *handler) OnSendHeaders(md metadata.MD) {
	h.reqHeadersCount++
	h.reqHeaders = md
}

func (h *handler) OnReceiveHeaders(md metadata.MD) {
	h.respHeadersCount++
	h.respHeaders = md
}

func (h *handler) OnReceiveResponse(msg proto.Message) {
	jsm := jsonpb.Marshaler{Indent: "  "}
	respStr, err := jsm.MarshalToString(msg)
	if err != nil {
		panic(fmt.Errorf("failed to generate JSON form of response message: %v", err))
	}
	h.respMessages = append(h.respMessages, respStr)

	fmt.Println(respStr)
}

func (h *handler) OnReceiveTrailers(stat *status.Status, md metadata.MD) {
	h.respTrailersCount++
	h.respTrailers = md
	h.respStatus = stat
}
