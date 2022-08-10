package kafka_datasource

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/Shopify/sarama/mocks"
	log "github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const defaultPartition = 0

// newMockKafkaBroker creates a MockBroker to test ConsumerGroups.
func newMockKafkaBroker(t *testing.T, topic, group string, fr *sarama.FetchResponse) *sarama.MockBroker {
	mockBroker := sarama.NewMockBroker(t, 0)

	mockMetadataResponse := sarama.NewMockMetadataResponse(t).
		SetBroker(mockBroker.Addr(), mockBroker.BrokerID()).
		SetLeader(topic, defaultPartition, mockBroker.BrokerID()).
		SetController(mockBroker.BrokerID())

	mockProducerResponse := sarama.NewMockProduceResponse(t).
		SetError(topic, 0, sarama.ErrNoError).
		SetVersion(2)

	mockOffsetResponse := sarama.NewMockOffsetResponse(t).
		SetOffset(topic, defaultPartition, sarama.OffsetOldest, 0).
		SetOffset(topic, defaultPartition, sarama.OffsetNewest, 1).
		SetVersion(1)

	mockCoordinatorResponse := sarama.NewMockFindCoordinatorResponse(t).
		SetCoordinator(sarama.CoordinatorType(0), group, mockBroker)

	mockJoinGroupResponse := sarama.NewMockJoinGroupResponse(t)

	mockSyncGroupResponse := sarama.NewMockSyncGroupResponse(t).
		SetMemberAssignment(&sarama.ConsumerGroupMemberAssignment{
			Version:  0,
			Topics:   map[string][]int32{topic: {0}},
			UserData: nil,
		})

	mockHeartbeatResponse := sarama.NewMockHeartbeatResponse(t)

	mockOffsetFetchResponse := sarama.NewMockOffsetFetchResponse(t).
		SetOffset(group, topic, defaultPartition, 0, "", sarama.KError(0))

	// Need to mock ApiVersionsRequest when we upgrade Sarama

	//mockApiVersionsResponse := sarama.NewMockApiVersionsResponse(t)
	mockOffsetCommitResponse := sarama.NewMockOffsetCommitResponse(t)
	mockBroker.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest":        mockMetadataResponse,
		"ProduceRequest":         mockProducerResponse,
		"OffsetRequest":          mockOffsetResponse,
		"OffsetFetchRequest":     mockOffsetFetchResponse,
		"FetchRequest":           sarama.NewMockSequence(fr),
		"FindCoordinatorRequest": mockCoordinatorResponse,
		"JoinGroupRequest":       mockJoinGroupResponse,
		"SyncGroupRequest":       mockSyncGroupResponse,
		"HeartbeatRequest":       mockHeartbeatResponse,
		//"ApiVersionsRequest":     mockApiVersionsResponse,
		"OffsetCommitRequest": mockOffsetCommitResponse,
	})

	return mockBroker
}

// testConsumerGroupHandler implements sarama.ConsumerGroupHandler interface for testing purposes.
type testConsumerGroupHandler struct {
	processMessage func(msg *sarama.ConsumerMessage)
	ctx            context.Context
	cancel         context.CancelFunc
}

func newDefaultConsumerGroupHandler(processMessage func(msg *sarama.ConsumerMessage)) *testConsumerGroupHandler {
	ctx, cancel := context.WithCancel(context.Background())
	return &testConsumerGroupHandler{
		processMessage: processMessage,
		ctx:            ctx,
		cancel:         cancel,
	}
}

func (d *testConsumerGroupHandler) Setup(_ sarama.ConsumerGroupSession) error {
	d.cancel() // ready for consuming
	return nil
}

func (d *testConsumerGroupHandler) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }
func (d *testConsumerGroupHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		d.processMessage(msg)
		sess.MarkMessage(msg, "") // Commit the message and advance the offset.
	}
	return nil
}

func newTestConsumerGroup(groupID string, brokers []string) (sarama.ConsumerGroup, error) {
	kConfig := mocks.NewTestConfig()
	kConfig.Version = sarama.MaxVersion
	kConfig.Consumer.Return.Errors = true
	kConfig.ClientID = "graphql-go-tools-test"
	kConfig.Consumer.Offsets.Initial = sarama.OffsetNewest

	// Start with a client
	client, err := sarama.NewClient(brokers, kConfig)
	if err != nil {
		return nil, err
	}

	// Create a new consumer group
	return sarama.NewConsumerGroupFromClient(groupID, client)
}

func TestKafkaMockBroker(t *testing.T) {
	var (
		testMessageKey   = sarama.StringEncoder("test.message.key")
		testMessageValue = sarama.StringEncoder("test.message.value")
		topic            = "test.topic"
		consumerGroup    = "consumer.group"
	)

	fr := &sarama.FetchResponse{Version: 11}
	mockBroker := newMockKafkaBroker(t, topic, consumerGroup, fr)
	defer mockBroker.Close()

	brokerAddr := []string{mockBroker.Addr()}

	cg, err := newTestConsumerGroup(consumerGroup, brokerAddr)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, cg.Close())
	}()

	called := 0

	// Stop after 15 seconds and return an error.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	processMessage := func(msg *sarama.ConsumerMessage) {
		defer cancel()

		t.Logf("Processed message topic: %s, key: %s, value: %s, ", msg.Topic, msg.Key, msg.Value)
		key, _ := testMessageKey.Encode()
		value, _ := testMessageValue.Encode()
		require.Equal(t, key, msg.Key)
		require.Equal(t, value, msg.Value)
		require.Equal(t, topic, msg.Topic)
		called++
	}

	handler := newDefaultConsumerGroupHandler(processMessage)

	errCh := make(chan error, 1)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		// Start consuming. Consume is a blocker call and it runs handler.ConsumeClaim at background.
		errCh <- cg.Consume(ctx, []string{topic}, handler)
	}()

	// Ready for consuming
	<-handler.ctx.Done()

	// Add a message to the topic. KafkaConsumerGroupBridge group will fetch that message and trigger ConsumeClaim method.
	fr.AddMessage(topic, defaultPartition, testMessageKey, testMessageValue, 0)

	// When this context is canceled, the processMessage function has been called and run without any problem.
	<-ctx.Done()

	wg.Wait()

	// KafkaConsumerGroupBridge is stopped here.
	require.NoError(t, <-errCh)
	require.Equal(t, 1, called)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
}

// It's just a simple example of graphql federation gateway server, it's NOT a production ready code.
func logger() log.Logger {
	logger, err := zap.NewDevelopmentConfig().Build()
	if err != nil {
		panic(err)
	}

	return log.NewZapLogger(logger, log.DebugLevel)
}

func TestKafkaConsumerGroup_StartConsuming_And_Stop(t *testing.T) {
	var (
		testMessageKey   = sarama.StringEncoder("test.message.key")
		testMessageValue = sarama.StringEncoder("test.message.value")
		topic            = "test.topic"
		consumerGroup    = "consumer.group"
	)

	fr := &sarama.FetchResponse{Version: 11}
	mockBroker := newMockKafkaBroker(t, topic, consumerGroup, fr)
	defer mockBroker.Close()

	// Add a message to the topic. The consumer group will fetch that message and trigger ConsumeClaim method.
	fr.AddMessage(topic, defaultPartition, testMessageKey, testMessageValue, 0)

	options := GraphQLSubscriptionOptions{
		BrokerAddresses: []string{mockBroker.Addr()},
		Topics:          []string{topic},
		GroupID:         consumerGroup,
		ClientID:        "graphql-go-tools-test",
		KafkaVersion:    testMockKafkaVersion,
	}
	options.Sanitize()
	require.NoError(t, options.Validate())

	saramaConfig := sarama.NewConfig()
	saramaConfig.Version = SaramaSupportedKafkaVersions[options.KafkaVersion]

	cg, err := NewKafkaConsumerGroup(logger(), saramaConfig, &options)
	require.NoError(t, err)

	messages := make(chan *sarama.ConsumerMessage)
	cg.StartConsuming(messages)

	msg := <-messages
	expectedKey, _ := testMessageKey.Encode()
	require.Equal(t, expectedKey, msg.Key)

	expectedValue, _ := testMessageValue.Encode()
	require.Equal(t, expectedValue, msg.Value)

	require.NoError(t, cg.Close())

	done := make(chan struct{})
	go func() {
		defer func() {
			close(done)
		}()

		cg.WaitUntilConsumerStop()
	}()

	select {
	case <-time.After(15 * time.Second):
		require.Fail(t, "KafkaConsumerGroup could not closed in 15 seconds")
	case <-done:
	}
}

func TestKafkaConsumerGroup_Config_StartConsumingLatest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consumedMsgCh := make(chan *sarama.ConsumerMessage)
	var mockTopicName = "test.mock.topic"

	// Create a new Kafka consumer handler here. We'll test ConsumeClaim method.
	// If the StartConsumingLatest config option is true, it resets the offset,
	// and we'll observe this behavior.
	kg := &kafkaConsumerGroupHandler{
		ctx:      ctx,
		messages: consumedMsgCh,
		log:      logger(),
		options: &GraphQLSubscriptionOptions{
			StartConsumingLatest: true,
			Topics:               []string{mockTopicName},
			GroupID:              "test.consumer.group",
			ClientID:             "test.client.id",
		},
	}
	session := &mockConsumerGroupSession{
		resetOffsetParams: make(map[string]interface{}),
	}

	claim := &mockConsumerGroupClaim{
		topicName: mockTopicName,
		messages:  make(chan *sarama.ConsumerMessage, 1),
	}
	// Produce a test message.
	claim.messages <- &sarama.ConsumerMessage{
		Topic:     mockTopicName,
		Partition: defaultPartition,
		Key:       []byte("key"),
		Value:     []byte("value"),
	}

	errCh := make(chan error)
	go func() {
		errCh <- kg.ConsumeClaim(session, claim)
	}()

	select {
	case <-consumedMsgCh:
		// Test message has been consumed
	case <-time.After(15 * time.Second):
		require.Fail(t, "the message could not be consumed")
	}

	// This will stop ConsumeClaim method, and it will return with an error or nil.
	close(claim.messages)
	require.NoError(t, <-errCh)

	// If the StartConsumingLatest switch works without any problem, we observe the following changes:

	// sarama.ConsumerGroupSession
	require.Equal(t, mockTopicName, session.resetOffsetParams["topic"])
	require.Equal(t, int32(defaultPartition), session.resetOffsetParams["partition"])
	require.Equal(t, sarama.OffsetNewest, session.resetOffsetParams["offset"])
	require.Equal(t, "", session.resetOffsetParams["metadata"])

	// sarama.ConsumerGroupClaim
	require.False(t, session.markMessageCalled)
}

type mockConsumerGroupSession struct {
	markMessageCalled bool
	resetOffsetParams map[string]interface{}
}

func (m *mockConsumerGroupSession) Claims() map[string][]int32 {
	panic("implement me")
}

func (m *mockConsumerGroupSession) MemberID() string {
	panic("implement me")
}

func (m *mockConsumerGroupSession) GenerationID() int32 {
	panic("implement me")
}

func (m *mockConsumerGroupSession) MarkOffset(topic string, partition int32, offset int64, metadata string) {
	panic("implement me")
}

func (m *mockConsumerGroupSession) Commit() {
	panic("implement me")
}

func (m *mockConsumerGroupSession) ResetOffset(topic string, partition int32, offset int64, metadata string) {
	m.resetOffsetParams["topic"] = topic
	m.resetOffsetParams["partition"] = partition
	m.resetOffsetParams["offset"] = offset
	m.resetOffsetParams["metadata"] = metadata
}

func (m *mockConsumerGroupSession) MarkMessage(msg *sarama.ConsumerMessage, metadata string) {
	m.markMessageCalled = true
}

func (m *mockConsumerGroupSession) Context() context.Context {
	panic("implement me")
}

var _ sarama.ConsumerGroupSession = (*mockConsumerGroupSession)(nil)

type mockConsumerGroupClaim struct {
	topicName string
	messages  chan *sarama.ConsumerMessage
}

func (m *mockConsumerGroupClaim) Topic() string {
	return m.topicName
}

func (m *mockConsumerGroupClaim) Partition() int32 {
	return defaultPartition
}

func (m *mockConsumerGroupClaim) InitialOffset() int64 {
	return 0
}

func (m *mockConsumerGroupClaim) HighWaterMarkOffset() int64 {
	return 0
}

func (m *mockConsumerGroupClaim) Messages() <-chan *sarama.ConsumerMessage {
	return m.messages
}

var _ sarama.ConsumerGroupClaim = (*mockConsumerGroupClaim)(nil)
