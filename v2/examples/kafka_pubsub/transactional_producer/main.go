package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

type arguments struct {
	enableTransaction bool
	abortTransaction  bool
	product           string
	broker            string
	help              bool
}

func usage() {
	var msg = `Usage: transactional_producer [options] ...

Simple test tool to utilize transactional producer API

Options:
  -h, --help               Print this message and exit.
  -b  --broker             Apache Kafka broker to connect (default: localhost:9092).
  -p, --product            Comma seperated list of product.
      --enable-transaction Enable transactional producer and commit after producing 10 messages.
      --abort-transaction  Abort the initialized transaction.
`
	_, err := fmt.Fprintf(os.Stdout, msg)
	if err != nil {
		panic(err)
	}
}

type Stock struct {
	Stock Product `json:"stock"`
}

type Product struct {
	Name    string `json:"name"`
	Price   int    `json:"price"`
	InStock int    `json:"in_stock"`
}

func main() {

	args := &arguments{}
	log.SetFlags(0)

	// Parse command line parameters
	f := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.BoolVar(&args.help, "h", false, "")
	f.BoolVar(&args.help, "help", false, "")
	f.BoolVar(&args.enableTransaction, "enable-transaction", false, "")
	f.BoolVar(&args.abortTransaction, "abort-transaction", false, "")
	f.StringVar(&args.product, "p", "", "")
	f.StringVar(&args.product, "product", "", "")
	f.StringVar(&args.broker, "b", "", "")
	f.StringVar(&args.broker, "broker", "", "")

	if err := f.Parse(os.Args[1:]); err != nil {
		log.Fatalf("Failed to parse flags: %v", err)
	}

	if args.help {
		usage()
		return
	}

	if args.product == "" {
		log.Fatalf("product cannot be empty")
	}

	if args.broker == "" {
		args.broker = "localhost:9092"
	}

	if !args.enableTransaction && args.abortTransaction {
		log.Fatalf("invalid configuration: abort-transaction=true")
	}

	rand.Seed(time.Now().UnixNano())

	producerConfig := &kafka.ConfigMap{
		"client.id":         fmt.Sprintf("transactional-producer-%d", rand.Intn(10000)),
		"bootstrap.servers": args.broker,
	}

	if args.enableTransaction {
		producerConfig.SetKey("transactional.id", fmt.Sprintf("transactional-producer-%d", rand.Intn(10000)))
		producerConfig.SetKey("enable.idempotence", true)
	}

	producer, err := kafka.NewProducer(producerConfig)
	if err != nil {
		log.Fatalf("Failed to create a new Producer instance: %s", err)
	}

	defer producer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if args.enableTransaction {
		err = producer.InitTransactions(ctx)
		if err != nil {
			log.Fatalf("Failed to initalize a new transaction: %s", err)
		}

		err = producer.BeginTransaction()
		if err != nil {
			log.Fatalf("Failed to begin a new transaction: %s", err)
		}
		log.Printf("\nTransaction has been initialized\n\n")
	}

	topic := fmt.Sprintf("test.topic.%s", args.product)

	for i := 0; i < 10; i++ {
		stock := Stock{
			Stock: Product{
				Name:    args.product,
				Price:   rand.Intn(10000),
				InStock: rand.Intn(1000),
			},
		}

		data, err := json.Marshal(stock)
		if err != nil {
			log.Fatalf("Failed to encode the message: %s", err)
		}

		log.Printf("Enqueued message to %s: %s", topic, string(data))

		err = producer.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
			Value:          data},
			nil,
		)
		if err != nil {
			log.Fatalf("Failed to produce message: %s", err)
		}
		<-time.After(time.Second)
	}

	if args.enableTransaction {
		if args.abortTransaction {
			err = producer.AbortTransaction(ctx)
			if err != nil {
				log.Fatalf("Failed to abort the transaction: %s", err)
			}
			log.Printf("\nTransaction has been aborted\n")
			return
		}

		err = producer.CommitTransaction(ctx)
		if err != nil {
			log.Fatalf("Failed to commit produced messages: %s", err)
		}
		log.Printf("\nProduced messages have been committed\n")
	}
}
