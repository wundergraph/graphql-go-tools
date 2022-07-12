package grpc_datasource

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/fullstorydev/grpcurl"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/wundergraph/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/literal"
)

type Planner struct {
	v         *plan.Visitor
	rootField int
	config    Configuration
}

func (p *Planner) Register(visitor *plan.Visitor, config plan.DataSourceConfiguration, _ bool) error {
	p.v = visitor
	visitor.Walker.RegisterEnterFieldVisitor(p)
	return json.Unmarshal(config.Custom, &p.config)
}

func (p *Planner) DownstreamResponseFieldAlias(_ int) (alias string, exists bool) {
	return
}

func (p *Planner) DataSourcePlanningBehavior() plan.DataSourcePlanningBehavior {
	return plan.DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      false,
		OverrideFieldPathFromAlias: false,
	}
}

func (p *Planner) EnterField(ref int) {
	p.rootField = ref
}

func (p *Planner) configureInput() []byte {
	input := httpclient.SetInputBody(nil, []byte(p.config.Request.Body))

	header, err := json.Marshal(p.config.Request.Header)
	if err == nil && len(header) != 0 && !bytes.Equal(header, literal.NULL) {
		input = httpclient.SetInputHeader(input, header)
	}

	return input
}

func (p *Planner) descriptorSource() grpcurl.DescriptorSource {
	files := &descriptorpb.FileDescriptorSet{}
	var fs descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(p.config.Protoset, &fs); err != nil {
		p.v.Walker.StopWithInternalErr(err)
		return nil
	}
	files.File = append(files.File, fs.File...)

	src, err := grpcurl.DescriptorSourceFromFileDescriptorSet(files)
	if err != nil {
		p.v.Walker.StopWithInternalErr(err)
		return nil
	}
	return src
}

func (p *Planner) ConfigureFetch() plan.FetchConfiguration {
	input := p.configureInput()
	source := &Source{
		config:           p.config.Grpc,
		descriptorSource: p.descriptorSource(),
		dialContext: func(ctx context.Context, target string) (conn *grpc.ClientConn, err error) {
			conn, err = grpc.DialContext(ctx, target,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithBlock(),
			)
			if err != nil {
				return nil, err
			}
			return conn, err
		},
	}

	return plan.FetchConfiguration{
		Input:             string(input),
		DataSource:        source,
		DisableDataLoader: true,
	}
}

func (p *Planner) ConfigureSubscription() plan.SubscriptionConfiguration {
	return plan.SubscriptionConfiguration{}
}
