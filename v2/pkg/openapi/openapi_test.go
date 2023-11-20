package openapi

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astprinter"
	"github.com/stretchr/testify/require"
)

func testFixtureFile(t *testing.T, version, name string) {
	asyncapiDoc, err := os.ReadFile(fmt.Sprintf("./fixtures/%s/%s", version, name))
	require.NoError(t, err)

	doc, report := ImportOpenAPIDocumentString(string(asyncapiDoc))
	if report.HasErrors() {
		t.Fatal(report.Error())
	}
	require.NoError(t, err)
	w := &bytes.Buffer{}
	err = astprinter.PrintIndent(doc, nil, []byte("  "), w)
	require.NoError(t, err)

	if strings.HasSuffix(name, ".yaml") {
		name = strings.Trim(name, ".yaml")
	} else if strings.HasSuffix(name, ".json") {
		name = strings.Trim(name, ".json")
	} else {
		require.Fail(t, "unrecognized file: %s", name)
	}

	graphqlDoc, err := os.ReadFile(fmt.Sprintf("./fixtures/%s/%s.graphql", version, name))
	require.NoError(t, err)
	require.Equal(t, string(graphqlDoc), w.String())
}

func TestOpenAPI_v3_0_0(t *testing.T) {
	t.Run("petstore-expanded.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "petstore-expanded.yaml")
	})

	t.Run("petstore.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "petstore.yaml")
	})

	t.Run("example_oas7.json", func(t *testing.T) {
		// Source: https://github.com/IBM/openapi-to-graphql/blob/master/packages/openapi-to-graphql/test/fixtures/example_oas7.json
		testFixtureFile(t, "v3.0.0", "example_oas7.json")
	})

	t.Run("EmployeesApiBasic.yaml", func(t *testing.T) {
		// Source https://github.com/zosconnect/test-samples/blob/main/oas/EmployeesApiBasic.yaml
		testFixtureFile(t, "v3.0.0", "EmployeesApiBasic.yaml")
	})

	t.Run("EmployeesApi.yaml", func(t *testing.T) {
		// Source https://github.com/zosconnect/test-samples/blob/main/oas/EmployeesApiBasic.yaml
		testFixtureFile(t, "v3.0.0", "EmployeesApi.yaml")
	})

	t.Run("example_oas3.json", func(t *testing.T) {
		// Source: https://github.com/IBM/openapi-to-graphql/blob/master/packages/openapi-to-graphql/test/fixtures/example_oas3.json
		testFixtureFile(t, "v3.0.0", "example_oas3.json")
	})

	t.Run("carts-api-oas.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "carts-api-oas.yaml")
	})
}
