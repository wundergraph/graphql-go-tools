package execution

type Transformation interface {
	Transform(input []byte) []byte
}
