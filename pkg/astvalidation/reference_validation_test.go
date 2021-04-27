package astvalidation

import (
	"encoding/json"
	"io/ioutil"
	"sync"
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astnormalization"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/asttransform"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

func TestReferenceValidation(t *testing.T) {
	parseReference(t)
	for k, v := range refTests {
		t.Run(k, func(t_ *testing.T) {
			runRule(t_, v)
		})
	}
}

func runRule(t *testing.T, refTs []testRef) {
	for _, v := range refTs {
		t.Run(v.Name, func(t_ *testing.T) {
			t.Log(v.Query)
			queryDoc, report := doc(v.Query)
			err := report.ExternalErrors
			if len(err) > 0 {
				// errors in parsing queries
				// check them separately
				checkErrors(t_, v, err)
				return
			}
			report.Reset()
			norm := astnormalization.NewNormalizer(true, true)
			norm.NormalizeNamedOperation(queryDoc, refSchemas[v.Schema], []byte(v.Name), &report)
			err = report.ExternalErrors
			checkErrors(t_, v, err)
		})
	}
}

func checkErrors(t *testing.T, v testRef, err []operationreport.ExternalError) {
	if len(v.Errors) == len(err) {
		for i := range err {
			refL := v.Errors[i].Locations
			if len(refL) == len(err[i].Locations) {
				for j, l := range err[i].Locations {
					if l.Column == uint32(refL[j].Column) &&
						l.Line == uint32(refL[j].Line) {
						continue
					}
					t.Fatalf("\nexpected: %+v\ngot: %+v\n", l, refL[j])
				}
				continue
			}
			t.Fatalf("\nexpected: %+v\ngot: %+v\n", refL, err[i].Locations)
		}
		return
	}
	t.Fatalf("\nexpected: %+v\ngot: %+v\n", v.Errors, err)
}

func doc(d string) (*ast.Document, operationreport.Report) {
	astDoc, report := astparser.ParseGraphqlDocumentString(d)
	return &astDoc, report
}

// schemasRef the format schemas as from testdata/schemas.json
type schemasRef struct {
	Schemas []string `json:"schemas"`
}

// testRef the format of a test as from testdata/tests.json
type testRef struct {
	Name   string `json:"name"`
	Rule   string `json:"rule"`
	Query  string `json:"query"`
	Schema int    `json:"schema"`
	Errors []struct {
		Message   string `json:"message"`
		Locations []struct {
			Line   int `json:"line"`
			Column int `json:"column"`
		} `json:"locations"`
	} `json:"errors"`
}

var (
	refSchemas   []*ast.Document
	refTests     map[string][]testRef
	parseRefOnce sync.Once
)

func parseReference(t *testing.T) {
	parseRefOnce.Do(func() {
		r, err := ioutil.ReadFile("./testdata/schemas.json")
		if err != nil {
			t.Fatal(err.Error())
			return
		}
		var refS schemasRef
		err = json.Unmarshal(r, &refS)
		if err != nil {
			t.Fatal(err.Error())
			return
		}
		for _, v := range refS.Schemas {
			s, e := doc(v)
			if e.HasErrors() {
				t.Fatal(e.Error())
			}
			err := asttransform.MergeDefinitionWithBaseSchema(s)
			if err != nil {
				t.Fatal(err.Error())
			}
			refSchemas = append(refSchemas, s)
		}
		r, err = ioutil.ReadFile("./testdata/tests.json")
		if err != nil {
			t.Fatal(err.Error())
			return
		}
		var refT struct {
			Tests []testRef `json:"tests"`
		}
		err = json.Unmarshal(r, &refT)
		if err != nil {
			t.Fatal(err.Error())
			return
		}
		// group unit tests by rule
		refTests = make(map[string][]testRef)
		for _, v := range refT.Tests {
			refTests[v.Rule] = append(refTests[v.Rule], v)
		}
	})
}
