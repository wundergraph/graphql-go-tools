package graphql_datasource

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewSchemaConfiguration(t *testing.T) {
	validSchema := `
		type Query {
			hello: String
		}
		
		type User {
			id: ID!
			name: String!
		}
	`

	validFederationSchema := `
		type Query {
			me: User
		}
		
		type User @key(fields: "id") {
			id: ID!
			name: String!
		}
	`

	invalidSchema := `
		type Query {
			hello: String
		
		# Missing closing brace
	`

	tests := []struct {
		name           string
		upstreamSchema string
		federationCfg  *FederationConfiguration
		expectError    bool
		errorContains  string
	}{
		{
			name:           "empty upstream schema should fail",
			upstreamSchema: "",
			federationCfg:  nil,
			expectError:    true,
			errorContains:  "upstream schema is required",
		},
		{
			name:           "valid schema without federation should succeed",
			upstreamSchema: validSchema,
			federationCfg:  nil,
			expectError:    false,
		},
		{
			name:           "federation disabled should succeed",
			upstreamSchema: validSchema,
			federationCfg: &FederationConfiguration{
				Enabled: false,
			},
			expectError: false,
		},
		{
			name:           "federation enabled without ServiceSDL should fail",
			upstreamSchema: validSchema,
			federationCfg: &FederationConfiguration{
				Enabled:    true,
				ServiceSDL: "",
			},
			expectError:   true,
			errorContains: "federation service SDL is required",
		},
		{
			name:           "federation enabled with valid ServiceSDL should succeed",
			upstreamSchema: validSchema,
			federationCfg: &FederationConfiguration{
				Enabled:    true,
				ServiceSDL: validFederationSchema,
			},
			expectError: false,
		},
		{
			name:           "invalid upstream schema should fail",
			upstreamSchema: invalidSchema,
			federationCfg:  nil,
			expectError:    true,
			errorContains:  "unable to parse upstream schema",
		},
		{
			name: "complex valid federation schema should succeed",
			upstreamSchema: `
				type Query {
					users: [User!]!
					products: [Product!]!
				}
				
				type User @key(fields: "id") {
					id: ID!
					name: String!
					email: String!
				}
				
				type Product @key(fields: "upc") {
					upc: String!
					name: String!
					price: Int!
				}
			`,
			federationCfg: &FederationConfiguration{
				Enabled: true,
				ServiceSDL: `
					type Query {
						users: [User!]!
						products: [Product!]!
					}
					
					type User @key(fields: "id") {
						id: ID!
						name: String!
						email: String!
					}
					
					type Product @key(fields: "upc") {
						upc: String!
						name: String!
						price: Int!
					}
				`,
			},
			expectError: false,
		},
		{
			name: "schema with unions and interfaces should succeed",
			upstreamSchema: `
				type Query {
					search: [SearchResult!]!
				}
				
				union SearchResult = User | Product
				
				interface Node {
					id: ID!
				}
				
				type User implements Node {
					id: ID!
					name: String!
				}
				
				type Product implements Node {
					id: ID!
					name: String!
				}
			`,
			federationCfg: nil,
			expectError:   false,
		},
		{
			name: "schema with custom scalars should succeed",
			upstreamSchema: `
				scalar DateTime
				scalar JSON
				
				type Query {
					user(id: ID!): User
				}
				
				type User {
					id: ID!
					name: String!
					createdAt: DateTime!
					metadata: JSON
				}
			`,
			federationCfg: nil,
			expectError:   false,
		},
		{
			name: "schema with directives should succeed",
			upstreamSchema: `
				directive @deprecated(reason: String = "No longer supported") on FIELD_DEFINITION | ENUM_VALUE
				
				type Query {
					user(id: ID!): User
					oldUser(id: ID!): User @deprecated(reason: "Use user instead")
				}
				
				type User {
					id: ID!
					name: String!
					email: String @deprecated
				}
			`,
			federationCfg: nil,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Trim whitespace for empty schema test cases
			upstreamSchema := strings.TrimSpace(tt.upstreamSchema)
			if upstreamSchema == "" && tt.upstreamSchema != "" {
				// This handles the whitespace-only case
				upstreamSchema = tt.upstreamSchema
			}

			cfg, err := NewSchemaConfiguration(upstreamSchema, tt.federationCfg)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					require.Contains(t, err.Error(), tt.errorContains)
				}
				require.Nil(t, cfg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, cfg)
				require.Equal(t, upstreamSchema, cfg.upstreamSchema)
				require.NotNil(t, cfg.upstreamSchemaAst)
				if tt.federationCfg != nil {
					require.Equal(t, tt.federationCfg, cfg.federation)
				}
			}
		})
	}
}

func TestNewSchemaConfiguration_FederationConfigurationCases(t *testing.T) {
	validSchema := `
		type Query {
			hello: String
		}
	`

	tests := []struct {
		name          string
		federationCfg *FederationConfiguration
		expectError   bool
		errorContains string
	}{
		{
			name:          "nil federation config should succeed",
			federationCfg: nil,
			expectError:   false,
		},
		{
			name: "federation config with Enabled false should succeed",
			federationCfg: &FederationConfiguration{
				Enabled:    false,
				ServiceSDL: "some sdl", // Should be ignored when disabled
			},
			expectError: false,
		},
		{
			name: "federation config with Enabled true and valid SDL should succeed",
			federationCfg: &FederationConfiguration{
				Enabled: true,
				ServiceSDL: `
					type Query {
						hello: String
					}
				`,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := NewSchemaConfiguration(validSchema, tt.federationCfg)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, but got: %s", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %s", err.Error())
					return
				}
				if cfg == nil {
					t.Errorf("expected non-nil config but got nil")
					return
				}
			}
		})
	}
}

func TestNewSchemaConfiguration_SchemaWithUnionShouldContainTypename(t *testing.T) {
	validSchema := `
		union SearchResult = User | Product

		type User {
			id: ID!
			name: String!
		}

		type Product {
			id: ID!
			name: String!
		}

		type Query {
			getResult: SearchResult!
		}
	`
	t.Run("Should contain typename for non federation schema", func(t *testing.T) {
		cfg, err := NewSchemaConfiguration(validSchema, nil)
		require.NoError(t, err)

		for _, utd := range cfg.upstreamSchemaAst.UnionTypeDefinitions {
			require.Len(t, utd.FieldsDefinition.Refs, 1)
			require.Equal(t, "__typename", cfg.upstreamSchemaAst.FieldDefinitionNameString(utd.FieldsDefinition.Refs[0]))
		}
	})

	t.Run("Should contain typename for federation schema", func(t *testing.T) {
		cfg, err := NewSchemaConfiguration(validSchema, &FederationConfiguration{
			Enabled:    true,
			ServiceSDL: validSchema,
		})

		require.NoError(t, err)

		for _, utd := range cfg.upstreamSchemaAst.UnionTypeDefinitions {
			require.Len(t, utd.FieldsDefinition.Refs, 1)
			require.Equal(t, "__typename", cfg.upstreamSchemaAst.FieldDefinitionNameString(utd.FieldsDefinition.Refs[0]))
		}
	})

}
