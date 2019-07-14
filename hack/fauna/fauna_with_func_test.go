package fauna

import (
	f "github.com/fauna/faunadb-go/faunadb"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestFaunaFunc(t *testing.T) {

	faunaSecret := os.Getenv("FAUNA_SECRET")
	if faunaSecret == "" {
		t.Fatal("must set env FAUNA_SECRET")
	}

	httpClient := &http.Client{
		Timeout:   time.Duration(5) * time.Second,
		Transport: NewLoggingRoundTripper(),
	}

	client := f.NewFaunaClient(faunaSecret, f.HTTP(httpClient))

	run(t, client, f.If(f.Exists(f.Function("posts")), f.Delete(f.Function("posts")), "nothing"))

	run(t, client, f.CreateFunction(f.Obj{
		"name":        "posts",
		"permissions": f.Obj{"call": "public"},
		"role":        "server",
		"body": f.Query(f.Lambda(
			f.Arr{"fields"},
			f.Obj{
				"posts": f.Select("data", f.Map(
					f.Paginate(
						f.Match(f.Index("posts")),
					),
					f.Lambda("post", f.Map(
						f.Var("fields"),
						f.Lambda("field", f.Select(f.Arr{"name"}, f.Var("field"))))),
				)),
			})),
	}))

	/*	result, err := client.Query(
		f.Obj{
			"data": f.Obj{
				"posts": f.Select(f.Arr{"data"}, f.Map(
					f.Paginate(
						f.Match(f.Index("posts")),
					),
					f.Lambda("post", f.Let(
						f.Obj{
							"post": f.Get(f.Var("post")),
						},
						f.Obj{
							"id":          f.Select(f.Arr{"ref", "id"}, f.Var("post")),
							"description": f.Select(f.Arr{"data", "description"}, f.Var("post")),
							"comments": f.Select("data", f.Map(
								f.Paginate(
									f.MatchTerm(f.Index("comment_by_post"), f.Select("ref", f.Var("post"))),
								),
								f.Lambda("comment", f.Let(
									f.Obj{
										"comment": f.Get(f.Var("comment")),
									}, f.Obj{
										"text": f.Select(f.Arr{"data", "text"}, f.Var("comment")),
									})),
							)),
						},
					)),
				)),
			},
		},
	)*/

	/*	run(t, client, f.Call(
		f.Function("posts"),
		f.Arr{f.Obj{
			"id":          f.Obj{},
			"description": f.Obj{},
		}},
		/*f.Obj{
			"posts": f.Obj{
				"_from": f.Obj{
					"match": "posts",
				},
				"id":          f.Arr{"ref", "id"},
				"description": f.Arr{"data", "description"},
				"comments": f.Obj{
					"_from": f.Obj{
						"matchTerm": f.Obj{
							"index": "comment_by_post",
							"arg":   "ref",
						},
					},
					"text": f.Arr{"data", "text"},
				},
			},
		}))*/
}
