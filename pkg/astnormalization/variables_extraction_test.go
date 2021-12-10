package astnormalization

import (
	"testing"
)

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
}

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