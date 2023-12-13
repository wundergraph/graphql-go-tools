package astnormalization

import (
	"testing"
)

func TestVariablesExtraction(t *testing.T) {
	t.Run("simple http bin example", func(t *testing.T) {
		runWithVariables(t, extractVariables, variablesExtractionDefinition, `
			mutation HttpBinPost {
			  httpBinPost(input: {foo: "bar"}){
				headers {
				  userAgent
				}
				data {
				  foo
				}
			  }
			}`, "", `
			mutation HttpBinPost($a: HttpBinPostInput){
			  httpBinPost(input: $a){
				headers {
				  userAgent
				}
				data {
				  foo
				}
			  }
			}`, ``, `{"a":{"foo":"bar"}}`)
	})
	t.Run("enum", func(t *testing.T) {
		runWithVariables(t, extractVariables, forumExampleSchema, `
			mutation EnumOperation {
			  useEnum(simpleEnum: Foo)
			}`,
			"EnumOperation", `
			mutation EnumOperation($a: SimpleEnum) {
			  useEnum(simpleEnum: $a)
			}`,
			``,
			`{"a":"Foo"}`)
	})
	t.Run("variables in argument", func(t *testing.T) {
		runWithVariables(t, extractVariables, variablesExtractionDefinition, `
			mutation HttpBinPost($foo: String! = "bar") {
			  httpBinPost(input: {foo: $foo}){
				headers {
				  userAgent
				}
				data {
				  foo
				}
			  }
			}`, "", `
			mutation HttpBinPost($foo: String! = "bar") {
			  httpBinPost(input: {foo: $foo}){
				headers {
				  userAgent
				}
				data {
				  foo
				}
			  }
			}`, ``, ``)
	})
	t.Run("multiple queries", func(t *testing.T) {
		runWithVariables(t, extractVariables, forumExampleSchema, `
			mutation Register {
			  createUser(input: {user: {id: "jens" username: "jens"}}){
				user {
				  id
				  username
				}
			  }
			}
			mutation CreatePost {
			  createPost(input: {post: {authorId: "jens" title: "my post" body: "my body"}}){
				post {
				  id
				  title
				  body
				  userByAuthorId {
					username
				  }
				}
			  }
			}`, "Register", `
			mutation Register($a: CreateUserInput) {
			  createUser(input: $a){
				user {
				  id
				  username
				}
			  }
			}
			mutation CreatePost {
			  createPost(input: {post: {authorId: "jens" title: "my post" body: "my body"}}){
				post {
				  id
				  title
				  body
				  userByAuthorId {
					username
				  }
				}
			  }
			}`, ``, `{"a":{"user":{"id":"jens","username":"jens"}}}`)
	})
	t.Run("values on directives should be ignored", func(t *testing.T) {
		runWithVariables(t, extractVariables, forumExampleSchema, `
			mutation Register($a: CreateUserInput @foo(name: "bar")) {
			  createUser(input: $a){
				user {
				  id
				  username
				}
			  }
			}`, "Register", `
			mutation Register($a: CreateUserInput @foo(name: "bar")) {
			  createUser(input: $a){
				user {
				  id
				  username
				}
			  }
			}`, ``, ``)
	})
	t.Run("complex nesting", func(t *testing.T) {
		runWithVariables(t, extractVariables, authSchema, `
			mutation Login ($phoneNumber: String!) {
				Login: postPasswordlessStart(
					postPasswordlessStartInput: {
						applicationId: "123"
						loginId: $phoneNumber
					}
				) {
					code
				}
			}`, "Login", `
			mutation Login ($phoneNumber: String! $a: String) {
				Login: postPasswordlessStart(
					postPasswordlessStartInput: {
						applicationId: $a
						loginId: $phoneNumber
					}
				) {
					code
				}
			}`, ``, `{"a":"123"}`)
	})
	t.Run("complex nesting with existing variable", func(t *testing.T) {
		runWithVariables(t, extractVariables, authSchema, `
			mutation Login ($phoneNumber: String!) {
				Login: postPasswordlessStart(
					postPasswordlessStartInput: {
						applicationId: "123"
						loginId: $phoneNumber
					}
				) {
					code
				}
			}`, "Login", `
			mutation Login ($phoneNumber: String! $a: String) {
				Login: postPasswordlessStart(
					postPasswordlessStartInput: {
						applicationId: $a
						loginId: $phoneNumber
					}
				) {
					code
				}
			}`, `{"phoneNumber":"456"}`, `{"a":"123","phoneNumber":"456"}`)
	})
	t.Run("complex nesting with deep nesting", func(t *testing.T) {
		runWithVariables(t, extractVariables, authSchema, `
			mutation Login ($phoneNumber: String!) {
				Login: postPasswordlessStart(
					postPasswordlessStartInput: {
						nested: {
							applicationId: "123"
							loginId: $phoneNumber
						}
					}
				) {
					code
				}
			}`, "Login", `
			mutation Login ($phoneNumber: String! $a: String) {
				Login: postPasswordlessStart(
					postPasswordlessStartInput: {
						nested: {
							applicationId: $a
							loginId: $phoneNumber
						}
					}
				) {
					code
				}
			}`, `{"phoneNumber":"456"}`, `{"a":"123","phoneNumber":"456"}`)
	})
	t.Run("complex nesting with deep nesting and lists", func(t *testing.T) {
		runWithVariables(t, extractVariables, authSchema, `
			mutation Login ($phoneNumber: String!) {
				Login: postPasswordlessStartList(
					postPasswordlessStartInput: [{
						nested: {
							applicationId: "123"
							loginId: $phoneNumber
						}
					}]
				) {
					code
				}
			}`, "Login", `
			mutation Login ($phoneNumber: String! $a: String) {
				Login: postPasswordlessStartList(
					postPasswordlessStartInput: [{
						nested: {
							applicationId: $a
							loginId: $phoneNumber
						}
					}]
				) {
					code
				}
			}`, `{"phoneNumber":"456"}`, `{"a":"123","phoneNumber":"456"}`)
	})
	t.Run("complex nesting with variable in list", func(t *testing.T) {
		runWithVariables(t, extractVariables, authSchema, `
			mutation Login ($input: postPasswordlessStartInput!) {
				Login: postPasswordlessStartList(
					postPasswordlessStartInput: [$input]
				) {
					code
				}
			}`, "Login", `
			mutation Login ($input: postPasswordlessStartInput!) {
				Login: postPasswordlessStartList(
					postPasswordlessStartInput: [$input]
				) {
					code
				}
			}`, ``, ``)
	})
	t.Run("nested inline string", func(t *testing.T) {
		runWithVariables(t, extractVariables, nexusSchema, `
			mutation Draw ($drawDate: AWSDate!, $play: PlayInput!) {
				AddTicket: addCartItem(
					item: {
						drawDate: $drawDate
						fractional: false
						play: $play
						quantity: 1
						regionGameId: "lucky7|UAE"
					}
				) {
					id
				}
			}`, "Draw", `
			mutation Draw ($drawDate: AWSDate!, $play: PlayInput! $a: Boolean! $b: Int! $c: String!) {
				AddTicket: addCartItem(
					item: {
						drawDate: $drawDate
						fractional: $a
						play: $play
						quantity: $b
						regionGameId: $c
					}
				) {
					id
				}
			}`, `{}`, `{"c":"lucky7|UAE","b":1,"a":false}`)
	})

	t.Run("same string", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				foo(input: {string: "foo"})
				foo(input: {string: "foo"})
			}`,
			"Foo", `
			mutation Foo($a: FooInput) {
				foo(input: $a)
				foo(input: $a)
			}`,
			`{}`, `{"a":{"string":"foo"}}`)
	})

	t.Run("same string arg", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				bar(string: "foo")
				bar(string: "foo")
			}`,
			"Foo", `
			mutation Foo($a: String) {
				bar(string: $a)
				bar(string: $a)
			}`,
			`{}`, `{"a":"foo"}`)
	})

	t.Run("same string arg with user variables", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo ($another: String) {
				another: bar(string: $another)
				bar(string: "foo")
				bar(string: "foo")
			}`,
			"Foo", `
			mutation Foo($another: String $a: String) {
				another: bar(string: $another)
				bar(string: $a)
				bar(string: $a)
			}`,
			`{"another":"foo"}`, `{"a":"foo","another":"foo"}`)
	})

	t.Run("same int arg", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				baz(int: 1)
				baz(int: 1)
			}`,
			"Foo", `
			mutation Foo($a: Int) {
				baz(int: $a)
				baz(int: $a)
			}`,
			`{}`, `{"a":1}`)
	})

	t.Run("same strings", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				foo(input: {strings: ["foo", "bar"]})
				foo(input: {strings: ["foo", "bar"]})
			}`,
			"Foo", `
			mutation Foo($a: FooInput) {
				foo(input: $a)
				foo(input: $a)
			}`,
			`{}`, `{"a":{"strings":["foo","bar"]}}`)
	})

	t.Run("same int", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				foo(input: {int: 1})
				foo(input: {int: 1})
			}`,
			"Foo", `
			mutation Foo($a: FooInput) {
				foo(input: $a)
				foo(input: $a)
			}`,
			`{}`, `{"a":{"int":1}}`)
	})

	t.Run("same ints", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				foo(input: {ints: [1, 2]})
				foo(input: {ints: [1, 2]})
			}`,
			"Foo", `
			mutation Foo($a: FooInput) {
				foo(input: $a)
				foo(input: $a)
			}`,
			`{}`, `{"a":{"ints":[1,2]}}`)
	})

	t.Run("same float", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				foo(input: {float: 1.1})
				foo(input: {float: 1.1})
			}`,
			"Foo", `
			mutation Foo($a: FooInput) {
				foo(input: $a)
				foo(input: $a)
			}`,
			`{}`, `{"a":{"float":1.1}}`)
	})

	t.Run("same floats", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				foo(input: {floats: [1.1, 2.2]})
				foo(input: {floats: [1.1, 2.2]})
			}`,
			"Foo", `
			mutation Foo($a: FooInput) {
				foo(input: $a)
				foo(input: $a)
			}`,
			`{}`, `{"a":{"floats":[1.1,2.2]}}`)
	})

	t.Run("same boolean", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				foo(input: {boolean: true})
				foo(input: {boolean: true})
			}`,
			"Foo", `
			mutation Foo($a: FooInput) {
				foo(input: $a)
				foo(input: $a)
			}`,
			`{}`, `{"a":{"boolean":true}}`)
	})

	t.Run("same booleans", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				foo(input: {booleans: [true, false]})
				foo(input: {booleans: [true, false]})
			}`,
			"Foo", `
			mutation Foo($a: FooInput) {
				foo(input: $a)
				foo(input: $a)
			}`,
			`{}`, `{"a":{"booleans":[true,false]}}`)
	})

	t.Run("same id", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				foo(input: {id: "foo"})
				foo(input: {id: "foo"})
			}`,
			"Foo", `
			mutation Foo($a: FooInput) {
				foo(input: $a)
				foo(input: $a)
			}`, `{}`, `{"a":{"id":"foo"}}`)
	})

	t.Run("same ids", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				foo(input: {ids: ["foo", "bar"]})
				foo(input: {ids: ["foo", "bar"]})
			}`,
			"Foo", `
			mutation Foo($a: FooInput) {
				foo(input: $a)
				foo(input: $a)
			}`, `{}`, `{"a":{"ids":["foo","bar"]}}`)
	})

	t.Run("same enum", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				foo(input: {enum: Foo})
				foo(input: {enum: Foo})
			}`,
			"Foo", `
			mutation Foo($a: FooInput) {
				foo(input: $a)
				foo(input: $a)
			}`, `{}`, `{"a":{"enum":"Foo"}}`)
	})

	t.Run("same enums", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				foo(input: {enums: [Foo, Bar]})
				foo(input: {enums: [Foo, Bar]})
			}`,
			"Foo", `
			mutation Foo($a: FooInput) {
				foo(input: $a)
				foo(input: $a)
			}`, `{}`, `{"a":{"enums":["Foo","Bar"]}}`)
	})

	t.Run("same input", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				foo(input: {input: {string: "foo"}})
				foo(input: {input: {string: "foo"}})
			}`,
			"Foo", `
			mutation Foo($a: FooInput) {
				foo(input: $a)
				foo(input: $a)
			}`, `{}`, `{"a":{"input":{"string":"foo"}}}`)
	})

	t.Run("same inputs", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				foo(
					input: {
						inputs: [
							{string: "foo"}
							{string: "bar"}
						]
					}
				)
				foo(
					input: {
						inputs: [
							{string: "foo"}
							{string: "bar"}
						]
					}
				)
			}`,
			"Foo", `
			mutation Foo($a: FooInput) {
				foo(
					input: $a
				)
				foo(
					input: $a
				)
			}`, `{}`, `{"a":{"inputs":[{"string":"foo"},{"string":"bar"}]}}`)
	})

	t.Run("same customScalar", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				foo(input: {customScalar: "foo"})
				foo(input: {customScalar: "foo"})
			}`,
			"Foo", `
			mutation Foo($a: FooInput) {
				foo(input: $a)
				foo(input: $a)
			}`, `{}`, `{"a":{"customScalar":"foo"}}`)
	})

	t.Run("same customScalars", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				foo(input: {customScalars: ["foo", "bar"]})
				foo(input: {customScalars: ["foo", "bar"]})
			}`,
			"Foo", `
			mutation Foo($a: FooInput) {
				foo(input: $a)
				foo(input: $a)
			}`, `{}`, `{"a":{"customScalars":["foo","bar"]}}`)
	})

	t.Run("same variable", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo($a: String!) {
				foo(input: {string: $a})
				foo(input: {string: $a})
			}`,
			"Foo", `
			mutation Foo($a: String!) {
				foo(input: {string: $a})
				foo(input: {string: $a})
			}`, `{"a":"foo"}`, `{"a":"foo"}`)
	})

	t.Run("same variables", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo($a: String!, $b: String!) {
				foo(input: {string: $a})
				foo(input: {string: $b})
			}`,
			"Foo", `
			mutation Foo($a: String!, $b: String!) {
				foo(input: {string: $a})
				foo(input: {string: $b})
			}`, `{"a":"foo","b":"bar"}`, `{"a":"foo","b":"bar"}`)
	})

	t.Run("ignore user variables same string", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo($fromUser: MyInput!) {
				another: foo(input: $fromUser)
				foo(input: {string: "foo"})
				foo(input: {string: "foo"})
			}`,
			"Foo", `
			mutation Foo($fromUser: MyInput! $a: FooInput) {
				another: foo(input: $fromUser)
				foo(input: $a)
				foo(input: $a)
			}`,
			`{"fromUser":{"string":"foo"}}`, `{"a":{"string":"foo"},"fromUser":{"string":"foo"}}`)
	})

	t.Run("don't re-use same input of different type", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				foo(input: {string: "foo"})
				bat(input: {string: "foo"})
			}`,
			"Foo", `
			mutation Foo($a: FooInput $b: SimilarMyInput) {
				foo(input: $a)
				bat(input: $b)
			}`, `{}`, `{"b":{"string":"foo"},"a":{"string":"foo"}}`)
	})

	t.Run("don't re-use same nested input of different type", func(t *testing.T) {
		runWithVariables(t, extractVariables, sameVariableExtraction, `
			mutation Foo {
				foo(input: {input: {string: "foo"}})
				bat(input: {input: {string: "foo"}})
			}`,
			"Foo", `
			mutation Foo($a: FooInput $b: SimilarMyInput) {
				foo(input: $a)
				bat(input: $b)
			}`, `{}`, `{"b":{"input":{"string":"foo"}},"a":{"input":{"string":"foo"}}}`)
	})
}

const (
	variablesExtractionDefinition = `
		schema { mutation: Mutation }
		type Mutation {
			httpBinPost(input: HttpBinPostInput): HttpBinPostResponse
		}
		input HttpBinPostInput {
			foo: String!
		}
		type HttpBinPostResponse {
			headers: Headers
			data: HttpBinPostResponseData
		}
		type HttpBinPostResponseData {
			foo: String
		}
		type Headers {
			userAgent: String!
			host: String!
			acceptEncoding: String
			Authorization: String
		}
		scalar String
	`
)

const (
	sameVariableExtraction = `

		schema { query: Query mutation: Mutation }

		scalar ID
		scalar CustomScalar

		enum MyEnum {
			FOO
			BAR
		}

		type Query {
			hello: String
		}

		type Mutation {
			foo(input: FooInput): String
			bar(string: String): String
			baz(int: Int): String
			bat(input: SimilarMyInput): String
		}

		input MyInput {
			string: String
			strings: [String]
			int: Int
			ints: [Int]
			float: Float
			floats: [Float]
			boolean: Boolean
			booleans: [Boolean]
			id: ID
			ids: [ID]
			enum: MyEnum
			enums: [MyEnum]
			input: MyInput
			inputs: [MyInput]
			customScalar: CustomScalar
			customScalars: [CustomScalar]
		}

		input SimilarMyInput {
			string: String
			input: SimilarMyInput
		}
`
)

const forumExampleSchema = `
schema {
	mutation: Mutation
}
scalar String
enum SimpleEnum {
	Foo
	Bar
}
type Mutation {
	createUser(input: CreateUserInput): CreateUser
	createPost(input: CreatePostInput): CreatePost
	useEnum(simpleEnum: SimpleEnum): String
}
input CreateUserInput {
	user: UserInput
}
input UserInput {
	id: String!
	username: String!
}
input CreatePostInput {
	post: PostInput
}
input PostInput {
	authorId: String!
	title: String!
	body: String!
}
type CreateUser {
	user: User
}
type CreatePost {
	post: Post
}
type User {
	id: String!
  	username: String!
}
type Post {
  id: String!
  title: String!
  body: String!
  userByAuthorId: User
}
`

const authSchema = `
type Mutation {
  postPasswordlessStart(postPasswordlessStartInput: postPasswordlessStartInput): PostPasswordlessStart
  postPasswordlessStartList(postPasswordlessStartInput: [postPasswordlessStartInput]): PostPasswordlessStart
  postPasswordlessLogin(postPasswordlessLoginInput: postPasswordlessLoginInput): PostPasswordlessLogin
}

type PostPasswordlessStart {
  code: String
}

input postPasswordlessStartInput {
  applicationId: String
  loginId: String
  nested: postPasswordlessStartInput
}

type PostPasswordlessLogin {
  refreshToken: String
  token: String
  user: User
}

type User {
  username: String
  verified: Boolean
  firstName: String
  lastName: String
  email: String
  mobilePhone: String
  timezone: String
}

input postPasswordlessLoginInput {
  code: String
  ipAddress: String
  metaData: MetaDataInput
}

input MetaDataInput {
  device: DeviceInput
}

input DeviceInput {
  name: String
}
`

const nexusSchema = `
type Mutation {
    postPasswordlessStart(postPasswordlessStartInput: postPasswordlessStartInput): PostPasswordlessStartResponse
    postPasswordlessLogin(postPasswordlessLoginInput: postPasswordlessLoginInput): PostPasswordlessLoginResponse
    postJwtRefresh(postJwtRefreshInput: postJwtRefreshInput): PostJwtRefreshResponse
    acceptPoolInvite(poolId: String!): Boolean!
    addCartItem(item: AddCartItemInput!): Cart!
    addTicketToPool(poolId: String!, ticketId: String!): Boolean!
    archiveAggTicket(archived: Boolean!, id: ID!): AggTicket!
    cancelOrder(input: CancelOrderInput!): Order!
    createCancelOrderTask(input: CreateCancelOrderTaskInput!): CancelOrderTask!
    createLocationGame(locationGame: CreateLocationGameInput!): LocationGame!
    createPool(pool: CreatePoolInput!): Pool!
    createRecurringOrder(recurringOrder: CreateRecurringOrderInput!): RecurringOrder!
    createRegionGame(regionGame: CreateRegionGameInput!): RegionGame!
    createRegionGameDraw(regionGameDraw: CreateRegionGameDrawInput!): RegionGameDraw!
    deleteLocationGame(id: ID!): Boolean!
    deletePool(id: ID!): Boolean!
    deleteRecurringOrder(id: ID!): Boolean!
    deleteRegionGame(id: ID!): Boolean!
    emptyCart: Cart!
    expressCheckout: Order!
    generateFreeTicket(freeTicket: GenerateFreeTicketInput!): [Ticket]!
    inviteToPool(poolId: String!, userId: String!): PoolInvite!
    leavePool(poolId: String!): Boolean!
    ledgerTransfer(options: TransferOptions, requests: [TransferRequest!]!): [LedgerTransferResponse!]!
    markTaskComplete(input: MarkTaskCompleteInput!): Task!
    markTaskFailed(input: MarkTaskFailedInput!): Task!
    registerDevice(device: RegisterDeviceInput!): Device!
    rejectPoolInvite(poolId: String!): Boolean!
    removeCartItem(index: Int!): Cart!
    removeTicketFromPool(ticketId: String!): Boolean!
    sendReceiptDuplicate(orderId: String!): Boolean!
    startWinningsProcess(input: StartWinningsProcessInput!): StepFunctionsExecution!
    unregisterDevice(device: UnregisterDeviceInput!): Boolean!
    updateBigWinningTask(bigWinningTask: UpdateBigWinningTaskInput!): BigWinningTask!
    updateCancelOrderTask(input: UpdateCancelOrderTaskInput!): CancelOrderTask!
    updateLocationGame(locationGame: UpdateLocationGameInput!): LocationGame!
    updatePool(pool: UpdatePoolInput!): Pool!
    updatePricingRule(pricingRule: UpdatePricingRuleInput!): PricingRule!
    updateProfile(profile: UpdateProfileInput): User!
    updateRecurringOrder(recurringOrder: UpdateRecurringOrderInput!): RecurringOrder!
    updateRegionGame(regionGame: UpdateRegionGameInput!): RegionGame!
    updateRegionGameDraw(regionGameDraw: UpdateRegionGameDrawInput!): RegionGameDraw!
    validateBigWinningNotificationTask(id: ID!): BigWinningNotificationTask!
}

union PostPasswordlessStartResponse = UnspecifiedHttpResponse | PostPasswordlessStartOK | PostPasswordlessStartBadRequest | PostPasswordlessStartNoAuthProvided | PostPasswordlessStartUserNotFound | PostPasswordlessStartInternalError

type UnspecifiedHttpResponse {
    statusCode: Int!
}

type PostPasswordlessStartOK {
    code: String
}

type PostPasswordlessStartBadRequest {
    message: String
}

type PostPasswordlessStartNoAuthProvided {
    message: String
}

type PostPasswordlessStartUserNotFound {
    message: String
}

type PostPasswordlessStartInternalError {
    message: String
}

input postPasswordlessStartInput {
    applicationId: String!
    loginId: String!
}

union PostPasswordlessLoginResponse = UnspecifiedHttpResponse | PostPasswordlessLoginOK | PostPasswordlessLoginNotRegisteredForApp | PostPasswordlessLoginPasswordChangeRequested | PostPasswordlessLoginEmailNotVerified | PostPasswordlessLoginRegistrationNotVerified | PostPasswordlessLoginTwoFactorEnabled | PostPasswordlessLoginBadRequest | PostPasswordlessLoginInternalError

type PostPasswordlessLoginOK {
    refreshToken: String
    token: String
    user: User
}

type NexusUser {
    username: String
    verified: Boolean
    firstName: String
    lastName: String
    email: String
    mobilePhone: String
    timezone: String
}

type PostPasswordlessLoginNotRegisteredForApp {
    message: String
}

type PostPasswordlessLoginPasswordChangeRequested {
    changePasswordReason: String
}

type PostPasswordlessLoginEmailNotVerified {
    message: String
}

type PostPasswordlessLoginRegistrationNotVerified {
    message: String
}

type PostPasswordlessLoginTwoFactorEnabled {
    twoFactorId: String
}

type PostPasswordlessLoginBadRequest {
    message: String
}

type PostPasswordlessLoginInternalError {
    message: String
}

input postPasswordlessLoginInput {
    code: String!
    ipAddress: String
    metaData: MetaDataInput
}

input MetaDataInput {
    device: DeviceInput
}

input DeviceInput {
    name: String
}

union PostJwtRefreshResponse = UnspecifiedHttpResponse | PostJwtRefreshOK | PostJwtRefreshBadRequest | PostJwtRefreshNoAuthProvided | PostJwtRefreshTokenNotFound | PostJwtRefreshInternalError

type PostJwtRefreshOK {
    refreshToken: String
    token: String
}

type PostJwtRefreshBadRequest {
    message: String
}

type PostJwtRefreshNoAuthProvided {
    message: String
}

type PostJwtRefreshTokenNotFound {
    message: String
}

type PostJwtRefreshInternalError {
    message: String
}

input postJwtRefreshInput {
    refreshToken: String
    token: String
}

scalar AWSDate

scalar AWSDateTime

scalar AWSJSON

scalar AWSTime

scalar AWSEmail

type AggTicket {
    archived: Boolean!
    draw: RegionGameDraw!
    drawDate: AWSDate!
    game: RegionGame!
    id: ID!
    regionGameId: String!
    tickets: [Ticket!]!
    userId: String!
}

type AggTicketsResult {
    items: [AggTicket!]!
    nextToken: String
}

type BigWinningNotificationTask {
    drawDate: AWSDate!
    id: ID!
    regionGameId: String!
    status: BigWinningTaskStatus!
}

type BigWinningNotificationTasksResult {
    items: [BigWinningNotificationTask!]
    nextToken: String
}

type BigWinningTask {
    drawDate: AWSDate!
    id: ID!
    regionGameId: String!
    status: BigWinningTaskStatus!
}

type BigWinningTasksResult {
    items: [BigWinningTask!]
    nextToken: String
}

type CancelOrderTask {
    createdAt: AWSDateTime!
    id: ID!
    orderId: String!
    status: CancelOrderTaskStatus!
    userId: String!
}

type CancelOrderTasksResult {
    items: [CancelOrderTask!]
    nextToken: String
}

type Cart {
    id: ID!
    items: [CartItem!]!
    serviceFee: Price!
    total: Price!
    userId: String!
}

type CartItem {
    drawDate: AWSDate!
    fractional: Boolean!
    play: Play!
    price: Price!
    quantity: Int!
    regionGameId: String!
}

type Currency {
    code: String!
}

type Device {
    deviceId: ID!
    provider: PushNotificationProvider!
    token: String!
}

type DrawResults {
    prizes: AWSJSON
    result: AWSJSON
}

type FreeTicket {
    drawDate: AWSDate!
    generatedTicketId: String
    id: ID!
    regionGameId: String!
    status: String!
}

type FreeTicketsResult {
    items: [FreeTicket!]!
    nextToken: String
}

type GameSchemas {
    play: AWSJSON
    prizes: AWSJSON
    result: AWSJSON
}

type Ledger {
    balance: Price
    id: ID!
    transactions: [LedgerTransaction!]!
    type: LedgerType
}

type LedgerTransaction {
    amount: Price!
    createdAt: AWSDateTime!
    description: String
    id: ID!
    ledgerId: String!
    reference: String!
    relatedTransactionId: String!
}

type LedgerTransactionsResult {
    items: [LedgerTransaction!]!
    nextToken: String
}

type LedgerTransferResponse {
    amount: Price!
    description: String
    destinationLedgerId: String!
    destinationTransactionId: String!
    reference: String!
    sourceLedgerId: String!
    sourceTransactionId: String!
}

type LedgersResult {
    items: [Ledger!]!
    nextToken: String
}

type LocationGame {
    enabled: Boolean!
    fractions: Int
    game: RegionGame!
    id: ID!
}

type LocationGamesResult {
    items: [LocationGame!]!
    nextToken: String
}

type Order {
    createdAt: AWSDateTime
    fulfilledAt: AWSDateTime
    id: ID!
    isCanceled: Boolean!
    items: [OrderItem!]!
    locationId: String
    refundAmount: Price
    refundDestination: RefundDestinationEnum
    serviceFee: Price!
    status: OrderStatus!
    submittedAt: AWSDateTime
    total: Price!
}

type OrderItem {
    cancelAction: ActionEnum
    fractional: Boolean!
    id: ID!
    play: Play!
    price: Price!
    quantity: Int!
    regionGameId: String!
    ticketId: String
}

type OrdersResult {
    items: [Order!]!
    nextToken: String
}

type Play {
    options: AWSJSON
    pick: [String!]!
}

type Pool {
    id: ID!
    name: String!
    userCount: Int!
}

type PoolInvite {
    status: PoolInviteStatus!
    user: User!
    userId: String!
}

type PoolInvitesResult {
    items: [PoolInvite!]!
    nextToken: String
}

type PoolUser {
    joinedAt: AWSDateTime!
    user: User!
}

type PoolUsersResult {
    items: [PoolUser!]!
    nextToken: String
}

type PoolsResult {
    items: [Pool!]!
    nextToken: String
}

type PreNotifications {
    email: Boolean
    push: Boolean
}

type Price {
    amount: Float!
    currency: Currency!
}

type PricingRule {
    actor: String
    id: String!
    latest: Int
    rules: AWSJSON!
    type: PricingRuleType!
    version: Int!
}

type PricingRulesResult {
    items: [PricingRule!]!
    nextToken: String
}

type Query {
    aggTicket(id: ID!): AggTicket!
    aggTickets(filters: AggTicketsFilters, pagination: Pagination): AggTicketsResult!
    bigWinningNotificationTask(id: ID!): BigWinningNotificationTask!
    bigWinningNotificationTasks(filters: BigWinningNotificationTasksFilters!, pagination: Pagination): BigWinningNotificationTasksResult!
    bigWinningTask(id: ID): BigWinningTask!
    bigWinningTasks(filters: BigWinningsTaskFilters!, pagination: Pagination): BigWinningTasksResult!
    cancelOrderTask(id: ID!): CancelOrderTask!
    cancelOrderTasks(filters: CancelOrderTasksFilters!, pagination: Pagination): CancelOrderTasksResult!
    cart(userId: ID): Cart!
    freeTicket(id: ID!): FreeTicket!
    freeTickets(filters: FreeTicketsFilters!, pagination: Pagination): FreeTicketsResult!
    ledger(id: ID!): Ledger!
    ledgerTransaction(ledgerId: ID!, transactionId: String!): LedgerTransaction
    ledgerTransactions(filters: LedgerTransactionsFilters, ledgerId: ID!, pagination: Pagination): LedgerTransactionsResult!
    ledgers: LedgersResult!
    locationGame(id: ID!): LocationGame!
    locationGames(filters: LocationGamesFilters!, pagination: Pagination): LocationGamesResult!
    order(id: ID!): Order!
    orders(filters: OrderFilters, pagination: Pagination): OrdersResult!
    pool(id: ID!): Pool
    poolInvites(id: ID!, pagination: Pagination): PoolInvitesResult!
    poolUsers(id: ID!, pagination: Pagination): PoolUsersResult!
    pools(pagination: Pagination): PoolsResult!
    pricingRule(id: ID!): PricingRule
    pricingRules(pagination: Pagination, type: ID!): PricingRulesResult!
    profile: User!
    quoteRegionGame(fractional: Boolean!, play: AWSJSON!, regionGameId: ID!): Price!
    recurringOrder(id: ID!): RecurringOrder!
    recurringOrders(pagination: Pagination): RecurringOrdersResult!
    regionGame(id: ID!): RegionGame!
    regionGameDraw(id: ID!): RegionGameDraw!
    regionGameDraws(filters: RegionGameDrawsFilters!, pagination: Pagination): RegionGameDrawsResult!
    regionGames(filters: RegionGamesFilters!, pagination: Pagination): RegionGamesResult!
    task(id: ID): Task!
    tasks(filters: TaskFilters): TasksResult!
    ticket(id: ID!): Ticket!
    tickets(filters: TicketsFilters, pagination: Pagination): TicketsResult!
}

type RecurringOrder {
    enabled: Boolean!
    expectedPrice: Price!
    fractional: Boolean!
    id: ID!
    locationId: String!
    play: Play!
    regionGameId: String!
}

type RecurringOrdersResult {
    items: [RecurringOrder!]!
    nextToken: String
}

type RegionGame {
    autoPayoutLimit: Price
    closingTime: Int!
    currency: String!
    drawTime: AWSTime!
    draws: [RegionGameDraw!]
    gameId: String!
    id: ID!
    lastDrawDate: AWSDate
    lastDrawResult: String
    name: String!
    nextDrawDate: AWSDate
    nextDrawPrize: Float
    regionId: String!
    regionName: String!
    resultUpdatedAt: AWSDateTime
    schemas: GameSchemas
    timeZone: String!
}

type RegionGameDraw {
    closingDateTime: AWSDateTime
    date: AWSDate!
    id: ID!
    parsedResult: DrawResults
    prize: Float
    regionGameId: String!
    result: String
    resultUpdatedAt: AWSDateTime
    verifiedResult: DrawResults
}

type RegionGameDrawsResult {
    items: [RegionGameDraw!]
    nextToken: String
}

type RegionGamesResult {
    items: [RegionGame!]
    nextToken: String
}

type StepFunctionsExecution {
    executionArn: String!
    startDate: Float!
}

type Task {
    execution: String
    id: String!
    input: AWSJSON
    output: AWSJSON
    process: TaskProcess!
    state: TaskState!
    status: TaskStatus!
    statusReason: String
    statusUpdatedAt: AWSDateTime
    token: String
}

type TasksResult {
    items: [Task!]
    nextToken: String
}

type Ticket {
    drawDate: AWSDate!
    fraction: Int
    id: ID!
    locationId: String
    options: AWSJSON
    pick: AWSJSON!
    poolId: String
    regionGameId: String!
    totalFractions: Int
    totalWinnings: Price
    type: String!
    winnings: Price
}

type TicketsResult {
    items: [Ticket!]!
    nextToken: String
}

type User {
    email: AWSEmail!
    id: ID!
    name: String!
    preferences: UserPreferences!
    updatedAt: AWSDateTime!
}

type UserPreferences {
    notifications: PreNotifications!
}

enum ActionEnum {
    Keep
    Void
}

enum BigWinningNotificationTaskStatus {
    Complete
    Pending
}

enum BigWinningTaskStatus {
    Complete
    Pending
}

enum CancelOrderTaskStatus {
    Complete
    Pending
}

enum FreeTicketStatus {
    Complete
    Pending
}

enum LedgerType {
    Balance
    Cash
    Credits
    Winnings
}

enum OrderStatus {
    Canceled
    Draft
    Fulfilled
    Paid
    PaymentFailed
    PendingPayment
}

enum PoolInviteStatus {
    Accepted
    Pending
    Rejected
}

enum PricingRuleType {
    CART
    GAME
}

enum PushNotificationProvider {
    FCM
}

enum RefundDestinationEnum {
    Balance
    Credits
    Exact
    PaymentMethod
}

enum TaskProcess {
    Winnings
}

enum TaskState {
    AddPaymentMethod
    BigWinner
    CalculateWinnings
    FulfillOrder
    IssueWinnings
    PreCalculateWinnings
    PreIssueWinnings
    PreVerifyResults
    ProcessPayment
    SendReceipt
    SendResults
    VerifyResults
}

enum TaskStatus {
    Complete
    Failed
    Pending
}

input AddCartItemInput {
    drawDate: AWSDate!
    fractional: Boolean!
    play: PlayInput
    quantity: Int!
    regionGameId: String!
}

input AggTicketsFilters {
    archived: Boolean
    fromDate: AWSDate
    regionGameId: String
    toDate: AWSDate
}

input BigWinningNotificationTasksFilters {
    regionGameDrawId: String!
    status: BigWinningNotificationTaskStatus
}

input BigWinningsTaskFilters {
    regionGameDrawId: String!
    status: BigWinningTaskStatus
}

input CancelItemsInput {
    action: ActionEnum!
    id: ID!
}

input CancelOrderInput {
    action: ActionEnum!
    items: [CancelItemsInput!]
    orderId: ID!
    refundAmount: PriceInput
    refundDestination: RefundDestinationEnum
}

input CancelOrderTasksFilters {
    fromDate: AWSDateTime
    status: CancelOrderTaskStatus!
    toDate: AWSDateTime
    userId: String
}

input CreateCancelOrderTaskInput {
    orderId: String!
}

input CreateLocationGameInput {
    enabled: Boolean!
    fractions: Int
    gameId: String!
    locationId: String!
    regionId: String!
}

input CreatePoolInput {
    name: String!
}

input CreateRecurringOrderInput {
    fractional: Boolean!
    play: PlayInput!
    regionGameId: String!
}

input CreateRegionGameDrawInput {
    closingDateTime: AWSDateTime
    date: AWSDate!
    regionGameId: String!
    result: String
    verifiedResult: RegionGameDrawResultInput
}

input CreateRegionGameInput {
    autoPayoutLimit: PriceInput
    closingTime: Int!
    currency: String!
    drawTime: AWSTime!
    gameId: ID!
    lastDrawDate: AWSDate
    lastDrawResult: String
    name: String!
    nextDrawDate: AWSDate
    nextDrawPrize: Float
    prizes: AWSJSON
    regionId: ID!
    regionName: String!
    resultUpdatedAt: AWSDateTime
    timeZone: String!
}

input CurrencyInput {
    code: String!
}

input FreeTicketsFilters {
    regionGameDrawId: String!
    status: FreeTicketStatus
}

input FulfilledItem {
    id: ID!
    ticketId: String
}

input GenerateFreeTicketInput {
    drawDate: AWSDate!
    id: String!
    play: PlayInput!
}

input LedgerTransactionsFilters {
    fromDate: AWSDateTime
    toDate: AWSDateTime
}

input LocationGamesFilters {
    locationId: String!
}

input MarkTaskCompleteInput {
    id: ID!
}

input MarkTaskFailedInput {
    id: ID!
    reason: String
}

input OrderFilters {
    fromDate: AWSDateTime
    status: OrderStatus
    toDate: AWSDateTime
    userId: String
}

input Pagination {
    limit: Int
    nextToken: String
}

input PlayInput {
    options: AWSJSON
    pick: [String!]!
}

input PreNotificationsInput {
    email: Boolean
    push: Boolean
}

input PriceInput {
    amount: Float!
    currency: CurrencyInput!
}

input RegionGameDrawResultInput {
    prizes: AWSJSON!
    result: AWSJSON!
}

input RegionGameDrawsFilters {
    regionGameId: String!
}

input RegionGamesFilters {
    regionId: String!
}

input RegisterDeviceInput {
    deviceId: ID!
    provider: PushNotificationProvider!
    token: String!
}

input StartWinningsProcessInput {
    date: String!
    regionGameId: String!
}

input TaskFilters {
    process: TaskProcess
    state: TaskState
    status: TaskStatus
}

input TicketsFilters {
    fromDate: AWSDate
    regionGameId: String
    toDate: AWSDate
}

input TransferOptions {
    idempotencyKey: String
}

input TransferRequest {
    amount: PriceInput!
    description: String
    destinationLedgerId: String!
    reference: String!
    sourceLedgerId: String!
}

input UnregisterDeviceInput {
    deviceId: ID!
    provider: PushNotificationProvider!
}

input UpdateBigWinningTaskInput {
    id: ID!
    status: BigWinningTaskStatus!
}

input UpdateCancelOrderTaskInput {
    id: ID!
    status: CancelOrderTaskStatus!
}

input UpdateLocationGameInput {
    enabled: Boolean!
    fractions: Int
    id: ID!
    regionId: String!
}

input UpdatePoolInput {
    id: ID!
    name: String
}

input UpdatePricingRuleInput {
    latest: Int!
    rules: AWSJSON!
    type: PricingRuleType!
}

input UpdateProfileInput {
    email: AWSEmail
    name: String
    preferences: UserPreferencesInput
}

input UpdateRecurringOrderInput {
    enabled: Boolean!
    fractional: Boolean!
    id: ID!
    play: PlayInput!
    regionGameId: String!
}

input UpdateRegionGameDrawInput {
    closingDateTime: AWSDateTime
    id: ID!
    result: String
    verifiedResult: RegionGameDrawResultInput
}

input UpdateRegionGameInput {
    autoPayoutLimit: PriceInput
    closingTime: Int
    currency: String
    drawTime: AWSTime
    id: ID!
    lastDrawDate: AWSDate
    lastDrawResult: String
    name: String
    nextDrawDate: AWSDate
    nextDrawPrize: Float
    prizes: AWSJSON
    regionName: String
    resultUpdatedAt: AWSDateTime
    timeZone: String
}

input UserPreferencesInput {
    notifications: PreNotificationsInput!
}`
