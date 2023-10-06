package graph

import (
	"github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/accounts/graph/model"
)

var histories = []model.History{
	&model.Purchase{
		Product:  &model.Product{Upc: "top-1"},
		Wallet:   walletOne,
		Quantity: 1,
	},
	&model.Sale{
		Product:  &model.Product{Upc: "top-2"},
		Rating:   5,
		Location: "Germany",
	},
	&model.Purchase{
		Product:  &model.Product{Upc: "top-3"},
		Wallet:   walletTwo,
		Quantity: 3,
	},
}

var allHistories = []model.History{
	&model.Purchase{
		Product:  &model.Product{Upc: "top-1"},
		Wallet:   walletOne,
		Quantity: 1,
	},
	&model.Sale{
		Product:  &model.Product{Upc: "top-1"},
		Rating:   1,
		Location: "Germany",
	},
	&model.Purchase{
		Product:  &model.Product{Upc: "top-2"},
		Wallet:   walletTwo,
		Quantity: 2,
	},
	&model.Sale{
		Product:  &model.Product{Upc: "top-2"},
		Rating:   2,
		Location: "UK",
	},
	&model.Purchase{
		Product:  &model.Product{Upc: "top-3"},
		Wallet:   walletTwo,
		Quantity: 3,
	},
	&model.Sale{
		Product:  &model.Product{Upc: "top-3"},
		Rating:   3,
		Location: "Ukraine",
	},
}
