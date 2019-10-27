package execution

import (
	"bytes"
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"text/template"
	"time"
)

type HttpJsonDataSourcePlanner struct {
	walker                *astvisitor.Walker
	definition, operation *ast.Document
	args                  []Argument
}

func (h *HttpJsonDataSourcePlanner) DirectiveName() []byte {
	return []byte("HttpJsonDataSource")
}

func (h *HttpJsonDataSourcePlanner) Initialize(walker *astvisitor.Walker, operation, definition *ast.Document, args []Argument, resolverParameters []ResolverParameter) {
	h.walker, h.operation, h.definition, h.args = walker, operation, definition, args
}

func (h *HttpJsonDataSourcePlanner) Plan() (DataSource, []Argument) {
	return &HttpJsonDataSource{}, h.args
}

func (h *HttpJsonDataSourcePlanner) EnterInlineFragment(ref int) {

}

func (h *HttpJsonDataSourcePlanner) LeaveInlineFragment(ref int) {

}

func (h *HttpJsonDataSourcePlanner) EnterSelectionSet(ref int) {

}

func (h *HttpJsonDataSourcePlanner) LeaveSelectionSet(ref int) {

}

func (h *HttpJsonDataSourcePlanner) EnterField(ref int) {

}

func (h *HttpJsonDataSourcePlanner) LeaveField(ref int) {
	definition, exists := h.walker.FieldDefinition(ref)
	if !exists {
		return
	}
	directive, exists := h.definition.FieldDefinitionDirectiveByName(definition, []byte("HttpJsonDataSource"))
	if !exists {
		return
	}
	value, exists := h.definition.DirectiveArgumentValueByName(directive, literal.URL)
	if !exists {
		return
	}
	variableValue := h.definition.StringValueContentBytes(value.Ref)
	arg := &StaticVariableArgument{
		Name:  literal.URL,
		Value: variableValue,
	}
	h.args = append([]Argument{arg}, h.args...)
	value, exists = h.definition.DirectiveArgumentValueByName(directive, literal.HOST)
	if !exists {
		return
	}
	variableValue = h.definition.StringValueContentBytes(value.Ref)
	arg = &StaticVariableArgument{
		Name:  literal.HOST,
		Value: variableValue,
	}
	h.args = append([]Argument{arg}, h.args...)
}

type HttpJsonDataSource struct{}

func (r *HttpJsonDataSource) Resolve(ctx Context, args ResolvedArgs) []byte {

	hostArg := args.ByKey(literal.HOST)
	urlArg := args.ByKey(literal.URL)

	if hostArg == nil || urlArg == nil {
		return nil
	}

	url := string(hostArg) + string(urlArg)
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		url = "https://" + url
	}

	if strings.Contains(url, "{{") {
		tmpl, err := template.New("url").Parse(url)
		if err != nil {
			log.Fatal(err)
		}
		out := bytes.Buffer{}
		data := make(map[string]string, len(args))
		for i := 0; i < len(args); i++ {
			data[string(args[i].Key)] = string(args[i].Value)
		}
		err = tmpl.Execute(&out, data)
		if err != nil {
			log.Fatal(err)
		}
		url = out.String()
	}

	client := http.Client{
		Timeout: time.Second * 10,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 1024,
			TLSHandshakeTimeout: 0 * time.Second,
		},
	}

	fmt.Printf("Making Request: %s\n", url)

	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return []byte(err.Error())
	}

	request.Header.Add("Accept", "application/json")

	res, err := client.Do(request)
	if err != nil {
		return []byte(err.Error())
	}

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return []byte(err.Error())
	}

	fmt.Printf("Response:\n%s\n", string(data))

	return bytes.ReplaceAll(data, literal.BACKSLASH, nil)
}
