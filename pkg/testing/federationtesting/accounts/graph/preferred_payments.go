package graph

import "github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/accounts/graph/model"

var debitCard = model.Card{
	Name:          "VISA",
	IsContactless: true,
}

var creditCard = model.Card{
	Name:          "MasterCard",
	IsContactless: false,
}

var fiftyGiftCard = model.GiftCard{
	Name:         "50 dollary doos",
	IsRefundable: false,
}

var millionGiftCard = model.GiftCard{
	Name:         "one MILLION dollars",
	IsRefundable: true,
}

var eur = model.Cash{
	Name:            "EUR",
	RequiresReceipt: true,
}

var usd = model.Cash{
	Name:            "USD",
	RequiresReceipt: false,
}
