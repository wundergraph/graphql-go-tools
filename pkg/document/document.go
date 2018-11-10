package document

// DirectiveAppliable applies to all structs containing document.Directives
type DirectiveAppliable interface {
	ObjectName() string
	GetDirectives() Directives
	DirectiveLocation() DirectiveLocation
}
