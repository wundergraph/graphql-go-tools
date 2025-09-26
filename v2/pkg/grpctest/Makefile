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

.PHONY: regenerate-proto-local
regenerate-proto-local:
	pnpx tsx --env-file ../../../../cosmo/cli/.env ../../../../cosmo/cli/src/index.ts grpc-service generate -i testdata/products.graphqls -o testdata/ -p productv1 -g "cosmo/pkg/proto/productv1;productv1" Product