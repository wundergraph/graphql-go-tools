package execution

import (
	"bytes"
	"encoding/json"
	log "github.com/jensneuse/abstractlogger"
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

type PipelineDataSourcePlannerFactoryFactory struct {
}

func (p PipelineDataSourcePlannerFactoryFactory) Initialize(base BaseDataSourcePlanner, configReader io.Reader) (DataSourcePlannerFactory, error) {
	factory := &PipelineDataSourcePlannerFactory{
		base: base,
	}
	return factory, json.NewDecoder(configReader).Decode(&factory.config)
}

type PipelineDataSourcePlannerFactory struct {
	base   BaseDataSourcePlanner
	config PipelineDataSourceConfig
}

func (p PipelineDataSourcePlannerFactory) DataSourcePlanner() DataSourcePlanner {
	return &PipelineDataSourcePlanner{
		BaseDataSourcePlanner: p.base,
		dataSourceConfig:      p.config,
	}
}

type PipelineDataSourcePlanner struct {
	BaseDataSourcePlanner
	dataSourceConfig  PipelineDataSourceConfig
	rawPipelineConfig []byte
}

func (h *PipelineDataSourcePlanner) Plan(args []Argument) (DataSource, []Argument) {

	source := PipelineDataSource{
		log: h.log,
	}

	err := source.pipeline.FromConfig(bytes.NewReader(h.rawPipelineConfig))
	if err != nil {
		h.log.Error("PipelineDataSourcePlanner.pipe.FromConfig", log.Error(err))
	}

	return &source, append(h.args,args...)
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
	h.rootField.setIfNotDefined(ref)
}

func (h *PipelineDataSourcePlanner) LeaveField(ref int) {
	if !h.rootField.isDefinedAndEquals(ref) {
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
