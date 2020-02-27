package execution

import (
	"bytes"
	"encoding/json"
	log "github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/pipeline/pkg/pipe"
	"io"
	"io/ioutil"
)

type PipelineDataSourceConfig struct {
	/*
		ConfigFilePath is the path where the pipeline configuration file can be found
		it needs to be in the json format according to the pipeline json schema
		see this url for more info: https://github.com/jensneuse/pipeline
	*/
	ConfigFilePath *string
	/*
			ConfigString is a string to configure the pipeline
			it needs to be in the json format according to the pipeline json schema
		   	see this url for more info: https://github.com/jensneuse/pipeline
			The PipelinDataSourcePlanner will always choose the configString over the configFilePath in case both are defined.
	*/
	ConfigString *string
	// InputJSON is the template to define a JSON object based on the request, parameters etc. which gets passed to the first pipeline step
	InputJSON string
}

func NewPipelineDataSourcePlanner(baseDataSourcePlanner BaseDataSourcePlanner) *PipelineDataSourcePlanner {
	return &PipelineDataSourcePlanner{
		BaseDataSourcePlanner: baseDataSourcePlanner,
	}
}

type PipelineDataSourcePlanner struct {
	BaseDataSourcePlanner
	dataSourceConfig  PipelineDataSourceConfig
	rootField         int
	rawPipelineConfig []byte
}

func (h *PipelineDataSourcePlanner) DataSourceName() string {
	return "PipelineDataSource"
}

func (h *PipelineDataSourcePlanner) Initialize(config DataSourcePlannerConfiguration) (err error) {
	h.walker, h.operation, h.definition = config.walker, config.operation, config.definition
	h.rootField = -1
	return json.NewDecoder(config.dataSourceConfiguration).Decode(&h.dataSourceConfig)
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

	if h.dataSourceConfig.ConfigString != nil {
		h.rawPipelineConfig = []byte(*h.dataSourceConfig.ConfigString)
	}
	if h.dataSourceConfig.ConfigFilePath != nil {
		var err error
		h.rawPipelineConfig, err = ioutil.ReadFile(*h.dataSourceConfig.ConfigFilePath)
		if err != nil {
			h.log.Error("PipelineDataSourcePlanner.readConfigFile", log.Error(err))
		}
	}

	h.args = append(h.args, &StaticVariableArgument{
		Name:  literal.INPUT_JSON,
		Value: []byte(h.dataSourceConfig.InputJSON),
	})

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
	log      log.Logger
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
