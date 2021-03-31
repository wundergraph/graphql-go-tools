package federation

import (
	"testing"
)

//func TestBuildBaseSchemaFromSDLs(t *testing.T) {
//	tests := []struct {
//		name     string
//		SDLs     []string
//		expected string
//		err      error
//	}{
//		{
//			SDLs:     []string{astronautTestSchema, missionTestSchema},
//			expected: astronautMissionTestSchema,
//			name:     "schemas with extensions directive",
//		},
//	}
//
//	for _, tc := range tests {
//		t.Run(tc.name, func(t *testing.T) {
//			actual, err := BuildBaseSchemaFromSDLs(tc.SDLs...)
//			assert.Equal(t, tc.err, err)
//			assert.Equal(t, tc.expected, actual)
//		})
//	}
//}

func TestBuildBaseSchemaFromSDLs(t *testing.T) {
	_, err := BuildBaseSchemaFromSDLs(baseScheme, astronautTestSchema, missionTestSchema)
	if err != nil {
		t.Errorf(err.Error())
	}
}

const (
	baseScheme = `
	type Query {}
	type Mutation {}
`

	astronautTestSchema = `
type Astronaut @key(fields: "id") {
	id: ID!
	name: String
	name: String
	id: ID!
}
extend type Mutation {
	addAstronaut(id: ID!, name: String!): Astronaut
}
extend type Query {
	astronaut(id: ID!): Astronaut
	astronauts: [Astronaut]
}`

	missionTestSchema = `
extend type Astronaut @key(fields: "id") {
	id: ID! @external
	missions: [Mission]
}
type Mission {
	id: ID!
	crew: [Astronaut]
	designation: String!
	startDate: String
	endDate: String
}
extend type Query {
	mission(id: ID!): Mission
	missions: [Mission]
}`

	astronautMissionTestSchema = `
type Astronaut {
  id: ID!
  name: String
  missions: [Mission]
}

type Mission {
  id: ID!
  crew: [Astronaut]
  designation: String!
  startDate: String
  endDate: String
}

type Mutation {
  addAstronaut(id: ID!, name: String!): Astronaut
}

type Query {
  astronaut(id: ID!): Astronaut
  astronauts: [Astronaut]
  mission(id: ID!): Mission
  missions: [Mission]
}
`
)
