package cost_estimator

import (
	"fmt"
	confluent_kafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	kaytuAzure "github.com/kaytu-io/kaytu-azure-describer/pkg/kaytu-es-sdk"
	"github.com/kaytu-io/kaytu-engine/pkg/cost-estimator/db"
	"github.com/kaytu-io/kaytu-util/pkg/kaytu-es-sdk"
	"github.com/kaytu-io/kaytu-util/pkg/postgres"
	"go.uber.org/zap"
	"strings"
)

type HttpHandler struct {
	db     db.Database
	client kaytu.Client
	//awsClient   kaytuAws.Client
	azureClient   kaytuAzure.Client
	kafkaProducer *confluent_kafka.Producer
	kafkaTopic    string

	logger *zap.Logger
}

func InitializeHttpHandler(
	postgresHost string, postgresPort string, postgresDb string, postgresUsername string, postgresPassword string, postgresSSLMode string,
	elasticSearchPassword, elasticSearchUsername, elasticSearchAddress string,
	logger *zap.Logger,
) (h *HttpHandler, err error) {
	h = &HttpHandler{}
	h.logger = logger

	h.logger.Info("Initializing http handler")

	defaultAccountID := "default"
	h.client, err = kaytu.NewClient(kaytu.ClientConfig{
		Addresses: []string{elasticSearchAddress},
		Username:  &elasticSearchUsername,
		Password:  &elasticSearchPassword,
		AccountID: &defaultAccountID,
	})
	if err != nil {
		return nil, err
	}

	//h.awsClient = kaytuAws.Client{
	//	Client: h.client,
	//}
	h.azureClient = kaytuAzure.Client{
		Client: h.client,
	}
	h.logger.Info("Initialized elasticSearch", zap.String("client", fmt.Sprintf("%v", h.client)))

	cfg := postgres.Config{
		Host:    postgresHost,
		Port:    postgresPort,
		User:    postgresUsername,
		Passwd:  postgresPassword,
		DB:      postgresDb,
		SSLMode: postgresSSLMode,
	}
	orm, err := postgres.NewClient(&cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("new postgres client: %w", err)
	}
	h.logger.Info("Connected to the postgres database")

	db := db.NewDatabase(orm)
	err = db.Initialize()
	if err != nil {
		return nil, err
	}
	h.db = db
	h.logger.Info("Initialized postgres database")

	kafkaProducer, err := newKafkaProducer(strings.Split(KafkaService, ","))
	if err != nil {
		return nil, err
	}
	h.kafkaProducer = kafkaProducer
	h.kafkaTopic = KafkaTopic

	return h, nil
}

func newKafkaProducer(brokers []string) (*confluent_kafka.Producer, error) {
	return confluent_kafka.NewProducer(&confluent_kafka.ConfigMap{
		"bootstrap.servers":            strings.Join(brokers, ","),
		"linger.ms":                    100,
		"compression.type":             "lz4",
		"message.timeout.ms":           10000,
		"queue.buffering.max.messages": 100000,
		"message.max.bytes":            104857600,
	})
}
