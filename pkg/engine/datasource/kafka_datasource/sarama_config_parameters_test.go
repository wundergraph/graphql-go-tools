package kafka_datasource

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/resolve"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/require"
)

// Possible errors with dockertest setup:
//
// Error: API error (404): could not find an available, non-overlapping IPv4 address pool among the defaults to assign to the network
// Solution: docker network prune

const (
	testBrokerAddr    = "localhost:9092"
	testClientID      = "graphql-go-tools-test"
	messageTemplate   = "topic: %s - message: %d"
	testTopic         = "start-consuming-latest-test"
	testConsumerGroup = "start-consuming-latest-cg"
	testSASLUser      = "admin"
	testSASLPassword  = "admin-secret"
	initialBrokerPort = 9092
	maxIdleConsumer   = 10 * time.Second
)

// See the following blogpost to understand how Kafka listeners works:
// https://www.confluent.io/blog/kafka-listeners-explained/

type member struct {
	hostname string
	port     int
}

type kafkaCluster struct {
	pool             *dockertest.Pool
	network          *docker.Network
	saslAuthRequired bool
	members          map[int]member
	//environmentVariables  []string
	environmentVariablesm map[string]string
}

func newKafkaCluster(t *testing.T) *kafkaCluster {
	pool, err := dockertest.NewPool("")
	require.NoError(t, err)

	require.NoError(t, pool.Client.Ping())

	network, err := pool.Client.CreateNetwork(docker.CreateNetworkOptions{Name: "bitnami_kafka_network"})
	require.NoError(t, err)

	return &kafkaCluster{
		pool:    pool,
		network: network,
		members: make(map[int]member),
		environmentVariablesm: map[string]string{
			// KRaft settings
			"KAFKA_CFG_PROCESS_ROLES": "controller,broker",
			"KAFKA_KRAFT_CLUSTER_ID":  "tyk-kafka-test-cluster",
			// Listeners
			"KAFKA_CFG_LISTENER_SECURITY_PROTOCOL_MAP": "PLAINTEXT:PLAINTEXT,CONTROLLER:PLAINTEXT",
			"KAFKA_CFG_CONTROLLER_LISTENER_NAMES":      "CONTROLLER",
			//Clustering
			"KAFKA_CFG_OFFSETS_TOPIC_REPLICATION_FACTOR":         "1",
			"KAFKA_CFG_TRANSACTION_STATE_LOG_REPLICATION_FACTOR": "1",
			"KAFKA_CFG_TRANSACTION_STATE_LOG_MIN_ISR":            "1",
		},
	}
}

func getPortID(port int) string {
	return fmt.Sprintf("%d/tcp", port)
}

func getNodeID(port int) int {
	return port % initialBrokerPort
}

func (k *kafkaCluster) startKafka(t *testing.T, port int) *dockertest.Resource {
	t.Logf("Trying to run Kafka on %d", port)

	// We need a deterministic way to produce broker IDs. Kafka produces random IDs if we don't set
	// deliberately. We need to use the same ID to handle node restarts properly.
	// All port numbers have to be bigger or equal to 9092
	//
	// * If the port number is 9092, nodeID is 0
	// * If the port number is 9094, nodeID is 2
	nodeID := getNodeID(port)
	hostname := fmt.Sprintf("kafka-%d", nodeID)

	k.members[nodeID] = member{
		hostname: hostname,
		port:     port,
	}

	k.environmentVariablesm["KAFKA_CFG_NODE_ID"] = strconv.Itoa(nodeID)
	k.environmentVariablesm["KAFKA_CFG_LISTENERS"] = fmt.Sprintf("PLAINTEXT://:%d,CONTROLLER://:%d", port, port+1)
	k.environmentVariablesm["KAFKA_CFG_ADVERTISED_LISTENERS"] = fmt.Sprintf("PLAINTEXT://localhost:%d", port)

	voters := fmt.Sprintf("%d@%s:%d", nodeID, hostname, port+1)
	for id, clusterMember := range k.members {
		if id == nodeID {
			continue
		}
		voters = fmt.Sprintf("%s,%d@%s:%d", voters, id, clusterMember.hostname, clusterMember.port+1)
	}
	k.environmentVariablesm["KAFKA_CFG_CONTROLLER_QUORUM_VOTERS"] = voters

	var environmentVariables []string
	for key, value := range k.environmentVariablesm {
		environmentVariables = append(environmentVariables, fmt.Sprintf("%s=%s", key, value))
	}

	// Name and Hostname have to be unique
	portID := getPortID(port)
	resource, err := k.pool.RunWithOptions(&dockertest.RunOptions{
		Name:       fmt.Sprintf("kafka-tyk-graphql-%d", port),
		Repository: "bitnami/kafka",
		Tag:        "3.6.0",
		NetworkID:  k.network.ID,
		Hostname:   hostname,
		Env:        environmentVariables,
		PortBindings: map[docker.Port][]docker.PortBinding{
			docker.Port(portID): {{HostIP: "localhost", HostPort: portID}},
		},
		ExposedPorts: []string{portID},
	}, func(config *docker.HostConfig) {
		config.RestartPolicy = docker.RestartOnFailure(10)
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
		if k.saslAuthRequired {
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

func (k *kafkaCluster) start(t *testing.T, numMembers int) map[string]*dockertest.Resource {
	t.Cleanup(func() {
		require.NoError(t, k.pool.Client.RemoveNetwork(k.network.ID))
	})

	resources := make(map[string]*dockertest.Resource)
	var port = initialBrokerPort // Initial port
	for i := 0; i < numMembers; i++ {
		portID := getPortID(port)
		resources[portID] = k.startKafka(t, port)

		// Increase the port numbers. Every member uses different a hostname and port numbers.
		// It was good for debugging:
		//
		// Member 1:
		// 9092 - BROKER
		// 9093 - CONTROLLER
		//
		// Member 2:
		// 9094 - BROKER
		// 9095 - CONTROLLER
		port = port + 2
	}
	require.NotEmpty(t, resources)
	return resources
}

func (k *kafkaCluster) restart(t *testing.T, port int, broker *dockertest.Resource) (*dockertest.Resource, error) {
	if err := broker.Close(); err != nil {
		return nil, err
	}

	delete(k.members, getNodeID(port))

	return k.startKafka(t, port), nil
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
		case <-time.After(maxIdleConsumer):
			require.Failf(t, "all produced messages could not be consumed", "consumer is idle for %s", maxIdleConsumer)
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
	// config.Consumer.Offsets.Initial only takes effect when offsets are not committed to Kafka.
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
	k := newKafkaCluster(t)

	k.saslAuthRequired = true
	k.environmentVariablesm["KAFKA_CFG_SASL_ENABLED_MECHANISMS"] = "PLAIN"
	k.environmentVariablesm["KAFKA_CFG_SASL_MECHANISM_INTER_BROKER_PROTOCOL"] = "PLAIN"
	k.environmentVariablesm["KAFKA_CLIENT_USERS"] = testSASLUser
	k.environmentVariablesm["KAFKA_CLIENT_PASSWORDS"] = testSASLPassword
	k.environmentVariablesm["KAFKA_CFG_LISTENER_SECURITY_PROTOCOL_MAP"] = "PLAINTEXT:SASL_PLAINTEXT,CONTROLLER:PLAINTEXT"

	brokers := k.start(t, 1)
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

		runTest := func() {
			ctx := resolve.NewContext(context.Background())
			defer ctx.Context().Done()

			kc := &KafkaConsumerGroupBridge{
				ctx: ctx.Context(),
				log: logger(),
			}

			sc, err := kc.prepareSaramaConfig(options)
			require.NoError(t, err)

			st := sc.Consumer.Group.Rebalance.Strategy
			require.Equal(t, name, st.Name())
		}
		runTest()
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
			Topics:          []string{testTopic},
			GroupID:         testConsumerGroup,
			ClientID:        testClientID,
			IsolationLevel:  isolationLevel,
		}
		options.Sanitize()
		require.NoError(t, options.Validate())

		runtTest := func() {
			ctx := resolve.NewContext(context.Background())
			defer ctx.Context().Done()

			kc := &KafkaConsumerGroupBridge{
				ctx: ctx.Context(),
				log: logger(),
			}

			sc, err := kc.prepareSaramaConfig(options)
			require.NoError(t, err)
			sc.Consumer.IsolationLevel = value
		}
		runtTest()
	}
}

func TestSarama_Config_SASL_Authentication(t *testing.T) {
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

	ctx := resolve.NewContext(context.Background())
	defer ctx.Context().Done()

	kc := &KafkaConsumerGroupBridge{
		ctx: ctx.Context(),
		log: logger(),
	}

	sc, err := kc.prepareSaramaConfig(options)
	require.NoError(t, err)
	require.True(t, sc.Net.SASL.Enable)
	require.Equal(t, "foobar", sc.Net.SASL.User)
	require.Equal(t, "password", sc.Net.SASL.Password)
}

func TestSarama_Kafka_Cluster(t *testing.T) {
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
	k.startKafka(t, port)

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
