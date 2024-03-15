package astjson

import (
	"bytes"
	"testing"

	"github.com/buger/jsonparser"
	"github.com/stretchr/testify/assert"
)

func TestJSON_ParsePrint(t *testing.T) {
	js := &JSON{}
	input := `{"data":{"_entities":[{"stock":8},{"stock":2},{"stock":5}]}}`
	err := js.ParseObject([]byte(input))
	assert.NoError(t, err)
	out := &bytes.Buffer{}
	err = js.PrintNode(js.Nodes[js.RootNode], out)
	assert.NoError(t, err)
	assert.Equal(t, input, out.String())
	dataNodeRef := js.Get(js.RootNode, []string{"data"})
	assert.NotEqualf(t, -1, dataNodeRef, "data node not found")
	dataNode := js.Nodes[dataNodeRef]
	out.Reset()
	err = js.PrintNode(dataNode, out)
	assert.NoError(t, err)
	assert.Equal(t, `{"_entities":[{"stock":8},{"stock":2},{"stock":5}]}`, out.String())
}

func TestJSON_ParsePrintArray(t *testing.T) {
	js := &JSON{}
	err := js.ParseObject([]byte(`{"strings": ["Alex", "true", "123",true,123,0.123,"foo"]}`))
	assert.NoError(t, err)
	out := &bytes.Buffer{}
	err = js.PrintRoot(out)
	assert.NoError(t, err)
	assert.Equal(t, `{"strings":["Alex","true","123",true,123,0.123,"foo"]}`, out.String())
}

func TestJSON_InitResolvable(t *testing.T) {
	js := &JSON{}
	dataRoot, errorsRoot, err := js.InitResolvable(nil)
	assert.NoError(t, err)
	assert.NotEqual(t, -1, dataRoot)
	assert.NotEqual(t, -1, errorsRoot)
	root := js.DebugPrintNode(js.RootNode)
	data := js.DebugPrintNode(dataRoot)
	errors := js.DebugPrintNode(errorsRoot)
	assert.Equal(t, `{"errors":[],"data":{}}`, root)
	assert.Equal(t, `{}`, data)
	assert.Equal(t, `[]`, errors)

	js = &JSON{}
	dataRoot, errorsRoot, err = js.InitResolvable([]byte(`{"name":"Jens"}`))
	assert.NoError(t, err)
	assert.NotEqual(t, -1, dataRoot)
	assert.NotEqual(t, -1, errorsRoot)
	root = js.DebugPrintNode(js.RootNode)
	data = js.DebugPrintNode(dataRoot)
	errors = js.DebugPrintNode(errorsRoot)
	assert.Equal(t, `{"errors":[],"data":{"name":"Jens"}}`, root)
	assert.Equal(t, `{"name":"Jens"}`, data)
	assert.Equal(t, `[]`, errors)

}

func TestJSON_MergeArrays(t *testing.T) {
	js := &JSON{}
	dataRoot, errorsRoot, err := js.InitResolvable([]byte(`{"name":"Jens"}`))
	assert.NoError(t, err)
	assert.NotEqual(t, -1, dataRoot)
	assert.NotEqual(t, -1, errorsRoot)
	exampleGraphQLErrorsObject := []byte(`{"errors":[{"message":"Cannot query field \"foo\" on type \"Query\".","locations":[{"line":1,"column":3}]}]}`)
	example, err := js.AppendObject(exampleGraphQLErrorsObject)
	assert.NoError(t, err)
	assert.NotEqual(t, -1, example)
	errorsRef := js.Get(example, []string{"errors"})
	assert.NotEqual(t, -1, errorsRef)
	js.MergeArrays(errorsRoot, errorsRef)
	root := js.DebugPrintNode(js.RootNode)
	data := js.DebugPrintNode(dataRoot)
	errors := js.DebugPrintNode(errorsRoot)
	assert.Equal(t, `{"errors":[{"message":"Cannot query field \"foo\" on type \"Query\".","locations":[{"line":1,"column":3}]}],"data":{"name":"Jens"}}`, root)
	assert.Equal(t, `{"name":"Jens"}`, data)
	assert.Equal(t, `[{"message":"Cannot query field \"foo\" on type \"Query\".","locations":[{"line":1,"column":3}]}]`, errors)
}

func TestJSON_ParsePrintNested(t *testing.T) {
	js := &JSON{}
	input := `{"data":{"_entities":[{"stock":8},{"stock":2},{"stock":5}]}}`
	err := js.ParseObject([]byte(input))
	assert.NoError(t, err)
	out := &bytes.Buffer{}
	err = js.PrintNode(js.Nodes[js.RootNode], out)
	assert.NoError(t, err)
	assert.Equal(t, input, out.String())
	dataNodeRef := js.Get(js.RootNode, []string{"data", "_entities"})
	assert.NotEqualf(t, -1, dataNodeRef, "data node not found")
	dataNode := js.Nodes[dataNodeRef]
	out.Reset()
	err = js.PrintNode(dataNode, out)
	assert.NoError(t, err)
	assert.Equal(t, `[{"stock":8},{"stock":2},{"stock":5}]`, out.String())
}

func TestJSON_ParseAppendSetPrint(t *testing.T) {
	js := &JSON{}
	input := `{"data":{"_entities":[{"stock":8},{"stock":2},{"stock":5}]}}`
	err := js.ParseObject([]byte(input))
	assert.NoError(t, err)

	nothing, err := js.AppendObject([]byte(`{"nothing":"here"}`))
	assert.NoError(t, err)
	assert.NotEqual(t, -1, nothing)
	replaced := js.SetObjectField(js.RootNode, nothing, []string{"data", "_entities"})
	assert.True(t, replaced)

	out := &bytes.Buffer{}
	err = js.PrintNode(js.Nodes[js.RootNode], out)
	assert.NoError(t, err)
	assert.Equal(t, `{"data":{"_entities":{"nothing":"here"}}}`, out.String())

	nothing, err = js.AppendObject([]byte(`{"nothing":"there"}`))
	assert.NoError(t, err)
	assert.NotEqual(t, -1, nothing)

	replaced = js.SetObjectField(js.RootNode, nothing, []string{"data", "_entities", "nothing"})
	assert.True(t, replaced)

	out.Reset()
	err = js.PrintNode(js.Nodes[js.RootNode], out)
	assert.NoError(t, err)
	assert.Equal(t, `{"data":{"_entities":{"nothing":{"nothing":"there"}}}}`, out.String())

	another, err := js.AppendObject([]byte(`{"another":true}`))
	assert.NoError(t, err)
	assert.NotEqual(t, -1, another)

	trueField := js.Get(another, []string{"another"})
	assert.NotEqual(t, -1, trueField)

	notReplaced := js.SetObjectField(js.RootNode, trueField, []string{"another"})
	assert.False(t, notReplaced)

	out.Reset()
	err = js.PrintNode(js.Nodes[js.RootNode], out)
	assert.NoError(t, err)
	assert.Equal(t, `{"data":{"_entities":{"nothing":{"nothing":"there"}}},"another":true}`, out.String())

	number, err := js.AppendObject([]byte(`{"number":123}`))
	assert.NoError(t, err)
	assert.NotEqual(t, -1, another)

	oneTwoThree := js.Get(number, []string{"number"})
	assert.NotEqual(t, -1, oneTwoThree)

	notReplaced = js.SetObjectField(js.RootNode, oneTwoThree, []string{"number"})
	assert.False(t, notReplaced)

	out.Reset()
	err = js.PrintNode(js.Nodes[js.RootNode], out)
	assert.NoError(t, err)
	assert.Equal(t, `{"data":{"_entities":{"nothing":{"nothing":"there"}}},"another":true,"number":123}`, out.String())
}

func TestJSON_MergeNodes(t *testing.T) {
	js := &JSON{}
	err := js.ParseObject([]byte(`{"a":1,"b":2}`))
	assert.NoError(t, err)

	c, err := js.AppendObject([]byte(`{"c":3}`))
	assert.NoError(t, err)
	assert.NotEqual(t, -1, c)

	merged := js.MergeNodes(js.RootNode, c)
	assert.NotEqual(t, -1, merged)
	assert.Equal(t, js.RootNode, merged)

	out := &bytes.Buffer{}
	err = js.PrintNode(js.Nodes[js.RootNode], out)
	assert.NoError(t, err)
	assert.Equal(t, `{"a":1,"b":2,"c":3}`, out.String())

	anotherC, err := js.AppendObject([]byte(`{"c":3}`))
	assert.NoError(t, err)
	assert.NotEqual(t, -1, c)

	merged = js.MergeNodes(js.RootNode, anotherC)
	assert.NotEqual(t, -1, merged)
	assert.Equal(t, js.RootNode, merged)

	out.Reset()
	err = js.PrintNode(js.Nodes[js.RootNode], out)
	assert.NoError(t, err)
	assert.Equal(t, `{"a":1,"b":2,"c":3}`, out.String())

	overrideC, err := js.AppendObject([]byte(`{"c":true}`))
	assert.NoError(t, err)
	assert.NotEqual(t, -1, c)

	merged = js.MergeNodes(js.RootNode, overrideC)
	assert.NotEqual(t, -1, merged)
	assert.Equal(t, js.RootNode, merged)

	out.Reset()
	err = js.PrintNode(js.Nodes[js.RootNode], out)
	assert.NoError(t, err)
	assert.Equal(t, `{"a":1,"b":2,"c":true}`, out.String())
}

func TestJSON_MergeNodesNested(t *testing.T) {
	js := &JSON{}
	err := js.ParseObject([]byte(`{"a":1,"b":2,"c":{"d":4}}`))
	assert.NoError(t, err)

	ce, err := js.AppendObject([]byte(`{"c":{"e":5}}`))
	assert.NoError(t, err)
	assert.NotEqual(t, -1, ce)

	merged := js.MergeNodes(js.RootNode, ce)
	assert.NotEqual(t, -1, merged)
	assert.Equal(t, js.RootNode, merged)

	out := &bytes.Buffer{}
	err = js.PrintNode(js.Nodes[js.RootNode], out)
	assert.NoError(t, err)
	assert.Equal(t, `{"a":1,"b":2,"c":{"d":4,"e":5}}`, out.String())

	cef, err := js.AppendObject([]byte(`{"c":{"e":6,"f":7}}`))
	assert.NoError(t, err)
	assert.NotEqual(t, -1, cef)

	merged = js.MergeNodes(js.RootNode, cef)
	assert.NotEqual(t, -1, merged)
	assert.Equal(t, js.RootNode, merged)

	out.Reset()
	err = js.PrintNode(js.Nodes[js.RootNode], out)
	assert.NoError(t, err)
	assert.Equal(t, `{"a":1,"b":2,"c":{"d":4,"e":6,"f":7}}`, out.String())
}

func TestJSON_MergeNodesWithPath(t *testing.T) {
	js := &JSON{}
	err := js.ParseObject([]byte(`{"a":1}`))
	assert.NoError(t, err)

	c, err := js.AppendObject([]byte(`{"c":3}`))
	assert.NoError(t, err)
	assert.NotEqual(t, -1, c)

	merged := js.MergeNodesWithPath(js.RootNode, c, []string{"b"})
	assert.NotEqual(t, -1, merged)
	assert.Equal(t, js.RootNode, merged)

	out := &bytes.Buffer{}
	err = js.PrintNode(js.Nodes[js.RootNode], out)
	assert.NoError(t, err)
	assert.Equal(t, `{"a":1,"b":{"c":3}}`, out.String())

	d, err := js.AppendObject([]byte(`{"d":5}`))
	assert.NoError(t, err)
	assert.NotEqual(t, -1, c)

	merged = js.MergeNodesWithPath(js.RootNode, d, []string{"b", "c"})
	assert.NotEqual(t, -1, merged)
	assert.Equal(t, js.RootNode, merged)

	out.Reset()
	err = js.PrintNode(js.Nodes[js.RootNode], out)
	assert.NoError(t, err)
	assert.Equal(t, `{"a":1,"b":{"c":{"d":5}}}`, out.String())

	boolObj, err := js.AppendObject([]byte(`{"bool":true}`))
	assert.NoError(t, err)
	assert.NotEqual(t, -1, c)

	boolRef := js.Get(boolObj, []string{"bool"})
	assert.NotEqual(t, -1, boolRef)

	merged = js.MergeNodesWithPath(js.RootNode, boolRef, []string{"b", "c", "d"})
	assert.NotEqual(t, -1, merged)
	assert.Equal(t, js.RootNode, merged)

	out.Reset()
	err = js.PrintNode(js.Nodes[js.RootNode], out)
	assert.NoError(t, err)
	assert.Equal(t, `{"a":1,"b":{"c":{"d":true}}}`, out.String())
}

func TestJSON_AppendJSON(t *testing.T) {
	js := &JSON{}
	err := js.ParseObject([]byte(`{"a":1}`))
	assert.NoError(t, err)

	another := &JSON{}
	err = another.ParseObject([]byte(`{"c":3}`))
	assert.NoError(t, err)

	c, storageOffset, nodeOffset := js.AppendJSON(another)
	assert.NotEqual(t, -1, c)
	assert.Equal(t, 7, storageOffset)
	assert.Equal(t, 3, nodeOffset)

	merged := js.MergeNodes(js.RootNode, c)
	assert.NotEqual(t, -1, merged)
	assert.Equal(t, js.RootNode, merged)

	out := &bytes.Buffer{}
	err = js.PrintNode(js.Nodes[js.RootNode], out)
	assert.NoError(t, err)
	assert.Equal(t, `{"a":1,"c":3}`, out.String())
}

func TestJSON_GetArray(t *testing.T) {
	js := &JSON{}
	err := js.ParseArray([]byte(`[{"name":"Jens"},{"name":"Jannik"}]`))
	assert.NoError(t, err)
	jens := js.Get(js.RootNode, []string{"[0]", "name"})
	assert.NotEqual(t, -1, jens)
	out := &bytes.Buffer{}
	err = js.PrintNode(js.Nodes[jens], out)
	assert.NoError(t, err)
	assert.Equal(t, `"Jens"`, out.String())
	jannik := js.Get(js.RootNode, []string{"[1]", "name"})
	assert.NotEqual(t, -1, jannik)
	out.Reset()
	err = js.PrintNode(js.Nodes[jannik], out)
	assert.NoError(t, err)
	assert.Equal(t, `"Jannik"`, out.String())
	nonExistent := js.Get(js.RootNode, []string{"[2]", "name"})
	assert.Equal(t, -1, nonExistent)
}

func TestJSON_MergeObjects(t *testing.T) {
	js := &JSON{}
	err := js.ParseArray([]byte(`[{"name":"Jens"},{"pet":"dog"}]`))
	assert.NoError(t, err)
	merged := js.MergeObjects(js.Nodes[js.RootNode].ArrayValues)
	assert.NotEqual(t, -1, merged)
	out := &bytes.Buffer{}
	err = js.PrintNode(js.Nodes[merged], out)
	assert.NoError(t, err)
	assert.Equal(t, `{"name":"Jens","pet":"dog"}`, out.String())
}

func TestJSON_MergeObjectsDuplicates(t *testing.T) {
	js := &JSON{}
	err := js.ParseArray([]byte(`[{"name":"Jens"},{"pet":"dog"},{"name":"Jens"}]`))
	assert.NoError(t, err)
	merged := js.MergeObjects(js.Nodes[js.RootNode].ArrayValues)
	assert.NotEqual(t, -1, merged)
	out := &bytes.Buffer{}
	err = js.PrintNode(js.Nodes[merged], out)
	assert.NoError(t, err)
	assert.Equal(t, `{"name":"Jens","pet":"dog"}`, out.String())
}

func TestJSON_MergeObjectsDifferingDuplicates(t *testing.T) {
	js := &JSON{}
	err := js.ParseArray([]byte(`[{"name":"Jens"},{"pet":"dog"},{"name":"Jannik"}]`))
	assert.NoError(t, err)
	merged := js.MergeObjects(js.Nodes[js.RootNode].ArrayValues)
	assert.NotEqual(t, -1, merged)
	out := &bytes.Buffer{}
	err = js.PrintNode(js.Nodes[merged], out)
	assert.NoError(t, err)
	assert.Equal(t, `{"name":"Jannik","pet":"dog"}`, out.String())
}

func TestJSON_PrintObjectFlat(t *testing.T) {
	js := &JSON{}
	err := js.ParseObject([]byte(`{"name":"Jens","pet":"dog","age":30,"married":true,"height":1.8,"children":[{"name":"Jannik"},{"name":"Leonie"}],"address":{"street":"Musterstraße","number":123,"city":"Musterstadt"}}`))
	assert.NoError(t, err)
	out := &bytes.Buffer{}
	err = js.PrintObjectFlat(js.RootNode, out)
	assert.NoError(t, err)
	assert.Equal(t, `{"name":"Jens","pet":"dog","age":30,"married":true,"height":1.8}`, out.String())
}

func Benchmark_JsonParserJsonGet(b *testing.B) {
	input := []byte(`{"data":{"_entities":[{"stock":8},{"stock":2},{"stock":5}]}}`)
	expectedOut := []byte(`{"_entities":[{"stock":8},{"stock":2},{"stock":5}]}`)
	data := "data"
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		value, _, _, err := jsonparser.Get(input, data)
		if err != nil {
			b.Fatal(err)
		}
		if !bytes.Equal(expectedOut, value) {
			b.Fatal("not equal")
		}
	}
}

func BenchmarkJSON_ParsePrint(b *testing.B) {
	js := &JSON{}
	input := []byte(`{"data":{"_entities":[{"stock":8},{"stock":2},{"stock":5}]}}`)
	expectedOut := []byte(`{"_entities":[{"stock":8},{"stock":2},{"stock":5}]}`)
	dataPath := []string{"data"}
	out := &bytes.Buffer{}
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		err := js.ParseObject(input)
		if err != nil {
			b.Fatal(err)
		}
		ref := js.Get(js.RootNode, dataPath)
		out.Reset()
		err = js.PrintNode(js.Nodes[ref], out)
		if err != nil {
			b.Fatal(err)
		}
		if !bytes.Equal(expectedOut, out.Bytes()) {
			b.Fatal("not equal")
		}
	}
}

func BenchmarkJSON_MergeNodesNested(b *testing.B) {
	js := &JSON{}
	first := []byte(`{"a":1,"b":2,"c":{"d":4,"e":5,"f":6,"g":7,"h":8,"i":9,"j":10,"k":11,"l":12,"m":13,"n":14,"o":15,"p":16,"q":17,"r":18,"s":19,"t":20,"u":21,"v":22,"w":23,"x":24,"y":25,"z":26}}`)
	second := []byte(`{"c":{"e":5,"f":6,"g":7,"h":8,"i":9,"j":10,"k":11,"l":12,"m":13,"n":14,"o":15,"p":16,"q":17,"r":18,"s":19,"t":20,"u":21,"v":22,"w":23,"x":24,"y":25,"z":26}}`)
	third := []byte(`{"c":{"e":6,"f":7,"g":8,"h":9,"i":10,"j":11,"k":true,"l":13,"m":"Cosmo Rocks!","n":15,"o":16,"p":17,"q":18,"r":19,"s":20,"t":21,"u":22,"v":23,"w":24,"x":25,"y":26,"z":28}}`)
	expected := []byte(`{"a":1,"b":2,"c":{"d":4,"e":6,"f":7,"g":8,"h":9,"i":10,"j":11,"k":true,"l":13,"m":"Cosmo Rocks!","n":15,"o":16,"p":17,"q":18,"r":19,"s":20,"t":21,"u":22,"v":23,"w":24,"x":25,"y":26,"z":28}}`)
	out := &bytes.Buffer{}
	b.SetBytes(int64(len(first) + len(second) + len(third)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := js.ParseObject(first)
		if err != nil {
			b.Fatal(err)
		}
		ce, err := js.AppendObject(second)
		if err != nil {
			b.Fatal(err)
		}
		cef, err := js.AppendObject(third)
		if err != nil {
			b.Fatal(err)
		}
		js.MergeNodes(js.RootNode, ce)
		js.MergeNodes(js.RootNode, cef)
		out.Reset()
		err = js.PrintNode(js.Nodes[js.RootNode], out)
		if err != nil {
			b.Fatal(err)
		}
		if !bytes.Equal(expected, out.Bytes()) {
			b.Fatal("not equal")
		}
	}
}

func BenchmarkJSON_MergeNodesWithPath(b *testing.B) {
	js := &JSON{}
	first := []byte(`{"a":1}`)
	second := []byte(`{"c":3}`)
	third := []byte(`{"d":5}`)
	fourth := []byte(`{"bool":true}`)
	expected := []byte(`{"a":1,"b":{"c":{"d":true}}}`)
	out := &bytes.Buffer{}
	b.SetBytes(int64(len(first) + len(second) + len(third)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = js.ParseObject(first)
		c, _ := js.AppendObject(second)
		js.MergeNodesWithPath(js.RootNode, c, []string{"b"})
		d, _ := js.AppendObject(third)
		js.MergeNodesWithPath(js.RootNode, d, []string{"b", "c"})
		boolObj, _ := js.AppendObject(fourth)
		boolRef := js.Get(boolObj, []string{"bool"})
		js.MergeNodesWithPath(js.RootNode, boolRef, []string{"b", "c", "d"})
		out.Reset()
		err := js.PrintNode(js.Nodes[js.RootNode], out)
		if err != nil {
			b.Fatal(err)
		}
		if !bytes.Equal(expected, out.Bytes()) {
			b.Fatal("not equal")
		}
	}
}
