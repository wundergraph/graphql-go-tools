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
					nickname
				}`, `
				{
					dog {
						nickname
					}
				}
				fragment definedOnImplementorsButNotInterface on Pet {
					nickname
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
}
