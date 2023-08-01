package operationreport

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func TestPath_MarshalJSON(t *testing.T) {
	p1 := ast.PathItem{
		Kind:       ast.ArrayIndex,
		ArrayIndex: 1,
	}

	data, err := json.Marshal(p1)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "1" {
		t.Fatalf("want 1, got: %s", string(data))
	}

	var p2 ast.PathItem
	err = json.Unmarshal([]byte("1"), &p2)
	if err != nil {
		t.Fatal(err)
	}

	if p2.Kind != ast.ArrayIndex {
		t.Fatalf("want ArrayIndex, got: %d", p2.Kind)
	}
	if p2.ArrayIndex != 1 {
		t.Fatalf("want 1, got: %d", p2.ArrayIndex)
	}

	var p3 ast.PathItem
	err = json.Unmarshal([]byte("\"field\""), &p3)
	if err != nil {
		t.Fatal(err)
	}

	if p3.Kind != ast.FieldName {
		t.Fatalf("want FieldName, got: %d", p3.Kind)
	}
	if !bytes.Equal(p3.FieldName, []byte("field")) {
		t.Fatalf("want field, got: %s", p3.FieldName)
	}

	p4 := ast.PathItem{
		Kind:      ast.FieldName,
		FieldName: []byte("field"),
	}

	data, err = json.Marshal(p4)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "\"field\"" {
		t.Fatalf("want \"field\", got: %s", string(data))
	}

	var p5 ast.PathItem
	err = json.Unmarshal([]byte("\"field"), &p5)
	if err == nil {
		t.Fatalf("want err, got nil")
	}
}
