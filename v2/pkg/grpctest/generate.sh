protoc --go_out=productv1 --go_opt=paths=source_relative \
    --go-grpc_out=productv1 --go-grpc_opt=paths=source_relative \
    testdata/product.proto
