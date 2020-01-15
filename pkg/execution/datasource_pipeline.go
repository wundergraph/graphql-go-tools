package execution

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/pipeline/pkg/pipe"
	log "github.com/jensneuse/abstractlogger"
	"io"
	"io/ioutil"
)

func NewPipelineDataSourcePlanner(baseDataSourcePlanner BaseDataSourcePlanner) *PipelineDataSourcePlanner {
	return &PipelineDataSourcePlanner{
		BaseDataSourcePlanner: baseDataSourcePlanner,
	}
}

type PipelineDataSourcePlanner struct {
	BaseDataSourcePlanner
	rootField         int
	rawPipelineConfig []byte
}

func (h *PipelineDataSourcePlanner) DirectiveDefinition() []byte {
	data, _ := h.graphqlDefinitions.Find("directives/pipeline_datasource.graphql")
	return data
}

func (h *PipelineDataSourcePlanner) DirectiveName() []byte {
	return []byte("PipelineDataSource")
}

func (h *PipelineDataSourcePlanner) Initialize(walker *astvisitor.Walker, operation, definition *ast.Document, args []Argument, resolverParameters []ResolverParameter) {
	h.walker, h.operation, h.definition, h.args = walker, operation, definition, args
	h.rootField = -1
}

func (h *PipelineDataSourcePlanner) Plan() (DataSource, []Argument) {

	source := PipelineDataSource{
		log: h.log,
	}

	err := source.pipeline.FromConfig(bytes.NewReader(h.rawPipelineConfig))
	if err != nil {
		h.log.Error("PipelineDataSourcePlanner.pipe.FromConfig", log.Error(err))
	}

	return &source, h.args
}

func (h *PipelineDataSourcePlanner) EnterInlineFragment(ref int) {

}

func (h *PipelineDataSourcePlanner) LeaveInlineFragment(ref int) {

}

func (h *PipelineDataSourcePlanner) EnterSelectionSet(ref int) {

}

func (h *PipelineDataSourcePlanner) LeaveSelectionSet(ref int) {

}

func (h *PipelineDataSourcePlanner) EnterField(ref int) {
	if h.rootField == -1 {
		h.rootField = ref
	}
}

func (h *PipelineDataSourcePlanner) LeaveField(ref int) {
	if h.rootField != ref {
		return
	}
	definition, exists := h.walker.FieldDefinition(ref)
	if !exists {
		return
	}
	directive, exists := h.definition.FieldDefinitionDirectiveByName(definition, h.DirectiveName())
	if !exists {
		return
	}
	value, exists := h.definition.DirectiveArgumentValueByName(directive, literal.CONFIG_STRING)
	if exists {
		variableValue := h.definition.StringValueContentBytes(value.Ref)
		h.rawPipelineConfig = make([]byte, len(variableValue))
		copy(h.rawPipelineConfig, variableValue)
	}

	value, exists = h.definition.DirectiveArgumentValueByName(directive, literal.CONFIG_FILE_PATH)
	if exists {
		variableValue := h.definition.StringValueContentBytes(value.Ref)
		var err error
		h.rawPipelineConfig, err = ioutil.ReadFile(variableValue.String())
		if err != nil {
			h.log.Error("PipelineDataSourcePlanner.readConfigFile", log.Error(err))
		}
	}

	value, exists = h.definition.DirectiveArgumentValueByName(directive, literal.INPUT_JSON)
	if exists {
		variableValue := h.definition.StringValueContentBytes(value.Ref)
		arg := &StaticVariableArgument{
			Name:  literal.INPUT_JSON,
			Value: make([]byte, len(variableValue)),
		}
		copy(arg.Value, variableValue)
		h.args = append([]Argument{arg}, h.args...)
	}

	// args
	if h.operation.FieldHasArguments(ref) {
		args := h.operation.FieldArguments(ref)
		for _, i := range args {
			argName := h.operation.ArgumentNameBytes(i)
			value := h.operation.ArgumentValue(i)
			if value.Kind != ast.ValueKindVariable {
				continue
			}
			variableName := h.operation.VariableValueNameBytes(value.Ref)
			name := append([]byte(".arguments."), argName...)
			arg := &ContextVariableArgument{
				VariableName: variableName,
				Name:         make([]byte, len(name)),
			}
			copy(arg.Name, name)
			h.args = append(h.args, arg)
		}
	}
}

type PipelineDataSource struct {
	log  log.Logger
	pipeline pipe.Pipeline
}

func (r *PipelineDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) Instruction {

	inputJSON := args.ByKey(literal.INPUT_JSON)

	err := r.pipeline.Run(bytes.NewReader(inputJSON), out)
	if err != nil {
		r.log.Error("PipelineDataSource.pipe.Run", log.Error(err))
	}

	return CloseConnectionIfNotStream
}
