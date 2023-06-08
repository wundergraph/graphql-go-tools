package graph

import (
	"github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/accounts/graph/model"
)

var histories = []model.History{
	&model.Purchase{
		Product: &model.Product{Upc: "top-1"},
		Wallet:  walletOne,
	},
	&model.Sale{
		Product: &model.Product{Upc: "top-2"},
		Rating:  5,
	},
	&model.Purchase{
		Product: &model.Product{Upc: "top-3"},
		Wallet:  walletTwo,
	},
}

var allHistories = []model.History{
	&model.Purchase{
		Product: &model.Product{Upc: "top-1"},
		Wallet:  walletOne,
	},
	&model.Sale{
		Product: &model.Product{Upc: "top-1"},
		Rating:  1,
	},
	&model.Purchase{
		Product: &model.Product{Upc: "top-2"},
		Wallet:  walletTwo,
	},
	&model.Sale{
		Product: &model.Product{Upc: "top-2"},
		Rating:  2,
	},
	&model.Purchase{
		Product: &model.Product{Upc: "top-3"},
		Wallet:  walletTwo,
	},
	&model.Sale{
		Product: &model.Product{Upc: "top-3"},
		Rating:  3,
	},
}
