mkfile_dir := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
grpc_datasource_dir := $(mkfile_dir)/../engine/datasource/grpc_datasource

.PHONY: install-protoc
install-protoc:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

.PHONY: generate-proto
generate-proto: install-protoc
	protoc --go_out=productv1 --go_opt=paths=source_relative \
    	--go-grpc_out=productv1 --go-grpc_opt=paths=source_relative \
    	product.proto

.PHONY: build-plugin
build-plugin:
	go build -o plugin/plugin_service plugin/plugin_service.go

.PHONY: regenerate-proto
regenerate-proto:
	pnpx wgc@latest grpc-service generate -i testdata/products.graphqls -o testdata/ -p productv1 -g "cosmo/pkg/proto/productv1;productv1" ProductService
	mv $(mkfile_dir)/testdata/service.proto $(mkfile_dir)/product.proto
	rm -f $(mkfile_dir)/testdata/service.proto.lock.json

.PHONY: generate-mapping-code
generate-mapping-code: cleanup-mapping-file

.PHONY: generate-mapping-code-grpc-test
generate-mapping-code-grpc-test: regenerate-proto
	go run $(mkfile_dir)/cmd/mapping_helper/main.go $(mkfile_dir)/testdata/mapping.json $(mkfile_dir)/mapping/mapping.go
	go fmt $(mkfile_dir)/mapping/mapping.go

.PHONY: generate-mapping-code-grpc-datasource
generate-mapping-code-grpc-datasource: generate-mapping-code-grpc-test
	go run $(mkfile_dir)/cmd/mapping_helper/main.go -grpc-datasource $(mkfile_dir)/testdata/mapping.json $(grpc_datasource_dir)/mapping_test_helper.go
	go fmt $(grpc_datasource_dir)/mapping_test_helper.go

.PHONY: cleanup-mapping-file
cleanup-mapping-file: generate-mapping-code-grpc-datasource
	rm -f $(mkfile_dir)/testdata/mapping.json


.PHONY: regenerate-proto-local
regenerate-proto-local:
	pnpx tsx --env-file ../../../../cosmo/cli/.env ../../../../cosmo/cli/src/index.ts grpc-service generate -i testdata/products.graphqls -o testdata/ -p productv1 -g "cosmo/pkg/proto/productv1;productv1" ProductService
	mv $(mkfile_dir)/testdata/service.proto $(mkfile_dir)/product.proto
	rm -f $(mkfile_dir)/testdata/service.proto.lock.json

.PHONY: generate-mapping-code-local
generate-mapping-code-local: cleanup-mapping-file-local

.PHONY: generate-mapping-code-grpc-test-local
generate-mapping-code-grpc-test-local: regenerate-proto-local
	go run $(mkfile_dir)/cmd/mapping_helper/main.go $(mkfile_dir)/testdata/mapping.json $(mkfile_dir)/mapping/mapping.go
	go fmt $(mkfile_dir)/mapping/mapping.go

.PHONY: generate-mapping-code-grpc-datasource-local
generate-mapping-code-grpc-datasource-local: generate-mapping-code-grpc-test-local
	go run $(mkfile_dir)/cmd/mapping_helper/main.go -grpc-datasource $(mkfile_dir)/testdata/mapping.json $(grpc_datasource_dir)/mapping_test_helper.go
	go fmt $(grpc_datasource_dir)/mapping_test_helper.go

.PHONY: cleanup-mapping-file-local
cleanup-mapping-file-local: generate-mapping-code-grpc-datasource-local
	rm -f $(mkfile_dir)/testdata/mapping.json


.PHONY: generate-all
generate-all: generate-mapping-code generate-proto

.PHONY: generate-all-local
generate-all-local: generate-mapping-code-local generate-proto
