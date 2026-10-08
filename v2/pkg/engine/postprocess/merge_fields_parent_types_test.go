package postprocess

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// The rows below build the response tree as the planner emits it for the GraphQL in the row comment,
// run the merge pass, and compare the whole tree. Rows with a render table also resolve the merged
// tree against JSON inputs that differ in the runtime parent types.
//
// Notation in comments: `field@T` is a field selected inside `... on T`, a group `[d:T]` is a
// parent type condition at depth d, and groups are ORed.
func TestMergeFields_ParentTypeConditions(t *testing.T) {
	t.Parallel()

	type render struct {
		input string
		want  string
	}

	cases := []struct {
		name     string
		input    *resolve.Object
		expected *resolve.Object
		renders  []render
	}{
		{
			// [S1] The non-matching fragment comes first:
			//   product {
			//     category { id }
			//     ... on B {
			//       category {
			//         owner { name }
			//       }
			//     }
			//     ... on A {
			//       category {
			//         owner { name }
			//       }
			//     }
			//   }
			// owner and name must stay reachable for both A and B.
			name: "[S1] non-matching fragment first",
			input: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("product"),
						Value: &resolve.Object{
							Path: []string{"product"},
							Fields: []*resolve.Field{
								{
									Name: []byte("category"),
									Value: &resolve.Object{
										Path: []string{"category"},
										Fields: []*resolve.Field{
											{Name: []byte("id"), Value: &resolve.String{Path: []string{"id"}}},
										},
									},
								},
								{
									Name:        []byte("category"),
									OnTypeNames: [][]byte{[]byte("B")},
									Value: &resolve.Object{
										Path: []string{"category"},
										Fields: []*resolve.Field{
											{
												Name: []byte("owner"),
												Value: &resolve.Object{
													Path: []string{"owner"},
													Fields: []*resolve.Field{
														{Name: []byte("name"), Value: &resolve.String{Path: []string{"name"}}},
													},
												},
											},
										},
									},
								},
								{
									Name:        []byte("category"),
									OnTypeNames: [][]byte{[]byte("A")},
									Value: &resolve.Object{
										Path: []string{"category"},
										Fields: []*resolve.Field{
											{
												Name: []byte("owner"),
												Value: &resolve.Object{
													Path: []string{"owner"},
													Fields: []*resolve.Field{
														{Name: []byte("name"), Value: &resolve.String{Path: []string{"name"}}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("product"),
						Value: &resolve.Object{
							Path: []string{"product"},
							Fields: []*resolve.Field{
								{
									Name: []byte("category"),
									Value: &resolve.Object{
										Path: []string{"category"},
										Fields: []*resolve.Field{
											{Name: []byte("id"), Value: &resolve.String{Path: []string{"id"}}},
											{
												Name: []byte("owner"),
												ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
													{{Depth: 1, Names: [][]byte{[]byte("B"), []byte("A")}}},
												},
												Value: &resolve.Object{
													Path: []string{"owner"},
													Fields: []*resolve.Field{
														{
															Name: []byte("name"),
															ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																{{Depth: 2, Names: [][]byte{[]byte("B"), []byte("A")}}},
															},
															Value: &resolve.String{Path: []string{"name"}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			renders: []render{
				{
					input: `{
						"product": {
							"__typename": "A",
							"category": {
								"id": "1",
								"owner": {"name": "n"}
							}
						}
					}`,
					want: `{
						"product": {
							"category": {
								"id": "1",
								"owner": {"name": "n"}
							}
						}
					}`,
				},
				{
					input: `{
						"product": {
							"__typename": "B",
							"category": {
								"id": "1",
								"owner": {"name": "n"}
							}
						}
					}`,
					want: `{
						"product": {
							"category": {
								"id": "1",
								"owner": {"name": "n"}
							}
						}
					}`,
				},
				{
					input: `{
						"product": {
							"__typename": "C",
							"category": {
								"id": "1",
								"owner": {"name": "n"}
							}
						}
					}`,
					want: `{
						"product": {
							"category": {"id": "1"}
						}
					}`,
				},
			},
		},
		{
			// [S2] The matching fragment comes first:
			//   product {
			//     category { id }
			//     ... on A {
			//       category {
			//         owner { name }
			//       }
			//     }
			//     ... on B {
			//       category {
			//         owner { name }
			//       }
			//     }
			//   }
			name: "[S2] matching fragment first",
			input: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("product"),
						Value: &resolve.Object{
							Path: []string{"product"},
							Fields: []*resolve.Field{
								{
									Name: []byte("category"),
									Value: &resolve.Object{
										Path: []string{"category"},
										Fields: []*resolve.Field{
											{Name: []byte("id"), Value: &resolve.String{Path: []string{"id"}}},
										},
									},
								},
								{
									Name:        []byte("category"),
									OnTypeNames: [][]byte{[]byte("A")},
									Value: &resolve.Object{
										Path: []string{"category"},
										Fields: []*resolve.Field{
											{
												Name: []byte("owner"),
												Value: &resolve.Object{
													Path: []string{"owner"},
													Fields: []*resolve.Field{
														{Name: []byte("name"), Value: &resolve.String{Path: []string{"name"}}},
													},
												},
											},
										},
									},
								},
								{
									Name:        []byte("category"),
									OnTypeNames: [][]byte{[]byte("B")},
									Value: &resolve.Object{
										Path: []string{"category"},
										Fields: []*resolve.Field{
											{
												Name: []byte("owner"),
												Value: &resolve.Object{
													Path: []string{"owner"},
													Fields: []*resolve.Field{
														{Name: []byte("name"), Value: &resolve.String{Path: []string{"name"}}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("product"),
						Value: &resolve.Object{
							Path: []string{"product"},
							Fields: []*resolve.Field{
								{
									Name: []byte("category"),
									Value: &resolve.Object{
										Path: []string{"category"},
										Fields: []*resolve.Field{
											{Name: []byte("id"), Value: &resolve.String{Path: []string{"id"}}},
											{
												Name: []byte("owner"),
												ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
													{{Depth: 1, Names: [][]byte{[]byte("A"), []byte("B")}}},
												},
												Value: &resolve.Object{
													Path: []string{"owner"},
													Fields: []*resolve.Field{
														{
															Name: []byte("name"),
															ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																{{Depth: 2, Names: [][]byte{[]byte("A"), []byte("B")}}},
															},
															Value: &resolve.String{Path: []string{"name"}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			// [S3] An unrestricted sibling object plus a restricted one:
			//   product {
			//     category {
			//       owner { left }
			//     }
			//     ... on A {
			//       category {
			//         owner { right }
			//       }
			//     }
			//   }
			// owner itself has no condition, right keeps [2:A].
			name: "[S3] unrestricted sibling object, unrestricted first",
			input: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("product"),
						Value: &resolve.Object{
							Path: []string{"product"},
							Fields: []*resolve.Field{
								{
									Name: []byte("category"),
									Value: &resolve.Object{
										Path: []string{"category"},
										Fields: []*resolve.Field{
											{
												Name: []byte("owner"),
												Value: &resolve.Object{
													Path: []string{"owner"},
													Fields: []*resolve.Field{
														{Name: []byte("left"), Value: &resolve.String{Path: []string{"left"}}},
													},
												},
											},
										},
									},
								},
								{
									Name:        []byte("category"),
									OnTypeNames: [][]byte{[]byte("A")},
									Value: &resolve.Object{
										Path: []string{"category"},
										Fields: []*resolve.Field{
											{
												Name: []byte("owner"),
												Value: &resolve.Object{
													Path: []string{"owner"},
													Fields: []*resolve.Field{
														{Name: []byte("right"), Value: &resolve.String{Path: []string{"right"}}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("product"),
						Value: &resolve.Object{
							Path: []string{"product"},
							Fields: []*resolve.Field{
								{
									Name: []byte("category"),
									Value: &resolve.Object{
										Path: []string{"category"},
										Fields: []*resolve.Field{
											{
												Name: []byte("owner"),
												Value: &resolve.Object{
													Path: []string{"owner"},
													Fields: []*resolve.Field{
														{Name: []byte("left"), Value: &resolve.String{Path: []string{"left"}}},
														{
															Name: []byte("right"),
															ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																{{Depth: 2, Names: [][]byte{[]byte("A")}}},
															},
															Value: &resolve.String{Path: []string{"right"}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			renders: []render{
				{
					input: `{
						"product": {
							"__typename": "A",
							"category": {
								"owner": {"left": "L", "right": "R"}
							}
						}
					}`,
					want: `{
						"product": {
							"category": {
								"owner": {"left": "L", "right": "R"}
							}
						}
					}`,
				},
				{
					input: `{
						"product": {
							"__typename": "B",
							"category": {
								"owner": {"left": "L", "right": "R"}
							}
						}
					}`,
					want: `{
						"product": {
							"category": {
								"owner": {"left": "L"}
							}
						}
					}`,
				},
			},
		},
		{
			// [S3] The same selections, restricted fragment first:
			//   product {
			//     ... on A {
			//       category {
			//         owner { right }
			//       }
			//     }
			//     category {
			//       owner { left }
			//     }
			//   }
			// The unconditional category absorbs the conditional one, so left comes first in the output.
			name: "[S3] unrestricted sibling object, restricted first",
			input: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("product"),
						Value: &resolve.Object{
							Path: []string{"product"},
							Fields: []*resolve.Field{
								{
									Name:        []byte("category"),
									OnTypeNames: [][]byte{[]byte("A")},
									Value: &resolve.Object{
										Path: []string{"category"},
										Fields: []*resolve.Field{
											{
												Name: []byte("owner"),
												Value: &resolve.Object{
													Path: []string{"owner"},
													Fields: []*resolve.Field{
														{Name: []byte("right"), Value: &resolve.String{Path: []string{"right"}}},
													},
												},
											},
										},
									},
								},
								{
									Name: []byte("category"),
									Value: &resolve.Object{
										Path: []string{"category"},
										Fields: []*resolve.Field{
											{
												Name: []byte("owner"),
												Value: &resolve.Object{
													Path: []string{"owner"},
													Fields: []*resolve.Field{
														{Name: []byte("left"), Value: &resolve.String{Path: []string{"left"}}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("product"),
						Value: &resolve.Object{
							Path: []string{"product"},
							Fields: []*resolve.Field{
								{
									Name: []byte("category"),
									Value: &resolve.Object{
										Path: []string{"category"},
										Fields: []*resolve.Field{
											{
												Name: []byte("owner"),
												Value: &resolve.Object{
													Path: []string{"owner"},
													Fields: []*resolve.Field{
														{Name: []byte("left"), Value: &resolve.String{Path: []string{"left"}}},
														{
															Name: []byte("right"),
															ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																{{Depth: 2, Names: [][]byte{[]byte("A")}}},
															},
															Value: &resolve.String{Path: []string{"right"}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			renders: []render{
				{
					input: `{
						"product": {
							"__typename": "A",
							"category": {
								"owner": {"left": "L", "right": "R"}
							}
						}
					}`,
					want: `{
						"product": {
							"category": {
								"owner": {"left": "L", "right": "R"}
							}
						}
					}`,
				},
				{
					input: `{
						"product": {
							"__typename": "B",
							"category": {
								"owner": {"left": "L", "right": "R"}
							}
						}
					}`,
					want: `{
						"product": {
							"category": {
								"owner": {"left": "L"}
							}
						}
					}`,
				},
			},
		},
		{
			// [S21] Both siblings carry the same onTypeNames, and one has no parent condition:
			//   outer {
			//     product {
			//       ... on A {
			//         category { x }
			//       }
			//     }
			//     ... on OuterA {
			//       product {
			//         ... on A {
			//           category { x }
			//         }
			//       }
			//     }
			//   }
			// category keeps onTypeNames [A] and gets no group: A AND (true OR [1:OuterA]) is A.
			name: "[S21] same onTypeNames, one side without parent condition",
			input: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("outer"),
						Value: &resolve.Object{
							Path: []string{"outer"},
							Fields: []*resolve.Field{
								{
									Name: []byte("product"),
									Value: &resolve.Object{
										Path: []string{"product"},
										Fields: []*resolve.Field{
											{
												Name:        []byte("category"),
												OnTypeNames: [][]byte{[]byte("A")},
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{Name: []byte("x"), Value: &resolve.String{Path: []string{"x"}}},
													},
												},
											},
										},
									},
								},
								{
									Name:        []byte("product"),
									OnTypeNames: [][]byte{[]byte("OuterA")},
									Value: &resolve.Object{
										Path: []string{"product"},
										Fields: []*resolve.Field{
											{
												Name:        []byte("category"),
												OnTypeNames: [][]byte{[]byte("A")},
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{Name: []byte("x"), Value: &resolve.String{Path: []string{"x"}}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("outer"),
						Value: &resolve.Object{
							Path: []string{"outer"},
							Fields: []*resolve.Field{
								{
									Name: []byte("product"),
									Value: &resolve.Object{
										Path: []string{"product"},
										Fields: []*resolve.Field{
											{
												Name:        []byte("category"),
												OnTypeNames: [][]byte{[]byte("A")},
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{
															Name: []byte("x"),
															ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																{{Depth: 1, Names: [][]byte{[]byte("A")}}},
																{
																	{Depth: 2, Names: [][]byte{[]byte("OuterA")}},
																	{Depth: 1, Names: [][]byte{[]byte("A")}},
																},
															},
															Value: &resolve.String{Path: []string{"x"}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			renders: []render{
				{
					input: `{
						"outer": {
							"__typename": "OuterB",
							"product": {
								"__typename": "A",
								"category": {"x": "X"}
							}
						}
					}`,
					want: `{
						"outer": {
							"product": {
								"category": {"x": "X"}
							}
						}
					}`,
				},
				{
					input: `{
						"outer": {
							"__typename": "OuterB",
							"product": {
								"__typename": "B",
								"category": {"x": "X"}
							}
						}
					}`,
					want: `{
						"outer": {
							"product": {}
						}
					}`,
				},
			},
		},
		{
			// [S4] Two paths with conditions at different depths:
			//   outer {
			//     product {
			//       ... on A {
			//         category {
			//           owner { x }
			//         }
			//       }
			//     }
			//     ... on OuterA {
			//       product {
			//         category {
			//           owner { x }
			//         }
			//       }
			//     }
			//   }
			// x renders when product is A or when outer is OuterA. The fragment on A folds into the
			// groups of category as depth 0, because the merged category has no onTypeNames.
			name: "[S4] different depths",
			input: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("outer"),
						Value: &resolve.Object{
							Path: []string{"outer"},
							Fields: []*resolve.Field{
								{
									Name: []byte("product"),
									Value: &resolve.Object{
										Path: []string{"product"},
										Fields: []*resolve.Field{
											{
												Name:        []byte("category"),
												OnTypeNames: [][]byte{[]byte("A")},
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{
															Name: []byte("owner"),
															Value: &resolve.Object{
																Path: []string{"owner"},
																Fields: []*resolve.Field{
																	{Name: []byte("x"), Value: &resolve.String{Path: []string{"x"}}},
																},
															},
														},
													},
												},
											},
										},
									},
								},
								{
									Name:        []byte("product"),
									OnTypeNames: [][]byte{[]byte("OuterA")},
									Value: &resolve.Object{
										Path: []string{"product"},
										Fields: []*resolve.Field{
											{
												Name: []byte("category"),
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{
															Name: []byte("owner"),
															Value: &resolve.Object{
																Path: []string{"owner"},
																Fields: []*resolve.Field{
																	{Name: []byte("x"), Value: &resolve.String{Path: []string{"x"}}},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("outer"),
						Value: &resolve.Object{
							Path: []string{"outer"},
							Fields: []*resolve.Field{
								{
									Name: []byte("product"),
									Value: &resolve.Object{
										Path: []string{"product"},
										Fields: []*resolve.Field{
											{
												Name: []byte("category"),
												ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
													{{Depth: 1, Names: [][]byte{[]byte("OuterA")}}},
													{{Depth: 0, Names: [][]byte{[]byte("A")}}},
												},
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{
															Name: []byte("owner"),
															ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																{{Depth: 2, Names: [][]byte{[]byte("OuterA")}}},
																{{Depth: 1, Names: [][]byte{[]byte("A")}}},
															},
															Value: &resolve.Object{
																Path: []string{"owner"},
																Fields: []*resolve.Field{
																	{
																		Name: []byte("x"),
																		ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																			{{Depth: 3, Names: [][]byte{[]byte("OuterA")}}},
																			{{Depth: 2, Names: [][]byte{[]byte("A")}}},
																		},
																		Value: &resolve.String{Path: []string{"x"}},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			renders: []render{
				{
					input: `{
						"outer": {
							"__typename": "OuterA",
							"product": {
								"__typename": "A",
								"category": {
									"owner": {"x": "X"}
								}
							}
						}
					}`,
					want: `{
						"outer": {
							"product": {
								"category": {
									"owner": {"x": "X"}
								}
							}
						}
					}`,
				},
				{
					input: `{
						"outer": {
							"__typename": "OuterA",
							"product": {
								"__typename": "B",
								"category": {
									"owner": {"x": "X"}
								}
							}
						}
					}`,
					want: `{
						"outer": {
							"product": {
								"category": {
									"owner": {"x": "X"}
								}
							}
						}
					}`,
				},
				{
					input: `{
						"outer": {
							"__typename": "OuterB",
							"product": {
								"__typename": "A",
								"category": {
									"owner": {"x": "X"}
								}
							}
						}
					}`,
					want: `{
						"outer": {
							"product": {
								"category": {
									"owner": {"x": "X"}
								}
							}
						}
					}`,
				},
				{
					input: `{
						"outer": {
							"__typename": "OuterB",
							"product": {
								"__typename": "B",
								"category": {
									"owner": {"x": "X"}
								}
							}
						}
					}`,
					want: `{
						"outer": {
							"product": {}
						}
					}`,
				},
			},
		},
		{
			// [S5] Two paths whose depths are correlated:
			//   outer {
			//     product {
			//       category { __typename }
			//     }
			//     ... on OuterA {
			//       product {
			//         ... on A {
			//           category {
			//             owner { x }
			//           }
			//         }
			//       }
			//     }
			//     ... on OuterB {
			//       product {
			//         ... on B {
			//           category {
			//             owner { x }
			//           }
			//         }
			//       }
			//     }
			//   }
			// x renders for (OuterA, A) and (OuterB, B) only. The groups must not fold into one.
			name: "[S5] correlated depths",
			input: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("outer"),
						Value: &resolve.Object{
							Path: []string{"outer"},
							Fields: []*resolve.Field{
								{
									Name: []byte("product"),
									Value: &resolve.Object{
										Path: []string{"product"},
										Fields: []*resolve.Field{
											{
												Name: []byte("category"),
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{Name: []byte("__typename"), Value: &resolve.String{Path: []string{"__typename"}}},
													},
												},
											},
										},
									},
								},
								{
									Name:        []byte("product"),
									OnTypeNames: [][]byte{[]byte("OuterA")},
									Value: &resolve.Object{
										Path: []string{"product"},
										Fields: []*resolve.Field{
											{
												Name:        []byte("category"),
												OnTypeNames: [][]byte{[]byte("A")},
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{
															Name: []byte("owner"),
															Value: &resolve.Object{
																Path: []string{"owner"},
																Fields: []*resolve.Field{
																	{Name: []byte("x"), Value: &resolve.String{Path: []string{"x"}}},
																},
															},
														},
													},
												},
											},
										},
									},
								},
								{
									Name:        []byte("product"),
									OnTypeNames: [][]byte{[]byte("OuterB")},
									Value: &resolve.Object{
										Path: []string{"product"},
										Fields: []*resolve.Field{
											{
												Name:        []byte("category"),
												OnTypeNames: [][]byte{[]byte("B")},
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{
															Name: []byte("owner"),
															Value: &resolve.Object{
																Path: []string{"owner"},
																Fields: []*resolve.Field{
																	{Name: []byte("x"), Value: &resolve.String{Path: []string{"x"}}},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("outer"),
						Value: &resolve.Object{
							Path: []string{"outer"},
							Fields: []*resolve.Field{
								{
									Name: []byte("product"),
									Value: &resolve.Object{
										Path: []string{"product"},
										Fields: []*resolve.Field{
											{
												Name: []byte("category"),
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{Name: []byte("__typename"), Value: &resolve.String{Path: []string{"__typename"}}},
														{
															Name: []byte("owner"),
															ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																{
																	{Depth: 2, Names: [][]byte{[]byte("OuterA")}},
																	{Depth: 1, Names: [][]byte{[]byte("A")}},
																},
																{
																	{Depth: 2, Names: [][]byte{[]byte("OuterB")}},
																	{Depth: 1, Names: [][]byte{[]byte("B")}},
																},
															},
															Value: &resolve.Object{
																Path: []string{"owner"},
																Fields: []*resolve.Field{
																	{
																		Name: []byte("x"),
																		ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																			{
																				{Depth: 3, Names: [][]byte{[]byte("OuterA")}},
																				{Depth: 2, Names: [][]byte{[]byte("A")}},
																			},
																			{
																				{Depth: 3, Names: [][]byte{[]byte("OuterB")}},
																				{Depth: 2, Names: [][]byte{[]byte("B")}},
																			},
																		},
																		Value: &resolve.String{Path: []string{"x"}},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			renders: []render{
				{
					input: `{
						"outer": {
							"__typename": "OuterA",
							"product": {
								"__typename": "A",
								"category": {
									"__typename": "Category",
									"owner": {"x": "X"}
								}
							}
						}
					}`,
					want: `{
						"outer": {
							"product": {
								"category": {
									"__typename": "Category",
									"owner": {"x": "X"}
								}
							}
						}
					}`,
				},
				{
					input: `{
						"outer": {
							"__typename": "OuterB",
							"product": {
								"__typename": "B",
								"category": {
									"__typename": "Category",
									"owner": {"x": "X"}
								}
							}
						}
					}`,
					want: `{
						"outer": {
							"product": {
								"category": {
									"__typename": "Category",
									"owner": {"x": "X"}
								}
							}
						}
					}`,
				},
				{
					input: `{
						"outer": {
							"__typename": "OuterA",
							"product": {
								"__typename": "B",
								"category": {
									"__typename": "Category",
									"owner": {"x": "X"}
								}
							}
						}
					}`,
					want: `{
						"outer": {
							"product": {
								"category": {"__typename": "Category"}
							}
						}
					}`,
				},
				{
					input: `{
						"outer": {
							"__typename": "OuterB",
							"product": {
								"__typename": "A",
								"category": {
									"__typename": "Category",
									"owner": {"x": "X"}
								}
							}
						}
					}`,
					want: `{
						"outer": {
							"product": {
								"category": {"__typename": "Category"}
							}
						}
					}`,
				},
			},
		},
		{
			// [S6] Three paths to one leaf:
			//   parent {
			//     fields {
			//       ... on C {
			//         nestedParent { c }
			//       }
			//     }
			//     ... on A {
			//       fields {
			//         nestedParent { c }
			//       }
			//     }
			//     ... on B {
			//       fields {
			//         ... on D {
			//           nestedParent { c }
			//         }
			//       }
			//     }
			//   }
			// c renders for (any, C), (A, any) and (B, D).
			name: "[S6] three paths at different levels",
			input: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("parent"),
						Value: &resolve.Object{
							Path: []string{"parent"},
							Fields: []*resolve.Field{
								{
									Name: []byte("fields"),
									Value: &resolve.Object{
										Path: []string{"fields"},
										Fields: []*resolve.Field{
											{
												Name:        []byte("nestedParent"),
												OnTypeNames: [][]byte{[]byte("C")},
												Value: &resolve.Object{
													Path: []string{"nestedParent"},
													Fields: []*resolve.Field{
														{Name: []byte("c"), Value: &resolve.String{Path: []string{"c"}}},
													},
												},
											},
										},
									},
								},
								{
									Name:        []byte("fields"),
									OnTypeNames: [][]byte{[]byte("A")},
									Value: &resolve.Object{
										Path: []string{"fields"},
										Fields: []*resolve.Field{
											{
												Name: []byte("nestedParent"),
												Value: &resolve.Object{
													Path: []string{"nestedParent"},
													Fields: []*resolve.Field{
														{Name: []byte("c"), Value: &resolve.String{Path: []string{"c"}}},
													},
												},
											},
										},
									},
								},
								{
									Name:        []byte("fields"),
									OnTypeNames: [][]byte{[]byte("B")},
									Value: &resolve.Object{
										Path: []string{"fields"},
										Fields: []*resolve.Field{
											{
												Name:        []byte("nestedParent"),
												OnTypeNames: [][]byte{[]byte("D")},
												Value: &resolve.Object{
													Path: []string{"nestedParent"},
													Fields: []*resolve.Field{
														{Name: []byte("c"), Value: &resolve.String{Path: []string{"c"}}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("parent"),
						Value: &resolve.Object{
							Path: []string{"parent"},
							Fields: []*resolve.Field{
								{
									Name: []byte("fields"),
									Value: &resolve.Object{
										Path: []string{"fields"},
										Fields: []*resolve.Field{
											{
												Name: []byte("nestedParent"),
												ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
													{{Depth: 1, Names: [][]byte{[]byte("A")}}},
													{{Depth: 0, Names: [][]byte{[]byte("C")}}},
													{
														{Depth: 1, Names: [][]byte{[]byte("B")}},
														{Depth: 0, Names: [][]byte{[]byte("D")}},
													},
												},
												Value: &resolve.Object{
													Path: []string{"nestedParent"},
													Fields: []*resolve.Field{
														{
															Name: []byte("c"),
															ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																{{Depth: 2, Names: [][]byte{[]byte("A")}}},
																{{Depth: 1, Names: [][]byte{[]byte("C")}}},
																{
																	{Depth: 2, Names: [][]byte{[]byte("B")}},
																	{Depth: 1, Names: [][]byte{[]byte("D")}},
																},
															},
															Value: &resolve.String{Path: []string{"c"}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			renders: []render{
				{
					input: `{
						"parent": {
							"__typename": "A",
							"fields": {
								"__typename": "X",
								"nestedParent": {"c": "v"}
							}
						}
					}`,
					want: `{
						"parent": {
							"fields": {
								"nestedParent": {"c": "v"}
							}
						}
					}`,
				},
				{
					input: `{
						"parent": {
							"__typename": "X",
							"fields": {
								"__typename": "C",
								"nestedParent": {"c": "v"}
							}
						}
					}`,
					want: `{
						"parent": {
							"fields": {
								"nestedParent": {"c": "v"}
							}
						}
					}`,
				},
				{
					input: `{
						"parent": {
							"__typename": "B",
							"fields": {
								"__typename": "D",
								"nestedParent": {"c": "v"}
							}
						}
					}`,
					want: `{
						"parent": {
							"fields": {
								"nestedParent": {"c": "v"}
							}
						}
					}`,
				},
				{
					input: `{
						"parent": {
							"__typename": "B",
							"fields": {
								"__typename": "C",
								"nestedParent": {"c": "v"}
							}
						}
					}`,
					want: `{
						"parent": {
							"fields": {
								"nestedParent": {"c": "v"}
							}
						}
					}`,
				},
				{
					input: `{
						"parent": {
							"__typename": "X",
							"fields": {
								"__typename": "D",
								"nestedParent": {"c": "v"}
							}
						}
					}`,
					want: `{
						"parent": {
							"fields": {}
						}
					}`,
				},
				{
					input: `{
						"parent": {
							"__typename": "B",
							"fields": {
								"__typename": "X",
								"nestedParent": {"c": "v"}
							}
						}
					}`,
					want: `{
						"parent": {
							"fields": {}
						}
					}`,
				},
			},
		},
		{
			// [S7] An interface field under a fragment, with an inline fragment inside:
			//   someNestedInterfaces {
			//     __typename
			//     ... on SNT1 {
			//       otherInterfaces {
			//         __typename
			//         someObject { a }
			//         ... on ST1 {
			//           someObject { b }
			//         }
			//       }
			//     }
			//   }
			// someObject@ST1 folds its onTypeNames into the groups as depth 0. The groups
			// [1:SNT1] OR [1:SNT1, 0:ST1] equal [1:SNT1]; the merge does not minimize them.
			name: "[S7] interface field under a fragment, depth-0 fold",
			input: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("someNestedInterfaces"),
						Value: &resolve.Array{
							Path: []string{"someNestedInterfaces"},
							Item: &resolve.Object{
								Fields: []*resolve.Field{
									{Name: []byte("__typename"), Value: &resolve.String{Path: []string{"__typename"}}},
									{
										Name:        []byte("otherInterfaces"),
										OnTypeNames: [][]byte{[]byte("SNT1")},
										Value: &resolve.Array{
											Path: []string{"otherInterfaces"},
											Item: &resolve.Object{
												Fields: []*resolve.Field{
													{Name: []byte("__typename"), Value: &resolve.String{Path: []string{"__typename"}}},
													{
														Name: []byte("someObject"),
														Value: &resolve.Object{
															Path: []string{"someObject"},
															Fields: []*resolve.Field{
																{Name: []byte("a"), Value: &resolve.String{Path: []string{"a"}}},
															},
														},
													},
													{
														Name:        []byte("someObject"),
														OnTypeNames: [][]byte{[]byte("ST1")},
														Value: &resolve.Object{
															Path: []string{"someObject"},
															Fields: []*resolve.Field{
																{Name: []byte("b"), Value: &resolve.String{Path: []string{"b"}}},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("someNestedInterfaces"),
						Value: &resolve.Array{
							Path: []string{"someNestedInterfaces"},
							Item: &resolve.Object{
								Fields: []*resolve.Field{
									{Name: []byte("__typename"), Value: &resolve.String{Path: []string{"__typename"}}},
									{
										Name:        []byte("otherInterfaces"),
										OnTypeNames: [][]byte{[]byte("SNT1")},
										Value: &resolve.Array{
											Path: []string{"otherInterfaces"},
											Item: &resolve.Object{
												Fields: []*resolve.Field{
													{
														Name: []byte("__typename"),
														ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
															{{Depth: 1, Names: [][]byte{[]byte("SNT1")}}},
														},
														Value: &resolve.String{Path: []string{"__typename"}},
													},
													{
														Name: []byte("someObject"),
														ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
															{{Depth: 1, Names: [][]byte{[]byte("SNT1")}}},
															{
																{Depth: 1, Names: [][]byte{[]byte("SNT1")}},
																{Depth: 0, Names: [][]byte{[]byte("ST1")}},
															},
														},
														Value: &resolve.Object{
															Path: []string{"someObject"},
															Fields: []*resolve.Field{
																{
																	Name: []byte("a"),
																	ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																		{{Depth: 2, Names: [][]byte{[]byte("SNT1")}}},
																	},
																	Value: &resolve.String{Path: []string{"a"}},
																},
																{
																	Name: []byte("b"),
																	ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																		{
																			{Depth: 2, Names: [][]byte{[]byte("SNT1")}},
																			{Depth: 1, Names: [][]byte{[]byte("ST1")}},
																		},
																	},
																	Value: &resolve.String{Path: []string{"b"}},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			renders: []render{
				{
					input: `{
						"someNestedInterfaces": [
							{
								"__typename": "SNT1",
								"otherInterfaces": [
									{
										"__typename": "ST1",
										"someObject": {"a": "A", "b": "B"}
									},
									{
										"__typename": "ST2",
										"someObject": {"a": "A", "b": "B"}
									}
								]
							}
						]
					}`,
					want: `{
						"someNestedInterfaces": [
							{
								"__typename": "SNT1",
								"otherInterfaces": [
									{
										"__typename": "ST1",
										"someObject": {"a": "A", "b": "B"}
									},
									{
										"__typename": "ST2",
										"someObject": {"a": "A"}
									}
								]
							}
						]
					}`,
				},
				{
					input: `{
						"someNestedInterfaces": [
							{"__typename": "SNT2"}
						]
					}`,
					want: `{
						"someNestedInterfaces": [
							{"__typename": "SNT2"}
						]
					}`,
				},
			},
		},
		{
			// [S8] A subset path:
			//   parent {
			//     ... on B {
			//       fields {
			//         nestedParent { c }
			//       }
			//     }
			//     ... on B {
			//       fields {
			//         ... on D {
			//           nestedParent { c }
			//         }
			//       }
			//     }
			//   }
			// [1:B] OR [1:B, 0:D] equals [1:B]. Characterization: the merge keeps both groups.
			name: "[S8] subset path is kept, not minimized",
			input: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("parent"),
						Value: &resolve.Object{
							Path: []string{"parent"},
							Fields: []*resolve.Field{
								{
									Name:        []byte("fields"),
									OnTypeNames: [][]byte{[]byte("B")},
									Value: &resolve.Object{
										Path: []string{"fields"},
										Fields: []*resolve.Field{
											{
												Name: []byte("nestedParent"),
												Value: &resolve.Object{
													Path: []string{"nestedParent"},
													Fields: []*resolve.Field{
														{Name: []byte("c"), Value: &resolve.String{Path: []string{"c"}}},
													},
												},
											},
										},
									},
								},
								{
									Name:        []byte("fields"),
									OnTypeNames: [][]byte{[]byte("B")},
									Value: &resolve.Object{
										Path: []string{"fields"},
										Fields: []*resolve.Field{
											{
												Name:        []byte("nestedParent"),
												OnTypeNames: [][]byte{[]byte("D")},
												Value: &resolve.Object{
													Path: []string{"nestedParent"},
													Fields: []*resolve.Field{
														{Name: []byte("c"), Value: &resolve.String{Path: []string{"c"}}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("parent"),
						Value: &resolve.Object{
							Path: []string{"parent"},
							Fields: []*resolve.Field{
								{
									Name:        []byte("fields"),
									OnTypeNames: [][]byte{[]byte("B")},
									Value: &resolve.Object{
										Path: []string{"fields"},
										Fields: []*resolve.Field{
											{
												Name: []byte("nestedParent"),
												ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
													{{Depth: 1, Names: [][]byte{[]byte("B")}}},
													{
														{Depth: 1, Names: [][]byte{[]byte("B")}},
														{Depth: 0, Names: [][]byte{[]byte("D")}},
													},
												},
												Value: &resolve.Object{
													Path: []string{"nestedParent"},
													Fields: []*resolve.Field{
														{
															Name: []byte("c"),
															ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																{{Depth: 2, Names: [][]byte{[]byte("B")}}},
																{
																	{Depth: 2, Names: [][]byte{[]byte("B")}},
																	{Depth: 1, Names: [][]byte{[]byte("D")}},
																},
															},
															Value: &resolve.String{Path: []string{"c"}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			// [S9] Two fragment paths to a list field:
			//   product {
			//     category { id }
			//     ... on B {
			//       category {
			//         owners { name }
			//       }
			//     }
			//     ... on A {
			//       category {
			//         owners { name }
			//       }
			//     }
			//   }
			// An array adds no depth: name sits in the item object, and depth 2 is product.
			name: "[S9] two fragment paths to a list field",
			input: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("product"),
						Value: &resolve.Object{
							Path: []string{"product"},
							Fields: []*resolve.Field{
								{
									Name: []byte("category"),
									Value: &resolve.Object{
										Path: []string{"category"},
										Fields: []*resolve.Field{
											{Name: []byte("id"), Value: &resolve.String{Path: []string{"id"}}},
										},
									},
								},
								{
									Name:        []byte("category"),
									OnTypeNames: [][]byte{[]byte("B")},
									Value: &resolve.Object{
										Path: []string{"category"},
										Fields: []*resolve.Field{
											{
												Name: []byte("owners"),
												Value: &resolve.Array{
													Path: []string{"owners"},
													Item: &resolve.Object{
														Fields: []*resolve.Field{
															{Name: []byte("name"), Value: &resolve.String{Path: []string{"name"}}},
														},
													},
												},
											},
										},
									},
								},
								{
									Name:        []byte("category"),
									OnTypeNames: [][]byte{[]byte("A")},
									Value: &resolve.Object{
										Path: []string{"category"},
										Fields: []*resolve.Field{
											{
												Name: []byte("owners"),
												Value: &resolve.Array{
													Path: []string{"owners"},
													Item: &resolve.Object{
														Fields: []*resolve.Field{
															{Name: []byte("name"), Value: &resolve.String{Path: []string{"name"}}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("product"),
						Value: &resolve.Object{
							Path: []string{"product"},
							Fields: []*resolve.Field{
								{
									Name: []byte("category"),
									Value: &resolve.Object{
										Path: []string{"category"},
										Fields: []*resolve.Field{
											{Name: []byte("id"), Value: &resolve.String{Path: []string{"id"}}},
											{
												Name: []byte("owners"),
												ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
													{{Depth: 1, Names: [][]byte{[]byte("B"), []byte("A")}}},
												},
												Value: &resolve.Array{
													Path: []string{"owners"},
													Item: &resolve.Object{
														Fields: []*resolve.Field{
															{
																Name: []byte("name"),
																ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																	{{Depth: 2, Names: [][]byte{[]byte("B"), []byte("A")}}},
																},
																Value: &resolve.String{Path: []string{"name"}},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			renders: []render{
				{
					input: `{
						"product": {
							"__typename": "A",
							"category": {
								"id": "1",
								"owners": [
									{"name": "n"}
								]
							}
						}
					}`,
					want: `{
						"product": {
							"category": {
								"id": "1",
								"owners": [
									{"name": "n"}
								]
							}
						}
					}`,
				},
				{
					input: `{
						"product": {
							"__typename": "C",
							"category": {
								"id": "1",
								"owners": [
									{"name": "n"}
								]
							}
						}
					}`,
					want: `{
						"product": {
							"category": {"id": "1"}
						}
					}`,
				},
			},
		},
		{
			// [S10] Fields with different onTypeNames stay separate, and so do their descendants:
			//   parent {
			//     ... on B {
			//       fields {
			//         ... on E {
			//           nestedParent { c }
			//         }
			//       }
			//     }
			//     ... on B {
			//       fields {
			//         ... on D {
			//           nestedParent { c }
			//         }
			//       }
			//     }
			//   }
			// This preserves the field order in the output.
			name: "[S10] different onTypeNames stay separate",
			input: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("parent"),
						Value: &resolve.Object{
							Path: []string{"parent"},
							Fields: []*resolve.Field{
								{
									Name:        []byte("fields"),
									OnTypeNames: [][]byte{[]byte("B")},
									Value: &resolve.Object{
										Path: []string{"fields"},
										Fields: []*resolve.Field{
											{
												Name:        []byte("nestedParent"),
												OnTypeNames: [][]byte{[]byte("E")},
												Value: &resolve.Object{
													Path: []string{"nestedParent"},
													Fields: []*resolve.Field{
														{Name: []byte("c"), Value: &resolve.String{Path: []string{"c"}}},
													},
												},
											},
										},
									},
								},
								{
									Name:        []byte("fields"),
									OnTypeNames: [][]byte{[]byte("B")},
									Value: &resolve.Object{
										Path: []string{"fields"},
										Fields: []*resolve.Field{
											{
												Name:        []byte("nestedParent"),
												OnTypeNames: [][]byte{[]byte("D")},
												Value: &resolve.Object{
													Path: []string{"nestedParent"},
													Fields: []*resolve.Field{
														{Name: []byte("c"), Value: &resolve.String{Path: []string{"c"}}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("parent"),
						Value: &resolve.Object{
							Path: []string{"parent"},
							Fields: []*resolve.Field{
								{
									Name:        []byte("fields"),
									OnTypeNames: [][]byte{[]byte("B")},
									Value: &resolve.Object{
										Path: []string{"fields"},
										Fields: []*resolve.Field{
											{
												Name:        []byte("nestedParent"),
												OnTypeNames: [][]byte{[]byte("E")},
												ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
													{{Depth: 1, Names: [][]byte{[]byte("B")}}},
												},
												Value: &resolve.Object{
													Path: []string{"nestedParent"},
													Fields: []*resolve.Field{
														{
															Name: []byte("c"),
															ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																{
																	{Depth: 2, Names: [][]byte{[]byte("B")}},
																	{Depth: 1, Names: [][]byte{[]byte("E")}},
																},
															},
															Value: &resolve.String{Path: []string{"c"}},
														},
													},
												},
											},
											{
												Name:        []byte("nestedParent"),
												OnTypeNames: [][]byte{[]byte("D")},
												ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
													{{Depth: 1, Names: [][]byte{[]byte("B")}}},
												},
												Value: &resolve.Object{
													Path: []string{"nestedParent"},
													Fields: []*resolve.Field{
														{
															Name: []byte("c"),
															ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																{
																	{Depth: 2, Names: [][]byte{[]byte("B")}},
																	{Depth: 1, Names: [][]byte{[]byte("D")}},
																},
															},
															Value: &resolve.String{Path: []string{"c"}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			// [S12] Three nesting levels with a shared type at the top:
			//   a {
			//     __typename
			//     ... on B {
			//       b {
			//         c {
			//           conversation { id }
			//           ... on Message {
			//             conversation { id }
			//           }
			//         }
			//       }
			//     }
			//   }
			// b is a single-field object, so c is reached without a merge at that level.
			name: "[S12] shared type at a higher depth",
			input: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("a"),
						Value: &resolve.Object{
							Path: []string{"a"},
							Fields: []*resolve.Field{
								{Name: []byte("__typename"), Value: &resolve.String{Path: []string{"__typename"}}},
								{
									Name:        []byte("b"),
									OnTypeNames: [][]byte{[]byte("B")},
									Value: &resolve.Object{
										Path: []string{"b"},
										Fields: []*resolve.Field{
											{
												Name: []byte("c"),
												Value: &resolve.Object{
													Path: []string{"c"},
													Fields: []*resolve.Field{
														{
															Name: []byte("conversation"),
															Value: &resolve.Object{
																Path: []string{"conversation"},
																Fields: []*resolve.Field{
																	{Name: []byte("id"), Value: &resolve.String{Path: []string{"id"}}},
																},
															},
														},
														{
															Name:        []byte("conversation"),
															OnTypeNames: [][]byte{[]byte("Message")},
															Value: &resolve.Object{
																Path: []string{"conversation"},
																Fields: []*resolve.Field{
																	{Name: []byte("id"), Value: &resolve.String{Path: []string{"id"}}},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("a"),
						Value: &resolve.Object{
							Path: []string{"a"},
							Fields: []*resolve.Field{
								{Name: []byte("__typename"), Value: &resolve.String{Path: []string{"__typename"}}},
								{
									Name:        []byte("b"),
									OnTypeNames: [][]byte{[]byte("B")},
									Value: &resolve.Object{
										Path: []string{"b"},
										Fields: []*resolve.Field{
											{
												Name: []byte("c"),
												ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
													{{Depth: 1, Names: [][]byte{[]byte("B")}}},
												},
												Value: &resolve.Object{
													Path: []string{"c"},
													Fields: []*resolve.Field{
														{
															Name: []byte("conversation"),
															ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																{{Depth: 2, Names: [][]byte{[]byte("B")}}},
																{
																	{Depth: 2, Names: [][]byte{[]byte("B")}},
																	{Depth: 0, Names: [][]byte{[]byte("Message")}},
																},
															},
															Value: &resolve.Object{
																Path: []string{"conversation"},
																Fields: []*resolve.Field{
																	{
																		Name: []byte("id"),
																		ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																			{{Depth: 3, Names: [][]byte{[]byte("B")}}},
																			{
																				{Depth: 3, Names: [][]byte{[]byte("B")}},
																				{Depth: 1, Names: [][]byte{[]byte("Message")}},
																			},
																		},
																		Value: &resolve.String{Path: []string{"id"}},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			// [S13] The different-depths case with a scalar leaf:
			//   outer {
			//     product {
			//       ... on A {
			//         category { owner }
			//       }
			//     }
			//     ... on OuterA {
			//       product {
			//         category { owner }
			//       }
			//     }
			//   }
			name: "[S13] different depths on a scalar",
			input: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("outer"),
						Value: &resolve.Object{
							Path: []string{"outer"},
							Fields: []*resolve.Field{
								{
									Name: []byte("product"),
									Value: &resolve.Object{
										Path: []string{"product"},
										Fields: []*resolve.Field{
											{
												Name:        []byte("category"),
												OnTypeNames: [][]byte{[]byte("A")},
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{Name: []byte("owner"), Value: &resolve.String{Path: []string{"owner"}}},
													},
												},
											},
										},
									},
								},
								{
									Name:        []byte("product"),
									OnTypeNames: [][]byte{[]byte("OuterA")},
									Value: &resolve.Object{
										Path: []string{"product"},
										Fields: []*resolve.Field{
											{
												Name: []byte("category"),
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{Name: []byte("owner"), Value: &resolve.String{Path: []string{"owner"}}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("outer"),
						Value: &resolve.Object{
							Path: []string{"outer"},
							Fields: []*resolve.Field{
								{
									Name: []byte("product"),
									Value: &resolve.Object{
										Path: []string{"product"},
										Fields: []*resolve.Field{
											{
												Name: []byte("category"),
												ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
													{{Depth: 1, Names: [][]byte{[]byte("OuterA")}}},
													{{Depth: 0, Names: [][]byte{[]byte("A")}}},
												},
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{
															Name: []byte("owner"),
															ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																{{Depth: 2, Names: [][]byte{[]byte("OuterA")}}},
																{{Depth: 1, Names: [][]byte{[]byte("A")}}},
															},
															Value: &resolve.String{Path: []string{"owner"}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			renders: []render{
				{
					input: `{
						"outer": {
							"__typename": "OuterB",
							"product": {
								"__typename": "A",
								"category": {"owner": "O"}
							}
						}
					}`,
					want: `{
						"outer": {
							"product": {
								"category": {"owner": "O"}
							}
						}
					}`,
				},
				{
					input: `{
						"outer": {
							"__typename": "OuterB",
							"product": {
								"__typename": "B",
								"category": {"owner": "O"}
							}
						}
					}`,
					want: `{
						"outer": {
							"product": {}
						}
					}`,
				},
			},
		},
		{
			// [S14] A field with two onTypeNames under a restricted parent:
			//   outer {
			//     __typename
			//     ... on OuterA {
			//       product {
			//         category {
			//           __typename
			//           ... on A {
			//             owner { x }
			//           }
			//           ... on B {
			//             owner { x }
			//           }
			//         }
			//       }
			//     }
			//   }
			// The planner emits one owner with onTypeNames [A, B]. Both copies must keep [2:OuterA].
			name: "[S14] duplicated field keeps the parent groups",
			input: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("outer"),
						Value: &resolve.Object{
							Path: []string{"outer"},
							Fields: []*resolve.Field{
								{Name: []byte("__typename"), Value: &resolve.String{Path: []string{"__typename"}}},
								{
									Name:        []byte("product"),
									OnTypeNames: [][]byte{[]byte("OuterA")},
									Value: &resolve.Object{
										Path: []string{"product"},
										Fields: []*resolve.Field{
											{
												Name: []byte("category"),
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{Name: []byte("__typename"), Value: &resolve.String{Path: []string{"__typename"}}},
														{
															Name:        []byte("owner"),
															OnTypeNames: [][]byte{[]byte("A"), []byte("B")},
															Value: &resolve.Object{
																Path: []string{"owner"},
																Fields: []*resolve.Field{
																	{Name: []byte("x"), Value: &resolve.String{Path: []string{"x"}}},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("outer"),
						Value: &resolve.Object{
							Path: []string{"outer"},
							Fields: []*resolve.Field{
								{Name: []byte("__typename"), Value: &resolve.String{Path: []string{"__typename"}}},
								{
									Name:        []byte("product"),
									OnTypeNames: [][]byte{[]byte("OuterA")},
									Value: &resolve.Object{
										Path: []string{"product"},
										Fields: []*resolve.Field{
											{
												Name: []byte("category"),
												ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
													{{Depth: 1, Names: [][]byte{[]byte("OuterA")}}},
												},
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{
															Name: []byte("__typename"),
															ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																{{Depth: 2, Names: [][]byte{[]byte("OuterA")}}},
															},
															Value: &resolve.String{Path: []string{"__typename"}},
														},
														{
															Name:        []byte("owner"),
															OnTypeNames: [][]byte{[]byte("A")},
															ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																{{Depth: 2, Names: [][]byte{[]byte("OuterA")}}},
															},
															Value: &resolve.Object{
																Path: []string{"owner"},
																Fields: []*resolve.Field{
																	{
																		Name: []byte("x"),
																		ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																			{
																				{Depth: 3, Names: [][]byte{[]byte("OuterA")}},
																				{Depth: 1, Names: [][]byte{[]byte("A")}},
																			},
																		},
																		Value: &resolve.String{Path: []string{"x"}},
																	},
																},
															},
														},
														{
															Name:        []byte("owner"),
															OnTypeNames: [][]byte{[]byte("B")},
															ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																{{Depth: 2, Names: [][]byte{[]byte("OuterA")}}},
															},
															Value: &resolve.Object{
																Path: []string{"owner"},
																Fields: []*resolve.Field{
																	{
																		Name: []byte("x"),
																		ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																			{
																				{Depth: 3, Names: [][]byte{[]byte("OuterA")}},
																				{Depth: 1, Names: [][]byte{[]byte("B")}},
																			},
																		},
																		Value: &resolve.String{Path: []string{"x"}},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			// [S15] A nested list, with matrix: [[Item]]:
			//   outer {
			//     __typename
			//     ... on OuterA {
			//       matrix { name }
			//     }
			//   }
			// Propagation stops at a list of lists, and so does the resolver.
			// Characterization: name gets no condition. The whole matrix field is skipped by its own onTypeNames.
			name: "[S15] nested list stops propagation",
			input: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("outer"),
						Value: &resolve.Object{
							Path: []string{"outer"},
							Fields: []*resolve.Field{
								{Name: []byte("__typename"), Value: &resolve.String{Path: []string{"__typename"}}},
								{
									Name:        []byte("matrix"),
									OnTypeNames: [][]byte{[]byte("OuterA")},
									Value: &resolve.Array{
										Path: []string{"matrix"},
										Item: &resolve.Array{
											Item: &resolve.Object{
												Fields: []*resolve.Field{
													{Name: []byte("name"), Value: &resolve.String{Path: []string{"name"}}},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("outer"),
						Value: &resolve.Object{
							Path: []string{"outer"},
							Fields: []*resolve.Field{
								{Name: []byte("__typename"), Value: &resolve.String{Path: []string{"__typename"}}},
								{
									Name:        []byte("matrix"),
									OnTypeNames: [][]byte{[]byte("OuterA")},
									Value: &resolve.Array{
										Path: []string{"matrix"},
										Item: &resolve.Array{
											Item: &resolve.Object{
												Fields: []*resolve.Field{
													{Name: []byte("name"), Value: &resolve.String{Path: []string{"name"}}},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			// [S20] Many fragments at two depths fold into two groups:
			//   outer {
			//     product {
			//       category { id }
			//       ... on A {
			//         category {
			//           owner { x }
			//         }
			//       }
			//       ... on B {
			//         category {
			//           owner { x }
			//         }
			//       }
			//     }
			//     ... on OuterA {
			//       product {
			//         category {
			//           owner { x }
			//         }
			//       }
			//     }
			//     ... on OuterB {
			//       product {
			//         category {
			//           owner { x }
			//         }
			//       }
			//     }
			//   }
			name: "[S20] fragments at two depths fold into two groups",
			input: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("outer"),
						Value: &resolve.Object{
							Path: []string{"outer"},
							Fields: []*resolve.Field{
								{
									Name: []byte("product"),
									Value: &resolve.Object{
										Path: []string{"product"},
										Fields: []*resolve.Field{
											{
												Name: []byte("category"),
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{Name: []byte("id"), Value: &resolve.String{Path: []string{"id"}}},
													},
												},
											},
											{
												Name:        []byte("category"),
												OnTypeNames: [][]byte{[]byte("A")},
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{
															Name: []byte("owner"),
															Value: &resolve.Object{
																Path: []string{"owner"},
																Fields: []*resolve.Field{
																	{Name: []byte("x"), Value: &resolve.String{Path: []string{"x"}}},
																},
															},
														},
													},
												},
											},
											{
												Name:        []byte("category"),
												OnTypeNames: [][]byte{[]byte("B")},
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{
															Name: []byte("owner"),
															Value: &resolve.Object{
																Path: []string{"owner"},
																Fields: []*resolve.Field{
																	{Name: []byte("x"), Value: &resolve.String{Path: []string{"x"}}},
																},
															},
														},
													},
												},
											},
										},
									},
								},
								{
									Name:        []byte("product"),
									OnTypeNames: [][]byte{[]byte("OuterA")},
									Value: &resolve.Object{
										Path: []string{"product"},
										Fields: []*resolve.Field{
											{
												Name: []byte("category"),
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{
															Name: []byte("owner"),
															Value: &resolve.Object{
																Path: []string{"owner"},
																Fields: []*resolve.Field{
																	{Name: []byte("x"), Value: &resolve.String{Path: []string{"x"}}},
																},
															},
														},
													},
												},
											},
										},
									},
								},
								{
									Name:        []byte("product"),
									OnTypeNames: [][]byte{[]byte("OuterB")},
									Value: &resolve.Object{
										Path: []string{"product"},
										Fields: []*resolve.Field{
											{
												Name: []byte("category"),
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{
															Name: []byte("owner"),
															Value: &resolve.Object{
																Path: []string{"owner"},
																Fields: []*resolve.Field{
																	{Name: []byte("x"), Value: &resolve.String{Path: []string{"x"}}},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: &resolve.Object{
				Fields: []*resolve.Field{
					{
						Name: []byte("outer"),
						Value: &resolve.Object{
							Path: []string{"outer"},
							Fields: []*resolve.Field{
								{
									Name: []byte("product"),
									Value: &resolve.Object{
										Path: []string{"product"},
										Fields: []*resolve.Field{
											{
												Name: []byte("category"),
												Value: &resolve.Object{
													Path: []string{"category"},
													Fields: []*resolve.Field{
														{Name: []byte("id"), Value: &resolve.String{Path: []string{"id"}}},
														{
															Name: []byte("owner"),
															ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																{{Depth: 1, Names: [][]byte{[]byte("A"), []byte("B")}}},
																{{Depth: 2, Names: [][]byte{[]byte("OuterA"), []byte("OuterB")}}},
															},
															Value: &resolve.Object{
																Path: []string{"owner"},
																Fields: []*resolve.Field{
																	{
																		Name: []byte("x"),
																		ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
																			{{Depth: 2, Names: [][]byte{[]byte("A"), []byte("B")}}},
																			{{Depth: 3, Names: [][]byte{[]byte("OuterA"), []byte("OuterB")}}},
																		},
																		Value: &resolve.String{Path: []string{"x"}},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			(&mergeFields{}).Process(tc.input)
			assertResponseTreeEqual(t, tc.expected, tc.input)
			for _, r := range tc.renders {
				// Copy the tree first: a plan is copied before it is resolved, and the copy must keep the groups.
				root := tc.input.Copy().(*resolve.Object)
				var want bytes.Buffer
				require.NoError(t, json.Compact(&want, []byte(r.want)))
				assert.Equal(t, want.String(), renderResponse(t, root, r.input), "input %s", r.input)
			}
		})
	}

	// [S17] The subscription entry point runs the same merge:
	//   product {
	//     category { id }
	//     ... on B {
	//       category {
	//         owner { name }
	//       }
	//     }
	//     ... on A {
	//       category {
	//         owner { name }
	//       }
	//     }
	//   }
	t.Run("[S17] subscription path merges the same way", func(t *testing.T) {
		t.Parallel()
		input := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("product"),
					Value: &resolve.Object{
						Path: []string{"product"},
						Fields: []*resolve.Field{
							{
								Name: []byte("category"),
								Value: &resolve.Object{
									Path: []string{"category"},
									Fields: []*resolve.Field{
										{Name: []byte("id"), Value: &resolve.String{Path: []string{"id"}}},
									},
								},
							},
							{
								Name:        []byte("category"),
								OnTypeNames: [][]byte{[]byte("B")},
								Value: &resolve.Object{
									Path: []string{"category"},
									Fields: []*resolve.Field{
										{
											Name: []byte("owner"),
											Value: &resolve.Object{
												Path: []string{"owner"},
												Fields: []*resolve.Field{
													{Name: []byte("name"), Value: &resolve.String{Path: []string{"name"}}},
												},
											},
										},
									},
								},
							},
							{
								Name:        []byte("category"),
								OnTypeNames: [][]byte{[]byte("A")},
								Value: &resolve.Object{
									Path: []string{"category"},
									Fields: []*resolve.Field{
										{
											Name: []byte("owner"),
											Value: &resolve.Object{
												Path: []string{"owner"},
												Fields: []*resolve.Field{
													{Name: []byte("name"), Value: &resolve.String{Path: []string{"name"}}},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		expected := &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("product"),
					Value: &resolve.Object{
						Path: []string{"product"},
						Fields: []*resolve.Field{
							{
								Name: []byte("category"),
								Value: &resolve.Object{
									Path: []string{"category"},
									Fields: []*resolve.Field{
										{Name: []byte("id"), Value: &resolve.String{Path: []string{"id"}}},
										{
											Name: []byte("owner"),
											ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
												{{Depth: 1, Names: [][]byte{[]byte("B"), []byte("A")}}},
											},
											Value: &resolve.Object{
												Path: []string{"owner"},
												Fields: []*resolve.Field{
													{
														Name: []byte("name"),
														ParentOnTypeNames: [][]resolve.ParentOnTypeNames{
															{{Depth: 2, Names: [][]byte{[]byte("B"), []byte("A")}}},
														},
														Value: &resolve.String{Path: []string{"name"}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		(&mergeFields{}).ProcessSubscription(input)
		assertResponseTreeEqual(t, expected, input)
	})
}
