package fields

type Data struct {
	Types []Type
}

type Type struct {
	Name   string
	Fields []string
}
