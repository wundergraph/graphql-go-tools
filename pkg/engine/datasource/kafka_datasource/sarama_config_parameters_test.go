package kafka_datasource

import (
	"context"
	"errors"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/go-zookeeper/zk"
	"github.com/ory/dockertest"
	"github.com/ory/dockertest/docker"
	"github.com/stretchr/testify/require"
)

// Possible errors with dockertest setup:
//
// Error: API error (404): could not find an available, non-overlapping IPv4 address pool among the defaults to assign to the network
// Solution: docker prune network

var basicZooKeeperEnvVars = []string{
	"ALLOW_ANONYMOUS_LOGIN=yes",
}

var basicKafkaEnvVars = []string{
	"KAFKA_ZOOKEEPER_CONNECT=zookeeper:2181",
	"ALLOW_PLAINTEXT_LISTENER=yes",
	"KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://localhost:9092",
}

type kafkaBroker struct {
	pool    *dockertest.Pool
	network *docker.Network
}

func newKafkaBroker(t *testing.T) *kafkaBroker {
	pool, err := dockertest.NewPool("")
	require.NoError(t, err)

	require.NoError(t, pool.Client.Ping())

	network, err := pool.Client.CreateNetwork(docker.CreateNetworkOptions{Name: "zookeeper_kafka_network"})
	require.NoError(t, err)

	return &kafkaBroker{
		pool:    pool,
		network: network,
	}
}

func (k *kafkaBroker) startZooKeeper(t *testing.T) {
	t.Log("Trying to run ZooKeeper")

	resource, err := k.pool.RunWithOptions(&dockertest.RunOptions{
		Name:         "zookeeper-tyk-graphql",
		Repository:   "zookeeper",
		Tag:          "3.8.0",
		NetworkID:    k.network.ID,
		Hostname:     "zookeeper",
		ExposedPorts: []string{"2181"},
		Env:          basicZooKeeperEnvVars,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		if err = k.pool.Purge(resource); err != nil {
			require.NoError(t, err)
		}
	})

	conn, _, err := zk.Connect([]string{fmt.Sprintf("127.0.0.1:%s", resource.GetPort("2181/tcp"))}, 10*time.Second)
	require.NoError(t, err)

	defer conn.Close()

	retryFn := func() error {
		switch conn.State() {
		case zk.StateHasSession, zk.StateConnected:
			return nil
		default:
			return errors.New("not yet connected")
		}
	}

	require.NoError(t, k.pool.Retry(retryFn))
	t.Log("ZooKeeper has been started")
}

func (k *kafkaBroker) startKafka(t *testing.T) *dockertest.Resource {
	t.Log("Trying to run Kafka")

	resource, err := k.pool.RunWithOptions(&dockertest.RunOptions{
		Name:       "kafka-tyk-graphql",
		Repository: "bitnami/kafka",
		Tag:        "3.0.1",
		NetworkID:  k.network.ID,
		Hostname:   "kafka",
		Env:        basicKafkaEnvVars,
		PortBindings: map[docker.Port][]docker.PortBinding{
			"9092/tcp": {{HostIP: "localhost", HostPort: "9092/tcp"}},
		},
		ExposedPorts: []string{"9092/tcp"},
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, k.pool.Purge(resource))
	})

	retryFn := func() error {
		config := sarama.NewConfig()
		config.Producer.Return.Successes = true
		config.Producer.Return.Errors = true
		brokerAddr := fmt.Sprintf("localhost:%s", resource.GetPort("9092/tcp"))
		asyncProducer, err := sarama.NewAsyncProducer([]string{brokerAddr}, config)
		if err != nil {
			return err
		}
		defer asyncProducer.Close()

		var total int
	loop:
		for {
			total++
			if total > 100 {
				break
			}
			message := &sarama.ProducerMessage{
				Topic: "grahpql-go-tools-health-check",
				Value: sarama.StringEncoder("hello, world!"),
			}

			asyncProducer.Input() <- message

			select {
			case <-asyncProducer.Errors():
				// We should try again
				//
				// Possible error msg: kafka: Failed to produce message to topic grahpql-go-tools-health-check:
				// kafka server: In the middle of a leadership election, there is currently no leader for this
				// partition and hence it is unavailable for writes.
				continue loop
			case <-time.After(time.Second):
				continue loop
			case <-asyncProducer.Successes():
				break loop
			}

		}
		return nil
	}

	if err = k.pool.Retry(retryFn); err != nil {
		log.Fatalf("could not connect to kafka: %s", err)
	}
	require.NoError(t, k.pool.Retry(retryFn))

	t.Log("Kafka is ready to accept connections")
	return resource
}

func (k *kafkaBroker) start(t *testing.T) *dockertest.Resource {
	t.Cleanup(func() {
		require.NoError(t, k.pool.Client.RemoveNetwork(k.network.ID))
	})
	k.startZooKeeper(t)
	return k.startKafka(t)
}

func testAsyncProducer(t *testing.T, addr, topic string, start, end int) {
	config := sarama.NewConfig()
	asyncProducer, err := sarama.NewAsyncProducer([]string{addr}, config)
	require.NoError(t, err)

	for i := start; i < end; i++ {
		message := &sarama.ProducerMessage{
			Topic: topic,
			Value: sarama.StringEncoder(fmt.Sprintf("message-%d", i)),
		}
		asyncProducer.Input() <- message
	}
	require.NoError(t, asyncProducer.Close())
}

func testConsumeMessages(messages chan *sarama.ConsumerMessage, numberOfMessages int) (map[string]struct{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	allMessages := make(map[string]struct{})
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout exceeded")
		case msg := <-messages:
			value := string(msg.Value)
			allMessages[value] = struct{}{}
			if len(allMessages) >= numberOfMessages {
				return allMessages, nil
			}
		}
	}
}

func testStartConsumer(t *testing.T, options *GraphQLSubscriptionOptions) (*KafkaConsumerGroup, chan *sarama.ConsumerMessage) {
	ctx, cancel := context.WithCancel(context.Background())
	options.startedCallback = func() {
		cancel()
	}

	options.Sanitize()
	require.NoError(t, options.Validate())

	// Start a consumer
	saramaConfig := sarama.NewConfig()
	cg, err := NewKafkaConsumerGroup(logger(), saramaConfig, options)
	require.NoError(t, err)

	messages := make(chan *sarama.ConsumerMessage)
	cg.StartConsuming(messages)

	<-ctx.Done()

	return cg, messages
}

func TestSarama_StartConsumingLatest_True(t *testing.T) {
	// Test scenario:
	//
	// 1- Start a new consumer
	// 2- Produce 10 messages
	// 3- The consumer consumes the produced messages
	// 4- Stop the consumer
	// 5- Produce more messages
	// 6- Start a new consumer with the same consumer group name
	// 7- Produce more messages
	// 8- Consumer will consume the messages produced on step 7.

	// Important note about offset management in Kafka:
	//
	// config.Consumer.Offsets.Initial only takes effect when offsets are not committed to Kafka/Zookeeper.
	// If the consumer group already has offsets committed, the consumer will resume from the committed offset.

	k := newKafkaBroker(t)
	broker := k.start(t)

	const (
		testTopic         = "start-consuming-latest-test"
		testConsumerGroup = "start-consuming-latest-cg"
	)
	brokerAddr := broker.GetHostPort("9092/tcp")

	options := &GraphQLSubscriptionOptions{
		BrokerAddr:           brokerAddr,
		Topic:                testTopic,
		GroupID:              testConsumerGroup,
		ClientID:             "graphql-go-tools-test",
		StartConsumingLatest: true,
	}

	cg, messages := testStartConsumer(t, options)

	// Produce messages
	// message-1
	// ...
	// message-9
	testAsyncProducer(t, brokerAddr, testTopic, 0, 10)

	consumedMessages, err := testConsumeMessages(messages, 10)
	require.NoError(t, err)
	require.Len(t, consumedMessages, 10)

	// Consumed messages
	// message-1
	// ..
	// message-9
	for i := 0; i < 10; i++ {
		value := fmt.Sprintf("message-%d", i)
		require.Contains(t, consumedMessages, value)
	}

	// Stop the first consumer group
	require.NoError(t, cg.Close())

	// Produce more messages
	// message-10
	// ...
	// message-19
	testAsyncProducer(t, brokerAddr, testTopic, 10, 20)

	// Start a new consumer with the same consumer group name
	cg, messages = testStartConsumer(t, options)

	// Produce more messages
	// message-20
	// ...
	// message-29
	testAsyncProducer(t, brokerAddr, testTopic, 20, 30)

	consumedMessages, err = testConsumeMessages(messages, 10)
	require.NoError(t, err)
	require.Len(t, consumedMessages, 10)

	// Consumed messages
	// message-20
	// ..
	// message-29
	for i := 20; i < 30; i++ {
		value := fmt.Sprintf("message-%d", i)
		require.Contains(t, consumedMessages, value)
	}

	// Stop the second consumer group
	require.NoError(t, cg.Close())
}

func TestSarama_StartConsuming_And_Restart(t *testing.T) {
	// Test scenario:
	//
	// 1- Start a new consumer
	// 2- Produce 10 messages
	// 3- The consumer consumes the produced messages
	// 4- Stop the consumer
	// 5- Produce more messages
	// 6- Start a new consumer with the same consumer group name
	// 7- Produce more messages
	// 8- Consumer will consume all messages.

	k := newKafkaBroker(t)
	broker := k.start(t)

	const (
		testTopic         = "start-consuming-latest-test"
		testConsumerGroup = "start-consuming-latest-cg"
	)
	brokerAddr := broker.GetHostPort("9092/tcp")

	options := &GraphQLSubscriptionOptions{
		BrokerAddr:           brokerAddr,
		Topic:                testTopic,
		GroupID:              testConsumerGroup,
		ClientID:             "graphql-go-tools-test",
		StartConsumingLatest: false,
	}

	cg, messages := testStartConsumer(t, options)

	// Produce messages
	// message-1
	// ...
	// message-9
	testAsyncProducer(t, brokerAddr, testTopic, 0, 10)

	consumedMessages, err := testConsumeMessages(messages, 10)
	require.NoError(t, err)
	require.Len(t, consumedMessages, 10)

	// Consumed messages
	// message-1
	// ..
	// message-9
	for i := 0; i < 10; i++ {
		value := fmt.Sprintf("message-%d", i)
		require.Contains(t, consumedMessages, value)
	}

	// Stop the first consumer group
	require.NoError(t, cg.Close())

	// Produce more messages
	// message-10
	// ...
	// message-19
	testAsyncProducer(t, brokerAddr, testTopic, 10, 20)

	// Start a new consumer with the same consumer group name
	cg, messages = testStartConsumer(t, options)

	// Produce more messages
	// message-20
	// ...
	// message-29
	testAsyncProducer(t, brokerAddr, testTopic, 20, 30)

	consumedMessages, err = testConsumeMessages(messages, 20)
	require.NoError(t, err)
	require.Len(t, consumedMessages, 20)

	// Consumed all remaining messages in the topic
	// message-10
	// ..
	// message-29
	for i := 10; i < 30; i++ {
		value := fmt.Sprintf("message-%d", i)
		require.Contains(t, consumedMessages, value)
	}

	// Stop the second consumer group
	require.NoError(t, cg.Close())
}

func TestSarama_Balance_Strategy(t *testing.T) {
	const (
		testBrokerAddr    = "localhost:9092"
		testTopic         = "start-consuming-latest-test"
		testConsumerGroup = "start-consuming-latest-cg"
		testClientID      = "graphql-go-tools-test"
	)

	strategies := map[string]string{
		BalanceStrategyRange:      "range",
		BalanceStrategySticky:     "sticky",
		BalanceStrategyRoundRobin: "roundrobin",
		"":                        "range", // Sanitize function will set DefaultBalanceStrategy, it is BalanceStrategyRange.
	}

	for strategy, name := range strategies {
		options := &GraphQLSubscriptionOptions{
			BrokerAddr:      testBrokerAddr,
			Topic:           testTopic,
			GroupID:         testConsumerGroup,
			ClientID:        testClientID,
			BalanceStrategy: strategy,
		}
		options.Sanitize()
		require.NoError(t, options.Validate())

		kc := &KafkaConsumerGroupBridge{
			ctx: context.Background(),
			log: logger(),
		}

		sc, err := kc.prepareSaramaConfig(options)
		require.NoError(t, err)

		st := sc.Consumer.Group.Rebalance.Strategy
		require.Equal(t, name, st.Name())
	}
}

func TestSarama_Isolation_Level(t *testing.T) {
	const (
		testBrokerAddr    = "localhost:9092"
		testTopic         = "start-consuming-latest-test"
		testConsumerGroup = "start-consuming-latest-cg"
		testClientID      = "graphql-go-tools-test"
	)

	strategies := map[string]sarama.IsolationLevel{
		IsolationLevelReadUncommitted: sarama.ReadUncommitted,
		IsolationLevelReadCommitted:   sarama.ReadCommitted,
		"":                            sarama.ReadUncommitted, // Sanitize function will set DefaultIsolationLevel, it is sarama.ReadUncommitted.
	}

	for isolationLevel, value := range strategies {
		options := &GraphQLSubscriptionOptions{
			BrokerAddr:     testBrokerAddr,
			Topic:          testTopic,
			GroupID:        testConsumerGroup,
			ClientID:       testClientID,
			IsolationLevel: isolationLevel,
		}
		options.Sanitize()
		require.NoError(t, options.Validate())

		kc := &KafkaConsumerGroupBridge{
			ctx: context.Background(),
			log: logger(),
		}

		sc, err := kc.prepareSaramaConfig(options)
		require.NoError(t, err)

		sc.Consumer.IsolationLevel = value
	}
}
