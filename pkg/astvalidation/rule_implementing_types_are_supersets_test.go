package astvalidation

import (
	"testing"
)

func TestImplementingTypesAreSupersets(t *testing.T) {
	t.Run("Definition", func(t *testing.T) {
		t.Run("all implementing types are supersets of their interfaces", func(t *testing.T) {
			runDefinitionValidation(t, `
					interface IDType {
					  id: ID!
					}
					
					interface SoftDelete implements IDType {
					  id: ID!
					  deleted: Boolean!
					}
					
					type Record implements SoftDelete & IDType {
					  id: ID!
					  deleted: Boolean!
					  data: String!
					}
				`, Valid, ImplementingTypesAreSupersets(),
			)
		})

		t.Run("all implementing types and extensions are supersets of their interfaces", func(t *testing.T) {
			runDefinitionValidation(t, `
					interface IDType {
					  id: ID!
					}
					
					interface SoftDelete {
					  deleted: Boolean!
					}
					
					extend interface SoftDelete implements IDType {
					  id: ID!
					}
					
					type Record {
					  data: String!
					}
					
					extend type Record implements SoftDelete & IDType {
					  id: ID!
					  deleted: Boolean!
					}
				`, Valid, ImplementingTypesAreSupersets(),
			)
		})

		t.Run("not all implementing types are supersets of their interfaces", func(t *testing.T) {
			runDefinitionValidation(t, `
					interface IDType {
					  id: ID!
					}
					
					interface SoftDelete implements IDType {
					  id: ID!
					  deleted: Boolean!
					}
					
					type Record implements SoftDelete & IDType {
					  id: ID!
					  data: String!
					}
				`, Invalid, ImplementingTypesAreSupersets(),
			)
		})

		t.Run("not all implementing types and extensions are supersets of their interfaces", func(t *testing.T) {
			runDefinitionValidation(t, `
					interface IDType {
					  id: ID!
					}
					
					interface SoftDelete {
					  deleted: Boolean!
					}
					
					extend interface SoftDelete implements IDType {
					  id: ID!
					}
					
					type Record {
					  data: String!
					}
					
					extend type Record implements SoftDelete & IDType {
					  id: ID!
					}
				`, Invalid, ImplementingTypesAreSupersets(),
			)
		})

		t.Run("implementing type does not define any fields but interface has fields", func(t *testing.T) {
			runDefinitionValidation(t, `
					interface IDType {
					  id: ID!
					}
					
					interface SoftDelete implements IDType
				`, Invalid, ImplementingTypesAreSupersets(),
			)
		})
	})
}
