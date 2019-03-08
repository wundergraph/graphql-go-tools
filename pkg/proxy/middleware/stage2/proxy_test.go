package stage2

import (
	"bytes"
	"github.com/golang/mock/gomock"
	"testing"
)

func TestProxy(t *testing.T) {

	mockController := gomock.NewController(t)
	prismaA := NewMockPrisma(mockController)
	defer mockController.Finish()

	proxy := NewProxy()
	proxy.ConfigureSchema("a", schemaA, prismaA)

	prismaA.EXPECT().Query(gomock.Eq(prismaQuery1)).Return(prismaResponse1)
	response1, err1 := proxy.Request("a", query1)
	if err1 != nil {
		panic(err1)
	}
	if !bytes.Equal(response1, query1Response) {
		t.Fatalf("unexpected query1Reponse\nwant:\n%s\n\ngot:\n%s", string(query1Response), string(response1))
	}
}

type FakePrisma struct {
}

func (FakePrisma) Query(request []byte) (result []byte) {
	return prismaResponse1
}

func BenchmarkProxy(b *testing.B) {

	prismaA := FakePrisma{}

	proxy := NewProxy()
	proxy.ConfigureSchema("a", schemaA, prismaA)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		response1, err1 := proxy.Request("a", query1)
		if err1 != nil {
			panic(err1)
		}
		if !bytes.Equal(response1, query1Response) {
			b.Fatalf("unexpected query1Reponse\nwant:\n%s\n\ngot:\n%s", string(query1Response), string(response1))
		}
	}
}

var query1Response = []byte(`{
  "data": {
    "assets": [
      {
        "id": "cjsyjgosm6kp30852mk7ojj3e",
        "fileName": "A.jpg",
        "url": "https://media.graphcms.com//R2YHaAHYTJaZcQ45oWEP"
      },
      {
        "id": "cjsyjgzbi6kpw0852bcf29ykp",
        "fileName": "B.jpg",
        "url": "https://media.graphcms.com//TRwqQhZERjCxj96Mx6pu"
      }
    ],
    "castles": [
      {
        "id": "cjsyjigj06kt90852o0ijogi1",
        "name": "Burg Kronberg"
      },
      {
        "id": "cjsyjjkiv6kvr0852zm2mz5pg",
        "name": "Burg Münzenberg"
      }
    ]
  }
}`)

var prismaResponse1 = []byte(`{
  "data": {
    "assets": [
      {
        "id": "cjsyjgosm6kp30852mk7ojj3e",
        "fileName": "A.jpg",
        "handle": "R2YHaAHYTJaZcQ45oWEP"
      },
      {
        "id": "cjsyjgzbi6kpw0852bcf29ykp",
        "fileName": "B.jpg",
        "handle": "TRwqQhZERjCxj96Mx6pu"
      }
    ],
    "castles": [
      {
        "id": "cjsyjigj06kt90852o0ijogi1",
        "name": "Burg Kronberg"
      },
      {
        "id": "cjsyjjkiv6kvr0852zm2mz5pg",
        "name": "Burg Münzenberg"
      }
    ]
  }
}`)

var prismaQuery1 = []byte("query one {assets {id fileName handle} castles {id name}}")

var query1 = []byte(`query one {
  assets {
    id
    fileName
    url
  }
  castles {
    id
    name
  }
}`)

var schemaA = `type Asset implements Node {
  id: ID!
  handle: String!
  fileName: String!
  """
  Get the url for the asset with provided transformations applied.
  """
  url(transformation: ImageTransformationInput): String!
}

type Castle implements Node {
  id: ID!
  name: String
}


input ImageResizeInput {
  """
  The width in pixels to resize the image to. The value must be an integer from 1 to 10000.
  """
  width: Int

  """
  The height in pixels to resize the image to. The value must be an integer from 1 to 10000.
  """
  height: Int
}

"""
Transformations for Images
"""
input ImageTransformationInput {
  """
  Resizes the image
  """
  resize: ImageResizeInput
}

"""
An object with an ID
"""
interface Node {
  """
  The id of the object.
  """
  id: ID!
}

type Query {
  assets: [Asset]!
  castles: [Castle]!
}

schema {
  query: Query
}`
