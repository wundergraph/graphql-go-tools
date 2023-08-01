package resolve

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func newBufPair(data string, err string) *BufPair {
	bufPair := NewBufPair()

	if data != "" {
		bufPair.Data.WriteString(data)
	}

	if err != "" {
		bufPair.Errors.WriteString(err)
	}

	return bufPair
}

func TestDataLoader_Load(t *testing.T) {
	testFn := func(initialState map[int]fetchState, fn func(t *testing.T, ctrl *gomock.Controller) (fetch *SingleFetch, ctx *Context, expectedOutput string)) func(t *testing.T) {
		ctrl := gomock.NewController(t)
		dlFactory := newDataloaderFactory(NewFetcher(false))
		dl := dlFactory.newDataLoader(nil)
		if initialState != nil {
			dl.fetches = initialState
		}

		fetch, ctx, expectedOutput := fn(t, ctrl)

		return func(t *testing.T) {
			t.Helper()

			bufPair := NewBufPair()
			err := dl.Load(ctx, fetch, bufPair)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutput, bufPair.Data.String())
			ctrl.Finish()
		}
	}

	testFnErr := func(initialState map[int]fetchState, fn func(t *testing.T, ctrl *gomock.Controller) (fetch *SingleFetch, ctx *Context, expectedErr string)) func(t *testing.T) {
		ctrl := gomock.NewController(t)
		dlFactory := newDataloaderFactory(NewFetcher(false))
		dl := dlFactory.newDataLoader(nil)
		if initialState != nil {
			dl.fetches = initialState
		}

		fetch, ctx, expectedErr := fn(t, ctrl)

		return func(t *testing.T) {
			t.Helper()

			bufPair := NewBufPair()
			err := dl.Load(ctx, fetch, bufPair)
			assert.EqualError(t, err, expectedErr)
			ctrl.Finish()
		}
	}

	t.Run("root request", testFn(nil, func(t *testing.T, ctrl *gomock.Controller) (fetch *SingleFetch, ctx *Context, expectedOutput string) {
		userService := NewMockDataSource(ctrl)
		userService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`
				assert.Equal(t, expected, actual)
				pair := NewBufPair()
				pair.Data.WriteString(`{"me": {"id": "1234","username": "Me","__typename": "User"}}`)
				return writeGraphqlResponse(pair, w, false)
			}).
			Return(nil)

		return &SingleFetch{
			BufferId: 0,
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{
					{
						Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
						SegmentType: StaticSegmentType,
					},
				},
			},
			DataSource: userService,
		}, &Context{ctx: context.Background()}, `{"data":{"me": {"id": "1234","username": "Me","__typename": "User"}}}`
	}))

	t.Run("requires nested request", testFn(map[int]fetchState{
		1: &singleFetchState{
			nextIdx:     0,
			fetchErrors: nil,
			results:     []*BufPair{newBufPair(`{"someProp": {"id": 11}}`, ``), newBufPair(`{"someProp": {"id": 22}}`, ``)},
		},
	}, func(t *testing.T, ctrl *gomock.Controller) (fetch *SingleFetch, ctx *Context, expectedOutput string) {
		userService := NewMockDataSource(ctrl)
		userService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			Times(2).
			Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				switch {
				case strings.Contains(actual, "11"):
					expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"query($userId: ID!){user(id: $userId){ id username }","variables":{"userId":11}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"user": {"id":11, "username": "Username 11"}}`)
					return writeGraphqlResponse(pair, w, false)
				case strings.Contains(actual, "22"):
					expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"query($userId: ID!){user(id: $userId){ id username }","variables":{"userId":22}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"user": {"id":22, "username": "Username 22"}}`)
					return writeGraphqlResponse(pair, w, false)
				}

				return errors.New("unexpected call")
			}).
			Return(nil)

		return &SingleFetch{
			BufferId: 2,
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{
					{
						Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"query($userId: ID!){user(id: $userId){ id username }","variables":{"userId":`),
						SegmentType: StaticSegmentType,
					},
					{
						SegmentType:        VariableSegmentType,
						VariableKind:       ObjectVariableKind,
						VariableSourcePath: []string{"id"},
						Renderer:           NewPlainVariableRendererWithValidation(`{"type":"number"}`),
					},
					{
						Data:        []byte(`}}`),
						SegmentType: StaticSegmentType,
					},
				},
			},
			DataSource: userService,
		}, &Context{ctx: context.Background(), lastFetchID: 1, responseElements: []string{"someProp"}}, `{"data":{"user": {"id":11, "username": "Username 11"}}}`
	}))

	t.Run("fetch error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		dlFactory := newDataloaderFactory(NewFetcher(false))
		dl := dlFactory.newDataLoader(nil)
		dl.fetches = map[int]fetchState{
			1: &singleFetchState{
				nextIdx:     0,
				fetchErrors: nil,
				results:     []*BufPair{newBufPair(`{"someProp": {"id": 11}}`, ``), newBufPair(`{"someProp": {"id": 22}}`, ``)},
			},
		}

		userService := NewMockDataSource(ctrl)
		userService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			Times(2).
			Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				switch {
				case strings.Contains(actual, "11"):
					expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"query($userId: ID!){user(id: $userId){ id username }","variables":{"$userId":11}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"user": {"id":11, "username": "Username 11"}}`)
					return writeGraphqlResponse(pair, w, false)
				case strings.Contains(actual, "22"):
					return errors.New("failed to access http://localhost:4001")
				}

				return errors.New("unexpected call")
			}).
			Return(nil)

		bufPair := NewBufPair()
		err := dl.Load(
			&Context{ctx: context.Background(), lastFetchID: 1, responseElements: []string{"someProp"}},
			&SingleFetch{
				BufferId: 2,
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"query($userId: ID!){user(id: $userId){ id username }","variables":{"$userId":`),
							SegmentType: StaticSegmentType,
						},
						{
							SegmentType:        VariableSegmentType,
							VariableKind:       ObjectVariableKind,
							VariableSourcePath: []string{"id"},
							Renderer:           NewJSONVariableRendererWithValidation(`{"type":"number"}`),
						},
						{
							Data:        []byte(`}}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				DataSource: userService,
			},
			bufPair,
		)

		assert.NoError(t, err)
		expectedFetchState := map[int]fetchState{
			1: &singleFetchState{
				nextIdx:     0,
				fetchErrors: nil,
				results:     []*BufPair{newBufPair(`{"someProp": {"id": 11}}`, ``), newBufPair(`{"someProp": {"id": 22}}`, ``)},
			},
			2: &singleFetchState{
				nextIdx:     0,
				fetchErrors: []error{nil, errors.New("failed to access http://localhost:4001")},
				results:     []*BufPair{newBufPair(`{"user": {"id":11, "username": "Username 11"}}`, ``), newBufPair(``, ``)},
			},
		}
		assert.NoError(t, expectedFetchState[2].(*singleFetchState).fetchErrors[0])
		assert.EqualError(t, expectedFetchState[2].(*singleFetchState).fetchErrors[1], "failed to access http://localhost:4001")
	})

	t.Run("fetch error in non-corresponding call", testFn(map[int]fetchState{
		1: &singleFetchState{
			nextIdx:     0,
			fetchErrors: make([]error, 2),
			results:     []*BufPair{newBufPair(`{"user": {"id":11, "username": "Username 11"}}`, ``), newBufPair(`{"user": {"id":22, "username": "Username 22"}}`, ``)},
		},
	}, func(t *testing.T, ctrl *gomock.Controller) (fetch *SingleFetch, ctx *Context, expectedOutput string) {
		return &SingleFetch{
			BufferId: 1,
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{
					{
						Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"query($userId: ID!){user(id: $userId){ id username }","variables":{"$userId":`),
						SegmentType: StaticSegmentType,
					},
					{
						SegmentType:        VariableSegmentType,
						VariableKind:       ObjectVariableKind,
						VariableSourcePath: []string{"id"},
					},
					{
						Data:        []byte(`}}`),
						SegmentType: StaticSegmentType,
					},
				},
			},
			DataSource: nil,
		}, &Context{ctx: context.Background(), lastFetchID: 1, responseElements: []string{"someProp"}}, `{"user": {"id":11, "username": "Username 11"}}`
	}))

	t.Run("fetch errors in corresponding call", testFnErr(map[int]fetchState{
		1: &singleFetchState{
			nextIdx:     1,
			fetchErrors: []error{nil, errors.New("someError")},
			results:     []*BufPair{newBufPair(`{"user": {"id":11, "username": "Username 11"}}`, ``), newBufPair(``, ``)},
		},
	}, func(t *testing.T, ctrl *gomock.Controller) (fetch *SingleFetch, ctx *Context, expectedOutput string) {
		return &SingleFetch{
			BufferId: 1,
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{
					{
						Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"query($userId: ID!){user(id: $userId){ id username }","variables":{"$userId":`),
						SegmentType: StaticSegmentType,
					},
					{
						SegmentType:        VariableSegmentType,
						VariableKind:       ObjectVariableKind,
						VariableSourcePath: []string{"id"},
					},
					{
						Data:        []byte(`}}`),
						SegmentType: StaticSegmentType,
					},
				},
			},
			DataSource: nil,
		}, &Context{ctx: context.Background(), lastFetchID: 1, responseElements: []string{"someProp"}}, `someError`
	}))

	t.Run("doesn't requires nested request", testFn(map[int]fetchState{
		1: &singleFetchState{
			nextIdx:     1,
			fetchErrors: make([]error, 2),
			results:     []*BufPair{newBufPair(`{"user": {"id":11, "username": "Username 11"}}`, ``), newBufPair(`{"user": {"id":22, "username": "Username 22"}}`, ``)},
		},
	}, func(t *testing.T, ctrl *gomock.Controller) (fetch *SingleFetch, ctx *Context, expectedOutput string) {
		return &SingleFetch{
			BufferId: 1,
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{
					{
						Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"query($userId: ID!){user(id: $userId){ id username }","variables":{"$userId":`),
						SegmentType: StaticSegmentType,
					},
					{
						SegmentType:        VariableSegmentType,
						VariableKind:       ObjectVariableKind,
						VariableSourcePath: []string{"id"},
					},
					{
						Data:        []byte(`}}`),
						SegmentType: StaticSegmentType,
					},
				},
			},
			DataSource: nil,
		}, &Context{ctx: context.Background(), lastFetchID: 1, responseElements: []string{"someProp"}}, `{"user": {"id":22, "username": "Username 22"}}`
	}))

	t.Run("requires nested request with array in path", testFn(map[int]fetchState{
		1: &singleFetchState{
			nextIdx:     0,
			fetchErrors: nil,
			results:     []*BufPair{newBufPair(`{"someProp": [{"id": 11}, {"id": 22}]}`, ``), newBufPair(`{"someProp": [{"id": 11}, {"id": 22}]}`, ``)},
		},
	}, func(t *testing.T, ctrl *gomock.Controller) (fetch *SingleFetch, ctx *Context, expectedOutput string) {
		userService := NewMockDataSource(ctrl)
		userService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			Times(4).
			Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				switch {
				case strings.Contains(actual, "11"):
					expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"query($userId: ID!){user(id: $userId){ id username }","variables":{"$userId":11}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"user": {"id":11, "username": "Username 11"}}`)
					return writeGraphqlResponse(pair, w, false)
				case strings.Contains(actual, "22"):
					expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"query($userId: ID!){user(id: $userId){ id username }","variables":{"$userId":22}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"user": {"id":22, "username": "Username 22"}}`)
					return writeGraphqlResponse(pair, w, false)
				}

				return errors.New("unexpected call")
			}).
			Return(nil)

		return &SingleFetch{
			BufferId: 2,
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{
					{
						Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"query($userId: ID!){user(id: $userId){ id username }","variables":{"$userId":`),
						SegmentType: StaticSegmentType,
					},
					{
						SegmentType:        VariableSegmentType,
						VariableKind:       ObjectVariableKind,
						VariableSourcePath: []string{"id"},
						Renderer:           NewJSONVariableRendererWithValidation(`{"type":"number"}`),
					},
					{
						Data:        []byte(`}}`),
						SegmentType: StaticSegmentType,
					},
				},
			},
			DataSource: userService,
		}, &Context{ctx: context.Background(), lastFetchID: 1, responseElements: []string{"someProp", arrayElementKey}}, `{"data":{"user": {"id":11, "username": "Username 11"}}}`
	}))

	t.Run("requires nested request with null array in path", testFn(map[int]fetchState{
		1: &singleFetchState{
			nextIdx:     0,
			fetchErrors: nil,
			results:     []*BufPair{newBufPair(`{"someProp": null}`, ``), newBufPair(`{"someProp": [{"id": 11}, {"id": 22}]}`, ``)},
		},
	}, func(t *testing.T, ctrl *gomock.Controller) (fetch *SingleFetch, ctx *Context, expectedOutput string) {
		userService := NewMockDataSource(ctrl)
		userService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			Times(2).
			Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				switch {
				case strings.Contains(actual, "11"):
					expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"query($userId: ID!){user(id: $userId){ id username }","variables":{"$userId":11}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"user": {"id":11, "username": "Username 11"}}`)
					return writeGraphqlResponse(pair, w, false)
				case strings.Contains(actual, "22"):
					expected := `{"method":"POST","url":"http://localhost:4001","body":{"query":"query($userId: ID!){user(id: $userId){ id username }","variables":{"$userId":22}}`
					assert.Equal(t, expected, actual)
					pair := NewBufPair()
					pair.Data.WriteString(`{"user": {"id":22, "username": "Username 22"}}`)
					return writeGraphqlResponse(pair, w, false)
				}

				return errors.New("unexpected call")
			}).
			Return(nil)

		return &SingleFetch{
			BufferId: 2,
			InputTemplate: InputTemplate{
				Segments: []TemplateSegment{
					{
						Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"query($userId: ID!){user(id: $userId){ id username }","variables":{"$userId":`),
						SegmentType: StaticSegmentType,
					},
					{
						SegmentType:        VariableSegmentType,
						VariableKind:       ObjectVariableKind,
						VariableSourcePath: []string{"id"},
						Renderer:           NewJSONVariableRendererWithValidation(`{"type":"number"}`),
					},
					{
						Data:        []byte(`}}`),
						SegmentType: StaticSegmentType,
					},
				},
			},
			DataSource: userService,
		}, &Context{ctx: context.Background(), lastFetchID: 1, responseElements: []string{"someProp", arrayElementKey}}, `{"data":{"user": {"id":11, "username": "Username 11"}}}`
	}))
}

func TestDataLoader_LoadBatch(t *testing.T) {
	testFn := func(initialState map[int]fetchState, fn func(t *testing.T, ctrl *gomock.Controller) (fetch *BatchFetch, ctx *Context, expectedOutput string)) func(t *testing.T) {
		ctrl := gomock.NewController(t)
		dlFactory := newDataloaderFactory(NewFetcher(false))
		dl := dlFactory.newDataLoader(nil)
		if initialState != nil {
			dl.fetches = initialState
		}

		fetch, ctx, expectedOutput := fn(t, ctrl)

		return func(t *testing.T) {
			bufPair := NewBufPair()
			err := dl.LoadBatch(ctx, fetch, bufPair)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutput, bufPair.Data.String())
			ctrl.Finish()
		}
	}

	t.Run("requires nested request", testFn(map[int]fetchState{
		1: &batchFetchState{
			nextIdx:    0,
			fetchError: nil,
			results:    []*BufPair{newBufPair(`{"someProp": {"upc": "top-1"}}`, ``), newBufPair(`{"someProp": {"upc": "top-2"}}`, ``)},
		},
	}, func(t *testing.T, ctrl *gomock.Controller) (fetch *BatchFetch, ctx *Context, expectedOutput string) {
		batchFactory := NewMockDataSourceBatchFactory(ctrl)
		batchFactory.EXPECT().
			CreateBatch(
				[][]byte{
					[]byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}}}`),
					[]byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}}}`),
				},
			).Return(NewFakeDataSourceBatch(
			`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"},{"upc":"top-2","__typename":"Product"}]}}}`,
			[]resultedBufPair{
				{data: `{"name": "Trilby"}`},
				{data: `{"name": "Fedora"}`},
			}), nil)

		userService := NewMockDataSource(ctrl)
		userService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				expected := `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"},{"upc":"top-2","__typename":"Product"}]}}}`
				assert.Equal(t, expected, actual)
				pair := NewBufPair()
				pair.Data.WriteString(`[{"name": "Trilby"},{"name": "Fedora"}]`)
				return writeGraphqlResponse(pair, w, false)
			}).
			Return(nil)

		return &BatchFetch{
			Fetch: &SingleFetch{
				BufferId: 2,
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":`),
							SegmentType: StaticSegmentType,
						},
						{
							SegmentType:        VariableSegmentType,
							VariableKind:       ObjectVariableKind,
							VariableSourcePath: []string{"upc"},
							Renderer:           NewJSONVariableRendererWithValidation(`{"type":"string"}`),
						},
						{
							Data:        []byte(`,"__typename":"Product"}]}}}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				DataSource: userService,
			},
			BatchFactory: batchFactory,
		}, &Context{ctx: context.Background(), lastFetchID: 1, responseElements: []string{"someProp"}}, `{"name": "Trilby"}`
	}))

	t.Run("deeply nested fetch with varying fields", testFn(map[int]fetchState{
		1: &batchFetchState{
			nextIdx:    0,
			fetchError: nil,
			// The fetch is to fill in additional engine information.
			results: []*BufPair{
				newBufPair(`{"vehicle": {"__typename": "Car", "color": "black", "engine": {"model": "x"}}}`, ``),
				newBufPair(`{"vehicle": {"__typename": "Bicycle", "color": "yellow"}}`, ``),
				newBufPair(`{"vehicle": {"__typename": "Car", "color": "black", "engine": {"model": "y"}}}`, ``),
			},
		},
	}, func(t *testing.T, ctrl *gomock.Controller) (fetch *BatchFetch, ctx *Context, expectedOutput string) {
		batchFactory := NewMockDataSourceBatchFactory(ctrl)
		batchFactory.EXPECT().
			CreateBatch(
				[][]byte{
					[]byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Engine {horsepower}}}","variables":{"representations":[{"model":"x","__typename":"Engine"}]}}}`),
					[]byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Engine {horsepower}}}","variables":{"representations":[{"model":"y","__typename":"Engine"}]}}}`),
				},
			).Return(NewFakeDataSourceBatch(
			`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Engine {horsepower}}}","variables":{"representations":[{"model":"x","__typename":"Engine"},{"model":"y","__typename":"Engine"}]}}}`,
			[]resultedBufPair{
				{data: `{"horsepower": 200}`},
				{data: `{"horsepower": 400}`},
			}), nil)

		carService := NewMockDataSource(ctrl)
		carService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				expected := `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Engine {horsepower}}}","variables":{"representations":[{"model":"x","__typename":"Engine"},{"model":"y","__typename":"Engine"}]}}}`
				assert.Equal(t, expected, actual)
				pair := NewBufPair()
				pair.Data.WriteString(`[{"horsepower": 200},{"horsepower": 400}]`)
				return writeGraphqlResponse(pair, w, false)
			}).
			Return(nil)

		return &BatchFetch{
			Fetch: &SingleFetch{
				BufferId: 2,
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Engine {horsepower}}}","variables":{"representations":[{"model":`),
							SegmentType: StaticSegmentType,
						},
						{
							SegmentType:        VariableSegmentType,
							VariableKind:       ObjectVariableKind,
							VariableSourcePath: []string{"model"},
							Renderer:           NewJSONVariableRendererWithValidation(`{"type":"string"}`),
						},
						{
							Data:        []byte(`,"__typename":"Engine"}]}}}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
				DataSource: carService,
			},
			BatchFactory: batchFactory,
		}, &Context{ctx: context.Background(), lastFetchID: 1, responseElements: []string{"vehicle", "engine"}}, `{"horsepower": 200}`
	}))

	t.Run("doesn't requires nested request", testFn(map[int]fetchState{
		1: &batchFetchState{
			nextIdx:    1,
			fetchError: nil,
			results:    []*BufPair{newBufPair(`{"user": {"id":11, "username": "Username 11"}}`, ``), newBufPair(`{"user": {"id":22, "username": "Username 22"}}`, ``)},
		},
	}, func(t *testing.T, ctrl *gomock.Controller) (fetch *BatchFetch, ctx *Context, expectedOutput string) {
		return &BatchFetch{
			Fetch: &SingleFetch{
				BufferId: 1,
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"`),
							SegmentType: StaticSegmentType,
						},
						{
							SegmentType:        VariableSegmentType,
							VariableKind:       ObjectVariableKind,
							VariableSourcePath: []string{"upc"},
						},
						{
							Data:        []byte(`","__typename":"Product"}]}}}`),
							SegmentType: StaticSegmentType,
						},
					},
				},
			},
		}, &Context{ctx: context.Background(), lastFetchID: 1, responseElements: []string{"someProp"}}, `{"user": {"id":22, "username": "Username 22"}}`
	}))

	t.Run("fetch error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		dlFactory := newDataloaderFactory(NewFetcher(false))
		dl := dlFactory.newDataLoader(nil)
		dl.fetches = map[int]fetchState{
			1: &singleFetchState{
				nextIdx:     0,
				fetchErrors: nil,
				results:     []*BufPair{newBufPair(`{"someProp": {"upc": "top-1"}}`, ``), newBufPair(`{"someProp": {"upc": "top-2"}}`, ``)},
			},
		}

		expErr := errors.New("failed to access http://localhost:4003")

		batchFactory := NewMockDataSourceBatchFactory(ctrl)
		batchFactory.EXPECT().
			CreateBatch(
				[][]byte{
					[]byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"}]}}}`),
					[]byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-2","__typename":"Product"}]}}}`),
				},
			).Return(NewFakeDataSourceBatch(
			`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"},{"upc":"top-2","__typename":"Product"}]}}}`,
			[]resultedBufPair{}), nil)

		userService := NewMockDataSource(ctrl)
		userService.EXPECT().
			Load(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&bytes.Buffer{})).
			Do(func(ctx context.Context, input []byte, w io.Writer) (err error) {
				actual := string(input)
				expected := `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"top-1","__typename":"Product"},{"upc":"top-2","__typename":"Product"}]}}}`
				assert.Equal(t, expected, actual)
				return
			}).
			Return(expErr)

		err := dl.LoadBatch(
			&Context{ctx: context.Background(), lastFetchID: 1, responseElements: []string{"someProp"}},
			&BatchFetch{
				Fetch: &SingleFetch{
					BufferId: 2,
					InputTemplate: InputTemplate{
						Segments: []TemplateSegment{
							{
								Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":`),
								SegmentType: StaticSegmentType,
							},
							{
								SegmentType:        VariableSegmentType,
								VariableKind:       ObjectVariableKind,
								VariableSourcePath: []string{"upc"},
								Renderer:           NewJSONVariableRendererWithValidation(`{"type":"string"}`),
							},
							{
								Data:        []byte(`,"__typename":"Product"}]}}}`),
								SegmentType: StaticSegmentType,
							},
						},
					},
					DataSource: userService,
				},
				BatchFactory: batchFactory,
			},
			NewBufPair(),
		)

		assert.EqualError(t, err, expErr.Error())
	})
}
