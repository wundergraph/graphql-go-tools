package grpcdatasource

import (
	"fmt"

	"buf.build/go/hyperpb"
	"google.golang.org/grpc/encoding"
	"google.golang.org/protobuf/proto"
)

type v2HyperpbCodec struct{}

var _ encoding.Codec = v2HyperpbCodec{}

func (v2HyperpbCodec) Name() string { return "proto" }

func (v2HyperpbCodec) Marshal(v any) ([]byte, error) {
	if msg, ok := v.(*v2PreMarshaledInput); ok {
		return msg.wire, nil
	}
	msg, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("grpcdatasource v2HyperpbCodec: expected proto.Message, got %T", v)
	}
	return proto.Marshal(msg)
}

func (v2HyperpbCodec) Unmarshal(data []byte, v any) error {
	if msg, ok := v.(*hyperpb.Message); ok {
		return msg.Unmarshal(data)
	}
	protoMsg, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("grpcdatasource v2HyperpbCodec: expected proto.Message or *hyperpb.Message, got %T", v)
	}
	return proto.Unmarshal(data, protoMsg)
}
