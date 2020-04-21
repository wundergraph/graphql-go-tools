package fields

type Data struct {
	Types Types
}

type Types []Type

type Type struct {
	Name   string
	Fields []string
}
