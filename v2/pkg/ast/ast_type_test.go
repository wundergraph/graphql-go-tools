package ast

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDocument_TypesAreCompatibleIgnoringNullability(t *testing.T) {
	// Build a document with various type refs to test against.
	// We create types programmatically using the Add* methods.
	doc := NewSmallDocument()

	// Named types
	stringRef := doc.AddNamedType([]byte("String"))    // String
	intRef := doc.AddNamedType([]byte("Int"))          // Int
	stringRef2 := doc.AddNamedType([]byte("String"))   // String (second ref)
	profileRef := doc.AddNamedType([]byte("Profile"))  // Profile
	profileRef2 := doc.AddNamedType([]byte("Profile")) // Profile (second ref)

	// NonNull named types
	stringNonNullRef := doc.AddNonNullNamedType([]byte("String"))   // String!
	intNonNullRef := doc.AddNonNullNamedType([]byte("Int"))         // Int!
	profileNonNullRef := doc.AddNonNullNamedType([]byte("Profile")) // Profile!

	// List types
	listStringRef := doc.AddListType(doc.AddNamedType([]byte("String")))               // [String]
	listStringNonNullRef := doc.AddListType(doc.AddNonNullNamedType([]byte("String"))) // [String!]

	// NonNull list types
	nonNullListStringRef := doc.AddNonNullType(doc.AddListType(doc.AddNamedType([]byte("String"))))               // [String]!
	nonNullListStringNonNullRef := doc.AddNonNullType(doc.AddListType(doc.AddNonNullNamedType([]byte("String")))) // [String!]!

	tests := []struct {
		name    string
		left    int
		right   int
		deep    bool // expected result from TypesAreCompatibleDeep
		relaxed bool // expected result from TypesAreCompatibleIgnoringNullability
	}{
		{
			name:    "String vs String - identical",
			left:    stringRef,
			right:   stringRef2,
			deep:    true,
			relaxed: true,
		},
		{
			name:    "String! vs String - nullability differs",
			left:    stringNonNullRef,
			right:   stringRef,
			deep:    false,
			relaxed: true,
		},
		{
			name:    "String vs String! - nullability differs (reversed)",
			left:    stringRef,
			right:   stringNonNullRef,
			deep:    false,
			relaxed: true,
		},
		{
			name:    "String! vs Int! - different base types",
			left:    stringNonNullRef,
			right:   intNonNullRef,
			deep:    false,
			relaxed: false,
		},
		{
			name:    "String vs Int - different base types",
			left:    stringRef,
			right:   intRef,
			deep:    false,
			relaxed: false,
		},
		{
			name:    "String! vs Int - different base types, different nullability",
			left:    stringNonNullRef,
			right:   intRef,
			deep:    false,
			relaxed: false,
		},
		{
			name:    "Profile! vs Profile - non-scalar nullability differs",
			left:    profileNonNullRef,
			right:   profileRef,
			deep:    false,
			relaxed: true,
		},
		{
			name:    "Profile vs Profile - identical",
			left:    profileRef,
			right:   profileRef2,
			deep:    true,
			relaxed: true,
		},
		{
			name:    "[String] vs [String] - identical lists",
			left:    listStringRef,
			right:   listStringRef,
			deep:    true,
			relaxed: true,
		},
		{
			name:    "[String!] vs [String] - inner nullability differs",
			left:    listStringNonNullRef,
			right:   listStringRef,
			deep:    false,
			relaxed: true,
		},
		{
			name:    "[String]! vs [String] - outer nullability differs",
			left:    nonNullListStringRef,
			right:   listStringRef,
			deep:    false,
			relaxed: true,
		},
		{
			name:    "[String!]! vs [String] - both levels differ",
			left:    nonNullListStringNonNullRef,
			right:   listStringRef,
			deep:    false,
			relaxed: true,
		},
		{
			name:    "[String!]! vs [String!] - only outer differs",
			left:    nonNullListStringNonNullRef,
			right:   listStringNonNullRef,
			deep:    false,
			relaxed: true,
		},
		{
			name:    "[String] vs String - list vs non-list",
			left:    listStringRef,
			right:   stringRef,
			deep:    false,
			relaxed: false,
		},
		{
			name:    "[String]! vs String! - list vs non-list with NonNull",
			left:    nonNullListStringRef,
			right:   stringNonNullRef,
			deep:    false,
			relaxed: false,
		},
		{
			name:    "invalid ref left",
			left:    -1,
			right:   stringRef,
			deep:    false,
			relaxed: false,
		},
		{
			name:    "invalid ref right",
			left:    stringRef,
			right:   -1,
			deep:    false,
			relaxed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.deep, doc.TypesAreCompatibleDeep(tt.left, tt.right),
				"TypesAreCompatibleDeep")
			assert.Equal(t, tt.relaxed, doc.TypesAreCompatibleIgnoringNullability(tt.left, tt.right),
				"TypesAreCompatibleIgnoringNullability")
		})
	}
}
