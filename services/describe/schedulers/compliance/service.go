package compliance

import (
	"context"
	authAPI "github.com/opengovern/og-util/pkg/api"
	"github.com/opengovern/og-util/pkg/httpclient"
	"time"

	"github.com/opengovern/og-util/pkg/jq"

	"github.com/opengovern/og-util/pkg/opengovernance-es-sdk"
	"github.com/opengovern/og-util/pkg/ticker"
	"github.com/opengovern/opencomply/pkg/utils"
	"github.com/opengovern/opencomply/services/compliance/client"
	"github.com/opengovern/opencomply/services/describe/config"
	"github.com/opengovern/opencomply/services/describe/db"
	integrationClient "github.com/opengovern/opencomply/services/integration/client"
	"go.uber.org/zap"
)

const JobSchedulingInterval = 1 * time.Minute
const CleanupInterval = 10 * time.Minute

type JobScheduler struct {
	runSetupNatsStreams     func(context.Context) error
	conf                    config.SchedulerConfig
	logger                  *zap.Logger
	complianceClient        client.ComplianceServiceClient
	integrationClient       integrationClient.IntegrationServiceClient
	db                      db.Database
	jq                      *jq.JobQueue
	esClient                opengovernance.Client
	complianceIntervalHours time.Duration
}

func New(
	runSetupNatsStreams func(context.Context) error,
	conf config.SchedulerConfig,
	logger *zap.Logger,
	complianceClient client.ComplianceServiceClient,
	integrationClient integrationClient.IntegrationServiceClient,
	db db.Database,
	jq *jq.JobQueue,
	esClient opengovernance.Client,
	complianceIntervalHours time.Duration,
) *JobScheduler {
	return &JobScheduler{
		runSetupNatsStreams:     runSetupNatsStreams,
		conf:                    conf,
		logger:                  logger,
		complianceClient:        complianceClient,
		integrationClient:       integrationClient,
		db:                      db,
		jq:                      jq,
		esClient:                esClient,
		complianceIntervalHours: complianceIntervalHours,
	}
}

func (s *JobScheduler) Run(ctx context.Context) {
	utils.EnsureRunGoroutine(func() {
		s.RunScheduler()
	})
	utils.EnsureRunGoroutine(func() {
		s.RunEnqueueRunnersCycle()
	})
	utils.EnsureRunGoroutine(func() {
		s.RunPublisher(ctx, false)
	})
	utils.EnsureRunGoroutine(func() {
		s.RunPublisher(ctx, true)
	})
	utils.EnsureRunGoroutine(func() {
		s.RunSummarizer(ctx, false)
	})
	utils.EnsureRunGoroutine(func() {
		s.RunSummarizer(ctx, true)
	})
	utils.EnsureRunGoroutine(func() {
		s.logger.Fatal("ComplianceReportJobResult consumer exited", zap.Error(s.RunComplianceReportJobResultsConsumer(ctx)))
	})
	utils.EnsureRunGoroutine(func() {
		s.logger.Fatal("ComplianceSummarizerResult consumer exited", zap.Error(s.RunComplianceSummarizerResultsConsumer(ctx)))
	})
	utils.EnsureRunGoroutine(func() {
		s.CleanupComplianceResults(ctx)
	})
}

func (s *JobScheduler) CleanupComplianceResults(ctx context.Context) {
	s.logger.Info("Cleanup compliance results scheduler")

	t := ticker.NewTicker(CleanupInterval, time.Second*10)
	defer t.Stop()

	for ; ; <-t.C {
		integrations, err := s.integrationClient.ListIntegrations(&httpclient.Context{UserRole: authAPI.AdminRole}, nil)
		if err != nil {
			s.logger.Error("Failed to list sources", zap.Error(err))
			continue
		}
		integrationIds := make([]string, 0)
		for _, integration := range integrations.Integrations {
			integrationIds = append(integrationIds, integration.IntegrationID)
		}
		s.cleanupComplianceResultsNotInIntegrations(ctx, integrationIds)
	}
}

func (s *JobScheduler) RunScheduler() {
	s.logger.Info("Scheduling compliance jobs on a timer")

	t := ticker.NewTicker(JobSchedulingInterval, time.Second*10)
	defer t.Stop()

	for ; ; <-t.C {
		if err := s.runScheduler(); err != nil {
			s.logger.Error("failed to run compliance scheduler", zap.Error(err))
			ComplianceJobsCount.WithLabelValues("failure").Inc()
			continue
		}
	}
}

func (s JobScheduler) RunEnqueueRunnersCycle() {
	s.logger.Info("enqueue runners cycle on a timer")

	t := ticker.NewTicker(JobSchedulingInterval, time.Second*10)
	defer t.Stop()

	for ; ; <-t.C {
		if err := s.enqueueRunnersCycle(); err != nil {
			s.logger.Error("failed to run enqueue runners cycle", zap.Error(err))
			continue
		}
	}
}

func (s *JobScheduler) RunPublisher(ctx context.Context, manuals bool) {
	s.logger.Info("Scheduling publisher on a timer")

	t := ticker.NewTicker(JobSchedulingInterval, time.Second*10)
	defer t.Stop()

	for ; ; <-t.C {
		if err := s.runPublisher(ctx, manuals); err != nil {
			s.logger.Error("failed to run compliance publisher", zap.Error(err))
			ComplianceJobsCount.WithLabelValues("failure").Inc()
			continue
		}
	}
}

func (s *JobScheduler) RunSummarizer(ctx context.Context, manuals bool) {
	s.logger.Info("Scheduling compliance summarizer on a timer")

	t := ticker.NewTicker(SummarizerSchedulingInterval, time.Second*10)
	defer t.Stop()

	for ; ; <-t.C {
		if err := s.runSummarizer(ctx, manuals); err != nil {
			s.logger.Error("failed to run compliance summarizer", zap.Error(err))
			ComplianceJobsCount.WithLabelValues("failure").Inc()
			continue
		}
	}
}
