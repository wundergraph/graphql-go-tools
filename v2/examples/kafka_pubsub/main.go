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
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/Shopify/sarama"
)

var wg sync.WaitGroup

type arguments struct {
	products     string
	brokers      string
	help         bool
	enableSASL   bool
	saslUser     string
	saslPassword string
}

func usage() {
	var msg = `Usage: kafka_pubsub [options] ...

Simple test tool to generate test data

Options:
  -h, --help          Print this message and exit.
  -b  --brokers       Comma seperated list of broker addresses (default: localhost:9092).
  -p, --products      Comma seperated list of products.
      --enable-sasl   Enable Simple Authentication and Security Layer (SASL)
      --sasl-user     User for SASL
      --sasl-password Password for SASL
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

func produceMessages(ctx context.Context, product string, producer sarama.AsyncProducer) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
			stock := Stock{
				Stock: Product{
					Name:    product,
					Price:   rand.Intn(10000),
					InStock: rand.Intn(1000),
				},
			}

			data, err := json.Marshal(stock)
			if err != nil {
				log.Printf("failed to marshal Stock: %v", err)
				continue
			}

			topic := fmt.Sprintf("test.topic.%s", product)
			message := &sarama.ProducerMessage{
				Topic: topic,
				Value: sarama.StringEncoder(data),
			}
			producer.Input() <- message

			select {
			case <-producer.Successes():
				log.Printf("Enqueued message to %s: %s", topic, string(data))
			case errMsg := <-producer.Errors():
				log.Printf("Failed to enqueue message: %s, error: %s", string(data), errMsg.Error())
			}
		}
	}
}

func main() {
	args := &arguments{}
	log.SetFlags(0)

	// Parse command line parameters
	f := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.BoolVar(&args.help, "h", false, "")
	f.BoolVar(&args.help, "help", false, "")
	f.StringVar(&args.products, "p", "", "")
	f.StringVar(&args.products, "products", "", "")
	f.StringVar(&args.brokers, "b", "", "")
	f.StringVar(&args.brokers, "brokers", "", "")
	f.BoolVar(&args.enableSASL, "enable-sasl", false, "")
	f.StringVar(&args.saslUser, "sasl-user", "", "")
	f.StringVar(&args.saslPassword, "sasl-password", "", "")

	if err := f.Parse(os.Args[1:]); err != nil {
		log.Fatalf("Failed to parse flags: %v", err)
	}

	if args.help {
		usage()
		return
	}

	if args.brokers == "" {
		args.brokers = "localhost:9092"
	}

	if args.products == "" {
		log.Fatal("no products given")
	}

	var products []string
	products = strings.Split(args.products, ",")

	config := sarama.NewConfig()
	config.Producer.Return.Errors = true
	config.Producer.Return.Successes = true
	if args.enableSASL {
		if args.saslUser == "" {
			log.Fatalf("User cannot be empty")
		}
		if args.saslPassword == "" {
			log.Fatalf("Password cannot be empty")
		}
		config.Net.SASL.Enable = true
		config.Net.SASL.User = args.saslUser
		config.Net.SASL.Password = args.saslPassword
	}

	brokers := strings.Split(args.brokers, ",")
	asyncProducer, err := sarama.NewAsyncProducer(brokers, config)
	if err != nil {
		log.Fatalf("failed to create a new AsyncProducer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	wg.Add(1)
	go func() {
		defer wg.Done()

		<-signals
		cancel()
	}()

	for _, product := range products {
		product = strings.TrimSpace(product)

		wg.Add(1)
		go produceMessages(ctx, product, asyncProducer)
	}

	<-ctx.Done()
	asyncProducer.AsyncClose()

	wg.Wait()
	log.Println("Quit!")
}
