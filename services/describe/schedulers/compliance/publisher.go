package compliance

import (
	"encoding/json"
	"fmt"

	"github.com/opengovern/og-util/pkg/api"
	"github.com/opengovern/og-util/pkg/httpclient"
	"github.com/opengovern/opengovernance/services/describe/db/model"
	integrationapi "github.com/opengovern/opengovernance/services/integration/api/models"
	"golang.org/x/net/context"

	runner "github.com/opengovern/opengovernance/jobs/compliance-runner-job"
	complianceApi "github.com/opengovern/opengovernance/services/compliance/api"
	"go.uber.org/zap"
)

func (s *JobScheduler) runPublisher(ctx context.Context, manuals bool) error {
	s.logger.Info("runPublisher")
	ctx2 := &httpclient.Context{UserRole: api.AdminRole}
	ctx2.Ctx = ctx
	connectionsMap := make(map[string]*integrationapi.Integration)
	integrations, err := s.integrationClient.ListIntegrations(ctx2, nil)
	if err != nil {
		s.logger.Error("failed to get connections", zap.Error(err))
		return err
	}
	for _, integration := range integrations.Integrations {
		integration := integration
		connectionsMap[integration.IntegrationID] = &integration
	}

	queries, err := s.complianceClient.ListQueries(ctx2)
	if err != nil {
		s.logger.Error("failed to get queries", zap.Error(err))
		return err
	}
	queriesMap := make(map[string]*complianceApi.Query)
	for _, query := range queries {
		query := query
		queriesMap[query.ID] = &query
	}

	for i := 0; i < 10; i++ {
		err := s.db.UpdateTimeoutQueuedRunnerJobs()
		if err != nil {
			s.logger.Error("failed to update timed out runners", zap.Error(err))
		}

		err = s.db.UpdateTimedOutInProgressRunners()
		if err != nil {
			s.logger.Error("failed to update timed out runners", zap.Error(err))
		}

		runners, err := s.db.FetchCreatedRunners(manuals)
		if err != nil {
			s.logger.Error("failed to fetch created runners", zap.Error(err))
			continue
		}

		if len(runners) == 0 {
			break
		}

		for _, it := range runners {
			query, ok := queriesMap[it.QueryID]
			if !ok || query == nil {
				s.logger.Error("query not found", zap.String("queryId", it.QueryID), zap.Uint("runnerId", it.ID))
				_ = s.db.UpdateRunnerJob(it.ID, runner.ComplianceRunnerFailed, it.CreatedAt, nil, "query not found")
				continue
			}

			callers, err := it.GetCallers()
			if err != nil {
				s.logger.Error("failed to get callers", zap.Error(err), zap.Uint("runnerId", it.ID))
				_ = s.db.UpdateRunnerJob(it.ID, runner.ComplianceRunnerFailed, it.CreatedAt, nil, "failed to get callers")
				continue
			}
			var providerID *string
			if it.IntegrationID != nil && *it.IntegrationID != "" {
				if _, ok := connectionsMap[*it.IntegrationID]; ok {
					providerID = &connectionsMap[*it.IntegrationID].ProviderID
				} else {
					_ = s.db.UpdateRunnerJob(it.ID, runner.ComplianceRunnerFailed, it.CreatedAt, nil, "integration does not exist")
					continue
				}
			}
			job := runner.Job{
				ID:          it.ID,
				RetryCount:  it.RetryCount,
				ParentJobID: it.ParentJobID,
				CreatedAt:   it.CreatedAt,
				ExecutionPlan: runner.ExecutionPlan{
					Callers:       callers,
					Query:         *query,
					IntegrationID: it.IntegrationID,
					ProviderID:    providerID,
				},
			}

			jobJson, err := json.Marshal(job)
			if err != nil {
				_ = s.db.UpdateRunnerJob(job.ID, runner.ComplianceRunnerFailed, job.CreatedAt, nil, err.Error())
				s.logger.Error("failed to marshal job", zap.Error(err), zap.Uint("runnerId", it.ID))
				continue
			}

			s.logger.Info("publishing runner", zap.Uint("jobId", job.ID))
			topic := runner.JobQueueTopic
			if it.TriggerType == model.ComplianceTriggerTypeManual {
				topic = runner.JobQueueTopicManuals
			}
			seqNum, err := s.jq.Produce(ctx, topic, jobJson, fmt.Sprintf("job-%d-%d", job.ID, it.RetryCount))
			if err != nil {
				if err.Error() == "nats: no response from stream" {
					err = s.runSetupNatsStreams(ctx)
					if err != nil {
						s.logger.Error("Failed to setup nats streams", zap.Error(err))
						return err
					}
					seqNum, err = s.jq.Produce(ctx, topic, jobJson, fmt.Sprintf("job-%d-%d", job.ID, it.RetryCount))
					if err != nil {
						_ = s.db.UpdateRunnerJob(job.ID, runner.ComplianceRunnerFailed, job.CreatedAt, nil, err.Error())
						s.logger.Error("failed to send job", zap.Error(err), zap.Uint("runnerId", it.ID))
						continue
					}
				} else {
					_ = s.db.UpdateRunnerJob(job.ID, runner.ComplianceRunnerFailed, job.CreatedAt, nil, err.Error())
					s.logger.Error("failed to send job", zap.Error(err), zap.Uint("runnerId", it.ID), zap.String("error message", err.Error()))
					continue
				}
			}

			if seqNum != nil {
				_ = s.db.UpdateRunnerJobNatsSeqNum(job.ID, *seqNum)
			}
			_ = s.db.UpdateRunnerJob(job.ID, runner.ComplianceRunnerQueued, job.CreatedAt, nil, "")
		}
	}

	err = s.db.RetryFailedRunners()
	if err != nil {
		s.logger.Error("failed to retry failed runners", zap.Error(err))
		return err
	}

	return nil
}
