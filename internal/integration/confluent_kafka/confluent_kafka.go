package confluent_kafka

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"gopkg.in/confluentinc/confluent-kafka-go.v1/kafka"

	pb "github.com/brocaar/chirpstack-api/go/v3/as/integration"
	"github.com/brocaar/chirpstack-application-server/internal/config"
	"github.com/brocaar/chirpstack-application-server/internal/integration/marshaler"
	"github.com/brocaar/chirpstack-application-server/internal/integration/models"
	"github.com/brocaar/chirpstack-application-server/internal/logging"
	"github.com/brocaar/lorawan"
)

// Integration implements an Confluent Kafka integration.
type Integration struct {
	marshaler        marshaler.Type
	writer           *confluent_kafka.Producer
	eventKeyTemplate *template.Template
	config           config.IntegrationConfluentKafkaConfig
}

// New creates a new Kafka integration.
func New(m marshaler.Type, conf config.IntegrationConfluentKafkaConfig) (*Integration, error) {
	wc := confluent_kafka.ConfigMap{
		"bootstrap.servers": conf.Brokers,
		"security.protocol": conf.Mechanism,
	}

	// add tls config here
	//if conf.TLS {
	//wc.Set(
	//}

	if conf.Username != "" || conf.Password != "" {
		switch conf.Mechanism {
		case "sasl":
			wc.Set("sasl.mechanisms=SASL_SSL")
			// +++ and other sasl configuration goes below

			// +++
		case "ssl":
			wc.Set("sasl.mechanisms=SSL")
			// +++ and other ssl configuration goes below

			// +++
		default:
			return nil, fmt.Errorf("unknown sasl mechanism %s", conf.Mechanism)
		}

	}

	log.WithFields(log.Fields{
		"brokers": conf.Brokers,
		"topic":   conf.Topic,
	}).Info("integration/confluent_kafka: connecting to kafka broker(s)")

	w, err := confluent_kafka.NewProducer(&wc)

	log.Info("integration/confluent_kafka: connected to kafka broker(s)")

	kt, err := template.New("key").Parse(conf.EventKeyTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "parse key template")
	}

	i := Integration{
		marshaler:        m,
		writer:           w,
		eventKeyTemplate: kt,
		config:           conf,
	}
	return &i, nil
}

// HandleUplinkEvent sends an UplinkEvent.
func (i *Integration) HandleUplinkEvent(ctx context.Context, _ models.Integration, vars map[string]string, pl pb.UplinkEvent) error {
	return i.publish(ctx, pl.ApplicationId, pl.DevEui, "up", &pl)
}

// HandleJoinEvent sends a JoinEvent.
func (i *Integration) HandleJoinEvent(ctx context.Context, _ models.Integration, vars map[string]string, pl pb.JoinEvent) error {
	return i.publish(ctx, pl.ApplicationId, pl.DevEui, "join", &pl)
}

// HandleAckEvent sends an AckEvent.
func (i *Integration) HandleAckEvent(ctx context.Context, _ models.Integration, vars map[string]string, pl pb.AckEvent) error {
	return i.publish(ctx, pl.ApplicationId, pl.DevEui, "ack", &pl)
}

// HandleErrorEvent sends an ErrorEvent.
func (i *Integration) HandleErrorEvent(ctx context.Context, _ models.Integration, vars map[string]string, pl pb.ErrorEvent) error {
	return i.publish(ctx, pl.ApplicationId, pl.DevEui, "error", &pl)
}

// HandleStatusEvent sends a StatusEvent.
func (i *Integration) HandleStatusEvent(ctx context.Context, _ models.Integration, vars map[string]string, pl pb.StatusEvent) error {
	return i.publish(ctx, pl.ApplicationId, pl.DevEui, "status", &pl)
}

// HandleLocationEvent sends a LocationEvent.
func (i *Integration) HandleLocationEvent(ctx context.Context, _ models.Integration, vars map[string]string, pl pb.LocationEvent) error {
	return i.publish(ctx, pl.ApplicationId, pl.DevEui, "location", &pl)
}

// HandleTxAckEvent sends a TxAckEvent.
func (i *Integration) HandleTxAckEvent(ctx context.Context, _ models.Integration, vars map[string]string, pl pb.TxAckEvent) error {
	return i.publish(ctx, pl.ApplicationId, pl.DevEui, "txack", &pl)
}

// HandleIntegrationEvent sends an IntegrationEvent.
func (i *Integration) HandleIntegrationEvent(ctx context.Context, _ models.Integration, vars map[string]string, pl pb.IntegrationEvent) error {
	return i.publish(ctx, pl.ApplicationId, pl.DevEui, "integration", &pl)
}

func (i *Integration) publish(ctx context.Context, applicationID uint64, devEUIB []byte, event string, msg proto.Message) error {
	if i.writer == nil {
		return fmt.Errorf("integration closed")
	}

	var devEUI lorawan.EUI64
	copy(devEUI[:], devEUIB)

	deliveryChan := make(chan confluent_kafka.Event)

	b, err := marshaler.Marshal(i.marshaler, msg)
	if err != nil {
		return err
	}

	keyBuf := bytes.NewBuffer(nil)
	err = i.eventKeyTemplate.Execute(keyBuf, struct {
		ApplicationID uint64
		DevEUI        lorawan.EUI64
		EventType     string
	}{applicationID, devEUI, event})
	if err != nil {
		return errors.Wrap(err, "executing template")
	}
	key := keyBuf.Bytes()

	kmsg := confluent_kafka.Message{
		TopicPartition: confluent_kafka.TopicPartition{Topic: &i.config.Topic, Partition: confluent_kafka.PartitionAny},
		Value:          b,
		Headers: []confluent_kafka.Header{
			{
				Key:   "event",
				Value: []byte(event),
			},
		},
	}
	if len(key) > 0 {
		kmsg.Key = key
	}

	log.WithFields(log.Fields{
		"key":    string(key),
		"ctx_id": ctx.Value(logging.ContextIDKey),
	}).Info("integration/confluent_kafka: publishing message")

	if err := i.writer.Produce(&kmsg, deliveryChan); err != nil {
		return errors.Wrap(err, "writing message to kafka")
	}

	e := <-deliveryChan
	m := e.(*confluent_kafka.Message)
	if m.TopicPartition.Error != nil {
		log.Println("Delivery failed: %v\n", m.TopicPartition.Error)
	} else {
		log.Println("Delivered message to topic %s [%d] at offset %v\n",
			*m.TopicPartition.Topic, m.TopicPartition.Partition, m.TopicPartition.Offset)
	}
	close(deliveryChan)

	return nil
}

// DataDownChan return nil.
func (i *Integration) DataDownChan() chan models.DataDownPayload {
	return nil
}

// Close shuts down the integration, closing the Kafka writer.
func (i *Integration) Close() error {
	if i.writer == nil {
		return fmt.Errorf("integration already closed")
	}
	i.writer.Close()
	i.writer = nil
	return nil
}
