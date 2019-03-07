package lookup

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"testing"
)

func TestWalker(t *testing.T) {

	type check func(w *Walker)

	run := func(input string, checks ...check) {
		p := parser.NewParser()
		err := p.ParseTypeSystemDefinition([]byte(testDefinition))
		if err != nil {
			panic(err)
		}

		l := New(p, 256)
		l.ResetPool()

		err = p.ParseExecutableDefinition([]byte(input))
		if err != nil {
			panic(err)
		}

		walker := NewWalker(1024, 8)
		walker.SetLookup(l)
		walker.WalkExecutable()

		for i := range checks {
			checks[i](walker)
		}
	}

	mustPanic := func(wrapped check) check {
		return func(w *Walker) {
			defer func() {
				err := recover()
				if err == nil {
					panic(fmt.Errorf("want panic, got nothing"))
				}
			}()

			wrapped(w)
		}
	}

	argumentUsedInOperations := func(argumentName string, operationNames ...string) check {
		return func(w *Walker) {
			argSets := w.ArgumentSetIterable()
			for argSets.Next() {
				set, _ := argSets.Value()
				args := w.l.ArgumentsIterable(set)
				for args.Next() {
					arg, ref := args.Value()
					if string(w.l.p.CachedByteSlice(arg.Name)) == argumentName {

						operations := w.NodeUsageInOperationsIterator(ref)
						for i := range operationNames {
							wantName := operationNames[i]
							if !operations.Next() {
								panic(fmt.Errorf("argumentUsedInOperations: want next root operation with name '%s' for argument with name '%s', got nothing", wantName, argumentName))
							}
							ref := operations.Value()
							operationDefinition := w.l.OperationDefinition(ref)
							gotName := string(w.l.p.CachedByteSlice(operationDefinition.Name))
							if wantName != gotName {
								panic(fmt.Errorf("argumentUsedInOperations: want operation name: '%s', got: '%s'", wantName, gotName))
							}
						}

						return
					}
				}
			}
		}
	}

	wantFieldPath := func(forNamedField string, wantPath ...string) check {
		return func(w *Walker) {
			fields := w.FieldsIterable()
			for fields.Next() {
				field, _, parent := fields.Value()
				fieldName := string(w.l.CachedName(field.Name))
				if fieldName != forNamedField {
					continue
				}

				gotPath := w.FieldPath(parent)
				if len(wantPath) != len(gotPath) {
					panic(fmt.Errorf("wantFieldPath: want path with len: %d, got: %d", len(wantPath), len(gotPath)))
				}
				for i, wantName := range wantPath {
					gotName := string(w.l.CachedName(gotPath[len(gotPath)-1-i]))
					if gotName != wantName {
						panic(fmt.Errorf("wantFieldPath: want path field name: %s, got: %s (pos: %d)", wantName, gotName, i))
					}
				}
			}
		}
	}

	t.Run("argumentUsedInOperations", func(t *testing.T) {
		t.Run("get argument root from inside operation definition", func(t *testing.T) {
			run(`	query argOnRequiredArg($booleanArg: Boolean) {
						dog {
							isHousetrained(atOtherHomes: $booleanArg) @include(if: $booleanArg)
						}
					}`, argumentUsedInOperations("atOtherHomes", "argOnRequiredArg"))
		})
		t.Run("get argument root from inside fragment", func(t *testing.T) {
			run(`	query argOnRequiredArg($booleanArg: Boolean) {
						dog {
							...argOnOptional
						}
					}
					fragment argOnOptional on Dog {
						isHousetrained(atOtherHomes: $booleanArg) @include(if: $booleanArg)
					}`, argumentUsedInOperations("atOtherHomes", "argOnRequiredArg"))
		})
		t.Run("get argument root from inside fragment multiple times", func(t *testing.T) {
			run(`	query argOnRequiredArg($booleanArg: Boolean) {
						dog {
							...argOnOptional
							...argOnOptional
							...argOnOptional
						}
					}
					fragment argOnOptional on Dog {
						isHousetrained(atOtherHomes: $booleanArg) @include(if: $booleanArg)
					}`, argumentUsedInOperations("atOtherHomes", "argOnRequiredArg"))
		})
		t.Run("get argument root from inside fragment multiple times (check de-duplicating)", func(t *testing.T) {
			run(`	query argOnRequiredArg($booleanArg: Boolean) {
						dog {
							...argOnOptional
							...argOnOptional
							...argOnOptional
						}
					}
					fragment argOnOptional on Dog {
						isHousetrained(atOtherHomes: $booleanArg) @include(if: $booleanArg)
					}`, mustPanic(argumentUsedInOperations("atOtherHomes", "argOnRequiredArg", "argOnRequiredArg")))
		})
		t.Run("get argument root from inside nested fragment", func(t *testing.T) {
			run(`	query argOnRequiredArg($booleanArg: Boolean) {
						dog {
							...argOnOptional1
						}
					}
					fragment argOnOptional1 on Dog {
						... {
							...on Dog {
								...argOnOptional2
							}
						}
					}
					fragment argOnOptional2 on Dog {
						isHousetrained(atOtherHomes: $booleanArg) @include(if: $booleanArg)
					}`, argumentUsedInOperations("atOtherHomes", "argOnRequiredArg"))
		})
		t.Run("get argument root from inside fragment used in multiple operations", func(t *testing.T) {
			run(`	query argOnRequiredArg1($booleanArg: Boolean) {
						dog {
							...argOnOptional
						}
					}
					query argOnRequiredArg2($booleanArg: Boolean) {
						dog {
							...argOnOptional
						}
					}
					fragment argOnOptional on Dog {
						isHousetrained(atOtherHomes: $booleanArg) @include(if: $booleanArg)
					}`, argumentUsedInOperations("atOtherHomes", "argOnRequiredArg1", "argOnRequiredArg2"))
		})
	})
	t.Run("fieldPath", func(t *testing.T) {
		t.Run("nested 2 levels", func(t *testing.T) {
			run(`{dog{owner{name}}}`, wantFieldPath("name", "dog", "owner"))
		})
		t.Run("nested 3 levels", func(t *testing.T) {
			run(`{dog{owner{another{name}}}}`, wantFieldPath("name", "dog", "owner", "another"))
		})
		t.Run("with inline fragment", func(t *testing.T) {
			run(`{ dog { ... on Dog { owner { name } } } }`, wantFieldPath("name", "dog", "owner"))
		})
		t.Run("with nested inline fragments", func(t *testing.T) {
			run(`{ dog { ... on Dog { ... { owner { name } } } } }`, wantFieldPath("name", "dog", "owner"))
		})
		t.Run("with alias", func(t *testing.T) {
			run(`{dog{renamed:owner{name}}}`, wantFieldPath("name", "dog", "renamed"))
		})
	})
}
