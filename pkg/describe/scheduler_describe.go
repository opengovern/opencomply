package describe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	apiAuth "github.com/opengovern/og-util/pkg/api"
	"github.com/opengovern/og-util/pkg/httpclient"
	"github.com/opengovern/opengovernance/services/integration-v2/integration-type"
	"github.com/opengovern/opengovernance/services/integration-v2/models"
	"math/rand"
	"time"

	"github.com/opengovern/og-util/pkg/concurrency"
	"github.com/opengovern/og-util/pkg/describe"
	"github.com/opengovern/og-util/pkg/describe/enums"
	"github.com/opengovern/og-util/pkg/ticker"
	opengovernanceTrace "github.com/opengovern/og-util/pkg/trace"
	"github.com/opengovern/opengovernance/pkg/describe/api"
	apiDescribe "github.com/opengovern/opengovernance/pkg/describe/api"
	"github.com/opengovern/opengovernance/pkg/describe/db/model"
	"github.com/opengovern/opengovernance/pkg/describe/es"
	apiIntegration "github.com/opengovern/opengovernance/services/integration-v2/api/models"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

const (
	MaxQueued      = 5000
	MaxIn10Minutes = 5000
)

var ErrJobInProgress = errors.New("job already in progress")

type CloudNativeCall struct {
	dc   model.DescribeConnectionJob
	src  *apiIntegration.Integration
	cred *apiIntegration.Credential
}

func (s *Scheduler) RunDescribeJobScheduler(ctx context.Context) {
	s.logger.Info("Scheduling describe jobs on a timer")

	t := ticker.NewTicker(60*time.Second, time.Second*10)
	defer t.Stop()

	for ; ; <-t.C {
		s.scheduleDescribeJob(ctx)
	}
}

func (s *Scheduler) RunDescribeResourceJobCycle(ctx context.Context, manuals bool) error {
	ctx, span := otel.Tracer(opengovernanceTrace.JaegerTracerName).Start(ctx, opengovernanceTrace.GetCurrentFuncName())
	defer span.End()

	count, err := s.db.CountQueuedDescribeConnectionJobs(manuals)
	if err != nil {
		s.logger.Error("failed to get queue length", zap.String("spot", "CountQueuedDescribeConnectionJobs"), zap.Error(err))
		DescribeResourceJobsCount.WithLabelValues("failure", "queue_length").Inc()
		return err
	}

	if count > MaxQueued {
		DescribePublishingBlocked.WithLabelValues("cloud queued").Set(1)
		s.logger.Error("queue is full", zap.String("spot", "count > MaxQueued"), zap.Error(err))
		return errors.New("queue is full")
	} else {
		DescribePublishingBlocked.WithLabelValues("cloud queued").Set(0)
	}

	count, err = s.db.CountDescribeConnectionJobsRunOverLast10Minutes(manuals)
	if err != nil {
		s.logger.Error("failed to get last hour length", zap.String("spot", "CountDescribeConnectionJobsRunOverLastHour"), zap.Error(err))
		DescribeResourceJobsCount.WithLabelValues("failure", "last_hour_length").Inc()
		return err
	}

	if count > MaxIn10Minutes {
		DescribePublishingBlocked.WithLabelValues("hour queued").Set(1)
		s.logger.Error("too many jobs at last hour", zap.String("spot", "count > MaxQueued"), zap.Error(err))
		return errors.New("too many jobs at last hour")
	} else {
		DescribePublishingBlocked.WithLabelValues("hour queued").Set(0)
	}

	dcs, err := s.db.ListRandomCreatedDescribeConnectionJobs(ctx, int(s.MaxConcurrentCall), manuals)
	if err != nil {
		s.logger.Error("failed to fetch describe resource jobs", zap.String("spot", "ListRandomCreatedDescribeResourceJobs"), zap.Error(err))
		DescribeResourceJobsCount.WithLabelValues("failure", "fetch_error").Inc()
		return err
	}
	s.logger.Info("got the jobs", zap.Int("length", len(dcs)), zap.Int("limit", int(s.MaxConcurrentCall)))

	counts, err := s.db.CountRunningDescribeJobsPerResourceType(manuals)
	if err != nil {
		s.logger.Error("failed to resource type count", zap.String("spot", "CountRunningDescribeJobsPerResourceType"), zap.Error(err))
		DescribeResourceJobsCount.WithLabelValues("failure", "resource_type_count").Inc()
		return err
	}

	rand.Shuffle(len(dcs), func(i, j int) {
		dcs[i], dcs[j] = dcs[j], dcs[i]
	})

	rtCount := map[string]int{}
	for i := 0; i < len(dcs); i++ {
		dc := dcs[i]
		rtCount[dc.ResourceType]++

		maxCount := 25
		if m, ok := es.ResourceRateLimit[dc.ResourceType]; ok {
			maxCount = m
		}

		currentCount := 0
		for _, c := range counts {
			if c.ResourceType == dc.ResourceType {
				currentCount = c.Count
			}
		}
		if rtCount[dc.ResourceType]+currentCount > maxCount {
			dcs = append(dcs[:i], dcs[i+1:]...)
			i--
		}
	}

	s.logger.Info("preparing resource jobs to run", zap.Int("length", len(dcs)))

	wp := concurrency.NewWorkPool(len(dcs))
	integrationsMap := map[string]*apiIntegration.Integration{}
	for _, dc := range dcs {
		var integration *apiIntegration.Integration
		if v, ok := integrationsMap[dc.IntegrationID]; ok {
			integration = v
		} else {
			integration, err = s.integrationClient.GetIntegration(&httpclient.Context{UserRole: apiAuth.AdminRole}, dc.IntegrationID) // TODO: change service
			if err != nil {
				s.logger.Error("failed to get integration", zap.String("spot", "GetIntegrationByUUID"), zap.Error(err), zap.Uint("jobID", dc.ID))
				DescribeResourceJobsCount.WithLabelValues("failure", "get_integration").Inc()
				return err
			}

			integrationsMap[dc.IntegrationID] = integration
		}

		credential, err := s.integrationClient.GetCredential(&httpclient.Context{UserRole: apiAuth.AdminRole}, integration.CredentialID)
		if err != nil {
			s.logger.Error("failed to get credential", zap.String("spot", "GetCredentialByUUID"), zap.Error(err), zap.Uint("jobID", dc.ID))
			DescribeResourceJobsCount.WithLabelValues("failure", "get_credential").Inc()
			return err
		}
		c := CloudNativeCall{
			dc:   dc,
			src:  integration,
			cred: credential,
		}
		wp.AddJob(func() (interface{}, error) {
			err := s.enqueueCloudNativeDescribeJob(ctx, c.dc, c.cred.Secret)
			if err != nil {
				s.logger.Error("Failed to enqueueCloudNativeDescribeConnectionJob", zap.Error(err), zap.Uint("jobID", dc.ID))
				DescribeResourceJobsCount.WithLabelValues("failure", "enqueue").Inc()
				return nil, err
			}
			DescribeResourceJobsCount.WithLabelValues("successful", "").Inc()
			return nil, nil
		})
	}

	res := wp.Run()
	for _, r := range res {
		if r.Error != nil {
			s.logger.Error("failure on calling cloudNative describer", zap.Error(r.Error))
		}
	}

	return nil
}

func (s *Scheduler) RunDescribeResourceJobs(ctx context.Context, manuals bool) {
	t := ticker.NewTicker(time.Second*30, time.Second*10)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if err := s.RunDescribeResourceJobCycle(ctx, manuals); err != nil {
				s.logger.Error("failure while RunDescribeResourceJobCycle", zap.Error(err))
			}
			t.Reset(time.Second*30, time.Second*10)
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) scheduleDescribeJob(ctx context.Context) {
	s.logger.Info("running describe job scheduler")
	integrations, err := s.integrationClient.ListIntegrations(&httpclient.Context{UserRole: apiAuth.AdminRole}, nil)
	if err != nil {
		s.logger.Error("failed to get list of sources", zap.String("spot", "ListSources"), zap.Error(err))
		DescribeJobsCount.WithLabelValues("failure").Inc()
		return
	}

	for _, integration := range integrations.Integrations {
		s.logger.Info("running describe job scheduler for connection", zap.String("IntegrationID", integration.IntegrationID))
		integrationType, err := integration_type.IntegrationTypes[integration.IntegrationType]()
		if err != nil {
			s.logger.Error("failed to get integration type", zap.String("integrationType", string(integration.IntegrationType)),
				zap.String("spot", "ListDiscoveryResourceTypes"), zap.Error(err))
			continue
		}
		resourceTypes, err := integrationType.GetResourceTypesByLabels(integration.Labels)
		if err != nil {
			s.logger.Error("failed to get integration resourceTypes", zap.String("integrationType", string(integration.IntegrationType)),
				zap.String("spot", "ListDiscoveryResourceTypes"), zap.Error(err))
			continue
		}

		s.logger.Info("running describe job scheduler for connection for number of resource types",
			zap.String("integration_id", integration.IntegrationID),
			zap.String("integration_type", string(integration.IntegrationType)),
			zap.String("resource_types", fmt.Sprintf("%v", len(resourceTypes))))
		for _, resourceType := range resourceTypes {
			_, err = s.describe(integration, resourceType, true, false, false, nil, "system")
			if err != nil {
				s.logger.Error("failed to describe connection", zap.String("integration_id", integration.IntegrationID), zap.String("resource_type", resourceType), zap.Error(err))
			}
		}
	}

	if err := s.retryFailedJobs(ctx); err != nil {
		s.logger.Error("failed to retry failed jobs", zap.String("spot", "retryFailedJobs"), zap.Error(err))
		DescribeJobsCount.WithLabelValues("failure").Inc()
		return
	}

	DescribeJobsCount.WithLabelValues("successful").Inc()
}
func (s *Scheduler) retryFailedJobs(ctx context.Context) error {

	ctx, span := otel.Tracer(opengovernanceTrace.JaegerTracerName).Start(ctx, "GetFailedJobs")
	defer span.End()

	fdcs, err := s.db.GetFailedDescribeConnectionJobs(ctx)
	if err != nil {
		s.logger.Error("failed to fetch failed describe resource jobs", zap.String("spot", "GetFailedDescribeResourceJobs"), zap.Error(err))
		return err
	}
	s.logger.Info(fmt.Sprintf("found %v failed jobs before filtering", len(fdcs)))
	retryCount := 0

	for _, failedJob := range fdcs {
		err = s.db.RetryDescribeConnectionJob(failedJob.ID)
		if err != nil {
			return err
		}

		retryCount++
	}

	s.logger.Info(fmt.Sprintf("retrying %v failed jobs", retryCount))
	span.End()
	return nil
}

func (s *Scheduler) describe(integration apiIntegration.Integration, resourceType string, scheduled bool, costFullDiscovery bool,
	removeResources bool, parentId *uint, createdBy string) (*model.DescribeConnectionJob, error) {

	integrationType, err := integration_type.IntegrationTypes[integration.IntegrationType]()
	if err != nil {
		return nil, err
	}
	validResourceTypes, err := integrationType.GetResourceTypesByLabels(integration.Labels)
	if err != nil {
		return nil, err
	}
	valid := false
	for _, rt := range validResourceTypes {
		if rt == resourceType {
			valid = true
		}
	}
	if !valid {
		return nil, fmt.Errorf("invalid resource type for integration type: %s - %s", resourceType, integration.IntegrationType)
	}

	job, err := s.db.GetLastDescribeConnectionJob(integration.IntegrationID, resourceType)
	if err != nil {
		s.logger.Error("failed to get last describe job", zap.String("resource_type", resourceType), zap.String("integration_id", integration.IntegrationID), zap.Error(err))
		DescribeSourceJobsCount.WithLabelValues("failure").Inc()
		return nil, err
	}

	// TODO: get resource type list from integration type and annotations
	if job != nil {
		if scheduled {
			interval := s.discoveryIntervalHours

			if job.UpdatedAt.After(time.Now().Add(-interval)) {
				return nil, nil
			}
		}

		if job.Status == api.DescribeResourceJobCreated ||
			job.Status == api.DescribeResourceJobQueued ||
			job.Status == api.DescribeResourceJobInProgress ||
			job.Status == api.DescribeResourceJobOldResourceDeletion {
			return nil, ErrJobInProgress
		}
	}

	if integration.LastCheck.Before(time.Now().Add(-1 * 24 * time.Hour)) {
		healthCheckedSrc, err := s.integrationClient.IntegrationHealthcheck(&httpclient.Context{
			UserRole: apiAuth.EditorRole,
		}, integration.IntegrationID)
		if err != nil {
			s.logger.Error("failed to get source healthcheck", zap.String("resource_type", resourceType), zap.String("integration_id", integration.IntegrationID), zap.Error(err))
			DescribeSourceJobsCount.WithLabelValues("failure").Inc()
			return nil, err
		}
		integration = *healthCheckedSrc
	}

	if integration.State != models.IntegrationStateActive {
		return nil, errors.New("connection is not active")
	}

	triggerType := enums.DescribeTriggerTypeScheduled

	if !scheduled {
		triggerType = enums.DescribeTriggerTypeManual
	}
	if costFullDiscovery {
		triggerType = enums.DescribeTriggerTypeCostFullDiscovery
	}
	s.logger.Debug("Connection is due for a describe. Creating a job now", zap.String("IntegrationID", integration.IntegrationID), zap.String("resourceType", resourceType))
	daj := newDescribeConnectionJob(integration, resourceType, triggerType, parentId, createdBy)
	if removeResources {
		daj.Status = apiDescribe.DescribeResourceJobRemovingResources
	}
	err = s.db.CreateDescribeConnectionJob(&daj)
	if err != nil {
		s.logger.Error("failed to create describe resource job", zap.String("resource_type", resourceType), zap.String("integration_id", integration.IntegrationID), zap.Error(err))
		DescribeSourceJobsCount.WithLabelValues("failure").Inc()
		return nil, err
	}
	DescribeSourceJobsCount.WithLabelValues("successful").Inc()

	return &daj, nil
}

func newDescribeConnectionJob(a apiIntegration.Integration, resourceType string, triggerType enums.DescribeTriggerType,
	parentId *uint, createdBy string) model.DescribeConnectionJob {
	return model.DescribeConnectionJob{
		CreatedBy:       createdBy,
		ParentID:        parentId,
		IntegrationID:   a.IntegrationID,
		IntegrationType: a.IntegrationType,
		ProviderID:      a.ProviderID,
		TriggerType:     triggerType,
		ResourceType:    resourceType,
		Status:          apiDescribe.DescribeResourceJobCreated,
	}
}

func (s *Scheduler) enqueueCloudNativeDescribeJob(ctx context.Context, dc model.DescribeConnectionJob, cipherText string) error {
	ctx, span := otel.Tracer(opengovernanceTrace.JaegerTracerName).Start(ctx, opengovernanceTrace.GetCurrentFuncName())
	defer span.End()

	integrationType, err := integration_type.IntegrationTypes[dc.IntegrationType]()
	if err != nil {
		s.logger.Error("failed to get integrationType", zap.String("integration_type", string(dc.IntegrationType)), zap.Error(err))
		return err
	}

	s.logger.Debug("enqueueCloudNativeDescribeJob",
		zap.Uint("jobID", dc.ID),
		zap.String("IntegrationID", dc.IntegrationID),
		zap.String("ProviderID", dc.ProviderID),
		zap.String("integrationType", string(dc.IntegrationType)),
		zap.String("resourceType", dc.ResourceType),
	)

	input := describe.DescribeWorkerInput{
		JobEndpoint:               s.describeExternalEndpoint,
		DeliverEndpoint:           s.describeExternalEndpoint,
		EndpointAuth:              true,
		IngestionPipelineEndpoint: s.conf.ElasticSearch.IngestionEndpoint,
		UseOpenSearch:             s.conf.ElasticSearch.IsOpenSearch,

		VaultConfig: s.conf.Vault,

		DescribeJob: describe.DescribeJob{
			JobID:        dc.ID,
			ResourceType: dc.ResourceType,
			SourceID:     dc.IntegrationID,
			AccountID:    dc.ProviderID,
			DescribedAt:  dc.CreatedAt.UnixMilli(),
			SourceType:   dc.IntegrationType,
			CipherText:   cipherText,
			TriggerType:  dc.TriggerType,
			RetryCounter: 0,
		},
	}

	if err := s.db.QueueDescribeConnectionJob(dc.ID); err != nil {
		s.logger.Error("failed to QueueDescribeResourceJob",
			zap.Uint("jobID", dc.ID),
			zap.String("IntegrationID", dc.IntegrationID),
			zap.String("resourceType", dc.ResourceType),
			zap.Error(err),
		)
	}
	isFailed := false
	defer func() {
		if isFailed {
			err := s.db.UpdateDescribeConnectionJobStatus(dc.ID, apiDescribe.DescribeResourceJobFailed, "Failed to invoke lambda", "Failed to invoke lambda", 0, 0)
			if err != nil {
				s.logger.Error("failed to update describe resource job status",
					zap.Uint("jobID", dc.ID),
					zap.String("IntegrationID", dc.IntegrationID),
					zap.String("resourceType", dc.ResourceType),
					zap.Error(err),
				)
			}
		}
	}()

	input.EndpointAuth = false
	input.JobEndpoint = s.describeJobLocalEndpoint
	input.DeliverEndpoint = s.describeDeliverLocalEndpoint
	natsPayload, err := json.Marshal(input)
	if err != nil {
		s.logger.Error("failed to marshal cloud native req", zap.Uint("jobID", dc.ID), zap.String("IntegrationID", dc.IntegrationID), zap.String("resourceType", dc.ResourceType), zap.Error(err))
		isFailed = true
		return fmt.Errorf("failed to marshal cloud native req due to %w", err)
	}

	describerConfig := integrationType.GetDescriberConfiguration()

	topic := describerConfig.NatsScheduledJobsTopic
	if dc.TriggerType == enums.DescribeTriggerTypeManual {
		topic = describerConfig.NatsManualJobsTopic
	}
	seqNum, err := s.jq.Produce(ctx, topic, natsPayload, fmt.Sprintf("%s-%d-%d", dc.IntegrationType, input.DescribeJob.JobID, input.DescribeJob.RetryCounter))
	if err != nil {
		if err.Error() == "nats: no response from stream" {
			err = s.SetupNatsStreams(ctx)
			if err != nil {
				s.logger.Error("Failed to setup nats streams", zap.Error(err))
				return err
			}
			seqNum, err = s.jq.Produce(ctx, topic, natsPayload, fmt.Sprintf("%s-%d-%d", dc.IntegrationType, input.DescribeJob.JobID, input.DescribeJob.RetryCounter))
			if err != nil {
				s.logger.Error("failed to produce message to jetstream",
					zap.Uint("jobID", dc.ID),
					zap.String("IntegrationID", dc.IntegrationID),
					zap.String("resourceType", dc.ResourceType),
					zap.Error(err),
				)
				isFailed = true
				return fmt.Errorf("failed to produce message to jetstream due to %v", err)
			}
		} else {
			s.logger.Error("failed to produce message to jetstream",
				zap.Uint("jobID", dc.ID),
				zap.String("IntegrationID", dc.IntegrationID),
				zap.String("resourceType", dc.ResourceType),
				zap.Error(err),
				zap.String("error message", err.Error()),
			)
			isFailed = true
			return fmt.Errorf("failed to produce message to jetstream due to %v", err)
		}
	}
	if seqNum != nil {
		if err := s.db.UpdateDescribeConnectionJobNatsSeqNum(dc.ID, *seqNum); err != nil {
			s.logger.Error("failed to UpdateDescribeConnectionJobNatsSeqNum",
				zap.Uint("jobID", dc.ID),
				zap.Uint64("seqNum", *seqNum),
				zap.Error(err),
			)
		}
	}

	s.logger.Info("successful job trigger",
		zap.Uint("jobID", dc.ID),
		zap.String("IntegrationID", dc.IntegrationID),
		zap.String("resourceType", dc.ResourceType),
	)

	return nil
}
