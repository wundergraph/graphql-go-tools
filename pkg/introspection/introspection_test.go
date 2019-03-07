package introspection

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
)

func TestIntrospectionSerialization(t *testing.T) {
	inputData, err := ioutil.ReadFile("./testdata/swapi_introspection_response.json")
	if err != nil {
		panic(err)
	}

	var res Response

	err = json.Unmarshal(inputData, &res)
	if err != nil {
		panic(err)
	}

	outputData, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		panic(err)
	}

	err = ioutil.WriteFile("./testdata/out_swapi_introspection_response.json", outputData, os.ModePerm)
	if err != nil {
		panic(err)
	}
}
