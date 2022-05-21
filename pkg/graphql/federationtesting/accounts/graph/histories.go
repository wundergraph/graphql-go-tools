package graph

import (
	"github.com/wundergraph/graphql-go-tools/pkg/graphql/federationtesting/accounts/graph/model"
)

var histories = []model.History{
	&model.Purchase{
		Product: &model.Product{Upc: "top-1"},
		Wallet: &model.WalletType1{
			Currency:      "USD",
			Amount:        123,
			SpecialField1: "some special value 1",
		},
	},
	&model.Sale{
		Product: &model.Product{Upc: "top-2"},
		Rating:  5,
	},
	&model.Purchase{
		Product: &model.Product{Upc: "top-3"},
		Wallet: &model.WalletType2{
			Currency:      "USD",
			Amount:        123,
			SpecialField2: "some special value 2",
		},
	},
}
