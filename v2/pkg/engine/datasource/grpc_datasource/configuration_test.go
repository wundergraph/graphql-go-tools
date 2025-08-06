package grpcdatasource

import "testing"

func TestCompareKeyFields(t *testing.T) {
	tests := []struct {
		name     string
		left     string
		right    string
		expected bool
	}{
		{
			name:     "identical strings",
			left:     "id name",
			right:    "id name",
			expected: true,
		},
		{
			name:     "empty strings",
			left:     "",
			right:    "",
			expected: true,
		},
		{
			name:     "single field same",
			left:     "id",
			right:    "id",
			expected: true,
		},
		{
			name:     "single field different",
			left:     "id",
			right:    "name",
			expected: false,
		},
		{
			name:     "different order same fields",
			left:     "id name email",
			right:    "name email id",
			expected: true,
		},
		{
			name:     "comma separated same fields",
			left:     "id,name,email",
			right:    "name,email,id",
			expected: true,
		},
		{
			name:     "mixed separators same fields",
			left:     "id,name email",
			right:    "name email,id",
			expected: true,
		},
		{
			name:     "different number of fields",
			left:     "id name",
			right:    "id name email",
			expected: false,
		},
		{
			name:     "completely different fields",
			left:     "id name",
			right:    "email phone",
			expected: false,
		},
		{
			name:     "one field missing",
			left:     "id name email",
			right:    "id name",
			expected: false,
		},
		{
			name:     "extra whitespace handling",
			left:     "  id   name  ",
			right:    "name id",
			expected: true,
		},
		{
			name:     "extra whitespace with commas",
			left:     " id , name , email ",
			right:    "email,name,id",
			expected: true,
		},
		{
			name:     "multiple consecutive spaces",
			left:     "id    name     email",
			right:    "email name id",
			expected: true,
		},
		{
			name:     "mixed spaces and commas with whitespace",
			left:     "id,  name   email",
			right:    "email,  name id",
			expected: true,
		},
		{
			name:     "empty fields filtered out",
			left:     "id  ,  , name",
			right:    "name id",
			expected: true,
		},
		{
			name:     "only spaces and commas",
			left:     "  ,  ,  ",
			right:    "",
			expected: true,
		},
		{
			name:     "one empty one with fields",
			left:     "",
			right:    "id name",
			expected: false,
		},
		{
			name:     "duplicate fields in left",
			left:     "id name id",
			right:    "id name",
			expected: true,
		},
		{
			name:     "duplicate fields in both",
			left:     "id name id email",
			right:    "name email name id",
			expected: true,
		},
		{
			name:     "case sensitive comparison",
			left:     "ID name",
			right:    "id name",
			expected: false,
		},
		{
			name:     "single character fields",
			left:     "a b c",
			right:    "c a b",
			expected: true,
		},
		{
			name:     "complex field names",
			left:     "user_id created_at updated_at",
			right:    "updated_at user_id created_at",
			expected: true,
		},
		{
			name:     "simple selection set",
			left:     "id name { firstName lastName }",
			right:    "name id",
			expected: true,
		},
		{
			name:     "nested selection sets",
			left:     "id user { profile { name email } }",
			right:    "user id",
			expected: true,
		},
		{
			name:     "deeply nested selection sets",
			left:     "id foo { bar { baz { qux } } }",
			right:    "foo id",
			expected: true,
		},
		{
			name:     "multiple fields with selection sets",
			left:     "id name user { email } address { street city }",
			right:    "address user name id",
			expected: true,
		},
		{
			name:     "selection sets with different order",
			left:     "user { name email } id address { street }",
			right:    "id address user",
			expected: true,
		},
		{
			name:     "mixed fields and selection sets",
			left:     "id name user { profile { personal { firstName lastName } work { company } } } status",
			right:    "status user name id",
			expected: true,
		},
		{
			name:     "selection sets with spaces inside braces",
			left:     "id user {  name   email  } address",
			right:    "address user id",
			expected: true,
		},
		{
			name:     "selection sets with commas and spaces",
			left:     "id, user { name, email }, address { street, city }",
			right:    "address, user, id",
			expected: true,
		},
		{
			name:     "empty selection sets",
			left:     "id user { } address { }",
			right:    "address user id",
			expected: true,
		},
		{
			name:     "selection sets with nested empty braces",
			left:     "id user { profile { } contact { email } }",
			right:    "user id",
			expected: true,
		},
		{
			name:     "different nested structures same top level",
			left:     "user { name email } product { title price { amount currency } }",
			right:    "product { id sku } user { firstName lastName }",
			expected: true,
		},
		{
			name:     "selection sets vs simple fields different",
			left:     "id user { name }",
			right:    "id name",
			expected: false,
		},
		{
			name:     "only selection set fields",
			left:     "user { name email } address { street city }",
			right:    "address user",
			expected: true,
		},
		{
			name:     "complex real-world example",
			left:     "id user { profile { personal { name age } professional { company role } } } orders { items { product { name price } quantity } total }",
			right:    "orders user id",
			expected: true,
		},
		{
			name:     "unbalanced braces handled gracefully",
			left:     "id user { name email",
			right:    "id user { name email",
			expected: true,
		},
		{
			name:     "extra closing braces",
			left:     "id user { name } } extra",
			right:    "extra user id",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareKeyFields(tt.left, tt.right)
			if result != tt.expected {
				t.Errorf("compareKeyFields(%q, %q) = %v, expected %v",
					tt.left, tt.right, result, tt.expected)
			}
		})
	}
}

func TestKeySet(t *testing.T) {
	t.Run("add method", func(t *testing.T) {
		ks := make(keySet)
		ks.add("id", "name", "", "  ", "email")

		expected := keySet{
			"id":    struct{}{},
			"name":  struct{}{},
			"email": struct{}{},
		}

		if !ks.equals(expected) {
			t.Errorf("keySet.add() did not produce expected result. Got %v, expected %v", ks, expected)
		}
	})

	t.Run("equals method same sets", func(t *testing.T) {
		ks1 := keySet{
			"id":   struct{}{},
			"name": struct{}{},
		}
		ks2 := keySet{
			"id":   struct{}{},
			"name": struct{}{},
		}

		if !ks1.equals(ks2) {
			t.Error("keySet.equals() should return true for identical sets")
		}
	})

	t.Run("equals method different sizes", func(t *testing.T) {
		ks1 := keySet{
			"id":   struct{}{},
			"name": struct{}{},
		}
		ks2 := keySet{
			"id": struct{}{},
		}

		if ks1.equals(ks2) {
			t.Error("keySet.equals() should return false for sets of different sizes")
		}
	})

	t.Run("equals method different keys", func(t *testing.T) {
		ks1 := keySet{
			"id":   struct{}{},
			"name": struct{}{},
		}
		ks2 := keySet{
			"id":    struct{}{},
			"email": struct{}{},
		}

		if ks1.equals(ks2) {
			t.Error("keySet.equals() should return false for sets with different keys")
		}
	})

	t.Run("equals method empty sets", func(t *testing.T) {
		ks1 := make(keySet)
		ks2 := make(keySet)

		if !ks1.equals(ks2) {
			t.Error("keySet.equals() should return true for empty sets")
		}
	})
}

func TestStripSelectionSets(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no selection sets",
			input:    "id name email",
			expected: "id name email",
		},
		{
			name:     "simple selection set",
			input:    "id name { firstName lastName }",
			expected: "id name",
		},
		{
			name:     "nested selection sets",
			input:    "id user { profile { name email } }",
			expected: "id user",
		},
		{
			name:     "deeply nested selection sets",
			input:    "id foo { bar { baz { qux } } }",
			expected: "id foo",
		},
		{
			name:     "multiple selection sets",
			input:    "id user { name } address { street }",
			expected: "id user address",
		},
		{
			name:     "empty selection sets",
			input:    "id user { } address { }",
			expected: "id user address",
		},
		{
			name:     "selection sets with commas",
			input:    "id, user { name, email }, address",
			expected: "id user address",
		},
		{
			name:     "complex nested example",
			input:    "id user { profile { personal { name age } work { company } } } orders { items { product } }",
			expected: "id user orders",
		},
		{
			name:     "spaces inside selection sets",
			input:    "id user {  name   email  } address",
			expected: "id user address",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "only selection sets",
			input:    "user { name } address { street }",
			expected: "user address",
		},
		{
			name:     "unbalanced opening brace",
			input:    "id user { name email",
			expected: "id user",
		},
		{
			name:     "extra closing braces",
			input:    "id user { name } } extra",
			expected: "id user extra",
		},
		{
			name:     "consecutive spaces",
			input:    "id   name    user { email }",
			expected: "id name user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripSelectionSets(tt.input)
			if result != tt.expected {
				t.Errorf("stripSelectionSets(%q) = %q, expected %q",
					tt.input, result, tt.expected)
			}
		})
	}
}

// Benchmark tests for performance
func BenchmarkCompareKeyFields(b *testing.B) {
	testCases := []struct {
		name  string
		left  string
		right string
	}{
		{"simple", "id name", "name id"},
		{"complex", "id,name,email,phone,address", "address phone email name id"},
		{"long", "field1 field2 field3 field4 field5 field6 field7 field8 field9 field10", "field10 field9 field8 field7 field6 field5 field4 field3 field2 field1"},
		{"long and nested", "field1 field2 field3 field4 field5 field6 field7 field8 field9 field10 { field11 field12 field13 field14 field15 field16 field17 field18 field19 field20 }", "field20 field19 field18 field17 field16 field15 field14 field13 field12 field11 field10 field9 field8 field7 field6 field5 field4 field3 field2 field1"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				compareKeyFields(tc.left, tc.right)
			}
		})
	}
}
