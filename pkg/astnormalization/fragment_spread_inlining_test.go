package astnormalization

import "testing"

func TestInlineFragments(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run(fragmentSpreadInline, testDefinition, `	
				subscription sub {
					...multipleSubscriptions
				}
				fragment multipleSubscriptions on Subscription {
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
				}`, `
				subscription sub {
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
				}
				fragment multipleSubscriptions on Subscription {
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
				}`)
	})
	t.Run("simple 2x", func(t *testing.T) {
		run(fragmentSpreadInline, testDefinition, `	
				subscription sub {
					...multipleSubscriptions
					...multipleSubscriptions
				}
				fragment multipleSubscriptions on Subscription {
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
				}`, `
				subscription sub {
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
				}
				fragment multipleSubscriptions on Subscription {
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
				}`)
	})
	t.Run("nested", func(t *testing.T) {
		run(fragmentSpreadInline, testDefinition, `	
				subscription sub {
					...frag1
				}
				fragment frag1 on Subscription {
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
					...frag2
				}
				fragment frag2 on Subscription {
					frag2Field
				}`, `
				subscription sub {
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
					frag2Field
				}
				fragment frag1 on Subscription {
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
					frag2Field
				}
				fragment frag2 on Subscription {
					frag2Field
				}`)
	})
	t.Run("2x nested", func(t *testing.T) {
		run(fragmentSpreadInline, testDefinition, `	
				subscription sub {
					...frag1
				}
				fragment frag1 on Subscription {
					newMessage {
						body
						sender
						...messageFrag
						sender
						sender
						...nestedMessageFrag
					}
					disallowedSecondRootField
					...frag2
				}
				fragment messageFrag on Message {
					body
					sender
				}
				fragment nestedMessageFrag on Message {
					body
					sender
				}
				fragment frag2 on Subscription {
					frag2Field
				}`, `
				subscription sub {
					newMessage {
						body
						sender
						body
						sender
						sender
						sender
						body
						sender
					}
					disallowedSecondRootField
					frag2Field
				}
				fragment frag1 on Subscription {
					newMessage {
						body
						sender
						body
						sender
						sender
						sender
						body
						sender
					}
					disallowedSecondRootField
					frag2Field
				}
				fragment messageFrag on Message {
					body
					sender
				}
				fragment nestedMessageFrag on Message {
					body
					sender
				}
				fragment frag2 on Subscription {
					frag2Field
				}`)
	})
	t.Run("mergeFields interface fields into selection if type implements inferface", func(t *testing.T) {
		run(fragmentSpreadInline, testDefinition, `
				{
					dog {
						...definedOnImplementorsButNotInterface
					}
				}
				fragment definedOnImplementorsButNotInterface on Pet {
					name
				}`, `
				{
					dog {
						name
					}
				}
				fragment definedOnImplementorsButNotInterface on Pet {
					name
				}`)
	})
	t.Run("inline fragments if fragment type definition implements enclosing type definition", func(t *testing.T) {
		run(fragmentSpreadInline, testDefinition, `
				fragment conflictingDifferingResponses on Pet {
					...dogFrag
					...catFrag
				}
				fragment dogFrag on Dog {
					someValue: nickname
				}
				fragment catFrag on Cat {
					someValue: meowVolume
				}`, `
				fragment conflictingDifferingResponses on Pet {
					... on Dog {
						someValue: nickname	
					}
					... on Cat {
						someValue: meowVolume
					}
				}
				fragment dogFrag on Dog {
					someValue: nickname
				}
				fragment catFrag on Cat {
					someValue: meowVolume
				}`)
	})
	t.Run("inline fragment if fragment type is member of enclosing union type", func(t *testing.T) {
		run(fragmentSpreadInline, testDefinition, `
				query conflictingDifferingResponses {
					catOrDog {
						...catDogFrag
					}
				}
				fragment catDogFrag on CatOrDog {
					...catFrag
					...dogFrag
				}
				fragment catFrag on Cat {
					someValue: meowVolume
				}
				fragment dogFrag on Dog {
					someValue: name
				}`, `
				query conflictingDifferingResponses {
					catOrDog {
						... on Cat {
							someValue: meowVolume							
						}
						... on Dog {
							someValue: name
						}
					}
				}
				fragment catDogFrag on CatOrDog {
					... on Cat {
						someValue: meowVolume							
					}
					... on Dog {
						someValue: name
					}
				}
				fragment catFrag on Cat {
					someValue: meowVolume
				}
				fragment dogFrag on Dog {
					someValue: name
				}`)
	})
	t.Run("inline fragment of outer enclosing type inside union fragment could be inlined if enclosing type is member of union fragment", func(t *testing.T) {
		run(fragmentSpreadInline, testDefinition, `
				{
					dog {
						...fragOnObject
						...fragOnInterface
						...fragOnUnion
					}
				}
				fragment fragOnObject on Dog {
					name
				}
				fragment fragOnInterface on Pet {
					name
				}
				fragment fragOnUnion on CatOrDog {
					... on Dog {
						name
					}
				}`, `
				{
				dog {
						name
						name
						... on CatOrDog {
							... on Dog {
								name
							}
						}
					}
				}
				fragment fragOnObject on Dog {
					name
				}
				fragment fragOnInterface on Pet {
					name
				}
				fragment fragOnUnion on CatOrDog {
					... on Dog {
						name
					}
				}`)
	})
	t.Run("type inside union inside type", func(t *testing.T) {
		run(fragmentSpreadInline, testDefinition, `
				{
					dog {
						...unionWithObjectFragment
					}
				}
				fragment catOrDogNameFragment on CatOrDog {
					... on Cat {
						meowVolume
					}
				}
				fragment unionWithObjectFragment on Dog {
					...catOrDogNameFragment
				}`, `
				{
					dog {
						... on CatOrDog {
							... on Cat {
								meowVolume
							}
						}
					}
				}
				fragment catOrDogNameFragment on CatOrDog {
					... on Cat {
						meowVolume
					}
				}
				fragment unionWithObjectFragment on Dog {
						... on CatOrDog {
							... on Cat {
								meowVolume
							}
						}
				}`)
	})
	t.Run("inline fragment inside union inside interface inside type", func(t *testing.T) {
		run(fragmentSpreadInline, testDefinition, `
				{
					dog {
						...unionWithInterface
					}
				}
				fragment unionWithInterface on Pet {
					...dogOrHumanFragment
				}
				fragment dogOrHumanFragment on DogOrHuman {
					... on Dog {
						barkVolume
					}
				}`, `
				{
					dog {
						... on DogOrHuman {
							... on Dog {
								barkVolume
							}
						}
					}
				}
				fragment unionWithInterface on Pet {
					... on DogOrHuman {
						... on Dog {
							barkVolume
						}
					}
				}
				fragment dogOrHumanFragment on DogOrHuman {
					... on Dog {
						barkVolume
					}
				}`)
	})
	t.Run("non intersecting interfaces shouldn't merge", func(t *testing.T) {
		run(fragmentSpreadInline, testDefinition, `
				{
					dog {
						...nonIntersectingInterfaces
					}
				}
				fragment nonIntersectingInterfaces on Pet {
					...sentientFragment
				}
				fragment sentientFragment on Sentient {
					name
				}`, `
				{
					dog {
						... on Sentient {
							name
						}
					}
				}
				fragment nonIntersectingInterfaces on Pet {
					... on Sentient {
						name
					}
				}
				fragment sentientFragment on Sentient {
					name
				}`)
	})
}
