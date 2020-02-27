package execution

import (
	"encoding/json"
	log "github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	wasm "github.com/wasmerio/go-ext-wasm/wasmer"
	"io"
	"sync"
)

type WasmDataSourceConfig struct {
	WasmFile string
	Input    string
}

func NewWasmDataSourcePlanner(baseDataSourcePlanner BaseDataSourcePlanner) *WasmDataSourcePlanner {
	return &WasmDataSourcePlanner{
		BaseDataSourcePlanner: baseDataSourcePlanner,
	}
}

type WasmDataSourcePlanner struct {
	BaseDataSourcePlanner
	dataSourceConfig WasmDataSourceConfig
}

func (w *WasmDataSourcePlanner) DataSourceName() string {
	return "WasmDataSource"
}

func (w *WasmDataSourcePlanner) Initialize(config DataSourcePlannerConfiguration) (err error) {
	w.walker, w.operation, w.definition = config.walker, config.operation, config.definition
	return json.NewDecoder(config.dataSourceConfiguration).Decode(&w.dataSourceConfig)
}

func (w *WasmDataSourcePlanner) EnterInlineFragment(ref int) {

}

func (w *WasmDataSourcePlanner) LeaveInlineFragment(ref int) {

}

func (w *WasmDataSourcePlanner) EnterSelectionSet(ref int) {

}

func (w *WasmDataSourcePlanner) LeaveSelectionSet(ref int) {

}

func (w *WasmDataSourcePlanner) EnterField(ref int) {
	w.args = append(w.args,&StaticVariableArgument{
		Name: literal.WASMFILE,
		Value: []byte(w.dataSourceConfig.WasmFile),
	})
	w.args = append(w.args,&StaticVariableArgument{
		Name: literal.INPUT,
		Value: []byte(w.dataSourceConfig.Input),
	})
	// args
	if w.operation.FieldHasArguments(ref) {
		args := w.operation.FieldArguments(ref)
		for _, i := range args {
			argName := w.operation.ArgumentNameBytes(i)
			value := w.operation.ArgumentValue(i)
			if value.Kind != ast.ValueKindVariable {
				continue
			}
			variableName := w.operation.VariableValueNameBytes(value.Ref)
			name := append([]byte(".arguments."), argName...)
			arg := &ContextVariableArgument{
				VariableName: variableName,
				Name:         make([]byte, len(name)),
			}
			copy(arg.Name, name)
			w.args = append(w.args, arg)
		}
	}
}

func (w *WasmDataSourcePlanner) LeaveField(ref int) {

}

func (w *WasmDataSourcePlanner) Plan() (DataSource, []Argument) {
	return &WasmDataSource{
		log: w.log,
	}, w.args
}

type WasmDataSource struct {
	log      log.Logger
	instance wasm.Instance
	once     sync.Once
}

func (s *WasmDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) Instruction {

	input := args.ByKey(literal.INPUT)
	wasmFile := args.ByKey(literal.WASMFILE)

	s.log.Debug("WasmDataSource.Resolve.args",
		log.ByteString("input", input),
		log.ByteString("wasmFile", wasmFile),
	)

	s.once.Do(func() {
		wasmData, err := wasm.ReadBytes(string(wasmFile))
		if err != nil {
			s.log.Error("WasmDataSource.wasm.ReadBytes(string(wasmFile))",
				log.Error(err),
			)
			return
		}
		s.instance, err = wasm.NewInstance(wasmData)
		if err != nil {
			s.log.Error("WasmDataSource.wasm.NewInstance(wasmData)",
				log.Error(err),
			)
		}

		s.log.Debug("WasmDataSource.wasm.NewInstance OK")
	})

	inputLen := len(input)

	allocateInputResult, err := s.instance.Exports["allocate"](inputLen)
	if err != nil {
		s.log.Error("WasmDataSource.instance.Exports[\"allocate\"](inputLen)",
			log.Error(err),
		)
		return CloseConnectionIfNotStream
	}

	inputPointer := allocateInputResult.ToI32()

	memory := s.instance.Memory.Data()[inputPointer:]

	for i := 0; i < inputLen; i++ {
		memory[i] = input[i]
	}

	memory[inputLen] = 0

	result, err := s.instance.Exports["invoke"](inputPointer)
	if err != nil {
		s.log.Error("WasmDataSource.instance.Exports[\"invoke\"](inputPointer)",
			log.Error(err),
		)
		return CloseConnectionIfNotStream
	}

	start := result.ToI32()
	memory = s.instance.Memory.Data()

	var stop int32

	for i := start; i < int32(len(memory)); i++ {
		if memory[i] == 0 {
			stop = i
			break
		}
	}

	_, err = out.Write(memory[start:stop])
	if err != nil {
		s.log.Error("WasmDataSource.out.Write(memory[start:stop])",
			log.Error(err),
		)
		return CloseConnectionIfNotStream
	}

	deallocate := s.instance.Exports["deallocate"]
	_, err = deallocate(inputPointer, inputLen)
	if err != nil {
		s.log.Error("WasmDataSource.deallocate(inputPointer, inputLen)",
			log.Error(err),
		)
		return CloseConnectionIfNotStream
	}

	_, err = deallocate(start, stop-start)
	if err != nil {
		s.log.Error("WasmDataSource.deallocate(start, stop-start)",
			log.Error(err),
		)
		return CloseConnectionIfNotStream
	}

	return CloseConnectionIfNotStream
}
