# Federation example

This is a very basic example of the federation.
It is not meant to be used in production.

If you're looking for a complete ready-to-use Open Source Router for Federation,
have a look at the [Cosmo Router](https://github.com/wundergraph/cosmo) which is based on this engine library.

This example includes three services:
- `accounts`
- `products`
- `reviews`

Services defines a few queries and subscriptions.

## Getting started
1. Install go modules
```shell
go mod download
```
2. Run start script
```
chmod +x start.sh
./start.sh
```
3. To change subgraphs
- edit corresponding `<service>/graph/schema.graphqls` file
- run `go generate ./...`
- compose a new config via `compose.sh`