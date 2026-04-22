package grpcdatasource

import (
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/encoding/proto"
	_ "google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/mem"
)

var defaultCodec = encoding.GetCodecV2("proto")

type connectCodec struct{}

// Name implements [encoding.CodecV2].
func (c *connectCodec) Name() string {
	// we use the default proto codec to allow marshalling our own message but not
	// interfere with the default proto codec for servers to unmarshal it.
	return proto.Name
}

// Marshal implements [encoding.CodecV2].
func (c *connectCodec) Marshal(v any) (out mem.BufferSlice, err error) {
	switch v := v.(type) {
	case *PreWiredInputMessage:
		protoBytes, err := v.wire()
		if err != nil {
			return nil, err
		}

		if mem.IsBelowBufferPoolingThreshold(v.size) {
			out = append(out, mem.SliceBuffer(protoBytes))
			return out, nil
		} else {
			pool := mem.DefaultBufferPool()
			buf := pool.Get(v.size)

			copy(*buf, protoBytes)

			out = append(out, mem.NewBuffer(buf, pool))
			return out, nil
		}
	}

	return defaultCodec.Marshal(v)
}

// Unmarshal implements [encoding.CodecV2].
// TODO: Unmarshal to astjson
func (c *connectCodec) Unmarshal(data mem.BufferSlice, v any) error {
	return defaultCodec.Unmarshal(data, v)
}

var _ encoding.CodecV2 = (*connectCodec)(nil)
