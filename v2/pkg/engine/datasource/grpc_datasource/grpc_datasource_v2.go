package grpcdatasource

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	protoref "google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// DataSourceV2 is a parallel implementation of the gRPC datasource built on
// a compiled program IR. Queries whose shape is covered by the IR compiler
// run "natively" through the V2 runtime; everything else transparently falls
// back to the existing V1 DataSource.
//
// Architecture: IR → compile → runtime; flat index-based response frame;
// dual-backend schema (generated proto + dynamicpb); runtime-pooled arena.
//
// Implements resolve.DataSource. Opts into ContextSkippingDataSource since
// gRPC does not produce HTTP response contexts.
type DataSourceV2 struct {
	program  *v2Program
	schema   *v2SchemaRuntime
	fallback *DataSource // V1 — handles unsupported shapes
	pool     *arena.Pool
}

var (
	_ resolve.DataSource                = (*DataSourceV2)(nil)
	_ resolve.ContextSkippingDataSource = (*DataSourceV2)(nil)
)

// NewDataSourceV2 builds the V2 datasource. V1 is also constructed as the
// fallback — this keeps unsupported query shapes working from day one.
// Compilation errors in the V2 program are non-fatal: if the program cannot
// be compiled cleanly, all requests route through V1.
func NewDataSourceV2(client grpc.ClientConnInterface, config DataSourceConfig) (*DataSourceV2, error) {
	fallback, err := NewDataSource(client, config)
	if err != nil {
		return nil, err
	}
	schema, err := newV2SchemaRuntime(config.Compiler)
	if err != nil {
		return nil, err
	}
	program, err := compileV2Program(fallback.plan, schema, config.Compiler)
	if err != nil {
		return nil, err
	}
	return &DataSourceV2{
		program:  program,
		schema:   schema,
		fallback: fallback,
		pool:     arena.NewArenaPool(),
	}, nil
}

// SkipsHTTPResponseContext: marker for the loader's fast-path skip of
// httpclient.InjectResponseContext. Same as V1.
func (d *DataSourceV2) SkipsHTTPResponseContext() {}

// Load implements resolve.DataSource. When the compiled program covers the
// operation, the native path runs; otherwise V1 handles it transparently.
func (d *DataSourceV2) Load(ctx context.Context, headers http.Header, input []byte) (*astjson.Value, func(), error) {
	if !d.program.nativeOperation {
		return d.fallback.Load(ctx, headers, input)
	}

	if d.fallback.disabled {
		return d.fallback.Load(ctx, headers, input)
	}

	variables := gjson.Parse(string(input)).Get("body.variables")

	// convert headers to gRPC metadata
	if len(headers) > 0 {
		pairs := make([]string, 0, len(headers)*2)
		for headerName, headerValues := range headers {
			headerName = strings.ToLower(headerName)
			for _, v := range headerValues {
				pairs = append(pairs, headerName, v)
			}
		}
		ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
	}

	// Acquire per-request resources. Two pieces of pooled state:
	//   - frame: the flat node slice — populated by attach, serialized at end.
	//   - poolItem: the astjson arena for the final Value emitted to the
	//     loader. Cleanup releases it when the loader signals it's done.
	frame := v2FramePool.Get().(*v2ResponseFrameBuilder)
	frame.reset()
	poolItem := d.pool.Acquire(0)

	cleanup := func() {
		frame.reset()
		v2FramePool.Put(frame)
		d.pool.ReleaseMany([]*arena.PoolItem{poolItem})
	}
	done := false
	defer func() {
		if !done {
			cleanup()
		}
	}()

	root := frame.newObject()
	outputs := make(map[int]protoref.Message, len(d.program.stages))

	for _, stage := range d.program.stages {
		results := make([]v2NativeResponse, len(stage.fetches))
		errGrp, errGrpCtx := errgroup.WithContext(ctx)

		for idx := range stage.fetches {
			fetch := stage.fetches[idx]
			errGrp.Go(func() error {
				var (
					request protoref.Message
					skip    bool
					err     error
				)
				if fetch.kind == CallKindResolve {
					var dep protoref.Message
					if len(fetch.dependencies) > 0 {
						dep = outputs[fetch.dependencies[0]]
					}
					request, skip, err = fetch.request.buildResolveRequest(variables, dep, d.schema, d.fallback.rc)
				} else {
					request, err = fetch.request.buildRequest(variables, d.schema, d.fallback.rc)
				}
				if err != nil {
					return err
				}
				if skip {
					results[idx] = v2NativeResponse{kind: fetch.kind, responsePath: fetch.responsePath, skip: true}
					return nil
				}

				response := fetch.response.message.newMessage()
				if err := d.fallback.cc.Invoke(errGrpCtx, fetch.methodFullName, request.Interface(), response.Interface()); err != nil {
					return err
				}
				results[idx] = v2NativeResponse{
					kind:         fetch.kind,
					responsePath: fetch.responsePath,
					output:       response,
				}
				return nil
			})
		}
		if err := errGrp.Wait(); err != nil {
			return nil, nil, fmt.Errorf("v2 fetch failed: %w", err)
		}

		for idx, result := range results {
			if result.skip {
				continue
			}
			outputs[stage.fetches[idx].id] = result.output
			if err := stage.fetches[idx].response.attach(frame, root, result.output, result.kind, result.responsePath, d.fallback.mapping); err != nil {
				return nil, nil, err
			}
		}
	}

	// Materialize the frame into an *astjson.Value on the pool arena. One
	// traversal — no serialize/reparse round-trip.
	dataValue := frame.toAstjson(poolItem.Arena, root)
	envelope := astjson.ObjectValue(poolItem.Arena)
	envelope.Set(poolItem.Arena, "data", dataValue)

	done = true
	return envelope, cleanup, nil
}

// LoadWithFiles delegates to V1 — file uploads are not part of V2's scope.
func (d *DataSourceV2) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) (*astjson.Value, func(), error) {
	return d.fallback.LoadWithFiles(ctx, headers, input, files)
}
