package resolve

import (
"fmt"
	"testing"

	"github.com/buger/jsonparser"
)

var result = []byte(`
{
  "reviews": [
    {
      "author": {
        "reviews": [
          {
            "product": {
              "upc": "top-1"
            }
          },
          {
            "product": {
              "upc": "top-12"
            }
          }
        ],
        "id": "1234"
      }
    },
    {
      "author": {
        "reviews": [
          {
            "product": {
              "upc": "top-13"
            }
          },
          {
            "product": {
              "upc": "top-14"
            }
          }
        ],
        "id": "1234"
      }
    },
    {
      "author": {
        "reviews": [
          {
            "product": {
              "upc": "top-15"
            }
          }
        ],
        "id": "7777"
      }
    }
  ]
}
`)

func TestFlatten(t *testing.T) {
	jsonparser.Get(result, "")
	p := []string{"reviews", "@", "author", "reviews", "@", "product"}
	arg := [][]byte{result}

	var result []string
	for _, val := range flatten(arg, p...) {
		result = append(result, string(val))
	}

	fmt.Println(len(result))
	fmt.Println(result)
}

func flatten(input [][]byte, path ...string) [][]byte {
	if len(path) == 0 {
		return input
	}

	current, rest := path[0], path[1:]

	if current == "@" {
		temp := make([][]byte, 0, len(input))


		for i := range input {
			var vals [][]byte
			jsonparser.ArrayEach(input[i], func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
				vals = append(vals, value)
			})

			temp = append(temp, flatten(vals, rest...)...)
		}

		return temp
	}

	temp := make([][]byte, 0, len(input))

	for i := range input {
		el, _, _, err := jsonparser.Get(input[i], current)
		if err != nil {
			return nil
		}
		temp = append(temp, el)
	}

	return flatten(temp, rest...)
}

//func flatten(res [][]byte, path ...string) [][]byte {
//	if len(path) == 0 {
//		return res
//	}
//
//	current, rest := path[0], path[1:]
//	if current == "@" {
//
//	}
//
//	result := make([][]byte, 0, len(res))
//
//	for i := range res {
//		value, _, _, err := jsonparser.Get(res[i], current)
//		if err != nil {
//			return nil
//		}
//
//		selected :=
//	}
//}

