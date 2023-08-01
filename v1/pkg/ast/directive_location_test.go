package ast

import "testing"

func TestDirectiveLocations(t *testing.T) {
	locations := DirectiveLocations{}
	for i := range locations.storage {
		if locations.storage[i] == true {
			t.Fatal("want false")
		}
	}

	locations.Set(DirectiveLocationUnknown)
	locations.Set(ExecutableDirectiveLocationQuery)

	if locations.Get(ExecutableDirectiveLocationQuery) != true {
		t.Fatal("want true")
	}

	locations.Set(ExecutableDirectiveLocationMutation)

	if locations.Get(ExecutableDirectiveLocationMutation) != true {
		t.Fatal("want true")
	}

	locations.Set(TypeSystemDirectiveLocationEnum)

	iter := locations.Iterable()
	if !iter.Next() {
		t.Fatal("want next")
	}
	if iter.Value() != ExecutableDirectiveLocationQuery {
		t.Fatal("want ExecutableDirectiveLocationQuery")
	}
	if !iter.Next() {
		t.Fatal("want next")
	}
	if iter.Value() != ExecutableDirectiveLocationMutation {
		t.Fatal("want ExecutableDirectiveLocationMutation")
	}
	if !iter.Next() {
		t.Fatal("want next")
	}
	if iter.Value() != TypeSystemDirectiveLocationEnum {
		t.Fatal("want TypeSystemDirectiveLocationEnum")
	}
	if iter.Next() {
		t.Fatal("want false")
	}

	locations.Unset(ExecutableDirectiveLocationMutation)

	if locations.Get(ExecutableDirectiveLocationMutation) == true {
		t.Fatal("want false")
	}

	locations.Unset(TypeSystemDirectiveLocationEnum)
	if locations.Get(TypeSystemDirectiveLocationEnum) == true {
		t.Fatal("want false")
	}
}
