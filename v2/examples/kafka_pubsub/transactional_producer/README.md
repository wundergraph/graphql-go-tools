# transactional_producer

transactional_producer is a simple test tool to utilize transactional producer API of Apache Kafka.


### Build

This program uses [confluentinc/confluent-kafka-go/kafka](https://github.com/confluentinc/confluent-kafka-go/kafka) which needs
[librdkafka](https://github.com/edenhill/librdkafka). You can install librdkafka via a package manager:

macOS:

```
brew install openssl zstd pkg-config librdkafka
```

Build the tool:

```
export PKG_CONFIG_PATH="/opt/homebrew/opt/librdkafka/lib/pkgconfig:/opt/homebrew/opt/openssl@3/lib/pkgconfig"
go build -tags dynamic main.go
```

### Run

`transactional_producer` produces 10 messages and then quits.

Sample message body:

```json
{
	"stock": {
		"name": "product2",
		"price": 3843,
		"in_stock": 673
	}
}
```

### Setup Kafka Data Source

You can see the full example in [kafka_pubsub/README.md](../README.md).

### Enable transactional producer API

```
./main --product=product2 --enable-transaction
```

You need to add `"isolation_level": "ReadCommitted"` to the Kafka data source config. This prevents reading
dirty writes, and your program will only receive the committed messages.

```json
{
            "kind": "Kafka",
            "name": "kafka-consumer-group",
            "internal": false,
            "root_fields": [
              {
                "type": "Subscription",
                "fields": [
                  "stock"
                ]
              }
            ],
            "config": {
              "broker_addresses": ["localhost:9092"],
              "topic": "test.topic.{{.arguments.name}}",
              "group_id": "test.group",
              "client_id": "kafka-integration-{{.arguments.name}}",
              "isolation_level": "ReadCommitted"
            }
          }
```

#### Abort an initialized transaction

```
./main --product=product2 --enable-transaction --abort-transaction
```

With `--abort-transaction`, the initialized transaction will be aborted. If the isolation level is `ReadCommitted`, you do not receive
messages in the aborted transaction.

If the isolation level is `ReadUncommitted`, you will receive messages in the aborted transaction.

#### Disable transactional API

```
./main --product=product2
```

If you disable the transactional producer API, you will always receive published messages. 
