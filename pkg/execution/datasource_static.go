package execution

import (
	"encoding/json"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"io"
)

type StaticDataSourceConfig struct {
	Data string
}

func NewStaticDataSourcePlanner(baseDataSourcePlanner BaseDataSourcePlanner) *StaticDataSourcePlanner {
	return &StaticDataSourcePlanner{
		BaseDataSourcePlanner: baseDataSourcePlanner,
	}
}

type StaticDataSourcePlanner struct {
	BaseDataSourcePlanner
	dataSourceConfig StaticDataSourceConfig
}

func (s *StaticDataSourcePlanner) DataSourceName() string {
	return "StaticDataSource"
}

func (s *StaticDataSourcePlanner) Initialize(config DataSourcePlannerConfiguration) (err error) {
	s.walker, s.operation, s.definition = config.walker, config.operation, config.definition
	s.args = nil
	return json.NewDecoder(config.dataSourceConfiguration).Decode(&s.dataSourceConfig)
}

func (s *StaticDataSourcePlanner) EnterInlineFragment(ref int) {

}

func (s *StaticDataSourcePlanner) LeaveInlineFragment(ref int) {

}

func (s *StaticDataSourcePlanner) EnterSelectionSet(ref int) {

}

func (s *StaticDataSourcePlanner) LeaveSelectionSet(ref int) {

}

func (s *StaticDataSourcePlanner) EnterField(ref int) {
	if s.rootField.isDefined {
		return
	}
	s.rootField.setIfNotDefined(ref)
	s.args = append(s.args, &StaticVariableArgument{
		Name:  literal.DATA,
		Value: []byte(s.dataSourceConfig.Data),
	})
}

func (s *StaticDataSourcePlanner) LeaveField(ref int) {

}

func (s *StaticDataSourcePlanner) Plan() (DataSource, []Argument) {
	return &StaticDataSource{}, s.args
}

type StaticDataSource struct {
}

func (s StaticDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) Instruction {
	_, _ = out.Write(args.ByKey(literal.DATA))
	return CloseConnectionIfNotStream
}
