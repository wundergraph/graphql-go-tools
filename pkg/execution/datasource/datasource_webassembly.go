package datasource

import (
	"context"
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

type WasmDataSourcePlannerFactoryFactory struct {
}

func (w WasmDataSourcePlannerFactoryFactory) Initialize(base BasePlanner, configReader io.Reader) (PlannerFactory, error) {
	factory := &WasmDataSourcePlannerFactory{
		base: base,
	}
	return factory, json.NewDecoder(configReader).Decode(&factory.config)
}

type WasmDataSourcePlannerFactory struct {
	base   BasePlanner
	config WasmDataSourceConfig
}

func (w WasmDataSourcePlannerFactory) DataSourcePlanner() Planner {
	return &WasmDataSourcePlanner{
		BasePlanner:      w.base,
		dataSourceConfig: w.config,
	}
}

type WasmDataSourcePlanner struct {
	BasePlanner
	dataSourceConfig WasmDataSourceConfig
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
	w.Args = append(w.Args, &StaticVariableArgument{
		Name:  literal.WASMFILE,
		Value: []byte(w.dataSourceConfig.WasmFile),
	})
	w.Args = append(w.Args, &StaticVariableArgument{
		Name:  literal.INPUT,
		Value: []byte(w.dataSourceConfig.Input),
	})
	// Args
	if w.Operation.FieldHasArguments(ref) {
		args := w.Operation.FieldArguments(ref)
		for _, i := range args {
			argName := w.Operation.ArgumentNameBytes(i)
			value := w.Operation.ArgumentValue(i)
			if value.Kind != ast.ValueKindVariable {
				continue
			}
			variableName := w.Operation.VariableValueNameBytes(value.Ref)
			name := append([]byte(".arguments."), argName...)
			arg := &ContextVariableArgument{
				VariableName: variableName,
				Name:         make([]byte, len(name)),
			}
			copy(arg.Name, name)
			w.Args = append(w.Args, arg)
		}
	}
}

func (w *WasmDataSourcePlanner) LeaveField(ref int) {

}

func (w *WasmDataSourcePlanner) Plan(args []Argument) (DataSource, []Argument) {
	return &WasmDataSource{
		Log: w.Log,
	}, append(w.Args, args...)
}

type WasmDataSource struct {
	Log      log.Logger
	instance wasm.Instance
	once     sync.Once
}

func (s *WasmDataSource) Resolve(ctx context.Context, args ResolverArgs, out io.Writer) (n int, err error) {

	input := args.ByKey(literal.INPUT)
	wasmFile := args.ByKey(literal.WASMFILE)

	s.Log.Debug("WasmDataSource.Resolve.Args",
		log.ByteString("input", input),
		log.ByteString("wasmFile", wasmFile),
	)

	s.once.Do(func() {
		wasmData, err := wasm.ReadBytes(string(wasmFile))
		if err != nil {
			s.Log.Error("WasmDataSource.wasm.ReadBytes(string(wasmFile))",
				log.Error(err),
			)
			return
		}
		s.instance, err = wasm.NewInstance(wasmData)
		if err != nil {
			s.Log.Error("WasmDataSource.wasm.NewInstance(wasmData)",
				log.Error(err),
			)
		}

		s.Log.Debug("WasmDataSource.wasm.NewInstance OK")
	})

	inputLen := len(input)

	allocateInputResult, err := s.instance.Exports["allocate"](inputLen)
	if err != nil {
		s.Log.Error("WasmDataSource.instance.Exports[\"allocate\"](inputLen)",
			log.Error(err),
		)
		return n, err
	}

	inputPointer := allocateInputResult.ToI32()

	memory := s.instance.Memory.Data()[inputPointer:]

	for i := 0; i < inputLen; i++ {
		memory[i] = input[i]
	}

	memory[inputLen] = 0

	result, err := s.instance.Exports["invoke"](inputPointer)
	if err != nil {
		s.Log.Error("WasmDataSource.instance.Exports[\"invoke\"](inputPointer)",
			log.Error(err),
		)
		return n,err
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
		s.Log.Error("WasmDataSource.out.Write(memory[start:stop])",
			log.Error(err),
		)
		return n,err
	}

	deallocate := s.instance.Exports["deallocate"]
	_, err = deallocate(inputPointer, inputLen)
	if err != nil {
		s.Log.Error("WasmDataSource.deallocate(inputPointer, inputLen)",
			log.Error(err),
		)
		return n,err
	}

	_, err = deallocate(start, stop-start)
	if err != nil {
		s.Log.Error("WasmDataSource.deallocate(start, stop-start)",
			log.Error(err),
		)
		return n,err
	}

	return n,err
}
