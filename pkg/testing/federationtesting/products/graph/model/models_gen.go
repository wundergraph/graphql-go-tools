// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package model

type Product struct {
	Upc     string `json:"upc"`
	Name    string `json:"name"`
	Price   int    `json:"price"`
	InStock int    `json:"inStock"`
}

func (Product) IsEntity() {}

type Query struct {
}

type Subscription struct {
}
