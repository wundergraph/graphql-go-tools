package asyncapi

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astprinter"
)

func testFixtureFile(t *testing.T, name string) {
	asyncapiDoc, err := os.ReadFile(fmt.Sprintf("./fixtures/%s.yaml", name))
	require.NoError(t, err)
	doc, report := ImportAsyncAPIDocumentString(string(asyncapiDoc))
	if report.HasErrors() {
		t.Fatal(report.Error())
	}
	require.NoError(t, err)
	w := &bytes.Buffer{}
	err = astprinter.PrintIndent(doc, nil, []byte("  "), w)
	require.NoError(t, err)

	graphqlDoc, err := os.ReadFile(fmt.Sprintf("./fixtures/%s.graphql", name))
	require.NoError(t, err)
	require.Equal(t, string(graphqlDoc), w.String())
}

func TestImportAsyncAPIDocumentString(t *testing.T) {
	versions := []string{"2.0.0", "2.1.0", "2.2.0", "2.3.0", "2.4.0"}
	for _, version := range versions {
		asyncapiDoc, err := os.ReadFile(fmt.Sprintf("./fixtures/streetlights-kafka-%s.yaml", version))
		require.NoError(t, err)
		doc, report := ImportAsyncAPIDocumentString(string(asyncapiDoc))
		if report.HasErrors() {
			t.Fatal(report.Error())
		}
		require.NoError(t, err)
		w := &bytes.Buffer{}
		err = astprinter.PrintIndent(doc, nil, []byte("  "), w)
		require.NoError(t, err)

		graphqlDoc, err := os.ReadFile("./fixtures/streetlights-kafka-2.4.0-and-below.graphql")
		require.NoError(t, err)
		require.Equal(t, string(graphqlDoc), w.String())
	}
}

func TestImportAsyncAPIDocumentString_Fixtures(t *testing.T) {
	t.Run("email-service-2.0.0.yaml", func(t *testing.T) {
		testFixtureFile(t, "email-service-2.0.0")
	})

	t.Run("payment-system-sample-2.2.0.yaml", func(t *testing.T) {
		testFixtureFile(t, "payment-system-sample-2.2.0")
	})

	t.Run("payment-sample-2.0.0.yaml", func(t *testing.T) {
		testFixtureFile(t, "payment-sample-2.0.0")
	})

	t.Run("trading-sample-2.0.0.yaml", func(t *testing.T) {
		testFixtureFile(t, "trading-sample-2.0.0")
	})

	t.Run("print-service-api-2.0.0.yaml", func(t *testing.T) {
		testFixtureFile(t, "print-service-api-2.0.0")
	})

	t.Run("user-api-2.0.0.yaml", func(t *testing.T) {
		testFixtureFile(t, "user-api-2.0.0")
	})
}
