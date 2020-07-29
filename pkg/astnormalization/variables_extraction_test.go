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
}

const forumExampleSchema = `
schema {
	mutation: Mutation
}
scalar String
type Mutation {
	createUser(input: CreateUserInput): CreateUser
	createPost(input: CreatePostInput): CreatePost
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
