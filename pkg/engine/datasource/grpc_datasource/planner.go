package grpc_datasource

import (
	"context"

	"github.com/fullstorydev/grpcurl"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/wundergraph/graphql-go-tools/pkg/engine/plan"
)

type Planner struct {
	v         *plan.Visitor
	rootField int
	config    Configuration
}

func (p *Planner) Register(visitor *plan.Visitor, _ plan.DataSourceConfiguration, _ bool) error {
	p.v = visitor
	visitor.Walker.RegisterEnterFieldVisitor(p)
	return nil
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

func (p *Planner) configureInput() string {
	return ""
}

func (p *Planner) descriptorSource() grpcurl.DescriptorSource {
	files := &descriptorpb.FileDescriptorSet{}
	var fs descriptorpb.FileDescriptorSet
	_ = proto.Unmarshal(p.config.Protoset, &fs)
	files.File = append(files.File, fs.File...)

	src, _ := grpcurl.DescriptorSourceFromFileDescriptorSet(files)
	return src
}

func (p *Planner) ConfigureFetch() plan.FetchConfiguration {
	return plan.FetchConfiguration{
		Input: p.configureInput(),
		DataSource: &Source{
			config:           p.config,
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
		},
	}
}

func (p *Planner) ConfigureSubscription() plan.SubscriptionConfiguration {

	return plan.SubscriptionConfiguration{}
}
