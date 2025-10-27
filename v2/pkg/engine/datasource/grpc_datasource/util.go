package grpcdatasource

// initializeSlice initializes a slice with a given length and a given value.
func initializeSlice[T any](len int, zero T) []T {
	s := make([]T, len)
	for i := range s {
		s[i] = zero
	}
	return s
}
