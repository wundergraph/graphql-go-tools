package kafka_datasource

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
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
	testBrokerAddr         = "localhost:9092"
	testClientID           = "graphql-go-tools-test"
	messageTemplate        = "topic: %s - message: %d"
	testTopic              = "start-consuming-latest-test"
	testConsumerGroup      = "start-consuming-latest-cg"
	testSASLUser           = "admin"
	testSASLPassword       = "admin-secret"
	initialBrokerPort      = 9092
	maxIdleConsumerSeconds = 10 * time.Second
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
		Name:         "zookeeper-graphql",
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

	// We need a deterministic way to produce broker IDs. Kafka produces random IDs if we don't set
	// deliberately. We need to use the same ID to handle node restarts properly.
	// All port numbers have to be bigger or equal to 9092
	//
	// * If the port number is 9092, brokerID is 0
	// * If the port number is 9094, brokerID is 2
	brokerID := port % initialBrokerPort

	envVars = append(envVars, fmt.Sprintf("KAFKA_CFG_BROKER_ID=%d", brokerID))
	envVars = append(envVars, fmt.Sprintf("KAFKA_ADVERTISED_LISTENERS=INSIDE://%s:%d,OUTSIDE://localhost:%d", hostname, internalPort, port))
	envVars = append(envVars, fmt.Sprintf("KAFKA_LISTENERS=INSIDE://0.0.0.0:%d,OUTSIDE://0.0.0.0:%d", internalPort, port))

	portID := getPortID(port)

	// Name and Hostname have to be unique
	resource, err := k.pool.RunWithOptions(&dockertest.RunOptions{
		Name:       fmt.Sprintf("kafka-graphql-%d", port),
		Repository: "bitnami/kafka",
		Tag:        "3.1",
		NetworkID:  k.network.ID,
		Hostname:   hostname,
		Env:        envVars,
		PortBindings: map[docker.Port][]docker.PortBinding{
			docker.Port(portID): {{HostIP: "localhost", HostPort: portID}},
		},
		ExposedPorts: []string{portID},
	}, func(config *docker.HostConfig) {
		config.RestartPolicy = docker.RestartOnFailure(10)
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
		err := k.pool.Purge(resource)
		if err != nil {
			err = errors.Unwrap(errors.Unwrap(err))
			_, ok := err.(*docker.NoSuchContainer)
			if ok {
				// we closed this resource manually
				err = nil
			}
		}
		require.NoError(t, err)
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
				return fmt.Errorf("tried 100 times but no messages have been produced")
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
	var port = initialBrokerPort // Initial port
	for i := 0; i < numMembers; i++ {
		var envVars []string
		envVars = append(envVars, k.kafkaRunOptions.envVars...)
		portID := getPortID(port)
		resources[portID] = k.startKafka(t, port, envVars)

		// Increase the port numbers. Every member uses different a hostname and port numbers.
		// It was good for debugging:
		//
		// Member 1:
		// 9092 - INSIDE
		// 9093 - OUTSIDE
		//
		// Member 2:
		// 9094 - INSIDE
		// 9095 - OUTSIDE
		port = port + 2
	}
	require.NotEmpty(t, resources)
	return resources
}

func (k *kafkaCluster) restart(t *testing.T, port int, broker *dockertest.Resource, options ...kafkaClusterOption) (*dockertest.Resource, error) {
	if err := broker.Close(); err != nil {
		return nil, err
	}

	for _, opt := range options {
		opt(&k.kafkaRunOptions)
	}
	if len(k.kafkaRunOptions.envVars) == 0 {
		k.kafkaRunOptions.envVars = defaultKafkaEnvVars
	}

	var envVars []string
	envVars = append(envVars, k.kafkaRunOptions.envVars...)
	return k.startKafka(t, port, envVars), nil
}

func (k *kafkaCluster) addNewBroker(t *testing.T, port int, options ...kafkaClusterOption) (*dockertest.Resource, error) {
	for _, opt := range options {
		opt(&k.kafkaRunOptions)
	}
	if len(k.kafkaRunOptions.envVars) == 0 {
		k.kafkaRunOptions.envVars = defaultKafkaEnvVars
	}

	var envVars []string
	envVars = append(envVars, k.kafkaRunOptions.envVars...)
	return k.startKafka(t, port, envVars), nil
}

func produceTestMessages(t *testing.T, options *GraphQLSubscriptionOptions, messages map[string][]string) {
	config := sarama.NewConfig()
	if options.SASL.Enable {
		config.Net.SASL.Enable = true
		config.Net.SASL.User = options.SASL.User
		config.Net.SASL.Password = options.SASL.Password
	}

	asyncProducer, err := sarama.NewAsyncProducer(options.BrokerAddresses, config)
	require.NoError(t, err)

	for _, topic := range options.Topics {
		values, ok := messages[topic]
		if ok {
			for _, value := range values {
				message := &sarama.ProducerMessage{
					Topic: topic,
					Value: sarama.StringEncoder(value),
				}
				asyncProducer.Input() <- message
			}
		}
	}
}

func consumeTestMessages(t *testing.T, messages chan *sarama.ConsumerMessage, producedMessages map[string][]string) {
	var expectedNumMessages int
	for _, values := range producedMessages {
		expectedNumMessages += len(values)
	}

	consumedMessages := make(map[string][]string)
	var numMessages int
L:
	for {
		select {
		case <-time.After(maxIdleConsumerSeconds):
			require.Failf(t, "all produced messages could not be consumed", "consumer is idle for %s", maxIdleConsumerSeconds)
		case msg := <-messages:
			numMessages++
			topic := msg.Topic
			value := string(msg.Value)
			consumedMessages[topic] = append(consumedMessages[topic], value)
			if numMessages >= expectedNumMessages {
				break L
			}
		}
	}

	require.Equal(t, producedMessages, consumedMessages)
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
	saramaConfig.Consumer.Return.Errors = true

	cg, err := NewKafkaConsumerGroup(logger(), saramaConfig, options)
	require.NoError(t, err)

	messages := make(chan *sarama.ConsumerMessage)
	cg.StartConsuming(messages)

	<-ctx.Done()

	return cg, messages
}

func skipWhenShort(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping kafka docker tests in short mode")
	}
}

func getBrokerAddresses(brokers map[string]*dockertest.Resource) (brokerAddresses []string) {
	for portID, broker := range brokers {
		brokerAddresses = append(brokerAddresses, broker.GetHostPort(portID))
	}
	return brokerAddresses
}

func publishMessagesContinuously(t *testing.T, ctx context.Context, options *GraphQLSubscriptionOptions) {
	config := sarama.NewConfig()
	if options.SASL.Enable {
		config.Net.SASL.Enable = true
		config.Net.SASL.User = options.SASL.User
		config.Net.SASL.Password = options.SASL.Password
	}

	asyncProducer, err := sarama.NewAsyncProducer(options.BrokerAddresses, config)
	require.NoError(t, err)

	var i int
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		for _, topic := range options.Topics {
			message := &sarama.ProducerMessage{
				Topic: topic,
				Value: sarama.StringEncoder(fmt.Sprintf(messageTemplate, topic, i)),
			}
			asyncProducer.Input() <- message
		}
		i++
	}
}

func TestSarama_StartConsumingLatest_True(t *testing.T) {
	skipWhenShort(t)

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
		Topics:               []string{testTopic},
		GroupID:              testConsumerGroup,
		ClientID:             "graphql-go-tools-test",
		StartConsumingLatest: true,
	}

	cg, messages := testStartConsumer(t, options)

	// Produce messages
	// message-1
	// message-9
	testMessages := map[string][]string{
		testTopic: {"message-1", "message-2"},
	}
	produceTestMessages(t, options, testMessages)
	consumeTestMessages(t, messages, testMessages)

	// Stop the first consumer group
	require.NoError(t, cg.Close())

	// Produce more messages
	// message-3
	// message-4
	// These messages will be ignored by the consumer.
	testMessages = map[string][]string{
		testTopic: {"message-3", "message-4"},
	}
	produceTestMessages(t, options, testMessages)

	// Start a new consumer with the same consumer group name
	cg, messages = testStartConsumer(t, options)

	// Produce more messages
	// message-5
	// message-6
	testMessages = map[string][]string{
		testTopic: {"message-5", "message-6"},
	}
	produceTestMessages(t, options, testMessages)
	consumeTestMessages(t, messages, testMessages)

	// Stop the second consumer group
	require.NoError(t, cg.Close())
}

func TestSarama_StartConsuming_And_Restart(t *testing.T) {
	skipWhenShort(t)

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
	brokers := k.start(t, 1)

	options := &GraphQLSubscriptionOptions{
		BrokerAddresses:      getBrokerAddresses(brokers),
		Topics:               []string{testTopic},
		GroupID:              testConsumerGroup,
		ClientID:             "graphql-go-tools-test",
		StartConsumingLatest: false,
	}

	cg, messages := testStartConsumer(t, options)

	// Produce messages
	testMessages := map[string][]string{
		testTopic: {"message-1", "message-2"},
	}
	produceTestMessages(t, options, testMessages)
	consumeTestMessages(t, messages, testMessages)

	// Stop the first consumer group
	require.NoError(t, cg.Close())

	// Produce more messages
	testMessages = map[string][]string{
		testTopic: {"message-3", "message-4"},
	}
	produceTestMessages(t, options, testMessages)

	// Start a new consumer with the same consumer group name
	cg, messages = testStartConsumer(t, options)

	// Produce more messages
	testMessages = map[string][]string{
		testTopic: {"message-5", "message-6"},
	}
	produceTestMessages(t, options, testMessages)

	testMessages = map[string][]string{
		testTopic: {"message-3", "message-4", "message-5", "message-6"},
	}
	consumeTestMessages(t, messages, testMessages)

	// Stop the second consumer group
	require.NoError(t, cg.Close())
}

func TestSarama_ConsumerGroup_SASL_Authentication(t *testing.T) {
	skipWhenShort(t)

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
		Topics:               []string{testTopic},
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
	testMessages := map[string][]string{
		testTopic: {"message-1", "message-2"},
	}
	produceTestMessages(t, options, testMessages)
	consumeTestMessages(t, messages, testMessages)

	require.NoError(t, cg.Close())
}

func TestSarama_Balance_Strategy(t *testing.T) {
	skipWhenShort(t)

	strategies := map[string]string{
		BalanceStrategyRange:      "range",
		BalanceStrategySticky:     "sticky",
		BalanceStrategyRoundRobin: "roundrobin",
		"":                        "range", // Sanitize function will set DefaultBalanceStrategy, it is BalanceStrategyRange.
	}

	for strategy, name := range strategies {
		options := &GraphQLSubscriptionOptions{
			BrokerAddresses: []string{testBrokerAddr},
			Topics:          []string{testTopic},
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
	skipWhenShort(t)

	strategies := map[string]sarama.IsolationLevel{
		IsolationLevelReadUncommitted: sarama.ReadUncommitted,
		IsolationLevelReadCommitted:   sarama.ReadCommitted,
		"":                            sarama.ReadUncommitted, // Sanitize function will set DefaultIsolationLevel, it is sarama.ReadUncommitted.
	}

	for isolationLevel, value := range strategies {
		options := &GraphQLSubscriptionOptions{
			BrokerAddresses: []string{testBrokerAddr},
			Topics:          []string{testTopic},
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
	skipWhenShort(t)

	options := &GraphQLSubscriptionOptions{
		BrokerAddresses: []string{testBrokerAddr},
		Topics:          []string{testTopic},
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

func TestSarama_Multiple_Broker(t *testing.T) {
	skipWhenShort(t)

	k := newKafkaCluster(t)
	brokers := k.start(t, 3)

	options := &GraphQLSubscriptionOptions{
		BrokerAddresses:      getBrokerAddresses(brokers),
		Topics:               []string{testTopic},
		GroupID:              testConsumerGroup,
		ClientID:             "graphql-go-tools-test",
		StartConsumingLatest: false,
	}

	cg, messages := testStartConsumer(t, options)

	// Produce messages
	testMessages := map[string][]string{
		testTopic: {"message-1", "message-2"},
	}
	produceTestMessages(t, options, testMessages)
	consumeTestMessages(t, messages, testMessages)

	require.NoError(t, cg.Close())
}

func TestSarama_Cluster_Member_Restart(t *testing.T) {
	skipWhenShort(t)

	k := newKafkaCluster(t)
	brokers := k.start(t, 2)

	options := &GraphQLSubscriptionOptions{
		BrokerAddresses:      getBrokerAddresses(brokers),
		Topics:               []string{testTopic},
		GroupID:              testConsumerGroup,
		ClientID:             "graphql-go-tools-test",
		StartConsumingLatest: false,
	}

	cg, messages := testStartConsumer(t, options)

	// Stop one of the cluster members here.
	// Please take care that we don't update the initial list of broker addresses.
	for portID, broker := range brokers {
		t.Logf(fmt.Sprintf("Restart the member on %s", portID))
		port, err := strconv.Atoi(strings.Trim(portID, "/tcp"))
		require.NoError(t, err)

		newBroker, err := k.restart(t, port, broker)
		require.NoError(t, err)
		brokers[portID] = newBroker
		break
	}

	// Stop publishMessagesContinuously properly. A leaking goroutine
	// may lead to inconsistencies in the other tests.
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	wg.Add(1)
	go func() {
		defer wg.Done()
		publishMessagesContinuously(t, ctx, options)
	}()

L:
	for {
		select {
		case <-time.After(10 * time.Second):
			require.Fail(t, "No message received in 10 seconds")
		case msg, ok := <-messages:
			if !ok {
				require.Fail(t, "messages channel is closed")
			}
			t.Logf("Message received from %s: %v", msg.Topic, string(msg.Value))
			break L
		}
	}

	require.NoError(t, cg.Close())

	// Stop publishMessagesContinuously
	cancel()
	wg.Wait()
}

func TestSarama_Cluster_Add_Member(t *testing.T) {
	skipWhenShort(t)

	k := newKafkaCluster(t)
	brokers := k.start(t, 1)

	options := &GraphQLSubscriptionOptions{
		BrokerAddresses:      getBrokerAddresses(brokers),
		Topics:               []string{testTopic},
		GroupID:              testConsumerGroup,
		ClientID:             "graphql-go-tools-test",
		StartConsumingLatest: false,
	}

	cg, messages := testStartConsumer(t, options)

	// Add a new Kafka node to the cluster
	var ports []int
	for portID := range brokers {
		port, err := strconv.Atoi(strings.Trim(portID, "/tcp"))
		require.NoError(t, err)
		ports = append(ports, port)
	}
	// Find an unoccupied port for the new node.
	sort.Ints(ports)
	// [9092, 9094, 9096]
	port := ports[len(ports)-1] + 2 // A Kafka node uses 2 ports. Increase by 2 to find an unoccupied port.
	_, err := k.addNewBroker(t, port)
	require.NoError(t, err)

	// Stop publishMessagesContinuously properly. A leaking goroutine
	// may lead to inconsistencies in the other tests.
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	wg.Add(1)
	go func() {
		defer wg.Done()
		publishMessagesContinuously(t, ctx, options)
	}()

L:
	for {
		select {
		case <-time.After(10 * time.Second):
			require.Fail(t, "No message received in 10 seconds")
		case msg, ok := <-messages:
			if !ok {
				require.Fail(t, "messages channel is closed")
			}
			t.Logf("Message received from %s: %v", msg.Topic, string(msg.Value))
			break L
		}
	}

	require.NoError(t, cg.Close())

	// Stop publishMessagesContinuously
	cancel()
	wg.Wait()
}

func TestSarama_Subscribe_To_Multiple_Topics(t *testing.T) {
	skipWhenShort(t)

	k := newKafkaCluster(t)
	brokers := k.start(t, 1)

	options := &GraphQLSubscriptionOptions{
		BrokerAddresses:      getBrokerAddresses(brokers),
		Topics:               []string{"test-topic-1", "test-topic-2"},
		GroupID:              testConsumerGroup,
		ClientID:             "graphql-go-tools-test",
		StartConsumingLatest: false,
	}

	cg, messages := testStartConsumer(t, options)

	testMessages := map[string][]string{
		"test-topic-1": {"test-topic-1-message-1", "test-topic-1-message-2"},
		"test-topic-2": {"test-topic-2-message-1", "test-topic-2-message-2"},
	}

	produceTestMessages(t, options, testMessages)

	consumeTestMessages(t, messages, testMessages)
	require.NoError(t, cg.Close())
}
