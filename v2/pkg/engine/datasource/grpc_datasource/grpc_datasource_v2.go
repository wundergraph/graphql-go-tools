package grpcdatasource

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"buf.build/go/hyperpb"
	"github.com/cespare/xxhash/v2"
	"github.com/tidwall/gjson"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	protoref "google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
)

var _ resolve.DataSource = (*DataSourceV2)(nil)
var _ resolve.NativeDataSource = (*DataSourceV2)(nil)
var _ resolve.NativeMergeDataSource = (*DataSourceV2)(nil)

type DataSourceV2 struct {
	program       *v2Program
	schema        *v2SchemaRuntime
	fallback      *DataSource
	responseFrame sync.Pool
	valuePool     *arena.Pool
	hyperpbShared sync.Pool
	callOptions   []grpc.CallOption
}

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

	ds := &DataSourceV2{
		program:   program,
		schema:    schema,
		fallback:  fallback,
		valuePool: arena.NewArenaPool(),
	}
	ds.responseFrame.New = func() any {
		return newV2ResponseFrameBuilder()
	}
	ds.hyperpbShared.New = func() any {
		return new(hyperpb.Shared)
	}
	ds.callOptions = []grpc.CallOption{grpc.ForceCodec(v2HyperpbCodec{})}

	return ds, nil
}

func (d *DataSourceV2) Load(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
	value, cleanup, err := d.LoadValue(ctx, headers, input)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, err
	}
	return value.MarshalTo(nil), nil
}

func (d *DataSourceV2) LoadResult(ctx context.Context, headers http.Header, input []byte) (resolve.NativeMergeResult, func(), error) {
	if !d.program.nativeOperation || d.fallback.disabled {
		return nil, nil, nil
	}

	variables := gjson.Parse(unsafebytes.BytesToString(input)).Get("body.variables")
	builder := newJSONBuilder(nil, d.fallback.mapping, variables)
	response := d.acquireResponseFrame()
	var shareds []*hyperpb.Shared
	cleanup := func() {
		for _, shared := range shareds {
			d.releaseHyperpbShared(shared)
		}
		d.releaseResponseFrame(response)
	}

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

	root := response.newObject()
	outputs := make(map[int]protoref.Message, len(d.program.stages))
	for _, stage := range d.program.stages {
		results := make([]v2NativeResponse, len(stage.fetches))
		errGrp, errGrpCtx := errgroup.WithContext(ctx)

		for index := range stage.fetches {
			fetch := stage.fetches[index]
			errGrp.Go(func() error {
				var (
					request any
					skip    bool
					err     error
					shared  *hyperpb.Shared
				)

				if fetch.kind == CallKindResolve {
					var dependencyOutput protoref.Message
					if len(fetch.dependencies) > 0 {
						dependencyOutput = outputs[fetch.dependencies[0]]
					}
					request, skip, err = fetch.request.buildWithDependency(variables, dependencyOutput, d.schema, d.fallback.rc)
				} else {
					request, err = fetch.request.buildInput(variables, d.schema, d.fallback.rc)
				}
				if err != nil {
					return err
				}
				if skip {
					results[index] = v2NativeResponse{
						kind:         fetch.kind,
						responsePath: fetch.responsePath,
						skip:         true,
					}
					return nil
				}

				if fetch.response.message.generatedType == nil && fetch.response.message.hyperType != nil {
					shared = d.acquireHyperpbShared()
				}
				responseMessage := fetch.response.message.newDecodeMessage(shared)
				requestArg := request
				if message, ok := request.(protoref.Message); ok {
					requestArg = message.Interface()
				}
				if err := d.fallback.cc.Invoke(errGrpCtx, "/"+fetch.serviceName+"/"+fetch.methodName, requestArg, responseMessage.Interface(), d.callOptions...); err != nil {
					if shared != nil {
						d.releaseHyperpbShared(shared)
					}
					return err
				}
				if fetch.kind == CallKindEntity {
					if err := fetch.response.validateFederatedOutput(builder, responseMessage); err != nil {
						if shared != nil {
							d.releaseHyperpbShared(shared)
						}
						return err
					}
				}

				results[index] = v2NativeResponse{
					kind:         fetch.kind,
					responsePath: fetch.responsePath,
					output:       responseMessage,
					shared:       shared,
				}
				return nil
			})
		}

		if err := errGrp.Wait(); err != nil {
			cleanup()
			return nil, nil, nil
		}

		for index, result := range results {
			if result.skip {
				continue
			}

			outputs[stage.fetches[index].id] = result.output
			if result.shared != nil {
				shareds = append(shareds, result.shared)
			}

			if err := stage.fetches[index].response.attach(builder, response, root, result.output, result.kind, result.responsePath); err != nil {
				cleanup()
				return nil, nil, nil
			}
		}
	}

	return &v2NativeMergeResult{frame: response, root: root}, cleanup, nil
}

func (d *DataSourceV2) LoadWithFilesResult(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) (resolve.NativeMergeResult, func(), error) {
	return nil, nil, nil
}

func (d *DataSourceV2) LoadValue(ctx context.Context, headers http.Header, input []byte) (*astjson.Value, func(), error) {
	if !d.program.nativeOperation {
		return d.parseFallbackBytes(ctx, headers, input)
	}

	variables := gjson.Parse(unsafebytes.BytesToString(input)).Get("body.variables")
	builder := newJSONBuilder(nil, d.fallback.mapping, variables)
	response := d.acquireResponseFrame()
	item := d.valuePool.Acquire(xxhash.Sum64(input))
	var shareds []*hyperpb.Shared
	cleanup := func() {
		for _, shared := range shareds {
			d.releaseHyperpbShared(shared)
		}
		d.releaseResponseFrame(response)
		d.valuePool.Release(item)
	}

	if d.fallback.disabled {
		value, err := astjson.ParseBytesWithArena(item.Arena, builder.writeErrorBytes(fmt.Errorf("gRPC datasource needs to be enabled to be used")))
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		return value, cleanup, nil
	}

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

	root := response.newObject()
	outputs := make(map[int]protoref.Message, len(d.program.stages))
	for _, stage := range d.program.stages {
		results := make([]v2NativeResponse, len(stage.fetches))
		errGrp, errGrpCtx := errgroup.WithContext(ctx)

		for index := range stage.fetches {
			fetch := stage.fetches[index]
			errGrp.Go(func() error {
				var (
					request any
					skip    bool
					err     error
					shared  *hyperpb.Shared
				)

				if fetch.kind == CallKindResolve {
					var dependencyOutput protoref.Message
					if len(fetch.dependencies) > 0 {
						dependencyOutput = outputs[fetch.dependencies[0]]
					}
					request, skip, err = fetch.request.buildWithDependency(variables, dependencyOutput, d.schema, d.fallback.rc)
				} else {
					request, err = fetch.request.buildInput(variables, d.schema, d.fallback.rc)
				}
				if err != nil {
					return err
				}
				if skip {
					results[index] = v2NativeResponse{
						kind:         fetch.kind,
						responsePath: fetch.responsePath,
						skip:         true,
					}
					return nil
				}

				if fetch.response.message.generatedType == nil && fetch.response.message.hyperType != nil {
					shared = d.acquireHyperpbShared()
				}
				response := fetch.response.message.newDecodeMessage(shared)
				requestArg := request
				if message, ok := request.(protoref.Message); ok {
					requestArg = message.Interface()
				}
				if err := d.fallback.cc.Invoke(errGrpCtx, "/"+fetch.serviceName+"/"+fetch.methodName, requestArg, response.Interface(), d.callOptions...); err != nil {
					if shared != nil {
						d.releaseHyperpbShared(shared)
					}
					return err
				}
				if fetch.kind == CallKindEntity {
					if err := fetch.response.validateFederatedOutput(builder, response); err != nil {
						if shared != nil {
							d.releaseHyperpbShared(shared)
						}
						return err
					}
				}

				results[index] = v2NativeResponse{
					kind:         fetch.kind,
					responsePath: fetch.responsePath,
					output:       response,
					shared:       shared,
				}
				return nil
			})
		}

		if err := errGrp.Wait(); err != nil {
			value, parseErr := astjson.ParseBytesWithArena(item.Arena, builder.writeErrorBytes(err))
			if parseErr != nil {
				cleanup()
				return nil, nil, parseErr
			}
			return value, cleanup, nil
		}

		for index, result := range results {
			if result.skip {
				continue
			}

			outputs[stage.fetches[index].id] = result.output
			if result.shared != nil {
				shareds = append(shareds, result.shared)
			}

			if err := stage.fetches[index].response.attach(builder, response, root, result.output, result.kind, result.responsePath); err != nil {
				value, parseErr := astjson.ParseBytesWithArena(item.Arena, builder.writeErrorBytes(err))
				if parseErr != nil {
					cleanup()
					return nil, nil, parseErr
				}
				return value, cleanup, nil
			}
		}
	}

	value := response.dataEnvelopeValue(item.Arena, root)
	return value, cleanup, nil
}

func (d *DataSourceV2) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) ([]byte, error) {
	return d.fallback.LoadWithFiles(ctx, headers, input, files)
}

func (d *DataSourceV2) LoadWithFilesValue(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) (*astjson.Value, func(), error) {
	return d.parseFallbackBytesWithFiles(ctx, headers, input, files)
}

func (d *DataSourceV2) acquireResponseFrame() *v2ResponseFrameBuilder {
	frame, _ := d.responseFrame.Get().(*v2ResponseFrameBuilder)
	if frame == nil {
		frame = newV2ResponseFrameBuilder()
	}
	frame.reset()
	return frame
}

func (d *DataSourceV2) releaseResponseFrame(frame *v2ResponseFrameBuilder) {
	if frame == nil {
		return
	}
	frame.reset()
	d.responseFrame.Put(frame)
}

func (d *DataSourceV2) acquireHyperpbShared() *hyperpb.Shared {
	shared, _ := d.hyperpbShared.Get().(*hyperpb.Shared)
	if shared == nil {
		shared = new(hyperpb.Shared)
	}
	return shared
}

func (d *DataSourceV2) releaseHyperpbShared(shared *hyperpb.Shared) {
	if shared == nil {
		return
	}
	shared.Free()
	d.hyperpbShared.Put(shared)
}

func (d *DataSourceV2) parseFallbackBytes(ctx context.Context, headers http.Header, input []byte) (*astjson.Value, func(), error) {
	data, err := d.fallback.Load(ctx, headers, input)
	if err != nil {
		return nil, nil, err
	}
	item := d.valuePool.Acquire(xxhash.Sum64(input))
	value, err := astjson.ParseBytesWithArena(item.Arena, data)
	if err != nil {
		d.valuePool.Release(item)
		return nil, nil, err
	}
	return value, func() {
		d.valuePool.Release(item)
	}, nil
}

func (d *DataSourceV2) parseFallbackBytesWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) (*astjson.Value, func(), error) {
	data, err := d.fallback.LoadWithFiles(ctx, headers, input, files)
	if err != nil {
		return nil, nil, err
	}
	item := d.valuePool.Acquire(xxhash.Sum64(input))
	value, err := astjson.ParseBytesWithArena(item.Arena, data)
	if err != nil {
		d.valuePool.Release(item)
		return nil, nil, err
	}
	return value, func() {
		d.valuePool.Release(item)
	}, nil
}
