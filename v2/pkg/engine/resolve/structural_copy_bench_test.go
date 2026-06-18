package resolve

import (
	"testing"

	"github.com/wundergraph/astjson"
)

var structuralCopyBenchSink *astjson.Value

func BenchmarkStructuralCopy_L1Write_NoTransform(b *testing.B) {
	benchmarkStructuralCopy(b, structuralCopyBenchInputAlias, nil, func(loader *Loader, value *astjson.Value, provides *Object) *astjson.Value {
		return loader.structuralCopyNormalizedPassthrough(value, provides)
	})
}

func BenchmarkStructuralCopy_L1Write_WithTransform(b *testing.B) {
	benchmarkStructuralCopy(b, structuralCopyBenchInputAlias, structuralCopyBenchProvidesData(), func(loader *Loader, value *astjson.Value, provides *Object) *astjson.Value {
		return loader.structuralCopyNormalizedPassthrough(value, provides)
	})
}

func BenchmarkStructuralCopy_L1Read_NoTransform(b *testing.B) {
	benchmarkStructuralCopy(b, structuralCopyBenchInputSchema, nil, func(loader *Loader, value *astjson.Value, provides *Object) *astjson.Value {
		return loader.structuralCopyDenormalizedPassthrough(value, provides)
	})
}

func BenchmarkStructuralCopy_L1Read_WithTransform(b *testing.B) {
	benchmarkStructuralCopy(b, structuralCopyBenchInputSchema, structuralCopyBenchProvidesData(), func(loader *Loader, value *astjson.Value, provides *Object) *astjson.Value {
		return loader.structuralCopyDenormalizedPassthrough(value, provides)
	})
}

func BenchmarkStructuralCopy_L2Read_NoTransform(b *testing.B) {
	benchmarkStructuralCopy(b, structuralCopyBenchInputSchema, nil, func(loader *Loader, value *astjson.Value, provides *Object) *astjson.Value {
		return loader.structuralCopyDenormalized(value, provides)
	})
}

func BenchmarkStructuralCopy_L2Read_WithTransform(b *testing.B) {
	benchmarkStructuralCopy(b, structuralCopyBenchInputSchema, structuralCopyBenchProvidesData(), func(loader *Loader, value *astjson.Value, provides *Object) *astjson.Value {
		return loader.structuralCopyDenormalized(value, provides)
	})
}

func BenchmarkStructuralCopy_L2Write_NoTransform(b *testing.B) {
	benchmarkStructuralCopy(b, structuralCopyBenchInputAlias, nil, func(loader *Loader, value *astjson.Value, provides *Object) *astjson.Value {
		return loader.structuralCopyNormalized(value, provides)
	})
}

func BenchmarkStructuralCopy_L2Write_WithTransform(b *testing.B) {
	benchmarkStructuralCopy(b, structuralCopyBenchInputAlias, structuralCopyBenchProvidesData(), func(loader *Loader, value *astjson.Value, provides *Object) *astjson.Value {
		return loader.structuralCopyNormalized(value, provides)
	})
}

func benchmarkStructuralCopy(b *testing.B, inputs map[string]string, provides *Object, copyFn func(*Loader, *astjson.Value, *Object) *astjson.Value) {
	b.Helper()

	for name, input := range inputs {
		b.Run(name, func(b *testing.B) {
			sourceLoader, releaseSource := newLoaderCacheTransformTestLoader()
			defer releaseSource()
			copyLoader, releaseCopy := newLoaderCacheTransformTestLoader()
			defer releaseCopy()

			value := parseStructuralCopyBenchValue(b, sourceLoader, input)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				copyLoader.jsonArena.Reset()
				structuralCopyBenchSink = copyFn(copyLoader, value, provides)
			}
		})
	}
}

func parseStructuralCopyBenchValue(b *testing.B, loader *Loader, data string) *astjson.Value {
	b.Helper()

	value, err := astjson.ParseBytesWithArena(loader.jsonArena, []byte(data))
	if err != nil {
		b.Fatal(err)
	}
	return value
}

func structuralCopyBenchProvidesData() *Object {
	return &Object{
		Fields: []*Field{
			{Name: []byte("__typename"), Value: &String{}},
			{Name: []byte("productID"), OriginalName: []byte("id"), Value: &String{}},
			{Name: []byte("displayName"), OriginalName: []byte("name"), Value: &String{}},
			{Name: []byte("cost"), OriginalName: []byte("price"), Value: &Float{}},
			{Name: []byte("available"), OriginalName: []byte("inStock"), Value: &Boolean{}},
			{Name: []byte("ratingScore"), OriginalName: []byte("rating"), Value: &Float{}},
			{Name: []byte("skuCode"), OriginalName: []byte("sku"), Value: &String{}},
			{Name: []byte("brandName"), OriginalName: []byte("brand"), Value: &String{}},
			{Name: []byte("categoryName"), OriginalName: []byte("category"), Value: &String{}},
			{Name: []byte("shippingClass"), OriginalName: []byte("shipping"), Value: &String{}},
			{
				Name: []byte("details"),
				Value: &Object{
					Fields: []*Field{
						{Name: []byte("weightKg"), OriginalName: []byte("weight"), Value: &Float{}},
						{Name: []byte("countryCode"), OriginalName: []byte("country"), Value: &String{}},
					},
				},
			},
			{
				Name: []byte("variants"),
				Value: &Array{
					Item: &Object{
						Fields: []*Field{
							{Name: []byte("variantID"), OriginalName: []byte("id"), Value: &String{}},
							{Name: []byte("variantName"), OriginalName: []byte("name"), Value: &String{}},
						},
					},
				},
			},
		},
	}
}

var structuralCopyBenchInputAlias = map[string]string{
	"Small":  `{"__typename":"Product","productID":"p1","displayName":"Table","cost":42.5,"available":true,"ratingScore":4.8,"skuCode":"SKU-1","brandName":"Acme","categoryName":"Furniture","shippingClass":"standard"}`,
	"Nested": `{"__typename":"Product","productID":"p1","displayName":"Table","cost":42.5,"available":true,"ratingScore":4.8,"skuCode":"SKU-1","brandName":"Acme","categoryName":"Furniture","shippingClass":"standard","details":{"weightKg":12.4,"countryCode":"DE"}}`,
	"Array":  `{"__typename":"Product","productID":"p1","displayName":"Table","cost":42.5,"available":true,"ratingScore":4.8,"skuCode":"SKU-1","brandName":"Acme","categoryName":"Furniture","shippingClass":"standard","variants":[{"variantID":"v1","variantName":"Oak"},{"variantID":"v2","variantName":"Walnut"}]}`,
}

var structuralCopyBenchInputSchema = map[string]string{
	"Small":  `{"__typename":"Product","id":"p1","name":"Table","price":42.5,"inStock":true,"rating":4.8,"sku":"SKU-1","brand":"Acme","category":"Furniture","shipping":"standard"}`,
	"Nested": `{"__typename":"Product","id":"p1","name":"Table","price":42.5,"inStock":true,"rating":4.8,"sku":"SKU-1","brand":"Acme","category":"Furniture","shipping":"standard","details":{"weight":12.4,"country":"DE"}}`,
	"Array":  `{"__typename":"Product","id":"p1","name":"Table","price":42.5,"inStock":true,"rating":4.8,"sku":"SKU-1","brand":"Acme","category":"Furniture","shipping":"standard","variants":[{"id":"v1","name":"Oak"},{"id":"v2","name":"Walnut"}]}`,
}
