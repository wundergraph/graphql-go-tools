package kafka_datasource

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
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
// Solution: docker network prune

const (
	testBrokerAddr    = "localhost:9092"
	testClientID      = "graphql-go-tools-test"
	messageTemplate   = "message-%d"
	testTopic         = "start-consuming-latest-test"
	testConsumerGroup = "start-consuming-latest-cg"
	testSASLUser      = "admin"
	testSASLPassword  = "admin-secret"
)

var defaultZooKeeperEnvVars = []string{
	"ALLOW_ANONYMOUS_LOGIN=yes",
}

// See the following blogpost to understand how Kafka listeners works:
// https://www.confluent.io/blog/kafka-listeners-explained/

var defaultKafkaEnvVars = []string{
	"KAFKA_ZOOKEEPER_CONNECT=zookeeper:2181",
	"ALLOW_PLAINTEXT_LISTENER=yes",
	"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=INSIDE:PLAINTEXT,OUTSIDE:PLAINTEXT",
	"KAFKA_INTER_BROKER_LISTENER_NAME=INSIDE",
}

type kafkaCluster struct {
	pool            *dockertest.Pool
	network         *docker.Network
	kafkaRunOptions kafkaClusterOptions
}

func newKafkaCluster(t *testing.T) *kafkaCluster {
	pool, err := dockertest.NewPool("")
	require.NoError(t, err)

	require.NoError(t, pool.Client.Ping())

	network, err := pool.Client.CreateNetwork(docker.CreateNetworkOptions{Name: "zookeeper_kafka_network"})
	require.NoError(t, err)

	return &kafkaCluster{
		pool:    pool,
		network: network,
	}
}

func getPortID(port int) string {
	return fmt.Sprintf("%d/tcp", port)
}

func (k *kafkaCluster) startZooKeeper(t *testing.T) {
	t.Log("Trying to run ZooKeeper")

	resource, err := k.pool.RunWithOptions(&dockertest.RunOptions{
		Name:         "zookeeper-tyk-graphql",
		Repository:   "zookeeper",
		Tag:          "3.8.0",
		NetworkID:    k.network.ID,
		Hostname:     "zookeeper",
		ExposedPorts: []string{"2181"},
		Env:          defaultZooKeeperEnvVars,
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
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

type kafkaClusterOption func(k *kafkaClusterOptions)

type kafkaClusterOptions struct {
	envVars  []string
	saslAuth bool
}

func withKafkaEnvVars(envVars []string) kafkaClusterOption {
	return func(k *kafkaClusterOptions) {
		k.envVars = envVars
	}
}

func withKafkaSASLAuth() kafkaClusterOption {
	return func(k *kafkaClusterOptions) {
		k.saslAuth = true
	}
}

func (k *kafkaCluster) startKafka(t *testing.T, port int, envVars []string) *dockertest.Resource {
	t.Logf("Trying to run Kafka on %d", port)

	internalPort := port + 1
	hostname := fmt.Sprintf("kafka%d", port)

	envVars = append(envVars, fmt.Sprintf("KAFKA_ADVERTISED_LISTENERS=INSIDE://%s:%d,OUTSIDE://localhost:%d", hostname, internalPort, port))
	envVars = append(envVars, fmt.Sprintf("KAFKA_LISTENERS=INSIDE://0.0.0.0:%d,OUTSIDE://0.0.0.0:%d", internalPort, port))

	portID := getPortID(port)

	// Name and Hostname have to be unique
	resource, err := k.pool.RunWithOptions(&dockertest.RunOptions{
		Name:       fmt.Sprintf("kafka-tyk-graphql-%d", port),
		Repository: "bitnami/kafka",
		Tag:        "3.0.1",
		NetworkID:  k.network.ID,
		Hostname:   hostname,
		Env:        envVars,
		PortBindings: map[docker.Port][]docker.PortBinding{
			docker.Port(portID): {{HostIP: "localhost", HostPort: portID}},
		},
		ExposedPorts: []string{portID},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = false
		if k.kafkaRunOptions.saslAuth {
			wd, _ := os.Getwd()
			config.Mounts = []docker.HostMount{{
				Target: "/opt/bitnami/kafka/config/kafka_jaas.conf",
				Source: fmt.Sprintf("%s/testdata/kafka_jaas.conf", wd),
				Type:   "bind",
			}}
		}
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, k.pool.Purge(resource))
	})

	retryFn := func() error {
		config := sarama.NewConfig()
		config.Producer.Return.Successes = true
		config.Producer.Return.Errors = true
		if k.kafkaRunOptions.saslAuth {
			config.Net.SASL.Enable = true
			config.Net.SASL.User = testSASLUser
			config.Net.SASL.Password = testSASLPassword
		}

		brokerAddr := resource.GetHostPort(portID)
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

	t.Logf("Kafka is ready to accept connections on %d", port)
	return resource
}

func (k *kafkaCluster) start(t *testing.T, numMembers int, options ...kafkaClusterOption) map[string]*dockertest.Resource {
	for _, opt := range options {
		opt(&k.kafkaRunOptions)
	}
	if len(k.kafkaRunOptions.envVars) == 0 {
		k.kafkaRunOptions.envVars = defaultKafkaEnvVars
	}

	t.Cleanup(func() {
		require.NoError(t, k.pool.Client.RemoveNetwork(k.network.ID))
	})

	k.startZooKeeper(t)

	resources := make(map[string]*dockertest.Resource)
	var port = 9092
	for i := 0; i < numMembers; i++ {
		var envVars []string
		for _, envVar := range k.kafkaRunOptions.envVars {
			envVars = append(envVars, envVar)
		}
		portID := getPortID(port)
		resources[portID] = k.startKafka(t, port, envVars)
		port = port + 2
	}
	require.NotEmpty(t, resources)
	return resources
}

func testAsyncProducer(t *testing.T, options *GraphQLSubscriptionOptions, start, end int) {
	config := sarama.NewConfig()
	if options.SASL.Enable {
		config.Net.SASL.Enable = true
		config.Net.SASL.User = options.SASL.User
		config.Net.SASL.Password = options.SASL.Password
	}

	asyncProducer, err := sarama.NewAsyncProducer(options.BrokerAddresses, config)
	require.NoError(t, err)

	for i := start; i < end; i++ {
		message := &sarama.ProducerMessage{
			Topic: options.Topic,
			Value: sarama.StringEncoder(fmt.Sprintf(messageTemplate, i)),
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
	if options.SASL.Enable {
		saramaConfig.Net.SASL.Enable = true
		saramaConfig.Net.SASL.User = options.SASL.User
		saramaConfig.Net.SASL.Password = options.SASL.Password
	}
	cg, err := NewKafkaConsumerGroup(logger(), saramaConfig, options)
	require.NoError(t, err)

	messages := make(chan *sarama.ConsumerMessage)
	cg.StartConsuming(messages)

	<-ctx.Done()

	return cg, messages
}

func getBrokerAddresses(brokers map[string]*dockertest.Resource) (brokerAddresses []string) {
	for portID, broker := range brokers {
		brokerAddresses = append(brokerAddresses, broker.GetHostPort(portID))
	}
	return brokerAddresses
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

	k := newKafkaCluster(t)
	brokers := k.start(t, 1)

	const (
		testTopic         = "start-consuming-latest-test"
		testConsumerGroup = "start-consuming-latest-cg"
	)

	options := &GraphQLSubscriptionOptions{
		BrokerAddresses:      getBrokerAddresses(brokers),
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
	testAsyncProducer(t, options, 0, 10)

	consumedMessages, err := testConsumeMessages(messages, 10)
	require.NoError(t, err)
	require.Len(t, consumedMessages, 10)

	// Consumed messages
	// message-1
	// ..
	// message-9
	for i := 0; i < 10; i++ {
		value := fmt.Sprintf(messageTemplate, i)
		require.Contains(t, consumedMessages, value)
	}

	// Stop the first consumer group
	require.NoError(t, cg.Close())

	// Produce more messages
	// message-10
	// ...
	// message-19
	testAsyncProducer(t, options, 10, 20)

	// Start a new consumer with the same consumer group name
	cg, messages = testStartConsumer(t, options)

	// Produce more messages
	// message-20
	// ...
	// message-29
	testAsyncProducer(t, options, 20, 30)

	consumedMessages, err = testConsumeMessages(messages, 10)
	require.NoError(t, err)
	require.Len(t, consumedMessages, 10)

	// Consumed messages
	// message-20
	// ..
	// message-29
	for i := 20; i < 30; i++ {
		value := fmt.Sprintf(messageTemplate, i)
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

	k := newKafkaCluster(t)
	brokers := k.start(t, 3)

	options := &GraphQLSubscriptionOptions{
		BrokerAddresses:      getBrokerAddresses(brokers),
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
	testAsyncProducer(t, options, 0, 10)

	consumedMessages, err := testConsumeMessages(messages, 10)
	require.NoError(t, err)
	require.Len(t, consumedMessages, 10)

	// Consumed messages
	// message-1
	// ..
	// message-9
	for i := 0; i < 10; i++ {
		value := fmt.Sprintf(messageTemplate, i)
		require.Contains(t, consumedMessages, value)
	}

	// Stop the first consumer group
	require.NoError(t, cg.Close())

	// Produce more messages
	// message-10
	// ...
	// message-19
	testAsyncProducer(t, options, 10, 20)

	// Start a new consumer with the same consumer group name
	cg, messages = testStartConsumer(t, options)

	// Produce more messages
	// message-20
	// ...
	// message-29
	testAsyncProducer(t, options, 20, 30)

	consumedMessages, err = testConsumeMessages(messages, 20)
	require.NoError(t, err)
	require.Len(t, consumedMessages, 20)

	// Consumed all remaining messages in the topic
	// message-10
	// ..
	// message-29
	for i := 10; i < 30; i++ {
		value := fmt.Sprintf(messageTemplate, i)
		require.Contains(t, consumedMessages, value)
	}

	// Stop the second consumer group
	require.NoError(t, cg.Close())
}

func TestSarama_ConsumerGroup_SASL_Authentication(t *testing.T) {
	kafkaEnvVars := []string{
		"ALLOW_PLAINTEXT_LISTENER=yes",
		"KAFKA_OPTS=-Djava.security.auth.login.config=/opt/bitnami/kafka/config/kafka_jaas.conf",
		"KAFKA_ZOOKEEPER_CONNECT=zookeeper:2181",
		"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=INSIDE:PLAINTEXT,OUTSIDE:SASL_PLAINTEXT",
		"KAFKA_CFG_SASL_ENABLED_MECHANISMS=PLAIN",
		"KAFKA_CFG_SASL_MECHANISM_INTER_BROKER_PROTOCOL=PLAIN",
		"KAFKA_CFG_INTER_BROKER_LISTENER_NAME=INSIDE",
	}
	k := newKafkaCluster(t)
	brokers := k.start(t, 1, withKafkaEnvVars(kafkaEnvVars), withKafkaSASLAuth())

	options := &GraphQLSubscriptionOptions{
		BrokerAddresses:      getBrokerAddresses(brokers),
		Topic:                testTopic,
		GroupID:              testConsumerGroup,
		ClientID:             "graphql-go-tools-test",
		StartConsumingLatest: false,
		SASL: SASL{
			Enable:   true,
			User:     testSASLUser,
			Password: testSASLPassword,
		},
	}

	cg, messages := testStartConsumer(t, options)

	// Produce messages
	// message-1
	// ...
	// message-9
	testAsyncProducer(t, options, 0, 10)

	consumedMessages, err := testConsumeMessages(messages, 10)
	require.NoError(t, err)
	require.Len(t, consumedMessages, 10)

	// Consume messages
	// message-1
	// ..
	// message-9
	for i := 0; i < 10; i++ {
		value := fmt.Sprintf(messageTemplate, i)
		require.Contains(t, consumedMessages, value)
	}

	require.NoError(t, cg.Close())
}

func TestSarama_Balance_Strategy(t *testing.T) {
	strategies := map[string]string{
		BalanceStrategyRange:      "range",
		BalanceStrategySticky:     "sticky",
		BalanceStrategyRoundRobin: "roundrobin",
		"":                        "range", // Sanitize function will set DefaultBalanceStrategy, it is BalanceStrategyRange.
	}

	for strategy, name := range strategies {
		options := &GraphQLSubscriptionOptions{
			BrokerAddresses: []string{testBrokerAddr},
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
	strategies := map[string]sarama.IsolationLevel{
		IsolationLevelReadUncommitted: sarama.ReadUncommitted,
		IsolationLevelReadCommitted:   sarama.ReadCommitted,
		"":                            sarama.ReadUncommitted, // Sanitize function will set DefaultIsolationLevel, it is sarama.ReadUncommitted.
	}

	for isolationLevel, value := range strategies {
		options := &GraphQLSubscriptionOptions{
			BrokerAddresses: []string{testBrokerAddr},
			Topic:           testTopic,
			GroupID:         testConsumerGroup,
			ClientID:        testClientID,
			IsolationLevel:  isolationLevel,
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

func TestSarama_Config_SASL_Authentication(t *testing.T) {
	options := &GraphQLSubscriptionOptions{
		BrokerAddresses: []string{testBrokerAddr},
		Topic:           testTopic,
		GroupID:         testConsumerGroup,
		ClientID:        testClientID,
		SASL: SASL{
			Enable:   true,
			User:     "foobar",
			Password: "password",
		},
	}
	options.Sanitize()
	require.NoError(t, options.Validate())

	kc := &KafkaConsumerGroupBridge{
		ctx: context.Background(),
		log: logger(),
	}

	sc, err := kc.prepareSaramaConfig(options)
	require.NoError(t, err)
	require.True(t, sc.Net.SASL.Enable)
	require.Equal(t, "foobar", sc.Net.SASL.User)
	require.Equal(t, "password", sc.Net.SASL.Password)
}
