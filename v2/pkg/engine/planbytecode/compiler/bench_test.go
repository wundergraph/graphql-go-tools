package compiler

import (
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/planbytecode"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

var benchmarkSink int

func BenchmarkResponseTreeWalk(b *testing.B) {
	p := benchmarkPlan()
	response := p.Response

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkSink += walkFetchTree(response.Fetches)
		benchmarkSink += walkNodeTree(response.Data)
	}
}

func BenchmarkBytecodeOpWalk(b *testing.B) {
	program, err := Compile(benchmarkPlan())
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		count := 0

		for _, op := range program.Ops {
			switch op.Code {
			case planbytecode.OpFetchSubgraph, planbytecode.OpPasteAtPointer, planbytecode.OpProjectField, planbytecode.OpEmitLiteral:
				count++
			}
		}

		benchmarkSink += count
	}
}

func BenchmarkCompileStaticPlan(b *testing.B) {
	p := benchmarkPlan()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		program, err := Compile(p)

		if err != nil {
			b.Fatal(err)
		}

		benchmarkSink += len(program.Ops)
	}
}

func benchmarkPlan() *plan.SynchronousResponsePlan {
	return syncResponsePlan(
		resolve.Sequence(
			resolve.SingleWithPath(singleFetch("users", "query Users { user { id name reviews { body score } } }", []string{"data"}, nil), "query.user"),
			resolve.Parallel(
				resolve.SingleWithPath(singleFetch("reviews", "query Reviews { reviews { body score } }", []string{"data"}, []string{"reviews"}), "query.user.reviews"),
				resolve.SingleWithPath(singleFetch("products", "query Products { products { upc name } }", []string{"data"}, []string{"products"}), "query.user.products"),
				resolve.SingleWithPath(singleFetch("inventory", "query Inventory { inventory { inStock } }", []string{"data"}, []string{"inventory"}), "query.user.inventory"),
			),
		),
		rootObject(
			field("user", objectValue("user",
				field("id", stringValue("id")),
				field("name", stringValue("name", true)),
				field("reviews", arrayValue("reviews",
					rootObject(
						field("body", stringValue("body", true)),
						field("score", integerValue("score", true)),
					),
				)),
				field("kind", staticStringAt("kind", "User")),
			)),
		),
	)
}

func walkFetchTree(node *resolve.FetchTreeNode) int {
	if node == nil {
		return 0
	}

	count := 1

	for _, child := range node.ChildNodes {
		count += walkFetchTree(child)
	}

	return count
}

func walkNodeTree(node resolve.Node) int {
	switch n := node.(type) {
	case nil:
		return 0
	case *resolve.Object:
		count := 1

		for _, field := range n.Fields {
			count++
			count += walkNodeTree(field.Value)
		}

		return count
	case *resolve.Array:
		return 1 + walkNodeTree(n.Item)
	default:
		return 1
	}
}
