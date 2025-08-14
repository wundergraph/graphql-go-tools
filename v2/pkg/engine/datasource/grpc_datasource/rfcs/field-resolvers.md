---
title: "Field Resolvers in gRPC"
author: Ludwig Bedacht
---

## Introduction

Field resolvers are a fundamental concept in GraphQL that define how to fetch or compute data for individual fields in a GraphQL schema. They are the building blocks that connect GraphQL queries to actual data sources, whether those are databases, REST APIs, gRPC services, or any other data source.

In traditional GraphQL implementations, every field in a schema can have an associated resolver function that determines how to retrieve or compute the value for that specific field. This granular approach allows for precise control over data fetching, enables optimization strategies like batching and caching, and provides the flexibility to compose data from multiple sources seamlessly.

## Field Resolvers

### What are Field Resolvers?

A field resolver is a function that GraphQL executes to resolve the value of a specific field in the schema. When a GraphQL query is executed, the GraphQL engine walks through the query tree and calls the appropriate resolver for each field that needs to be resolved.

### Basic Resolver Structure

A field resolver is conceptually a function that takes several inputs and returns a value for the field. The resolver receives:

### Resolver Execution Flow

1. **Query Parsing**: GraphQL parses the incoming query into an Abstract Syntax Tree (AST)
2. **Execution Planning**: The engine determines which resolvers need to be called and in what order
3. **Field Resolution**: For each field in the query, the corresponding resolver is executed
4. **Result Assembly**: The results from all resolvers are assembled into the final response structure

### Types of Resolvers

1. **Scalar Resolvers**: Return simple values (strings, numbers, booleans)
2. **Object Resolvers**: Return complex objects that may have their own nested resolvers
3. **List Resolvers**: Return arrays of values or objects
4. **Custom Scalar Resolvers**: Handle custom scalar types like Date, JSON, etc.

### Benefits of Field Resolvers

- **Granular Control**: Each field can have custom logic for data fetching
- **Composition**: Data from multiple sources can be combined at the field level
- **Optimization**: Resolvers can implement batching, caching, and other performance optimizations
- **Security**: Field-level authorization can be implemented in resolvers
- **Flexibility**: Different fields can use different data sources or computation methods

### Example

Consider a GraphQL schema that demonstrates field resolvers with arguments:

```graphql
type Query {
  user(id: ID!): User
  users(limit: Int, offset: Int): [User!]!
}

type User {
  id: ID!
  name: String!
  email: String!
  posts(limit: Int = 10, status: PostStatus, orderBy: PostOrderBy): [Post!]!
}

type Post {
  id: ID!
  title: String!
  content(format: ContentFormat = MARKDOWN): String!
  comments(limit: Int = 5, orderBy: CommentOrder): [Comment!]!
  likes(count: Boolean = false): LikesResult!
}

enum PostStatus {
  DRAFT
  PUBLISHED
  ARCHIVED
}
enum PostOrderBy {
  CREATED_AT
  TITLE
  POPULARITY
}
enum ContentFormat {
  MARKDOWN
  HTML
  PLAIN
}
enum CommentOrder {
  NEWEST
  OLDEST
  POPULAR
}
```

In this schema, field resolvers handle arguments to:

- `posts(limit, status, orderBy)` - Filter and paginate user's posts based on arguments
- `content(format)` - Return post content in different formats
- `comments(limit, orderBy)` - Control comment pagination and sorting
- `likes(count)` - Return either like count or full like data based on the boolean argument

Each field resolver receives these arguments and can use them to customize the data fetching logic, API calls, or computations performed for that specific field.

## Field Resolvers in gRPC

There is no specific support for field resolvers in Protobuf. The design focuses on explicitness.
In order to use field resolvers, we need to create a concept which provides the arguments to the gRPC request and allows the user to provide the response in a way that the engine can interpret the result.

## Concept

Let's focus on the previous example and see how we can implement field resolvers for the `posts` field.

A typical Protobuf schema for the Query `user` would look like this:

```protobuf
service UserService {
  rpc QueryUser(QueryUserRequest) returns (QueryUserResponse);
}

message QueryUserRequest {
  string id = 1;
}

message QueryUserResponse {
  User user = 1;
}
```

We provide the `id` parameter defined on the root field `user` in the GraphQL schema.
Now let's imagine we have the following GraphQL request:

```graphql
query {
  user(id: "123") {
    id
    name
    publishedPosts: posts(limit: 10, status: PUBLISHED, orderBy: CREATED_AT) {
      title
    }
    draftPosts: posts(limit: 10, status: DRAFT, orderBy: CREATED_AT) {
      title
    }
  }
}
```

We want to fetch the posts for the user with the id `123`, and also compute the value for the `posts` field.

## Approach 1: Generate Lookup rpc calls for each field with a resolver.

```protobuf
service UserService {
  rpc QueryUser(QueryUserRequest) returns (QueryUserResponse);
  rpc ResolveUserPosts(ResolveUserPostsRequest) returns (ResolveUserPostsResponse);
}

message QueryUserRequest {
  string id = 1;
}

message QueryUserResponse {
  User user = 1;
}

message ResolveUserPostsInput {
    string id = 1;
    int32 limit = 2;
    PostStatus status = 3;
    PostOrderBy orderBy = 4;
}

message ResolveUserPostsRequest {
  repeated ResolveUserPostsInput input = 1;
}

message ResolveUserPostsOutput {
    repeated Post posts = 1;
}

message ResolveUserPostsResponse {
  repeated ResolveUserPostsOutput output = 1;
}

```

In this approach, we generate a new rpc call for each field with a resolver.
The corresponding request and response messages will be generated by the GraphQL to Protobuf transformation.
Requests will inherit all the arguments from the root field and the field with a resolver.
Responses will contain a list of the results for each field with a resolver.

In order to solve the `n+1` problem, we will provide all the arguments for all fields as a list of `ResolveUserPostsInput` messages.
On the response side, we will provide a list of `ResolveUserPostsOutput` messages.

The engine will then make sure to apply the results to the corresponding fields in the GraphQL JSON response.

### Advantages

- **Easy Field Addition**: We can easily add new fields to the request and response.
- **Automatic Code Generation**: The GraphQL to Protobuf transformation will generate the message types for the fields with a resolver and apply them to the corresponding request and response messages.
- **Good Developer Experience**: Programmatically this is a very good User experience, as the will just need to implement the additional rpc calls and the engine will take care of the rest.
- **Type Safety**: Each field resolver has strongly-typed request/response messages, preventing runtime errors
- **Clear API Contract**: Each resolver has an explicit protobuf definition, making the API self-documenting
- **Independent Scaling**: Different field resolvers can be scaled independently based on usage patterns
- **Error Isolation**: Failures in one field resolver don't affect others
- **Testing**: Each resolver can be unit tested in isolation
- **Caching**: Individual field resolvers can have different caching strategies
- **Authorization**: Field-level permissions can be implemented per resolver

### Disadvantages

- **Message Type Generation**: We need to generate a new message type for each field with a resolver.
- **Resolver Code Generation**: We need to generate the resolver code for each field.
- **Increased Server Load**: Each field with a resolver will be a separate rpc call and will increase the load on the server.
- **Network Overhead**: Multiple RPC calls increase latency and network traffic
- **Service Discovery Complexity**: Need to manage multiple service endpoints
- **Code Generation Complexity**: Significantly more generated code to maintain
- **Deployment Complexity**: More RPC methods to deploy and version
- **Debugging Difficulty**: Request tracing becomes more complex across multiple calls
- **Nested Field Resolver Complexity**: Handling nested field resolvers requires additional RPC calls and complex coordination between services

## Approach 2: Generate message types for each field with a resolver.

```protobuf
service UserService {
  rpc QueryUser(QueryUserRequest) returns (QueryUserResponse);
}

message QueryUserRequest {
  message FieldArgs {
    message PostsArgs {
        int32 limit = 1;
        PostStatus status = 2;
        PostOrderBy orderBy = 3;
    }
    PostsArgs posts = 1;
  }

  repeated FieldArgs args = 1;
  string id = 2;
}

message QueryUserResponse {
  message FieldResult {
    message PostsResult {
      repeated Post posts = 1;
    }

    PostsResult posts = 1;
  }

  repeated FieldResult results = 1;
  User user = 2;
}
```

### Advantages

- **Easy Field Addition**: We can easily add new fields to the request and response.
- **Automatic Code Generation**: The GraphQL to Protobuf transformation will generate the message types for the fields with a resolver and apply them to the corresponding request and response messages.
- **Single Network Call**: All field resolution happens in one RPC, reducing latency
- **Transactional Consistency**: All field data can be fetched in a single database transaction
- **Simpler Service Discovery**: Only one service endpoint to manage
- **Better Performance**: Batching reduces overhead compared to multiple calls

### Disadvantages

- **Code Generation Complexity**: The generation logic will be more complex than the first approach, as we need to determine all the types with field arguments and apply the corresponding message types to the request and response messages.
- **Message Size Bloat**: Request/response messages can become very large with many nested field types
- **Tight Coupling**: Adding new fields requires updating the main service contract
- **Limited Reusability**: Field argument types are tied to specific parent types
- **Versioning Complexity**: Changes to any field affect the entire service contract
- **Memory Usage**: Large message structures consume more memory
- **Serialization Overhead**: Larger messages take more time to serialize/deserialize
- **Nested Field Resolver Complexity**: Deeply nested field resolvers create exponentially complex message structures and generation logic

## Approach 3: Introduce Metadata to every Request

This approach focuses on a very simple solution. We update our request and response types a bit to include a `metadata` field.
Protobuf provides us with a well known type that can be used for this purpose - `google.protobuf.Struct`.

```protobuf
import "google/protobuf/struct.proto";

service UserService {
  rpc QueryUser(QueryUserRequest) returns (QueryUserResponse);
}

message RequestMetadata {
    repeated google.protobuf.Struct FieldArgs = 1;
}

message QueryUserRequest {
  RequestMetadata metadata = 1;
  string id = 2;
}

message ResponseMetadata {
  repeated google.protobuf.Struct FieldResults = 1;
}

message QueryUserResponse {
  ResponseMetadata metadata = 1;
  User user = 2;
}

```

The `google.protobuf.Struct` type can represents a map of string to any type. It can also contain lists or recursive structs.

Instead of generating a new rpc call or new message types for each field, we can just pass request metadata to the request, which contains the arguments for each field.

Going back to our example request:

```graphql
query {
  user(id: "123") {
    id
    name
    publishedPosts: posts(limit: 10, status: PUBLISHED, orderBy: CREATED_AT) {
      title
    }
    draftPosts: posts(limit: 10, status: DRAFT, orderBy: CREATED_AT) {
      title
    }
  }
}
```

We could translate it to the following request - visualized as JSON:

````json
{
  "metadata": {
    "fieldArgs": [
      {
        "posts": {
          "args": {
            "limit": 10,
            "status": "PUBLISHED",
            "orderBy": "CREATED_AT"
          }
        }
      },
      {
        "posts": {
          "args": {
            "limit": 10,
            "status": "DRAFT",
            "orderBy": "CREATED_AT"
          }
        }
      }
    ]
  },
  "id": "123"
}

The response would look like this:

```json
{
    "metadata": {
        "fieldResults": [
            {
                // will be applied to the publishedPosts field
                "posts": [
                    {
                        "title": "Post 1"
                    },
                    {
                        "title": "Post 4"
                    }
                ]
            },
            {
                // will be applied to the draftPosts field
                "posts": [
                    {
                        "title": "Post 2"
                    },
                    {
                        "title": "Post 3"
                    }
                ]
            }
        ]
    },
    "user": {
        "id": "123",
        "name": "John Doe",
        // can be filled with the results (e.g. no arguments provided), but if there are results in the response metadata, the engine will apply them to the corresponding fields in the GraphQL JSON response based on the ordering
        "posts": []
    }
}
````

### Nested Field Resolver Example

Let's consider a more complex example with nested field resolvers:

```graphql
query {
  user(id: "123") {
    id
    name
    posts(limit: 5, status: PUBLISHED) {
      id
      title
      content(format: HTML)
      comments(limit: 3, orderBy: NEWEST) {
        id
        text
        author {
          name
        }
      }
    }
  }
}
```

This query has field resolvers at multiple levels:
- `posts` field on User (with limit and status arguments)
- `content` field on Post (with format argument)
- `comments` field on Post (with limit and orderBy arguments)

The request metadata would look like this:

```json
{
  "metadata": {
    "fieldArgs": [
      {
        "posts": {
          "args": {
            "limit": 5,
            "status": "PUBLISHED"
          },
          "fields": {
            "content": {
              "args": {
                "format": "HTML"
              }
            },
            "comments": {
              "args": {
                "limit": 3,
                "orderBy": "NEWEST"
              }
            }
          }
        }
      }
    ]
  },
  "id": "123"
}
```

The response would contain the corresponding nested results:

```json
{
  "metadata": {
    "fieldResults": [
      {
        "posts": [
          {
            "id": "1",
            "title": "Post 1",
            "content": "<p>HTML content for Post 1</p>",
            "comments": [
              {
                "id": "c1",
                "text": "Great post!",
                "author": { "name": "Alice" }
              },
              {
                "id": "c2", 
                "text": "Thanks for sharing",
                "author": { "name": "Bob" }
              }
            ]
          },
          {
            "id": "2", 
            "title": "Post 2",
            "content": "<p>HTML content for Post 2</p>",
            "comments": [
              {
                "id": "c3",
                "text": "Interesting perspective",
                "author": { "name": "Charlie" }
              }
            ]
          }
        ]
      }
    ]
  },
  "user": {
    "id": "123",
    "name": "John Doe",
    "posts": []
  }
}
```

The engine would populate the nested field resolver results directly into the final GraphQL response structure, creating the complete data with all resolved field values in their natural positions.

### Advantages

- **Generic Approach**: Untyped map structure provides a lot of flexibility.
- **Metadata Usage**: Can be also be used for other purposes, like passing additional information to the endpoint. (e.g. request depth)
- **Engine Support**: The engine will be able to apply the results to the corresponding fields in the GraphQL JSON response based on the ordering
- **Schema Evolution**: New fields can be added without changing protobuf definitions
- **Minimal Code Generation**: Very little additional generated code needed
- **Flexible Data Structures**: Can handle complex, nested argument structures easily
- **Backward Compatibility**: Old clients continue to work when new fields are added
- **Dynamic Field Resolution**: Can handle fields that are determined at runtime
- **Smaller Service Interface**: Keeps the main service contract clean and focused
- **Natural Nested Field Support**: Handles nested field resolvers elegantly without additional complexity

### Disadvantages

- **Degraded User Experience**: Providing a map structure requires the implementor to know the field names and the types of the arguments.
- **Runtime Type Errors**: No compile-time validation of field arguments structure
- **Debugging Complexity**: JSON-like structures are harder to debug than typed messages
- **Performance Overhead**: `google.protobuf.Struct` has serialization overhead compared to native types
- **Documentation Challenges**: Field argument schemas aren't self-documenting in protobuf
- **Validation Complexity**: Need custom validation logic for argument structures
