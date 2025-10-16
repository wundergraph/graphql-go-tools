mkfile_dir := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))

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
	pnpx wgc@latest grpc-service generate -i testdata/products.graphqls -o testdata/ -p productv1 -g "cosmo/pkg/proto/productv1;productv1" Product
	mv $(mkfile_dir)/testdata/service.proto $(mkfile_dir)/product.proto
	rm -f $(mkfile_dir)/testdata/service.proto.lock.json

.PHONY: generate-mapping-code
generate-mapping-code: regenerate-proto
	go run $(mkfile_dir)/cmd/mapping_helper/main.go $(mkfile_dir)/testdata/mapping.json $(mkfile_dir)/mapping/mapping.go
	go fmt $(mkfile_dir)/mapping/mapping.go
	rm -f $(mkfile_dir)/testdata/mapping.json


.PHONY: regenerate-proto-local
regenerate-proto-local:
	pnpx tsx --env-file ../../../../cosmo/cli/.env ../../../../cosmo/cli/src/index.ts grpc-service generate -i testdata/products.graphqls -o testdata/ -p productv1 -g "cosmo/pkg/proto/productv1;productv1" Product
	mv $(mkfile_dir)/testdata/service.proto $(mkfile_dir)/product.proto
	rm -f $(mkfile_dir)/testdata/service.proto.lock.json

.PHONY: generate-mapping-code-local
generate-mapping-code-local: regenerate-proto-local
	go run $(mkfile_dir)/cmd/mapping_helper/main.go $(mkfile_dir)/testdata/mapping.json $(mkfile_dir)/mapping/mapping.go
	go fmt $(mkfile_dir)/mapping/mapping.go
	rm -f $(mkfile_dir)/testdata/mapping.json


.PHONY: generate-all-local
generate-all-local: generate-mapping-code-local generate-proto

.PHONY: generate-all
generate-all: generate-mapping-code generate-proto