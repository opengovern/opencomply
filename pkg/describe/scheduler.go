package describe

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/opengovern/og-util/pkg/opengovernance-es-sdk"
	queryrunnerscheduler "github.com/opengovern/opengovernance/pkg/describe/schedulers/query-runner"
	integration_type "github.com/opengovern/opengovernance/services/integration/integration-type"
	queryrunner "github.com/opengovern/opengovernance/services/inventory/query-runner"

	envoyAuth "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/nats-io/nats.go/jetstream"
	authAPI "github.com/opengovern/og-util/pkg/api"
	esSinkClient "github.com/opengovern/og-util/pkg/es/ingest/client"
	"github.com/opengovern/og-util/pkg/httpclient"
	"github.com/opengovern/og-util/pkg/httpserver"
	"github.com/opengovern/og-util/pkg/jq"
	"github.com/opengovern/og-util/pkg/postgres"
	"github.com/opengovern/og-util/pkg/ticker"
	"github.com/opengovern/og-util/proto/src/golang"
	"github.com/opengovern/opengovernance/pkg/analytics"
	"github.com/opengovern/opengovernance/pkg/checkup"
	checkupAPI "github.com/opengovern/opengovernance/pkg/checkup/api"
	"github.com/opengovern/opengovernance/pkg/describe/api"
	"github.com/opengovern/opengovernance/pkg/describe/config"
	"github.com/opengovern/opengovernance/pkg/describe/db"
	"github.com/opengovern/opengovernance/pkg/describe/db/model"
	"github.com/opengovern/opengovernance/pkg/describe/schedulers/compliance"
	"github.com/opengovern/opengovernance/pkg/describe/schedulers/discovery"
	"github.com/opengovern/opengovernance/pkg/utils"
	"github.com/opengovern/opengovernance/services/compliance/client"
	"github.com/opengovern/opengovernance/services/compliance/runner"
	"github.com/opengovern/opengovernance/services/compliance/summarizer"
	integrationClient "github.com/opengovern/opengovernance/services/integration/client"
	inventoryClient "github.com/opengovern/opengovernance/services/inventory/client"
	metadataClient "github.com/opengovern/opengovernance/services/metadata/client"
	"github.com/opengovern/opengovernance/services/metadata/models"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	JobSchedulingInterval   = 1 * time.Minute
	JobSequencerInterval    = 1 * time.Minute
	JobTimeoutCheckInterval = 1 * time.Minute
	MaxJobInQueue           = 10000

	schedulerConsumerGroup = "describe-scheduler"
)

var DescribePublishingBlocked = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "opengovernance",
	Subsystem: "scheduler",
	Name:      "queue_job_publishing_blocked",
	Help:      "The gauge whether publishing tasks to a queue is blocked: 0 for resumed and 1 for blocked",
}, []string{"queue_name"})

var CheckupJobsCount = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "opengovernance",
	Subsystem: "scheduler",
	Name:      "schedule_checkup_jobs_total",
	Help:      "Count of checkup jobs in scheduler service",
}, []string{"status"})

var AnalyticsJobsCount = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "opengovernance",
	Subsystem: "scheduler",
	Name:      "schedule_analytics_jobs_total",
	Help:      "Count of analytics jobs in scheduler service",
}, []string{"status"})

var AnalyticsJobResultsCount = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "opengovernance",
	Subsystem: "scheduler",
	Name:      "schedule_analytics_job_results_total",
	Help:      "Count of analytics job results in scheduler service",
}, []string{"status"})

var LargeDescribeResourceMessage = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "og_scheduler_large_describe_resource_message",
	Help: "The gauge whether the describe resource message is too large: 0 for not large and 1 for large",
}, []string{"resource_type"})

type OperationMode string

const (
	OperationModeScheduler OperationMode = "scheduler"
	OperationModeReceiver  OperationMode = "receiver"
)

type Scheduler struct {
	id         string
	db         db.Database
	httpServer *HttpServer
	grpcServer *grpc.Server

	discoveryIntervalHours     time.Duration
	costDiscoveryIntervalHours time.Duration
	describeTimeoutHours       int64
	checkupIntervalHours       int64
	mustSummarizeIntervalHours int64
	analyticsIntervalHours     time.Duration
	complianceIntervalHours    time.Duration

	logger            *zap.Logger
	metadataClient    metadataClient.MetadataServiceClient
	complianceClient  client.ComplianceServiceClient
	integrationClient integrationClient.IntegrationServiceClient
	inventoryClient   inventoryClient.InventoryServiceClient
	sinkClient        esSinkClient.EsSinkServiceClient
	authGrpcClient    envoyAuth.AuthorizationClient
	es                opengovernance.Client

	jq *jq.JobQueue

	describeJobLocalEndpoint     string
	describeDeliverLocalEndpoint string
	describeExternalEndpoint     string
	keyARN                       string
	keyRegion                    string

	DoDeleteOldResources bool
	OperationMode        OperationMode
	MaxConcurrentCall    int64

	complianceScheduler  *compliance.JobScheduler
	discoveryScheduler   *discovery.Scheduler
	queryRunnerScheduler *queryrunnerscheduler.JobScheduler
	conf                 config.SchedulerConfig
}

func InitializeScheduler(
	id string,
	conf config.SchedulerConfig,
	postgresUsername string,
	postgresPassword string,
	postgresHost string,
	postgresPort string,
	postgresDb string,
	postgresSSLMode string,
	httpServerAddress string,
	describeTimeoutHours string,
	checkupIntervalHours string,
	mustSummarizeIntervalHours string,
	ctx context.Context,
) (s *Scheduler, err error) {
	if id == "" {
		return nil, fmt.Errorf("'id' must be set to a non empty string")
	}

	s = &Scheduler{
		id:                           id,
		OperationMode:                OperationMode(OperationModeConfig),
		describeJobLocalEndpoint:     DescribeLocalJobEndpoint,
		describeDeliverLocalEndpoint: DescribeLocalDeliverEndpoint,
		describeExternalEndpoint:     DescribeExternalEndpoint,
		keyARN:                       KeyARN,
		keyRegion:                    KeyRegion,
	}
	defer func() {
		if err != nil && s != nil {
			s.Stop()
		}
	}()
	s.logger, err = zap.NewProduction()
	if err != nil {
		return nil, err
	}

	s.conf = conf

	cfg := postgres.Config{
		Host:    postgresHost,
		Port:    postgresPort,
		User:    postgresUsername,
		Passwd:  postgresPassword,
		DB:      postgresDb,
		SSLMode: postgresSSLMode,
	}

	if s.OperationMode == OperationModeScheduler {
		cfg.Connection.MaxOpen = 50
		cfg.Connection.MaxIdle = 20
	}

	orm, err := postgres.NewClient(&cfg, s.logger)
	if err != nil {
		s.logger.Error("Failed to create postgres client", zap.Error(err))
		return nil, fmt.Errorf("new postgres client: %w", err)
	}

	jq, err := jq.New(conf.NATS.URL, s.logger)
	if err != nil {
		s.logger.Error("Failed to create job queue", zap.Error(err))
		return nil, err
	}
	s.jq = jq

	err = s.SetupNatsStreams(ctx)
	if err != nil {
		s.logger.Error("Failed to setup nats streams", zap.Error(err))
		return nil, err
	}

	s.logger.Info("Connected to the postgres database: ", zap.String("db", postgresDb))
	s.db = db.Database{ORM: orm}

	s.es, err = opengovernance.NewClient(opengovernance.ClientConfig{
		Addresses:     []string{conf.ElasticSearch.Address},
		Username:      &conf.ElasticSearch.Username,
		Password:      &conf.ElasticSearch.Password,
		IsOnAks:       &conf.ElasticSearch.IsOnAks,
		IsOpenSearch:  &conf.ElasticSearch.IsOpenSearch,
		AwsRegion:     &conf.ElasticSearch.AwsRegion,
		AssumeRoleArn: &conf.ElasticSearch.AssumeRoleArn,
	})
	if err != nil {
		s.logger.Error("Failed to create elasticsearch client", zap.Error(err))
		return nil, err
	}

	s.httpServer = NewHTTPServer(httpServerAddress, s.db, s)

	describeIntervalHours, err := strconv.ParseInt(DescribeIntervalHours, 10, 64)
	if err != nil {
		s.logger.Error("Failed to parse describe interval hours", zap.Error(err))
		return nil, err
	}
	s.discoveryIntervalHours = time.Duration(describeIntervalHours) * time.Hour

	costDiscoveryIntervalHours, err := strconv.ParseInt(CostDiscoveryIntervalHours, 10, 64)
	if err != nil {
		s.logger.Error("Failed to parse cost discovery interval hours", zap.Error(err))
		return nil, err
	}
	s.costDiscoveryIntervalHours = time.Duration(costDiscoveryIntervalHours) * time.Hour

	s.describeTimeoutHours, err = strconv.ParseInt(describeTimeoutHours, 10, 64)
	if err != nil {
		s.logger.Error("Failed to parse describe timeout hours", zap.Error(err))
		return nil, err
	}

	s.checkupIntervalHours, err = strconv.ParseInt(checkupIntervalHours, 10, 64)
	if err != nil {
		s.logger.Error("Failed to parse checkup interval hours", zap.Error(err))
		return nil, err
	}

	s.mustSummarizeIntervalHours, err = strconv.ParseInt(mustSummarizeIntervalHours, 10, 64)
	if err != nil {
		s.logger.Error("Failed to parse must summarize interval hours", zap.Error(err))
		return nil, err
	}

	analyticsIntervalHours, err := strconv.ParseInt(AnalyticsIntervalHours, 10, 64)
	if err != nil {
		s.logger.Error("Failed to parse analytics interval hours", zap.Error(err))
		return nil, err
	}
	s.analyticsIntervalHours = time.Duration(analyticsIntervalHours) * time.Hour

	s.complianceIntervalHours = time.Duration(conf.ComplianceIntervalHours) * time.Hour

	s.metadataClient = metadataClient.NewMetadataServiceClient(MetadataBaseURL)
	s.complianceClient = client.NewComplianceClient(ComplianceBaseURL)
	s.integrationClient = integrationClient.NewIntegrationServiceClient(IntegrationBaseURL)
	s.inventoryClient = inventoryClient.NewInventoryServiceClient(InventoryBaseURL)
	s.sinkClient = esSinkClient.NewEsSinkServiceClient(s.logger, EsSinkBaseURL)
	authGRPCConn, err := grpc.NewClient(AuthGRPCURI, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})))
	if err != nil {
		s.logger.Error("Failed to create auth grpc client", zap.Error(err))
		return nil, err
	}
	s.authGrpcClient = envoyAuth.NewAuthorizationClient(authGRPCConn)

	describeServer := NewDescribeServer(s.db, s.jq, s.authGrpcClient, s.logger, conf)
	s.grpcServer = grpc.NewServer(
		grpc.MaxRecvMsgSize(128 * 1024 * 1024),
	)

	golang.RegisterDescribeServiceServer(s.grpcServer, describeServer)

	s.DoDeleteOldResources, _ = strconv.ParseBool(DoDeleteOldResources)
	describeServer.DoProcessReceivedMessages, _ = strconv.ParseBool(DoProcessReceivedMsgs)
	s.MaxConcurrentCall, _ = strconv.ParseInt(MaxConcurrentCall, 10, 64)
	if s.MaxConcurrentCall <= 0 {
		s.MaxConcurrentCall = 5000
	}

	s.discoveryScheduler = discovery.New(
		conf,
		s.logger,
		s.complianceClient,
		s.db,
		s.es,
	)
	return s, nil
}

func (s *Scheduler) SetupNatsStreams(ctx context.Context) error {
	if err := s.jq.Stream(ctx, queryrunner.StreamName, "Query Runner job queues", []string{queryrunner.JobQueueTopic, queryrunner.JobResultQueueTopic}, 1000); err != nil {
		s.logger.Error("Failed to stream to Query Runner queue", zap.Error(err))
		return err
	}

	if err := s.jq.Stream(ctx, summarizer.StreamName, "compliance summarizer job queues", []string{summarizer.JobQueueTopic, summarizer.JobQueueTopicManuals, summarizer.ResultQueueTopic}, 1000); err != nil {
		s.logger.Error("Failed to stream to compliance summarizer queue", zap.Error(err))
		return err
	}

	if err := s.jq.Stream(ctx, runner.StreamName, "compliance runner job queues", []string{runner.JobQueueTopic, runner.JobQueueTopicManuals, runner.ResultQueueTopic}, 1000000); err != nil {
		s.logger.Error("Failed to stream to compliance runner queue", zap.Error(err))
		return err
	}

	if err := s.jq.Stream(ctx, analytics.StreamName, "analytics job queue", []string{analytics.JobQueueTopic, analytics.JobResultQueueTopic}, 1000); err != nil {
		s.logger.Error("Failed to stream to analytics queue", zap.Error(err))
		return err
	}

	if err := s.jq.Stream(ctx, checkup.StreamName, "checkup job queue", []string{checkup.JobsQueueName, checkup.ResultsQueueName}, 1000); err != nil {
		s.logger.Error("Failed to stream to checkup queue", zap.Error(err))
		return err
	}

	if err := s.jq.Stream(ctx, DescribeStreamName, "describe job queue", []string{DescribeResultsQueueName}, 1000000); err != nil {
		s.logger.Error("Failed to stream to describe queue", zap.Error(err))
		return err
	}

	if s.conf.ServerlessProvider == config.ServerlessProviderTypeLocal.String() {
		for itName, integrationType := range integration_type.IntegrationTypes {
			describerConfig := integrationType.GetConfiguration()
			if err := s.jq.Stream(ctx, describerConfig.NatsStreamName, fmt.Sprintf("%s describe job runner queue", itName), []string{describerConfig.NatsScheduledJobsTopic, describerConfig.NatsManualJobsTopic}, 200000); err != nil {
				s.logger.Error("Failed to stream to local integration type queue", zap.String("integration_type", string(itName)), zap.Error(err))
				return err
			}
		}
	}
	return nil
}

func (s *Scheduler) Run(ctx context.Context) error {
	err := s.db.Initialize()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup

	httpCtx := &httpclient.Context{
		UserRole: authAPI.ViewerRole,
	}
	httpCtx.Ctx = ctx
	describeJobIntM, err := s.metadataClient.GetConfigMetadata(httpCtx, models.MetadataKeyDescribeJobInterval)
	if err != nil {
		s.logger.Error("failed to set describe interval due to error", zap.Error(err))
	} else {
		if v, ok := describeJobIntM.GetValue().(int); ok {
			s.discoveryIntervalHours = time.Duration(v) * time.Hour
			s.logger.Info("set describe interval", zap.Int64("interval", int64(s.discoveryIntervalHours.Hours())))
		} else {
			s.logger.Error("failed to set describe interval due to invalid type", zap.String("type", string(describeJobIntM.GetType())))
		}
	}

	costDiscoveryJobIntM, err := s.metadataClient.GetConfigMetadata(httpCtx, models.MetadataKeyCostDiscoveryJobInterval)
	if err != nil {
		s.logger.Error("failed to set describe interval due to error", zap.Error(err))
	} else {
		if v, ok := costDiscoveryJobIntM.GetValue().(int); ok {
			s.costDiscoveryIntervalHours = time.Duration(v) * time.Hour
			s.logger.Info("set describe interval", zap.Int64("interval", int64(s.costDiscoveryIntervalHours.Hours())))
		} else {
			s.logger.Error("failed to set describe interval due to invalid type", zap.String("type", string(costDiscoveryJobIntM.GetType())))
		}
	}

	analyticsJobIntM, err := s.metadataClient.GetConfigMetadata(httpCtx, models.MetadataKeyMetricsJobInterval)
	if err != nil {
		s.logger.Error("failed to set describe interval due to error", zap.Error(err))
	} else {
		if v, ok := analyticsJobIntM.GetValue().(int); ok {
			s.analyticsIntervalHours = time.Duration(v) * time.Hour
			s.logger.Info("set analytics interval", zap.Int64("interval", int64(s.analyticsIntervalHours.Hours())))
		} else {
			s.logger.Error("failed to set analytics interval due to invalid type", zap.String("type", string(analyticsJobIntM.GetType())))
		}
	}

	complianceJobIntM, err := s.metadataClient.GetConfigMetadata(httpCtx, models.MetadataKeyComplianceJobInterval)
	if err != nil {
		s.logger.Error("failed to set describe interval due to error", zap.Error(err))
	} else {
		if v, ok := complianceJobIntM.GetValue().(int); ok {
			s.complianceIntervalHours = time.Duration(v) * time.Hour
			s.logger.Info("set compliance interval", zap.Int64("interval", int64(s.complianceIntervalHours.Hours())))
		} else {
			s.logger.Error("failed to set compliance interval due to invalid type", zap.String("type", string(complianceJobIntM.GetType())))
		}
	}

	s.logger.Info("starting scheduler")

	// Describe
	utils.EnsureRunGoroutine(func() {
		s.RunDescribeJobScheduler(ctx)
	})
	utils.EnsureRunGoroutine(func() {
		s.RunDescribeResourceJobs(ctx, false)
	})
	utils.EnsureRunGoroutine(func() {
		s.RunDescribeResourceJobs(ctx, true)
	})
	s.discoveryScheduler.Run(ctx)

	// Inventory summarizer
	utils.EnsureRunGoroutine(func() {
		s.RunAnalyticsJobScheduler(ctx)
	})

	wg.Add(1)
	utils.EnsureRunGoroutine(func() {
		s.logger.Fatal("AnalyticsJobResult consumer exited", zap.Error(s.RunAnalyticsJobResultsConsumer(ctx)))
		wg.Done()
	})

	// Query Runner
	s.queryRunnerScheduler = queryrunnerscheduler.New(
		func(ctx context.Context) error {
			return s.SetupNatsStreams(ctx)
		},
		s.conf,
		s.logger,
		s.db,
		s.jq,
		s.es,
		s.inventoryClient,
		s.complianceClient,
		s.metadataClient,
	)
	s.queryRunnerScheduler.Run(ctx)

	// Compliance
	s.complianceScheduler = compliance.New(
		func(ctx context.Context) error {
			return s.SetupNatsStreams(ctx)
		},
		s.conf,
		s.logger,
		s.complianceClient,
		s.integrationClient,
		s.db,
		s.jq,
		s.es,
		s.complianceIntervalHours,
	)
	s.complianceScheduler.Run(ctx)
	utils.EnsureRunGoroutine(func() {
		s.RunJobSequencer(ctx) // Deprecated
	})

	utils.EnsureRunGoroutine(func() {
		s.RunCheckupJobScheduler(ctx)
	})
	utils.EnsureRunGoroutine(func() {
		s.RunDeletedIntegrationsResourcesCleanup(ctx)
	})
	utils.EnsureRunGoroutine(func() {
		s.RunRemoveResourcesConnectionJobsCleanup()
	})
	wg.Add(1)
	utils.EnsureRunGoroutine(func() {
		s.logger.Fatal("CheckupJobResult consumer exited", zap.Error(s.RunCheckupJobResultsConsumer(ctx)))
		wg.Done()
	})
	utils.EnsureRunGoroutine(func() {
		s.RunScheduledJobCleanup()
	})

	wg.Add(1)
	utils.EnsureRunGoroutine(func() {
		s.logger.Fatal("DescribeJobResults consumer exited", zap.Error(s.RunDescribeJobResultsConsumer(ctx)))
		wg.Done()
	})
	s.logger.Info("starting receiver")
	lis, err := net.Listen("tcp", GRPCServerAddress)
	if err != nil {
		s.logger.Fatal("failed to listen on grpc port", zap.Error(err))
	}

	go func() {
		err := s.grpcServer.Serve(lis)
		if err != nil {
			s.logger.Fatal("failed to serve grpc server", zap.Error(err))
		}
	}()

	go func() {
		if err := httpserver.RegisterAndStart(ctx, s.logger, s.httpServer.Address, s.httpServer); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Fatal("failed to serve http server", zap.Error(err))
		}
	}()

	wg.Wait()

	return nil
}

func (s *Scheduler) RunDeletedIntegrationsResourcesCleanup(ctx context.Context) {
	ticker := ticker.NewTicker(time.Minute*10, time.Second*10)
	defer ticker.Stop()

	for range ticker.C {
		integrations, err := s.integrationClient.ListIntegrations(&httpclient.Context{UserRole: authAPI.AdminRole}, nil)
		if err != nil {
			s.logger.Error("Failed to list sources", zap.Error(err))
			continue
		}
		integrationIds := make([]string, 0)
		for _, integration := range integrations.Integrations {
			integrationIds = append(integrationIds, integration.IntegrationID)
		}
		s.cleanupDescribeResourcesNotInIntegrations(ctx, integrationIds)
	}
}

func (s *Scheduler) RunRemoveResourcesConnectionJobsCleanup() {
	ticker := ticker.NewTicker(2*time.Minute, time.Second*10)
	defer ticker.Stop()

	for range ticker.C {
		jobs, err := s.db.ListDescribeJobsByStatus(api.DescribeResourceJobRemovingResources)
		if err != nil {
			s.logger.Error("Failed to list jobs", zap.Error(err))
			continue
		}

		for _, j := range jobs {
			err = s.cleanupDescribeResourcesForConnectionAndResourceType(j.IntegrationID, j.ResourceType)
			if err != nil {
				s.logger.Error("Failed to remove old resources", zap.Error(err))
				continue
			}

			err = s.db.UpdateDescribeIntegrationJobStatus(j.ID, api.DescribeResourceJobSucceeded, "", "", 0, 0)
			if err != nil {
				s.logger.Error("Failed to update job", zap.Error(err))
				continue
			}
		}
	}
}

func (s *Scheduler) RunScheduledJobCleanup() {
	ticker := ticker.NewTicker(time.Hour, time.Second*10)
	defer ticker.Stop()
	for range ticker.C {
		tOlder := time.Now().AddDate(0, 0, -7)
		err := s.db.CleanupScheduledDescribeIntegrationJobsOlderThan(tOlder)
		if err != nil {
			s.logger.Error("Failed to cleanup describe resource jobs", zap.Error(err))
		}
		tOlderManual := time.Now().AddDate(0, 0, -30)
		err = s.db.CleanupManualDescribeIntegrationJobsOlderThan(tOlderManual)
		if err != nil {
			s.logger.Error("Failed to cleanup describe resource jobs", zap.Error(err))
		}
		err = s.db.CleanupComplianceJobsOlderThan(tOlderManual)
		if err != nil {
			s.logger.Error("Failed to cleanup compliance report jobs", zap.Error(err))
		}
	}
}

func (s *Scheduler) Stop() {
}

func (s *Scheduler) RunCheckupJobScheduler(ctx context.Context) {
	s.logger.Info("Scheduling checkup jobs on a timer")

	t := ticker.NewTicker(JobSchedulingInterval, time.Second*10)
	defer t.Stop()

	for ; ; <-t.C {
		s.scheduleCheckupJob(ctx)
	}
}

func (s *Scheduler) scheduleCheckupJob(ctx context.Context) {
	checkupJob, err := s.db.FetchLastCheckupJob()
	if err != nil {
		s.logger.Error("Failed to find the last job to check for CheckupJob", zap.Error(err))
		CheckupJobsCount.WithLabelValues("failure").Inc()
		return
	}

	if checkupJob == nil ||
		checkupJob.CreatedAt.Add(time.Duration(s.checkupIntervalHours)*time.Hour).Before(time.Now()) {
		job := newCheckupJob()
		err = s.db.AddCheckupJob(&job)
		if err != nil {
			CheckupJobsCount.WithLabelValues("failure").Inc()
			s.logger.Error("Failed to create CheckupJob",
				zap.Uint("jobId", job.ID),
				zap.Error(err),
			)
		}

		bytes, err := json.Marshal(checkup.Job{
			JobID:      job.ID,
			ExecutedAt: job.CreatedAt.UnixMilli(),
		})
		if err != nil {
			CheckupJobsCount.WithLabelValues("failure").Inc()
			s.logger.Error("Failed to marshal a checkup job as json", zap.Error(err), zap.Uint("jobId", job.ID))
		}

		if _, err := s.jq.Produce(ctx, checkup.JobsQueueName, bytes, fmt.Sprintf("job-%d", job.ID)); err != nil {
			CheckupJobsCount.WithLabelValues("failure").Inc()
			s.logger.Error("Failed to enqueue CheckupJob",
				zap.Uint("jobId", job.ID),
				zap.Error(err),
			)
			job.Status = checkupAPI.CheckupJobFailed
			err = s.db.UpdateCheckupJobStatus(job)
			if err != nil {
				s.logger.Error("Failed to update CheckupJob status",
					zap.Uint("jobId", job.ID),
					zap.Error(err),
				)
			}
		}
		CheckupJobsCount.WithLabelValues("successful").Inc()
	}
}

// RunCheckupJobResultsConsumer consumes messages from the checkupJobResultQueue queue.
// It will update the status of the jobs in the database based on the message.
// It will also update the jobs status that are not completed in certain time to FAILED
func (s *Scheduler) RunCheckupJobResultsConsumer(ctx context.Context) error {
	s.logger.Info("Consuming messages from the CheckupJobResultQueue queue")

	consumeCtx, err := s.jq.Consume(
		ctx,
		"checkup-scheduler",
		checkup.StreamName,
		[]string{checkup.ResultsQueueName},
		"checkup-scheduler",
		func(msg jetstream.Msg) {
			var result checkup.JobResult

			if err := json.Unmarshal(msg.Data(), &result); err != nil {
				s.logger.Error("Failed to unmarshal CheckupJobResult results", zap.Error(err))

				// when message cannot be unmarshal, there is no need to consume it again.
				if err := msg.Ack(); err != nil {
					s.logger.Error("Failed to ack the message", zap.Error(err))
				}

				return
			}

			s.logger.Info("Processing CheckupJobResult for Job",
				zap.Uint("jobId", result.JobID),
				zap.String("status", string(result.Status)),
			)

			if err := s.db.UpdateCheckupJob(result.JobID, result.Status, result.Error); err != nil {
				s.logger.Error("Failed to update the status of CheckupJob",
					zap.Uint("jobId", result.JobID),
					zap.Error(err))

				if err = msg.Nak(); err != nil {
					s.logger.Error("Failed to not ack the message", zap.Error(err))
				}

				return
			}

			if err := msg.Ack(); err != nil {
				s.logger.Error("Failed to ack the message", zap.Error(err))
			}
		},
	)
	if err != nil {
		return err
	}

	t := ticker.NewTicker(JobTimeoutCheckInterval, time.Second*10)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			if err := s.db.UpdateCheckupJobsTimedOut(s.checkupIntervalHours); err != nil {
				s.logger.Error("Failed to update timed out CheckupJob", zap.Error(err))
			}
		case <-ctx.Done():
			consumeCtx.Drain()
			consumeCtx.Stop()
			return nil
		}
	}
}

func newCheckupJob() model.CheckupJob {
	return model.CheckupJob{
		Status: checkupAPI.CheckupJobInProgress,
	}
}
