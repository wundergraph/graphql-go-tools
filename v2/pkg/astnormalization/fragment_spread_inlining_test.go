package astnormalization

import "testing"

func TestInlineFragments(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run(t, fragmentSpreadInline, testDefinition, `	
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
	t.Run("simple with directive", func(t *testing.T) {
		run(t, fragmentSpreadInline, testDefinition, `	
				subscription sub {
					...multipleSubscriptions @include(if: true)
				}
				fragment multipleSubscriptions on Subscription {
					newMessage {
						body
						sender
					}
					disallowedSecondRootField
				}`, `
				subscription sub {
					... on Subscription @include(if: true) {
						newMessage {
							body
							sender
						}
						disallowedSecondRootField
					}
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
		run(t, fragmentSpreadInline, testDefinition, `	
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
		run(t, fragmentSpreadInline, testDefinition, `	
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
					...frag2
				}
				fragment frag2 on Subscription {
					frag2Field
				}`)
	})
	t.Run("2x nested", func(t *testing.T) {
		run(t, fragmentSpreadInline, testDefinition, `	
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
				}`)
	})
	t.Run("5x nested", func(t *testing.T) {
		run(t, fragmentSpreadInline, `
				schema {
					query: Query
				}

				type Query {
					foo: Foo
				}

				type Foo {
					fooName: String
					some: Some
					bar: Bar
				}

				type Bar {
					barName: String
					baz: Baz
				}

				type Baz {
					bazName: String
					some: Some
				}

				type Some {
					something: String
				}
				`, `	
				query q {
					...QueryFragment
				}

				fragment QueryFragment on Query {
					foo {
						...FooFragment
					}
				}

				fragment SomeFragment on Some {
					something
				}

				fragment FooFragment on Foo {
					fooName
					some {
						...SomeFragment
					}
					bar {
						...BarFragment
					}
				}
				
				fragment BarFragment on Bar {
					barName
					baz {
						...BazFragment
					}
				}
				
				fragment BazFragment on Baz {
					bazName
					some {
						...SomeFragment
					}
				}`, `	
				query q {
					foo {
						fooName
						some {
							something
						}
						bar {
							barName
							baz {
								bazName
								some {
									something
								}
							}
						}
					}
				}

				fragment QueryFragment on Query {
					foo {
						...FooFragment
					}
				}

				fragment SomeFragment on Some {
					something
				}

				fragment FooFragment on Foo {
					fooName
					some {
						...SomeFragment
					}
					bar {
						...BarFragment
					}
				}
				
				fragment BarFragment on Bar {
					barName
					baz {
						...BazFragment
					}
				}
				
				fragment BazFragment on Baz {
					bazName
					some {
						...SomeFragment
					}
				}`)
	})
	t.Run("mergeFields interface fields into selection if type implements inferface", func(t *testing.T) {
		run(t, fragmentSpreadInline, testDefinition, `
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
		run(t, fragmentSpreadInline, testDefinition, `
				query conflictingDifferingResponses {
					pet {
						...conflictingDifferingResponses
					}
				}
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
				query conflictingDifferingResponses {
					pet {
						... on Dog {
							someValue: nickname	
						}
						... on Cat {
							someValue: meowVolume
						}
					}
				}
				fragment conflictingDifferingResponses on Pet {
					...dogFrag
					...catFrag
				}
				fragment dogFrag on Dog {
					someValue: nickname
				}
				fragment catFrag on Cat {
					someValue: meowVolume
				}`)
	})
	t.Run("inline fragment if fragment type is member of enclosing union type", func(t *testing.T) {
		run(t, fragmentSpreadInline, testDefinition, `
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
					...catFrag
					...dogFrag
				}
				fragment catFrag on Cat {
					someValue: meowVolume
				}
				fragment dogFrag on Dog {
					someValue: name
				}`)
	})
	t.Run("inline fragment of outer enclosing type inside union fragment could be inlined if enclosing type is member of union fragment", func(t *testing.T) {
		run(t, fragmentSpreadInline, testDefinition, `
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
		run(t, fragmentSpreadInline, testDefinition, `
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
					...catOrDogNameFragment
				}`)
	})
	t.Run("inline fragment inside union inside interface inside type", func(t *testing.T) {
		run(t, fragmentSpreadInline, testDefinition, `
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
					...dogOrHumanFragment
				}
				fragment dogOrHumanFragment on DogOrHuman {
					... on Dog {
						barkVolume
					}
				}`)
	})
	t.Run("inline fragment inside interface inside union inside type", func(t *testing.T) {
		run(t, fragmentSpreadInline, testDefinition, `
				{
					dog {
						...interfaceWithUnion
					}
				}
				fragment interfaceWithUnion on DogOrHuman {
					...petFragment
				}
				fragment petFragment on Pet {
					... on Dog {
						barkVolume
					}
				}`, `
				{
					dog {
						... on DogOrHuman {
							... on Pet {
								... on Dog {
									barkVolume
								}
							}
						}
					}
				}
				fragment interfaceWithUnion on DogOrHuman {
					...petFragment
				}
				fragment petFragment on Pet {
					... on Dog {
						barkVolume
					}
				}`)
	})
	t.Run("non intersecting interfaces shouldn't merge", func(t *testing.T) {
		run(t, fragmentSpreadInline, testDefinition, `
				{
					dog {
						...nonIntersectingInterfaces
					}
				}
				fragment nonIntersectingInterfaces on Pet {
					...sentientFragment # invalid fragment spread, but doesn't matter for test
				}
				fragment sentientFragment on Sentient {
					name
				}`, `
				{
					dog {
						...sentientFragment # invalid fragment spread, but doesn't matter for test
					}
				}
				fragment nonIntersectingInterfaces on Pet {
					...sentientFragment
				}
				fragment sentientFragment on Sentient {
					name
				}`, true)
	})
}
