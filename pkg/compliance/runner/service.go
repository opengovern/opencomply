package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/opengovern/og-util/pkg/api"
	"os"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/opengovern/og-util/pkg/config"
	esSinkClient "github.com/opengovern/og-util/pkg/es/ingest/client"
	"github.com/opengovern/og-util/pkg/httpclient"
	"github.com/opengovern/og-util/pkg/jq"
	"github.com/opengovern/og-util/pkg/opengovernance-es-sdk"
	"github.com/opengovern/og-util/pkg/source"
	"github.com/opengovern/og-util/pkg/steampipe"
	complianceApi "github.com/opengovern/opengovernance/pkg/compliance/api"
	complianceClient "github.com/opengovern/opengovernance/pkg/compliance/client"
	inventoryClient "github.com/opengovern/opengovernance/pkg/inventory/client"
	metadataClient "github.com/opengovern/opengovernance/pkg/metadata/client"
	onboardClient "github.com/opengovern/opengovernance/pkg/onboard/client"
	"go.uber.org/zap"
)

type Config struct {
	ElasticSearch         config.ElasticSearch
	NATS                  config.NATS
	Compliance            config.KaytuService
	Onboard               config.KaytuService
	Inventory             config.KaytuService
	Metadata              config.KaytuService
	EsSink                config.KaytuService
	Steampipe             config.Postgres
	PennywiseBaseURL      string `yaml:"pennywise_base_url"`
	PrometheusPushAddress string
}

type Worker struct {
	config           Config
	logger           *zap.Logger
	steampipeConn    *steampipe.Database
	esClient         opengovernance.Client
	jq               *jq.JobQueue
	complianceClient complianceClient.ComplianceServiceClient
	onboardClient    onboardClient.OnboardServiceClient
	inventoryClient  inventoryClient.InventoryServiceClient
	metadataClient   metadataClient.MetadataServiceClient
	sinkClient       esSinkClient.EsSinkServiceClient

	benchmarkCache map[string]complianceApi.Benchmark
}

var (
	ManualTrigger = os.Getenv("MANUAL_TRIGGER")
)

func NewWorker(
	config Config,
	logger *zap.Logger,
	prometheusPushAddress string,
	ctx context.Context,
) (*Worker, error) {
	err := steampipe.PopulateSteampipeConfig(config.ElasticSearch, source.CloudAWS)
	if err != nil {
		return nil, err
	}
	err = steampipe.PopulateSteampipeConfig(config.ElasticSearch, source.CloudAzure)
	if err != nil {
		return nil, err
	}

	if err := steampipe.PopulateKaytuPluginSteampipeConfig(config.ElasticSearch, config.Steampipe, config.PennywiseBaseURL); err != nil {
		return nil, err
	}

	time.Sleep(2 * time.Minute)

	steampipeConn, err := steampipe.StartSteampipeServiceAndGetConnection(logger)
	if err != nil {
		return nil, err
	}

	esClient, err := opengovernance.NewClient(opengovernance.ClientConfig{
		Addresses:     []string{config.ElasticSearch.Address},
		Username:      &config.ElasticSearch.Username,
		Password:      &config.ElasticSearch.Password,
		IsOnAks:       &config.ElasticSearch.IsOnAks,
		IsOpenSearch:  &config.ElasticSearch.IsOpenSearch,
		AwsRegion:     &config.ElasticSearch.AwsRegion,
		AssumeRoleArn: &config.ElasticSearch.AssumeRoleArn,
	})
	if err != nil {
		return nil, err
	}

	jq, err := jq.New(config.NATS.URL, logger)
	if err != nil {
		return nil, err
	}

	queueTopic := JobQueueTopic
	if ManualTrigger == "true" {
		queueTopic = JobQueueTopicManuals
	}

	if err := jq.Stream(ctx, StreamName, "compliance runner job queue", []string{queueTopic, ResultQueueTopic}, 1000000); err != nil {
		return nil, err
	}

	w := &Worker{
		config:           config,
		logger:           logger,
		steampipeConn:    steampipeConn,
		esClient:         esClient,
		jq:               jq,
		complianceClient: complianceClient.NewComplianceClient(config.Compliance.BaseURL),
		onboardClient:    onboardClient.NewOnboardServiceClient(config.Onboard.BaseURL),
		inventoryClient:  inventoryClient.NewInventoryServiceClient(config.Inventory.BaseURL),
		metadataClient:   metadataClient.NewMetadataServiceClient(config.Metadata.BaseURL),
		sinkClient:       esSinkClient.NewEsSinkServiceClient(logger, config.EsSink.BaseURL),
		benchmarkCache:   make(map[string]complianceApi.Benchmark),
	}
	ctx2 := &httpclient.Context{Ctx: ctx, UserRole: api.AdminRole}
	benchmarks, err := w.complianceClient.ListAllBenchmarks(ctx2, true)
	if err != nil {
		logger.Error("failed to get benchmarks", zap.Error(err))
		return nil, err
	}
	for _, benchmark := range benchmarks {
		w.benchmarkCache[benchmark.ID] = benchmark
	}

	return w, nil
}

// Run is a blocking function so you may decide to call it in another goroutine.
// It runs a NATS consumer and it will close it when the given context is closed.
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("starting to consume")

	queueTopic := JobQueueTopic
	consumer := ConsumerGroup
	if ManualTrigger == "true" {
		queueTopic = JobQueueTopicManuals
		consumer = ConsumerGroupManuals
	}

	consumeCtx, err := w.jq.ConsumeWithConfig(ctx, consumer, StreamName, []string{queueTopic},
		jetstream.ConsumerConfig{
			DeliverPolicy:     jetstream.DeliverAllPolicy,
			AckPolicy:         jetstream.AckExplicitPolicy,
			AckWait:           time.Hour,
			MaxDeliver:        1,
			InactiveThreshold: time.Hour,
			Replicas:          1,
			MemoryStorage:     false,
		}, nil,
		func(msg jetstream.Msg) {
			w.logger.Info("received a new job")
			w.logger.Info("committing")
			if err := msg.InProgress(); err != nil {
				w.logger.Error("failed to send the initial in progress message", zap.Error(err), zap.Any("msg", msg))
			}
			ticker := time.NewTicker(15 * time.Second)
			go func() {
				for range ticker.C {
					if err := msg.InProgress(); err != nil {
						w.logger.Error("failed to send an in progress message", zap.Error(err), zap.Any("msg", msg))
					}
				}
			}()

			_, _, err := w.ProcessMessage(ctx, msg)
			if err != nil {
				w.logger.Error("failed to process message", zap.Error(err))
			}
			ticker.Stop()

			if err := msg.Ack(); err != nil {
				w.logger.Error("failed to send the ack message", zap.Error(err), zap.Any("msg", msg))
			}

			//if requeue {
			//	if err := msg.Nak(); err != nil {
			//		w.logger.Error("failed to send a not ack message", zap.Error(err))
			//	}
			//}
			//
			//if commit {
			//	w.logger.Info("committing")
			//	if err := msg.Ack(); err != nil {
			//		w.logger.Error("failed to send an ack message", zap.Error(err))
			//	}
			//}

			w.logger.Info("processing a job completed")
		})
	if err != nil {
		return err
	}

	w.logger.Info("consuming")

	<-ctx.Done()
	consumeCtx.Drain()
	consumeCtx.Stop()

	return nil
}

func (w *Worker) ProcessMessage(ctx context.Context, msg jetstream.Msg) (commit bool, requeue bool, err error) {
	var job Job

	if err := json.Unmarshal(msg.Data(), &job); err != nil {
		return true, false, err
	}

	result := JobResult{
		Job:               job,
		StartedAt:         time.Now(),
		Status:            ComplianceRunnerInProgress,
		Error:             "",
		TotalFindingCount: nil,
	}

	defer func() {
		if err != nil {
			result.Error = err.Error()
			result.Status = ComplianceRunnerFailed
		} else {
			result.Status = ComplianceRunnerSucceeded
		}

		resultJson, err := json.Marshal(result)
		if err != nil {
			w.logger.Error("failed to create job result json", zap.Error(err))
			return
		}

		if _, err := w.jq.Produce(ctx, ResultQueueTopic, resultJson, fmt.Sprintf("compliance-runner-result-%d-%d", job.ID, job.RetryCount)); err != nil {
			w.logger.Error("failed to publish job result", zap.String("jobResult", string(resultJson)), zap.Error(err))
		}
	}()

	resultJson, err := json.Marshal(result)
	if err != nil {
		w.logger.Error("failed to create job in progress json", zap.Error(err))
		return true, false, err
	}

	if _, err := w.jq.Produce(ctx, ResultQueueTopic, resultJson, fmt.Sprintf("compliance-runner-inprogress-%d-%d", job.ID, job.RetryCount)); err != nil {
		w.logger.Error("failed to publish job in progress", zap.String("jobInProgress", string(resultJson)), zap.Error(err))
	}

	w.logger.Info("running job", zap.ByteString("job", msg.Data()))

	totalFindingCount, err := w.RunJob(ctx, job)
	if err != nil {
		return true, false, err
	}

	result.TotalFindingCount = &totalFindingCount
	return true, false, nil
}

func (w *Worker) Stop() error {
	w.steampipeConn.Conn().Close()
	steampipe.StopSteampipeService(w.logger)
	return nil
}
