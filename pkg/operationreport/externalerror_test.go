package operationreport

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestPath_MarshalJSON(t *testing.T) {
	p1 := PathItem{
		Kind:       ArrayIndex,
		ArrayIndex: 1,
	}

	data, err := json.Marshal(p1)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "1" {
		t.Fatalf("want 1, got: %s", string(data))
	}

	var p2 PathItem
	err = json.Unmarshal([]byte("1"), &p2)
	if err != nil {
		t.Fatal(err)
	}

	if p2.Kind != ArrayIndex {
		t.Fatalf("want ArrayIndex, got: %d", p2.Kind)
	}
	if p2.ArrayIndex != 1 {
		t.Fatalf("want 1, got: %d", p2.ArrayIndex)
	}

	var p3 PathItem
	err = json.Unmarshal([]byte("\"field\""), &p3)
	if err != nil {
		t.Fatal(err)
	}

	if p3.Kind != FieldName {
		t.Fatalf("want FieldName, got: %d", p3.Kind)
	}
	if !bytes.Equal(p3.FieldName, []byte("field")) {
		t.Fatalf("want field, got: %s", p3.FieldName)
	}

	p4 := PathItem{
		Kind:      FieldName,
		FieldName: []byte("field"),
	}

	data, err = json.Marshal(p4)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "\"field\"" {
		t.Fatalf("want \"field\", got: %s", string(data))
	}

	var p5 PathItem
	err = json.Unmarshal([]byte("\"field"), &p5)
	if err == nil {
		t.Fatalf("want err, got nil")
	}
	err = nil
}
