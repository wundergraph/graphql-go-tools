package graph

import "github.com/wundergraph/graphql-go-tools/pkg/testing/federationtesting/accounts/graph/model"

var users = []*model.User{
	{
		ID:               "1",
		Username:         "One",
		History:          histories,
		RealName:         "One Oneton",
		PreferredPayment: debitCard,
	},
	{
		ID:               "2",
		Username:         "Two",
		History:          histories,
		RealName:         "Two Twoson",
		PreferredPayment: creditCard,
	},
	{
		ID:               "3",
		Username:         "Three",
		History:          histories,
		RealName:         "Three Threeton",
		PreferredPayment: fiftyGiftCard,
	},
	{
		ID:               "4",
		Username:         "Four",
		History:          histories,
		RealName:         "Four Fourson",
		PreferredPayment: millionGiftCard,
	},
	{
		ID:               "5",
		Username:         "Five",
		History:          histories,
		RealName:         "Five Fiveson",
		PreferredPayment: eur,
	},
	{
		ID:               "1234",
		Username:         "Me",
		History:          histories,
		RealName:         "User Usington",
		PreferredPayment: usd,
	},
}
