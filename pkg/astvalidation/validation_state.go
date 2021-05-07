package astvalidation

// ValidationState is the outcome of a validation
type ValidationState int

const (
	UnknownState ValidationState = iota
	Valid
	Invalid
)
