package execution

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"io"
)

func NewTypeDataSourcePlanner(baseDataSourcePlanner BaseDataSourcePlanner) *TypeDataSourcePlanner {
	return &TypeDataSourcePlanner{
		BaseDataSourcePlanner: baseDataSourcePlanner,
	}
}

type TypeDataSourcePlanner struct {
	BaseDataSourcePlanner
}

func (t *TypeDataSourcePlanner) DataSourceName() string {
	return "TypeDataSource"
}

func (t *TypeDataSourcePlanner) Initialize(config DataSourcePlannerConfiguration) (err error) {
	t.walker, t.operation, t.definition = config.walker, config.operation, config.definition
	return nil
}

func (t *TypeDataSourcePlanner) EnterInlineFragment(ref int) {

}

func (t *TypeDataSourcePlanner) LeaveInlineFragment(ref int) {

}

func (t *TypeDataSourcePlanner) EnterSelectionSet(ref int) {

}

func (t *TypeDataSourcePlanner) LeaveSelectionSet(ref int) {

}

func (t *TypeDataSourcePlanner) EnterField(ref int) {
	if t.rootField.isDefined {
		return
	}
	t.rootField.setIfNotDefined(ref)
	// args
	if t.operation.FieldHasArguments(ref) {
		args := t.operation.FieldArguments(ref)
		for _, i := range args {
			argName := t.operation.ArgumentNameBytes(i)
			value := t.operation.ArgumentValue(i)
			if value.Kind != ast.ValueKindVariable {
				continue
			}
			variableName := t.operation.VariableValueNameBytes(value.Ref)
			arg := &ContextVariableArgument{
				VariableName: variableName,
				Name:         make([]byte, len(argName)),
			}
			copy(arg.Name, argName)
			t.args = append(t.args, arg)
		}
	}
}

func (t *TypeDataSourcePlanner) LeaveField(ref int) {

}

func (t *TypeDataSourcePlanner) Plan() (DataSource, []Argument) {
	return &TypeDataSource{}, t.args
}

type TypeDataSource struct {
}

func (t *TypeDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) Instruction {
	return CloseConnection
}
