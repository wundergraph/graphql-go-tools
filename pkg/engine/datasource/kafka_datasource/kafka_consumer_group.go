package kafka_datasource

import (
	"context"
	"sync"
	"time"

	"github.com/Shopify/sarama"
	"github.com/buger/jsonparser"
	log "github.com/jensneuse/abstractlogger"
)

const consumerGroupRetryInterval = time.Second

type KafkaConsumerGroupBridge struct {
	log log.Logger
	ctx context.Context
}

type KafkaConsumerGroup struct {
	consumerGroup   sarama.ConsumerGroup
	options         *GraphQLSubscriptionOptions
	log             log.Logger
	startedCallback func()
	wg              sync.WaitGroup
	ctx             context.Context
	cancel          context.CancelFunc
}

type kafkaConsumerGroupHandler struct {
	log             log.Logger
	startedCallback func()
	options         *GraphQLSubscriptionOptions
	messages        chan *sarama.ConsumerMessage
	ctx             context.Context
}

// Setup is run at the beginning of a new session, before ConsumeClaim.
func (k *kafkaConsumerGroupHandler) Setup(_ sarama.ConsumerGroupSession) error {
	k.log.Debug("kafkaConsumerGroupHandler.Setup",
		log.Strings("topics", k.options.Topics),
		log.String("groupID", k.options.GroupID),
		log.String("clientID", k.options.ClientID),
	)
	return nil
}

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited
// but before the offsets are committed for the very last time.
func (k *kafkaConsumerGroupHandler) Cleanup(_ sarama.ConsumerGroupSession) error {
	k.log.Debug("kafkaConsumerGroupHandler.Cleanup",
		log.Strings("topics", k.options.Topics),
		log.String("groupID", k.options.GroupID),
		log.String("clientID", k.options.ClientID),
	)
	return nil
}

// ConsumeClaim must start a consumer loop of ConsumerGroupClaim's Messages().
// Once the Messages() channel is closed, the Handler must finish its processing
// loop and exit.
func (k *kafkaConsumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	if k.options.StartConsumingLatest {
		// Reset the offset before start consuming and don't commit the consumed messages.
		// In this way, it will only read the latest messages.
		session.ResetOffset(claim.Topic(), claim.Partition(), sarama.OffsetNewest, "")
	}

	if k.startedCallback != nil {
		k.startedCallback()
	}

	for msg := range claim.Messages() {
		ctx, cancel := context.WithTimeout(k.ctx, time.Second*5)
		select {
		case k.messages <- msg:
			cancel()
			// If the client wants to most recent messages, don't commit the
			// offset and reset the offset to sarama.OffsetNewest, then start consuming.
			if !k.options.StartConsumingLatest {
				session.MarkMessage(msg, "") // Commit the message and advance the offset.
			}
		case <-ctx.Done():
			cancel()
			return nil
		}
	}
	k.log.Debug("kafkaConsumerGroupHandler.ConsumeClaim is gone",
		log.Strings("topics", k.options.Topics),
		log.String("groupID", k.options.GroupID),
		log.String("clientID", k.options.ClientID))
	return nil
}

// NewKafkaConsumerGroup creates a new sarama.ConsumerGroup and returns a new
// *KafkaConsumerGroup instance.
func NewKafkaConsumerGroup(log log.Logger, saramaConfig *sarama.Config, options *GraphQLSubscriptionOptions) (*KafkaConsumerGroup, error) {
	cg, err := sarama.NewConsumerGroup(options.BrokerAddresses, options.GroupID, saramaConfig)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &KafkaConsumerGroup{
		consumerGroup:   cg,
		startedCallback: options.startedCallback,
		log:             log,
		options:         options,
		ctx:             ctx,
		cancel:          cancel,
	}, nil
}

func (k *KafkaConsumerGroup) startConsuming(handler sarama.ConsumerGroupHandler) {
	defer k.wg.Done()

	defer func() {
		if err := k.consumerGroup.Close(); err != nil {
			k.log.Error("KafkaConsumerGroup.Close returned an error",
				log.Strings("topics", k.options.Topics),
				log.String("groupID", k.options.GroupID),
				log.String("clientID", k.options.ClientID),
				log.Error(err))
		}
	}()

	k.wg.Add(1)
	go func() {
		defer k.wg.Done()

		// Errors returns a read channel of errors that occurred during the consumer life-cycle.
		// By default, errors are logged and not returned over this channel.
		// If you want to implement any custom error handling, set your config's
		// Consumer.Return.Errors setting to true, and read from this channel.
		for err := range k.consumerGroup.Errors() {
			k.log.Error("KafkaConsumerGroup.Consumer",
				log.Strings("topics", k.options.Topics),
				log.String("groupID", k.options.GroupID),
				log.String("clientID", k.options.ClientID),
				log.Error(err))
		}
	}()

	// From Sarama documents:
	//
	// This method should be called inside an infinite loop, when a
	// server-side rebalance happens, the consumer session will need to be
	// recreated to get the new claims.
	for {
		select {
		case <-k.ctx.Done():
			return
		default:
		}

		k.log.Info("KafkaConsumerGroup.consumerGroup.Consume has been called",
			log.Strings("topics", k.options.Topics),
			log.String("groupID", k.options.GroupID),
			log.String("clientID", k.options.ClientID))

		// Blocking call
		err := k.consumerGroup.Consume(k.ctx, k.options.Topics, handler)
		if err != nil {
			k.log.Error("KafkaConsumerGroup.startConsuming",
				log.Strings("topics", k.options.Topics),
				log.String("groupID", k.options.GroupID),
				log.String("clientID", k.options.ClientID),
				log.Error(err))
		}
		// Rebalance or node restart takes time. Every Consume call
		// triggers a context switch on the CPU. We should prevent an
		// interrupt storm.
		<-time.After(consumerGroupRetryInterval)
	}
}

// StartConsuming initializes a new consumer group handler and starts consuming at
// background.
func (k *KafkaConsumerGroup) StartConsuming(messages chan *sarama.ConsumerMessage) {
	handler := &kafkaConsumerGroupHandler{
		log:             k.log,
		startedCallback: k.options.startedCallback,
		options:         k.options,
		messages:        messages,
		ctx:             k.ctx,
	}

	k.wg.Add(1)
	go k.startConsuming(handler)
}

// Close stops background goroutines and closes the underlying ConsumerGroup instance.
func (k *KafkaConsumerGroup) Close() error {
	select {
	case <-k.ctx.Done():
		// Already closed
		return nil
	default:
	}

	k.cancel()
	return k.consumerGroup.Close()
}

// WaitUntilConsumerStop waits until ConsumerGroup.Consume function stops.
func (k *KafkaConsumerGroup) WaitUntilConsumerStop() {
	k.wg.Wait()
}

func NewKafkaConsumerGroupBridge(ctx context.Context, logger log.Logger) *KafkaConsumerGroupBridge {
	if logger == nil {
		logger = log.NoopLogger
	}
	return &KafkaConsumerGroupBridge{
		ctx: ctx,
		log: logger,
	}
}

func (c *KafkaConsumerGroupBridge) prepareSaramaConfig(options *GraphQLSubscriptionOptions) (*sarama.Config, error) {
	sc := sarama.NewConfig()
	sc.Version = SaramaSupportedKafkaVersions[options.KafkaVersion]
	sc.ClientID = options.ClientID
	sc.Consumer.Return.Errors = true

	// Strategy for allocating topic partitions to members (default BalanceStrategyRange)
	// See this: https://chrzaszcz.dev/2021/09/kafka-assignors/
	// Sanitize function doesn't allow an empty BalanceStrategy parameter.
	switch options.BalanceStrategy {
	case BalanceStrategyRange:
		sc.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRange
	case BalanceStrategySticky:
		sc.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategySticky
	case BalanceStrategyRoundRobin:
		sc.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	}

	if options.StartConsumingLatest {
		// Start consuming from the latest offset after a client restart
		sc.Consumer.Offsets.Initial = sarama.OffsetNewest
	}

	// IsolationLevel support 2 mode:
	// 	- use `ReadUncommitted` (default) to consume and return all messages in message channel
	//	- use `ReadCommitted` to hide messages that are part of an aborted transaction
	switch options.IsolationLevel {
	case IsolationLevelReadCommitted:
		sc.Consumer.IsolationLevel = sarama.ReadCommitted
	case IsolationLevelReadUncommitted:
		sc.Consumer.IsolationLevel = sarama.ReadUncommitted
	}

	// SASL based authentication with broker. While there are multiple SASL authentication methods
	// the current implementation is limited to plaintext (SASL/PLAIN) authentication
	if options.SASL.Enable {
		sc.Net.SASL.Enable = true
		sc.Net.SASL.User = options.SASL.User
		sc.Net.SASL.Password = options.SASL.Password
	}

	return sc, nil
}

// Subscribe creates a new consumer group with given config and streams messages via next channel.
func (c *KafkaConsumerGroupBridge) Subscribe(ctx context.Context, options GraphQLSubscriptionOptions, next chan<- []byte) error {
	options.Sanitize()
	if err := options.Validate(); err != nil {
		return err
	}

	saramaConfig, err := c.prepareSaramaConfig(&options)
	if err != nil {
		return err
	}

	cg, err := NewKafkaConsumerGroup(c.log, saramaConfig, &options)
	if err != nil {
		return err
	}

	messages := make(chan *sarama.ConsumerMessage)
	cg.StartConsuming(messages)

	// Wait for messages.
	go func() {
		defer func() {
			if err := cg.Close(); err != nil {
				c.log.Error("KafkaConsumerGroup.Close returned an error",
					log.Strings("topics", options.Topics),
					log.String("groupID", options.GroupID),
					log.String("clientID", options.ClientID),
					log.Error(err),
				)
			}
			close(next)
		}()

		for {
			select {
			case <-c.ctx.Done():
				// Gateway context
				return
			case <-ctx.Done():
				// Request context
				return
			case msg, ok := <-messages:
				if !ok {
					return
				}
				// The "data" field contains the result of your GraphQL request.
				result, err := jsonparser.Set([]byte(`{}`), msg.Value, "data")
				if err != nil {
					return
				}
				next <- result
			}
		}
	}()

	return nil
}

var _ sarama.ConsumerGroupHandler = (*kafkaConsumerGroupHandler)(nil)
