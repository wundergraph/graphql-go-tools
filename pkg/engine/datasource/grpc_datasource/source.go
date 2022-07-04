package grpc_datasource

import (
	"bytes"
	"context"
	"io"
	"log"
	"time"

	"github.com/fullstorydev/grpcurl"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	reflectpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

var (
	null = []byte("null")
)

type Source struct {
}

func (s *Source) Load(ctx context.Context, input []byte, w io.Writer) (err error) {

	// return json.NewEncoder(w).Encode(s.introspectionData.Schema)

	sourceProtoFiles, err := grpcurl.DescriptorSourceFromProtoFiles([]string{"helloworld"}, "helloworld.proto")
	if err != nil {
		log.Fatal(err)
	}

	// And a corresponding client
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ccNoReflect, err := grpc.DialContext(ctx, "localhost:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		log.Fatal(err)
	}
	defer ccNoReflect.Close()

	refClient := grpcreflect.NewClient(context.Background(), reflectpb.NewServerReflectionClient(ccNoReflect))
	defer refClient.Reset()

	cc := ccNoReflect
	source := sourceProtoFiles

	h := &handler{reqMessages: []string{`{
"name":"Jens"
}`}}

	err = grpcurl.InvokeRPC(context.Background(), source, cc, "helloworld.Greeter/SayHello", []string{}, h, func(m proto.Message) error {
		// New function is almost identical, but the request supplier function works differently.
		// So we adapt the logic here to maintain compatibility.
		data, err := h.getRequestData()
		if err != nil {
			return err
		}
		return jsonpb.Unmarshal(bytes.NewReader(data), m)
	})
	if err != nil {
		log.Fatal(err)
	}

	return nil
}
