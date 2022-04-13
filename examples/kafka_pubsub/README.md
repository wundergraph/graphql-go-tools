# kafka_pubsub

Simple message producer for the Kafka data source implementation. 

## Run Kafka and ZooKeeper with Docker Compose:

Open a terminal run the following:

```
cd examples/kafka_pubsub
docker-compose up
```

With a properly configured Golang environment:

```
cd examples/kafka_pubsub
go run main.go -p=product1,product2
```

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

This command publishes messages to `test.topic.product1` and `test.topic.product2` topics. Run the command with `-h` to see help text.

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

Response:
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

Data source configuration
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