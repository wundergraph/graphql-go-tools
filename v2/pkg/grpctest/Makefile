PHONY: generate-proto
generate-proto:
	protoc --go_out=productv1 --go_opt=paths=source_relative \
    	--go-grpc_out=productv1 --go-grpc_opt=paths=source_relative \
    	product.proto

build-plugin:
	go build -o plugin/plugin_service plugin/plugin_service.go

