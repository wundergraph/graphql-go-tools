# grpctest Package

The `grpctest` package provides a comprehensive mock gRPC service implementation for testing the `grpc_datasource` functionality in graphql-go-tools. It includes a complete GraphQL schema, Protocol Buffers definitions, and mock service implementations to facilitate testing of gRPC-to-GraphQL integration.

## Overview

This package contains:

- **Mock gRPC Service**: A fully functional mock implementation of a product service with various data types and operations
- **GraphQL Schema**: A comprehensive schema covering various GraphQL features including queries, mutations, unions, interfaces, and complex nested types
- **Protocol Buffers**: Complete `.proto` definitions with generated Go code
- **Field Mappings**: Detailed mappings between GraphQL fields and gRPC message fields
- **Test Utilities**: Helper functions for schema loading, validation, and test data generation

## Key Components

### MockService (`mockservice.go`)

The core mock gRPC service implementation that provides:

- **Entity Lookups**: Product, Storage, and Warehouse entity resolution by ID
- **Query Operations**: Various query types including filtering, pagination, and complex nested queries
- **Mutation Operations**: Create and update operations for different entity types
- **Field Resolvers**: Custom field resolution for computed fields like `productCount`, `popularityScore`, etc.
- **Union Types**: Support for GraphQL unions (e.g., `SearchResult`, `ActionResult`, `Animal`)
- **Nullable Fields**: Comprehensive testing of optional/nullable field handling
- **Complex Lists**: Nested list structures and bulk operations

### Schema Management (`schema.go`)

Provides utilities for:

- Loading and parsing GraphQL schemas from embedded files
- Schema validation and transformation
- Protocol buffer schema access
- Test-friendly schema loading functions

### Field Mappings (`mapping/mapping.go`)

Defines the complete mapping between GraphQL and gRPC:

- **Query/Mutation RPCs**: Maps GraphQL operations to gRPC service methods
- **Field Mappings**: Maps GraphQL field names to protobuf field names
- **Argument Mappings**: Maps GraphQL arguments to gRPC request parameters
- **Entity Lookups**: Configuration for entity resolution patterns
- **Enum Mappings**: GraphQL enum to protobuf enum value mappings

## Code Generation

The package holds more than one service. The `Makefile` defines the same targets for each service:

- `products`: the main product service in this directory. It uses `google.protobuf` wrapper types for nullable scalars (`--legacy-wrapper-types`).
- `events`: a small event service in `events/`. It uses proto3 `optional` fields for nullable scalars.

```bash
# Regenerate the proto file, the mappings and the Go code of a service with the published wgc CLI
make generate-products
make generate-events

# The same with the CLI of a local cosmo checkout in the same parent directory as graphql-go-tools
make generate-products-local
make generate-events-local

# All services
make generate-all
make generate-all-local

# Only regenerate the Go code from the proto files
make generate-proto

# Build plugin service
make build-plugin
```

Before you use the `-local` targets, build the cosmo packages that the CLI uses:

```bash
cd ../cosmo/protographic && pnpm build
cd ../cli && pnpm build
```

The `--legacy-wrapper-types` flag of `wgc grpc-service generate` needs a wgc version that supports it.

To add a service, add its name to `services` in the `Makefile` and set its variables. See the comment at the top of the `Makefile`.

## File Structure

```
grpctest/
├── Readme.md                 # This documentation
├── Makefile                  # Build and generation commands
├── mockservice.go            # Main mock service implementation
├── schema.go                 # Schema loading utilities
├── product.proto             # Protocol buffer definitions
├── mapping/
│   └── mapping.go           # GraphQL-to-gRPC field mappings
├── productv1/               # Generated protobuf Go code
│   ├── product.pb.go
│   └── product_grpc.pb.go
├── testdata/                # Test schemas and data
│   ├── products.graphqls    # GraphQL schema
├── events/                  # Events service with proto3 optional fields
│   ├── mockservice.go       # Mock service implementation
│   ├── connect.go           # ConnectRPC adapter and server helper
│   ├── schema.go            # Schema loading and gRPC server helpers
│   ├── event.proto          # Protocol buffer definitions
│   ├── mapping/
│   │   └── mapping.go       # GraphQL-to-gRPC field mappings
│   ├── eventv1/             # Generated protobuf and ConnectRPC Go code
│   │   ├── event.pb.go
│   │   ├── event_grpc.pb.go
│   │   └── eventv1connect/
│   └── testdata/
│       └── events.graphqls  # GraphQL schema
├── plugin/                  # Plugin service implementation
└── cmd/                     # Command-line utilities
```

## Contributing

When adding new test cases or extending the mock service:

1. Update the GraphQL schema of the service, for example `testdata/products.graphqls`
2. Regenerate the proto file, the mappings and the Go code with `make generate-<service>` or `make generate-<service>-local`
3. Update the mock service implementation, for example `mockservice.go`
4. Update field configurations in `schema.go` if needed

## Integration with grpc_datasource

This package is specifically designed to test the `grpc_datasource` implementation and provides comprehensive coverage of:

- Request/response mapping
- Field resolution patterns
- Error handling scenarios
- Performance characteristics
- Edge cases and boundary conditions

The mock service implements realistic data patterns and edge cases that help ensure the gRPC data source implementation is robust and handles real-world scenarios correctly.

## Regenerate all files

To make generating super simple, you can run the following command:

```bash
# Generate all files
make generate-all

# Generate all files using the local wgc cli
make generate-all-local
```

This will regenerate the .proto file, generate the mapping.go file, and regenerate the protobuf Go code.

## Generating the mapping.go file

The mapping.go file is generated using the `mapping_helper` command. This command takes a mapping.json file and generates the mapping.go file.

You can either run the `mapping_helper` command manually or use the `make generate-<service>` command.
The recommended way is to use the Makefile.

### Use Makefile

```bash
# Regenerate the .proto file, the mapping files and the Go code of the products service
make generate-products
# or make generate-products-local (if you need to test local changes in protographic)

# Do the same for the events service
make generate-events
# or make generate-events-local

# Regenerate only the Go code from the existing .proto files with buf
make generate-proto
```

The `generate-<service>` targets run the `mapping_helper` command with the correct `-service` and `-func` arguments for the service.

### Setup a Run Configuration in VSCode

This is mostly useful for debugging the mapping helper.

```json
{
    "name": "Launch mapping helper",
    "type": "go",
    "request": "launch",
    "mode": "debug",
    "program": "${workspaceFolder}/v2/pkg/grpctest/cmd/mapping_helper/main.go",
    "args": [
        "${workspaceFolder}/v2/pkg/grpctest/testdata/mapping.json",
        "${workspaceFolder}/v2/pkg/grpctest/mapping/mapping.go"
    ],
    "cwd": "${workspaceFolder}/v2/pkg/grpctest/cmd/mapping_helper"
}
```

