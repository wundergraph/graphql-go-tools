package openapi

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/TykTechnologies/graphql-go-tools/pkg/astprinter"
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
		name = strings.TrimSuffix(name, ".yaml")
	} else if strings.HasSuffix(name, ".json") {
		name = strings.TrimSuffix(name, ".json")
	} else {
		require.Fail(t, "unrecognized file: %s", name)
	}

	graphqlDoc, err := os.ReadFile(fmt.Sprintf("./fixtures/%s/%s.graphql", version, name))
	fmt.Println(w.String())
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

	t.Run("unnamed-object.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "unnamed-object.yaml")
	})

	t.Run("carts-api-oas_tt_10604.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "carts-api-oas_tt_10604.yaml")
	})

	t.Run("tt-10696.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "tt-10696.yaml")
	})

	t.Run("tt-10696-unnamed-object.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "tt-10696-unnamed-object.yaml")
	})

	t.Run("tt-10696-unnamed-array-of-objects.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "tt-10696-unnamed-array-of-objects.yaml")
	})

	t.Run("tt-10696-unnamed-array-of-primitive-types.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "tt-10696-unnamed-array-of-primitive-types.yaml")
	})

	t.Run("enums-query.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "enums-query.yaml")
	})

	t.Run("enums-mutation.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "enums-mutation.yaml")
	})

	t.Run("enum-component.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "enum-component.yaml")
	})

	t.Run("enum-component-mutation.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "enum-component-mutation.yaml")
	})

	t.Run("enum-properties.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "enum-properties.yaml")
	})

	t.Run("oneOf-input-type.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "oneOf-input-type.yaml")
	})

	t.Run("oneOf-response-type.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "oneOf-response-type.yaml")
	})

	t.Run("oneOf-response-type-composition.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "oneOf-response-type-composition.yaml")
	})

	t.Run("allOf-input-type.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "allOf-input-type.yaml")
	})

	t.Run("allOf-response-type.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "allOf-response-type.yaml")
	})

	t.Run("allOf-input-type-composition.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "allOf-input-type-composition.yaml")
	})

	t.Run("allOf-response-type-composition.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "allOf-response-type-composition.yaml")
	})

	t.Run("allOf-query-composition.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "allOf-query-composition.yaml")
	})

	t.Run("allOf-query-response-type.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "allOf-query-response-type.yaml")
	})

	t.Run("anyOf-input-type.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "anyOf-input-type.yaml")
	})

	t.Run("anyOf-response-type.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "anyOf-response-type.yaml")
	})

	t.Run("anyOf-query-response-type.yaml", func(t *testing.T) {
		testFixtureFile(t, "v3.0.0", "anyOf-query-response-type.yaml")
	})
}
