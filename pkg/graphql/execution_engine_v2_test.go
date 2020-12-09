package graphql

import (
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
)

func TestNewEngineV2Configuration(t *testing.T) {
	var engineConfig EngineV2Configuration

	t.Run("should create a new engine v2 config", func(t *testing.T) {
		schema, err := NewSchemaFromString(countriesSchema)
		require.NoError(t, err)

		engineConfig = NewEngineV2Configuration(schema)
		assert.Len(t, engineConfig.plannerConfig.DataSources, 0)
		assert.Len(t, engineConfig.plannerConfig.Fields, 0)
		assert.Equal(t, countriesSchema, engineConfig.plannerConfig.Schema)
	})

	t.Run("should successfully add a data source", func(t *testing.T) {
		ds := plan.DataSourceConfiguration{Custom: []byte("1")}
		engineConfig.AddDataSource(ds)

		assert.Len(t, engineConfig.plannerConfig.DataSources, 1)
		assert.Equal(t, ds, engineConfig.plannerConfig.DataSources[0])
	})

	t.Run("should successfully set all data sources", func(t *testing.T) {
		ds := []plan.DataSourceConfiguration{
			{Custom: []byte("2")},
			{Custom: []byte("3")},
			{Custom: []byte("4")},
		}
		engineConfig.SetDataSources(ds)

		assert.Len(t, engineConfig.plannerConfig.DataSources, 3)
		assert.Equal(t, ds, engineConfig.plannerConfig.DataSources)
	})

	t.Run("should successfully add a field config", func(t *testing.T) {
		fieldConfig := plan.FieldConfiguration{FieldName: "a"}
		engineConfig.AddFieldConfiguration(fieldConfig)

		assert.Len(t, engineConfig.plannerConfig.Fields, 1)
		assert.Equal(t, fieldConfig, engineConfig.plannerConfig.Fields[0])
	})

	t.Run("should successfully set all field configs", func(t *testing.T) {
		fieldConfigs := plan.FieldConfigurations{
			{FieldName: "b"},
			{FieldName: "c"},
			{FieldName: "d"},
		}
		engineConfig.SetFieldConfiguration(fieldConfigs)

		assert.Len(t, engineConfig.plannerConfig.Fields, 3)
		assert.Equal(t, fieldConfigs, engineConfig.plannerConfig.Fields)
	})
}

func TestEngineResponseWriter_AsHTTPResponse(t *testing.T) {
	rw := NewEngineResultWriter()
	_, err := rw.Write([]byte(`{"key": "value"}`))
	require.NoError(t, err)

	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	response := rw.AsHTTPResponse(http.StatusOK, headers)
	body, err := ioutil.ReadAll(response.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
	assert.Equal(t, `{"key": "value"}`, string(body))
}
