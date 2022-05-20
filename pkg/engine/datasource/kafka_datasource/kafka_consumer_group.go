package kafka_datasource

import (
	"context"
	"sync"
	"time"

	"github.com/Shopify/sarama"
	"github.com/buger/jsonparser"
	log "github.com/jensneuse/abstractlogger"
)

type KafkaConsumerGroupBridge struct {
	log log.Logger
	ctx context.Context
}

type KafkaConsumerGroup struct {
	startedCtx    context.Context
	startedCancel context.CancelFunc

	consumerGroup sarama.ConsumerGroup
	options       *GraphQLSubscriptionOptions
	log           log.Logger
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
}

type kafkaConsumerGroupHandler struct {
	startedCtx    context.Context
	startedCancel context.CancelFunc

	log      log.Logger
	options  *GraphQLSubscriptionOptions
	messages chan *sarama.ConsumerMessage
	ctx      context.Context
}

// Setup is run at the beginning of a new session, before ConsumeClaim.
func (k *kafkaConsumerGroupHandler) Setup(_ sarama.ConsumerGroupSession) error {
	k.log.Debug("kafkaConsumerGroupHandler.Setup",
		log.String("topic", k.options.Topic),
		log.String("groupID", k.options.GroupID),
		log.String("clientID", k.options.ClientID),
	)
	return nil
}

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited
// but before the offsets are committed for the very last time.
func (k *kafkaConsumerGroupHandler) Cleanup(_ sarama.ConsumerGroupSession) error {
	k.log.Debug("kafkaConsumerGroupHandler.Cleanup",
		log.String("topic", k.options.Topic),
		log.String("groupID", k.options.GroupID),
		log.String("clientID", k.options.ClientID),
	)
	close(k.messages)
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

	k.startedCancel()
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
		log.String("topic", k.options.Topic),
		log.String("groupID", k.options.GroupID),
		log.String("clientID", k.options.ClientID))
	return nil
}

// NewKafkaConsumerGroup creates a new sarama.ConsumerGroup and returns a new
// *KafkaConsumerGroup instance.
func NewKafkaConsumerGroup(log log.Logger, saramaConfig *sarama.Config, options *GraphQLSubscriptionOptions) (*KafkaConsumerGroup, error) {
	cg, err := sarama.NewConsumerGroup([]string{options.BrokerAddr}, options.GroupID, saramaConfig)
	if err != nil {
		return nil, err
	}

	startedCtx, startedCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	return &KafkaConsumerGroup{
		startedCtx:    startedCtx,
		startedCancel: startedCancel,
		consumerGroup: cg,
		log:           log,
		options:       options,
		ctx:           ctx,
		cancel:        cancel,
	}, nil
}

func (k *KafkaConsumerGroup) startConsuming(handler sarama.ConsumerGroupHandler) {
	defer k.wg.Done()

	defer func() {
		if err := k.consumerGroup.Close(); err != nil {
			k.log.Error("KafkaConsumerGroup.Close returned an error",
				log.String("topic", k.options.Topic),
				log.String("groupID", k.options.GroupID),
				log.String("clientID", k.options.ClientID),
				log.Error(err))
		}
	}()

	// Blocking call
	err := k.consumerGroup.Consume(k.ctx, []string{k.options.Topic}, handler)
	if err != nil {
		k.log.Error("KafkaConsumerGroup.startConsuming",
			log.String("topic", k.options.Topic),
			log.String("groupID", k.options.GroupID),
			log.String("clientID", k.options.ClientID),
			log.Error(err))
	}
}

// StartConsuming initializes a new consumer group handler and starts consuming at
// background.
func (k *KafkaConsumerGroup) StartConsuming(messages chan *sarama.ConsumerMessage) {
	handler := &kafkaConsumerGroupHandler{
		startedCtx:    k.startedCtx,
		startedCancel: k.startedCancel,
		log:           k.log,
		options:       k.options,
		messages:      messages,
		ctx:           k.ctx,
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

// Subscribe creates a new consumer group with given config and streams messages via next channel.
func (c *KafkaConsumerGroupBridge) Subscribe(ctx context.Context, options GraphQLSubscriptionOptions, next chan<- []byte) error {
	options.Sanitize()
	if err := options.Validate(); err != nil {
		return err
	}

	saramaConfig := sarama.NewConfig()
	saramaConfig.Version = SaramaSupportedKafkaVersions[options.KafkaVersion]
	saramaConfig.ClientID = options.ClientID
	if options.StartConsumingLatest {
		// Start consuming from the latest offset after a client restart
		saramaConfig.Consumer.Offsets.Initial = sarama.OffsetNewest
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
					log.String("topic", options.Topic),
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
