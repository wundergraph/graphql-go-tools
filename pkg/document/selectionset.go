package document

import "fmt"

// SelectionSet as specified in:
// http://facebook.github.io/graphql/draft/#SelectionSet
type SelectionSet []Selection

// Fields returns all Fields from the SelectionSet
func (s SelectionSet) Fields(fragmentDefinitions FragmentDefinitions) (fields Fields, err error) {
	for _, selection := range s {
		switch selection.OfKind() {
		case SelectionKindField:
			field, ok := selection.(Field)
			if !ok {
				return nil, fmt.Errorf("selectionSet.Fields():expected document.Field, got %+v", selection)
			}
			fields = append(fields, field)
		case SelectionKindFragmentSpread:
			fragmentSpread, isFragmentSpread := selection.(FragmentSpread)
			if !isFragmentSpread {
				return nil, fmt.Errorf("selectionSet.Fields():expected document.FragmentSpread, got %+v", selection)
			}
			fragment, fragmentExists := fragmentDefinitions.GetByName(fragmentSpread.FragmentName)
			if !fragmentExists {
				return
			}
			fragmentSpreadFields, err := fragment.SelectionSet.Fields(fragmentDefinitions)
			if err != nil {
				return nil, err
			}
			fields = append(fields, fragmentSpreadFields...)
		case SelectionKindInlineFragment:
			inlineFragment, isInlineFragment := selection.(InlineFragment)
			if !isInlineFragment {
				return nil, fmt.Errorf("selectionSet.Fields():expected document.InlineFragment, got %+v", selection)
			}
			inlineFragmentFields, err := inlineFragment.SelectionSet.Fields(fragmentDefinitions)
			if err != nil {
				return nil, err
			}
			fields = append(fields, inlineFragmentFields...)
		}
	}

	return
}
