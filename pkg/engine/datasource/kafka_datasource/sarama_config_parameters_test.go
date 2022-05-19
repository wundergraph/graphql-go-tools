package kafka_datasource

import (
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

var basicZooKeeperEnvVars = []string{
	"ALLOW_ANONYMOUS_LOGIN=yes",
}

var basicKafkaEnvVars = []string{
	"KAFKA_ZOOKEEPER_CONNECT=zookeeper:2181",
	"ALLOW_PLAINTEXT_LISTENER=yes",
	"KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://localhost:9092",
}

type kafkaBrokerConfig struct {
	zookeeperEnvVars []string
	kafkaEnvVars     []string
}

type kafkaBroker struct {
	config  *kafkaBrokerConfig
	pool    *dockertest.Pool
	network *docker.Network
}

type kafkaBrokerOptions func(*kafkaBrokerConfig)

func zookeeperEnvVars(vars []string) kafkaBrokerOptions {
	return func(config *kafkaBrokerConfig) {
		config.zookeeperEnvVars = vars
	}
}

func kafkaEnvVars(vars []string) kafkaBrokerOptions {
	return func(config *kafkaBrokerConfig) {
		config.zookeeperEnvVars = vars
	}
}

func newKafkaBroker(t *testing.T, options ...kafkaBrokerOptions) *kafkaBroker {
	pool, err := dockertest.NewPool("")
	require.NoError(t, err)

	require.NoError(t, pool.Client.Ping())

	network, err := pool.Client.CreateNetwork(docker.CreateNetworkOptions{Name: "zookeeper_kafka_network"})
	require.NoError(t, err)

	var kc kafkaBrokerConfig
	for _, opt := range options {
		opt(&kc)
	}

	return &kafkaBroker{
		config:  &kc,
		pool:    pool,
		network: network,
	}
}

func (k *kafkaBroker) startZooKeeper(t *testing.T) {
	t.Log("Trying to run ZooKeeper")

	var envVars []string
	for _, cfg := range basicZooKeeperEnvVars {
		envVars = append(envVars, cfg)
	}
	for _, cfg := range k.config.zookeeperEnvVars {
		envVars = append(envVars, cfg)
	}

	resource, err := k.pool.RunWithOptions(&dockertest.RunOptions{
		Name:         "zookeeper-tyk-graphql",
		Repository:   "zookeeper",
		Tag:          "3.8.0",
		NetworkID:    k.network.ID,
		Hostname:     "zookeeper",
		ExposedPorts: []string{"2181"},
		Env:          envVars,
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

	var envVars []string
	for _, cfg := range basicKafkaEnvVars {
		envVars = append(envVars, cfg)
	}
	for _, cfg := range k.config.kafkaEnvVars {
		envVars = append(envVars, cfg)
	}

	resource, err := k.pool.RunWithOptions(&dockertest.RunOptions{
		Name:       "kafka-tyk-graphql",
		Repository: "bitnami/kafka",
		Tag:        "3.0.1",
		NetworkID:  k.network.ID,
		Hostname:   "kafka",
		Env:        envVars,
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

func TestSarama_StartConsumingLatest(t *testing.T) {
	k := newKafkaBroker(t)
	broker := k.start(t)
	fmt.Println(broker.GetHostPort("9092/tcp"))
}
