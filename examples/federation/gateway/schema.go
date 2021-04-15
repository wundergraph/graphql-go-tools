package main

const baseSchema = `
type Query {
	me: User
	topProducts(first: Int = 5): [Product]
}

type Subscription {
	updatedPrice: Product!
}
		
type User {
	id: ID!
	username: String
	reviews: [Review]
}

type Product {
	upc: String!
	name: String
	price: Int
	reviews: [Review]
}

type Review {
	body: String
	author: User
	product: Product
}`
