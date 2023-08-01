package graph

import "github.com/wundergraph/graphql-go-tools/v2/pkg/testing/federationtesting/accounts/graph/model"

var walletOne = &model.WalletType1{
	Currency:      "USD",
	Amount:        123,
	SpecialField1: "some special value 1",
}

var walletTwo = &model.WalletType2{
	Currency:      "USD",
	Amount:        123,
	SpecialField2: "some special value 2",
}
