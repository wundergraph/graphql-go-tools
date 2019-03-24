package proxy

// SchemaProvider exists because it's not usually the case for the handler to keep the schema around
// Think multi tenant SaaS applications where a handler might handle schemas for many tenants
// In case you just want to use one single schema simply use StaticSchemaProvider
type SchemaProvider interface {
	GetSchema(requestURI []byte) []byte
}

type StaticSchemaProvider struct {
	schema []byte
}

func (s StaticSchemaProvider) GetSchema(requestURI []byte) []byte {
	return s.schema
}

func NewStaticSchemaProvider(schema []byte) *StaticSchemaProvider {
	return &StaticSchemaProvider{
		schema: schema,
	}
}
