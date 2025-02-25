package permutations

import (
	"gonum.org/v1/gonum/stat/combin"
)

func OrderDS[T any](dataSources []T, order []int) (out []T) {
	out = make([]T, 0, len(dataSources))

	for _, i := range order {
		out = append(out, dataSources[i])
	}

	return out
}

func Generate[T any](dataSources []T) []*Permutation[T] {
	size := len(dataSources)
	elementsCount := len(dataSources)
	list := combin.Permutations(size, elementsCount)

	permutations := make([]*Permutation[T], 0, len(list))

	for _, v := range list {
		permutations = append(permutations, &Permutation[T]{
			Order:       v,
			DataSources: OrderDS(dataSources, v),
		})
	}

	return permutations
}

type Permutation[T any] struct {
	Order       []int
	DataSources []T
}
