# kafka_pubsub

Simple message producer for the Kafka data source implementation. 

## Run Kafka and ZooKeeper with Docker Compose:

Open a terminal run the following:

```
cd examples/kafka_pubsub
docker-compose up
```

You need to wait some time while the cluster is being formed. 

## Building an API to consume messages from Kafka cluster

You can find the full API definition in Tyk format here: `examples/kafka_pubsub/tyk-api-definition.json`. You just need to import this API definition. Here is 
some information about the API definition. 

GraphQL schema:

```graphql
type Product {
  name: String!
  price: Int!
  in_stock: Int!
}

type Query {
    topProducts(first: Int): [Product]
}

type Subscription {
  stock(name: String): Product!
}
```

Query variable:

```json
{
  "name": "product1"
}
```

Body:
```graphql
subscription($name: String) {
  stock(name: $name) {
    name
    price
    inStock
  }
}
```

Sample response:
```json
{
  "data": {
    "stock": {
      "name": "product2",
      "price": 7355,
      "in_stock": 696
    }
  }
}
```

The producer publishes a new message to `test.topic.$product_name` topic every second, and it updates `price` and `in_stock` in every message.

Here is a sample data source configuration. It is a part of `examples/kafka_pubsub/tyk-api-definition.json` file.

```json
 {
  "kind": "Kafka",
  "name": "kafka-consumer-group",
  "internal": false,
  "root_fields": [{
    "type": "Subscription",
    "fields": [
      "stock"
    ]
  }],
  "config": {
    "broker_addr": "localhost:9092",
    "topic": "test.topic.{{.arguments.name}}",
    "group_id": "test.group",
    "client_id": "tyk-kafka-integration-{{.arguments.name}}"
  }
}
```

## Publishing messages

With a properly configured Golang environment:

```
cd examples/kafka_pubsub
go run main.go -p=product1,product2
```

This command will publish messages to `test.topic.product1` and `test.topic.product2` topics every second.

Sample message:
```json
{
	"stock": {
		"name": "product1",
		"price": 803,
		"in_stock": 901
	}
}
```