.PHONY: install-protoc
install-protoc:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

PHONY: generate-proto
generate-proto: install-protoc
	protoc --go_out=productv1 --go_opt=paths=source_relative \
    	--go-grpc_out=productv1 --go-grpc_opt=paths=source_relative \
    	product.proto

build-plugin:
	go build -o plugin/plugin_service plugin/plugin_service.go

