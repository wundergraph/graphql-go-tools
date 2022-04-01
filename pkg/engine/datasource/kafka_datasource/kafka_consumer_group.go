package kafka_datasource

import (
	"context"
	"time"

	"github.com/Shopify/sarama"
	log "github.com/jensneuse/abstractlogger"
)

type KafkaConsumerGroup struct {
	log           log.Logger
	consumerGroup sarama.ConsumerGroup
	ctx           context.Context
}

type kafkaConsumerGroupHandler struct {
	log      log.Logger
	options  *GraphQLSubscriptionOptions
	messages chan *sarama.ConsumerMessage
	ctx      context.Context
}

func (k *kafkaConsumerGroupHandler) Setup(_ sarama.ConsumerGroupSession) error {
	k.log.Debug("kafkaConsumerGroupHandler.Setup", log.String("topic", k.options.Topic))
	return nil
}

func (k *kafkaConsumerGroupHandler) Cleanup(_ sarama.ConsumerGroupSession) error {
	k.log.Debug("kafkaConsumerGroupHandler.Cleanup", log.String("topic", k.options.Topic))
	close(k.messages)
	return nil
}

func (k *kafkaConsumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		ctx, cancel := context.WithTimeout(k.ctx, time.Second*5)
		select {
		case k.messages <- msg:
			cancel()
			session.MarkMessage(msg, "") // Commit the message and advance the offset.
		case <-ctx.Done():
			cancel()
		case <-k.ctx.Done():
			cancel()
			return nil
		}
	}
	k.log.Debug("kafkaConsumerGroupHandler.ConsumeClaim is gone")
	return nil
}

func (c *KafkaConsumerGroup) newConsumerGroup(options *GraphQLSubscriptionOptions) (sarama.ConsumerGroup, error) {
	sc := sarama.NewConfig()
	sc.Version = sarama.V2_7_0_0
	sc.ClientID = options.ClientID
	return sarama.NewConsumerGroup([]string{options.BrokerAddr}, options.GroupID, sc)
}

func (c *KafkaConsumerGroup) stopConsuming(ctx context.Context, cg sarama.ConsumerGroup) {
	select {
	case <-ctx.Done():
	case <-c.ctx.Done():
	}

	err := cg.Close()
	if err != nil {
		c.log.Error("KafkaConsumerGroup.stopConsuming", log.Error(err))
	}
}

func (c *KafkaConsumerGroup) startConsuming(ctx context.Context, cg sarama.ConsumerGroup, messages chan *sarama.ConsumerMessage, options *GraphQLSubscriptionOptions) {
	handler := &kafkaConsumerGroupHandler{
		log:      c.log,
		options:  options,
		messages: messages,
		ctx:      ctx,
	}

	go c.stopConsuming(ctx, cg)

	err := cg.Consume(ctx, []string{options.Topic}, handler)
	if err != nil {
		c.log.Error("KafkaConsumerGroup.startConsuming", log.Error(err))
	}
}

func (c *KafkaConsumerGroup) Subscribe(ctx context.Context, options GraphQLSubscriptionOptions, next chan<- []byte) error {
	cg, err := c.newConsumerGroup(&options)
	if err != nil {
		return err
	}

	messages := make(chan *sarama.ConsumerMessage)
	go c.startConsuming(ctx, cg, messages, &options)

	go func() {
		for {
			select {
			case <-c.ctx.Done():
				return
			case <-ctx.Done():
				return
			case msg, ok := <-messages:
				if !ok {
					return
				}
				// TODO: What about msg.Key and msg.Headers?
				next <- msg.Value
			}
		}
	}()

	return nil
}

//var _ GraphQLSubscriptionClient = (*KafkaConsumerGroup)(nil)
