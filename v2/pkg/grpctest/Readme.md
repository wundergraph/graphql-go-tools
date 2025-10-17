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

The package includes tools for regenerating Protocol Buffer definitions:

```bash
# Generate Go code from .proto files
make generate-proto

# Regenerate .proto from GraphQL schema (requires wgc CLI)
make regenerate-proto

# Regenerate .proto from GraphQL schema locally (requires cosmo to be checked out in the same parent directory as graphql-go-tools)
make regenerate-proto-local

# Build plugin service
make build-plugin
```

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
├── plugin/                  # Plugin service implementation
└── cmd/                     # Command-line utilities
```

## Contributing

When adding new test cases or extending the mock service:

1. Update the GraphQL schema in `testdata/products.graphqls`
2. Regenerate the Protocol Buffer definitions using `make regenerate-proto`
3. Update the mock service implementation in `mockservice.go`
4. Add corresponding field mappings in `mapping/mapping.go`
5. Update field configurations in `schema.go` if needed

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

You can either run the `mapping_helper` command manually or use the `make generate-mapping-code` command.
The recommended way is to use the Makefile.

### Use Makefile

```bash
# Regenerate .proto and mapping files 
make regenerate-proto
# or make regenerate-proto-local (if you need to test local changes in protographic)

# Generate the mapping.go file
make generate-mapping-code
```

This will run the `generate-mapping-code` command with the correct arguments.

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

