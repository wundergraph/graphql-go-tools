package execution

import (
	"encoding/json"
	wasm "github.com/wasmerio/go-ext-wasm/wasmer"
	"testing"
)

// nolint
func TestWasm(t *testing.T){
	bytes, _ := wasm.ReadBytes("./testdata/memory.wasm")
	instance, _ := wasm.NewInstance(bytes)
	defer instance.Close()

	person := Person{
		Id: "1",
	}

	input,_ := json.Marshal(person)
	inputLen := len(input)

	allocateInputResult, _ := instance.Exports["allocate"](inputLen)
	inputPointer := allocateInputResult.ToI32()

	memory := instance.Memory.Data()[inputPointer:]

	for i := 0;i<inputLen;i++{
		memory[i] = input[i]
	}

	memory[inputLen] = 0

	// Calls the `return_hello` exported function.
	// This function returns a start to a string.
	result, _ := instance.Exports["invoke"](inputPointer)

	// Gets the start value as an integer.
	start := result.ToI32()

	// Reads the memory.
	memory = instance.Memory.Data()

	//fmt.Println(string(memory[start : start+13]))

	var stop int32

	for i := start;i<int32(len(memory));i++{
		if memory[i] == 0 {
			stop = i
			break
		}
	}

	//fmt.Printf("out: %s\n",string(memory[start:stop]))

	deallocate := instance.Exports["deallocate"]
	deallocate(inputPointer, inputLen)
	deallocate(start, stop-start)
}

// nolint
func BenchmarkWasm(b *testing.B) {
	bytes, _ := wasm.ReadBytes("./testdata/memory.wasm")
	instance, _ := wasm.NewInstance(bytes)
	defer instance.Close()

	person := Person{
		Id: "1",
	}

	input,_ := json.Marshal(person)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0;i<b.N;i++{


		inputLen := len(input)

		allocateInputResult, err := instance.Exports["allocate"](inputLen)
		if err != nil {
			b.Fatal(err)
		}
		inputPointer := allocateInputResult.ToI32()

		memory := instance.Memory.Data()[inputPointer:]

		for i := 0;i<inputLen;i++{
			memory[i] = input[i]
		}

		memory[inputLen] = 0

		// Calls the `return_hello` exported function.
		// This function returns a start to a string.
		result, err := instance.Exports["invoke"](inputPointer)
		if err != nil {
			b.Fatal(err)
		}

		// Gets the start value as an integer.
		start := result.ToI32()

		// Reads the memory.
		memory = instance.Memory.Data()

		//fmt.Println(string(memory[start : start+13]))

		var stop int32

		for i := start;i<int32(len(memory));i++{
			if memory[i] == 0 {
				stop = i
				break
			}
		}

		deallocate := instance.Exports["deallocate"]
		_,err = deallocate(inputPointer, inputLen)
		if err != nil {
			b.Fatal(err)
		}
		_,err = deallocate(start, stop-start)
		if err != nil {
			b.Fatal(err)
		}
	}
}