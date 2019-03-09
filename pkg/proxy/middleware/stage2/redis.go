package stage2

type FakeRedis struct {
	Schemas map[string]string
}

func NewFakeRedis() *FakeRedis {
	return &FakeRedis{
		Schemas: make(map[string]string),
	}
}

func (f *FakeRedis) PutSchema(key string, schema string) {
	f.Schemas[key] = schema
}

func (f *FakeRedis) GetSchema(key string) (schema string, exists bool) {
	schema, exists = f.Schemas[key]
	return
}
