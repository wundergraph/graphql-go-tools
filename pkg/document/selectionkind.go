//go:generate go-enum -f=$GOFILE

package document

// SelectionKind marks of which kind a Selectable is
/*
ENUM(
Field
InlineFragment
FragmentSpread
)
*/
type SelectionKind int
