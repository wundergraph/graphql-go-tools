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
subscription ($name: String) {
    stock(name: $name) {
        name
        price
        in_stock
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


```json
 {
  "kind": "Kafka",
  "name": "kafka-consumer-group",
  "root_fields": [{
    "type": "Subscription",
    "fields": [
      "stock"
    ]
  }],
  "config": {
    "broker_addresses": ["localhost:9092"],
    "topics": ["test.topic.{{.arguments.name}}"],
    "group_id": "test.group",
    "client_id": "kafka-integration-{{.arguments.name}}"
  }
}
```

Another part of the configuration is under `graphql.engine.field_config`. It's an array of objects. 

```json
"field_configs": [
    {
      "type_name": "Subscription",
      "field_name": "stock",
      "disable_default_mapping": false,
      "path": [
        "stock"
      ]
    }
]
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

## SASL (Simple Authentication and Security Layer) Support

Kafka data source supports SASL in plain mode.

Run Kafka with the correct configuration:

```
docker-compose up kafka-sasl
```

With a properly configured Golang environment:

```
cd examples/kafka_pubsub
go run main.go -p=product1,product2 --enable-sasl --sasl-user=admin --sasl-password=admin-secret
```

`--enable-sasl` parameter enables SASL support on the client side. 

On the API definition side,

```json
{
  "broker_addresses": ["localhost:9092"],
  "topics": ["test.topic.product2"],
  "group_id": "test.group",
  "client_id": "kafka-integration-{{.arguments.name}}",
  "sasl": {
    "enable": true,
    "user": "admin",
    "password": "admin-secret"
  }
}
```
If SASL enabled and `user` is an empty string, gateway returns: 

```json
{
  "message": "sasl.user cannot be empty"
}
```

If SASL enabled and `password` is an empty string, gateway returns:

```json
{
  "message": "sasl.password cannot be empty"
}
```

If password/user is wrong:

```json
{
  "message": "kafka: client has run out of available brokers to talk to (Is your cluster reachable?)"
}
```

## Creating an Apache Kafka cluster

Simply run the following command to create an Apache Kafka cluster with 3 nodes:

```
docker-compose --file docker-compose-cluster.yml up
```

Cluster members:

* localhost:9092
* localhost:9093
* localhost:9094

**Important Note**: `kafka-topics` command is a part of Apache Kafka installation. You can choose to install Apache Kafka on your system or
execute it in the container.

### Creating a topic with a replication factor

```
kafka-topics --create --bootstrap-server localhost:9092 --topic test.topic.product1 --partitions 3 --replication-factor 3
```

This command creates `test.topic.product1` on the Kafka cluster. It spans over 3 partitions and has 3 replicas.

You can use `describe` command to inspect the topic:

```
kafka-topics --describe --bootstrap-server localhost:9092 --topic test.topic.product1
```

Sample result:

```
Topic: test.topic.product1	TopicId: MNfDKrvQQV6WZM2SQjI0og	PartitionCount: 3	ReplicationFactor: 3	Configs: segment.bytes=1073741824
	Topic: test.topic.product1	Partition: 0	Leader: 2	Replicas: 2,0,1	Isr: 2,0,1
	Topic: test.topic.product1	Partition: 1	Leader: 1	Replicas: 1,2,0	Isr: 1,2,0
	Topic: test.topic.product1	Partition: 2	Leader: 0	Replicas: 0,1,2	Isr: 0,1,2
```

### Deleting a topic

If you want to delete a topic and drop all messages, you can run the following command:

```
kafka-topics --describe --bootstrap-server localhost:9092 --topic test.topic.product1
```

### Publishing messages with multiple broker addresses

```
go run main.go --brokers=localhost:9092,localhost:9093,localhost:9094 --products=product1
```

Sample result:

```
Enqueued message to test.topic.product1: {"stock":{"name":"product1","price":8162,"in_stock":89}}
Enqueued message to test.topic.product1: {"stock":{"name":"product1","price":8287,"in_stock":888}}
```
