package grpc_datasource

import (
	"bytes"
	"context"
	"log"
	"net"
	"os"
	"testing"

	"github.com/fullstorydev/grpcurl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/wundergraph/graphql-go-tools/pkg/engine/datasource/grpc_datasource/testdata/starwars"
)

const bufSize = 1024 * 1024

var lis *bufconn.Listener

func TestMain(m *testing.M) {
	lis = bufconn.Listen(bufSize)
	s := grpc.NewServer()
	pb.RegisterStartwarsServiceServer(s, &pb.Server{})
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("Server exited with error: %v", err)
		}
	}()

	defer s.Stop()

	os.Exit(m.Run())
}

func bufDialer(context.Context, string) (net.Conn, error) {
	return lis.Dial()
}

func TestSource_Load(t *testing.T) {
	sourceProtoFiles, err := grpcurl.DescriptorSourceFromProtoFiles([]string{"testdata/starwars"}, "starwars.proto")
	require.NoError(t, err)

	src := Source{
		descriptorSource: sourceProtoFiles,
		dialContext: func(ctx context.Context, target string) (conn *grpc.ClientConn, err error) {
			conn, err = grpc.DialContext(ctx, target,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithBlock(),
				grpc.WithContextDialer(bufDialer),
			)
			if err != nil {
				return nil, err
			}
			return conn, err
		},
	}

	buf := &bytes.Buffer{}

	input := []byte(`{"package":"starwars","service":"StartwarsService","method":"GetHuman","body":{"id":1},"header":{"fizz":["buzz"]},"target":"bufnet"}`)
	require.NoError(t, src.Load(context.Background(), input, buf))
	assert.Equal(t, `{"id":"1","name":"Luke Skywalker","appearsIn":["NEWHOPE"],"homePlanet":"Earth","primaryFunction":"jedy"}`, buf.String())
	buf.Reset()

	input = []byte(`{"package":"starwars","service":"StartwarsService","method":"GetDroid","body":{"id":1},"header":{"authorization":["FFEEBB"]},"target":"bufnet"}`)
	require.NoError(t, src.Load(context.Background(), input, buf))
	assert.Equal(t, `{"id":"2","name":"C-3PO","appearsIn":["EMPIRE"],"homePlanet":"Alderaan","primaryFunction":"FFEEBB","type":"DROID"}`, buf.String())

}
