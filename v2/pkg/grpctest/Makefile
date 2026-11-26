mkfile_dir := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
grpc_datasource_dir := $(mkfile_dir)/../engine/datasource/grpc_datasource
cosmo_cli_dir := $(mkfile_dir)/../../../../cosmo/cli

# wgc is the published cosmo CLI. wgc_local runs the CLI from a cosmo checkout
# in the same parent directory as graphql-go-tools.
wgc := pnpx wgc@latest
wgc_local := pnpx tsx --env-file $(cosmo_cli_dir)/.env $(cosmo_cli_dir)/src/index.ts

# Each service sets these variables:
#   <service>_dir          directory with the service files
#   <service>_schema       GraphQL schema, relative to the service directory
#   <service>_proto        generated proto file, relative to the service directory
#   <service>_package      proto package
#   <service>_go_package   go_package option of the proto file
#   <service>_rpc_service  name of the gRPC service
#   <service>_flags        extra flags for `wgc grpc-service generate`
#   <service>_name         service name in the generated mappings
#   <service>_helper       generated mapping helper file in grpc_datasource
#   <service>_helper_func  function name of the mapping helper in grpc_datasource
services := products events

# The products service keeps the google.protobuf wrapper types for nullable scalars.
products_dir := $(mkfile_dir)
products_schema := testdata/products.graphqls
products_proto := product.proto
products_package := productv1
products_go_package := cosmo/pkg/proto/productv1;productv1
products_rpc_service := ProductService
products_flags := --legacy-wrapper-types
products_name := Products
products_helper := mapping_helper_test.go
products_helper_func := testMapping

# The events service uses proto3 optional fields for nullable scalars.
events_dir := $(mkfile_dir)/events
events_schema := testdata/events.graphqls
events_proto := event.proto
events_package := eventv1
events_go_package := github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/events/eventv1;eventv1
events_rpc_service := EventService
events_flags :=
events_name := Events
events_helper := mapping_helper_events_test.go
events_helper_func := eventsTestMapping

# service_targets defines the generation targets for the service $(1):
#   generate-$(1)        regenerates the proto, the mappings and the Go code with the published CLI
#   generate-$(1)-local  does the same with the local CLI
#   generate-$(1)-proto  regenerates the Go code from the proto file with buf
define service_targets
.PHONY: generate-$(1) generate-$(1)-local generate-$(1)-proto

generate-$(1): cli := $(wgc)
generate-$(1)-local: cli := $(wgc_local)

generate-$(1) generate-$(1)-local:
	cd $($(1)_dir) && $$(cli) grpc-service generate -i $($(1)_schema) -o testdata/ -p $($(1)_package) -g "$($(1)_go_package)" $($(1)_flags) $($(1)_rpc_service)
	mv $($(1)_dir)/testdata/service.proto $($(1)_dir)/$($(1)_proto)
	rm -f $($(1)_dir)/testdata/service.proto.lock.json
	mkdir -p $($(1)_dir)/mapping
	go run $(mkfile_dir)/cmd/mapping_helper -service $($(1)_name) $($(1)_dir)/testdata/mapping.json $($(1)_dir)/mapping/mapping.go
	go run $(mkfile_dir)/cmd/mapping_helper -grpc-datasource -service $($(1)_name) -func $($(1)_helper_func) $($(1)_dir)/testdata/mapping.json $(grpc_datasource_dir)/$($(1)_helper)
	gofmt -w $($(1)_dir)/mapping/mapping.go $(grpc_datasource_dir)/$($(1)_helper)
	rm -f $($(1)_dir)/testdata/mapping.json
	$(MAKE) generate-$(1)-proto

# buf drives protoc-gen-go (messages), protoc-gen-go-grpc (gRPC server and
# client) and protoc-gen-connect-go (Connect handler and client). See the
# buf.gen.yaml file of the service.
generate-$(1)-proto:
	cd $($(1)_dir) && go tool buf generate
endef

$(foreach service,$(services),$(eval $(call service_targets,$(service))))

.PHONY: generate-all
generate-all: $(addprefix generate-,$(services))

.PHONY: generate-all-local
generate-all-local: $(addsuffix -local,$(addprefix generate-,$(services)))

.PHONY: generate-proto
generate-proto: $(addsuffix -proto,$(addprefix generate-,$(services)))

.PHONY: build-plugin
build-plugin:
	go build -o plugin/plugin_service plugin/plugin_service.go
