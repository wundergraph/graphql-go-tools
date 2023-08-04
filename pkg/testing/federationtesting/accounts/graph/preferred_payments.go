package graph

import "github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/accounts/graph/model"

var debitCard = model.Card{
	Medium:   model.PaymentMediumDigital,
	CardType: model.CardTypeVisa,
}

var creditCard = model.Card{
	Medium:   model.PaymentMediumDigital,
	CardType: model.CardTypeMastercard,
}

var fiftyGiftCard = model.GiftCard{
	Medium: model.PaymentMediumBespoke,
}

var millionGiftCard = model.GiftCard{
	Medium: model.PaymentMediumBespoke,
}

var eur = model.Cash{
	Medium: model.PaymentMediumMaterial,
}

var usd = model.Cash{
	Medium: model.PaymentMediumMaterial,
}
