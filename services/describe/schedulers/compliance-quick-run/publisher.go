package compliance_quick_run

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/opengovern/og-util/pkg/api"
	"github.com/opengovern/og-util/pkg/httpclient"
	auditjob "github.com/opengovern/opencomply/jobs/compliance-quick-run-job"
	"github.com/opengovern/opencomply/services/describe/db/model"
	"go.uber.org/zap"
)

func (s *JobScheduler) runPublisher(ctx context.Context) error {
	ctx2 := &httpclient.Context{UserRole: api.AdminRole}
	ctx2.Ctx = ctx

	s.logger.Info("Query Runner publisher started")

	err := s.db.UpdateComplianceJobsTimedOut(false, 20)
	if err != nil {
		s.logger.Error("failed to update timed out query runners", zap.Error(err))
	}

	jobs, err := s.db.ListCreatedComplianceJobs(false)
	if err != nil {
		s.logger.Error("Fetch Created Query Runner Jobs Error", zap.Error(err))
		return err
	}
	s.logger.Info("Fetch Created Query Runner Jobs", zap.Any("Jobs Count", len(jobs)))
	for _, job := range jobs {
		auditJobMsg := auditjob.AuditJob{
			JobID:          job.ID,
			FrameworkID:    job.FrameworkID,
			IntegrationIDs: job.IntegrationIDs,
			IncludeResult:  job.IncludeResults,
		}

		jobJson, err := json.Marshal(auditJobMsg)
		if err != nil {
			_ = s.db.UpdateComplianceJob(job.ID, model.ComplianceJobFailed, "failed to marshal job")
			s.logger.Error("failed to marshal Query Runner Job", zap.Error(err), zap.Uint("runnerId", job.ID))
			continue
		}

		s.logger.Info("publishing audit job", zap.Uint("jobId", job.ID))
		topic := auditjob.JobQueueTopic
		_, err = s.jq.Produce(ctx, topic, jobJson, fmt.Sprintf("job-%d", job.ID))
		if err != nil {
			if err.Error() == "nats: no response from stream" {
				err = s.runSetupNatsStreams(ctx)
				if err != nil {
					s.logger.Error("Failed to setup nats streams", zap.Error(err))
					return err
				}
				_, err = s.jq.Produce(ctx, topic, jobJson, fmt.Sprintf("job-%d", job.ID))
				if err != nil {
					_ = s.db.UpdateComplianceJob(job.ID, model.ComplianceJobFailed, err.Error())
					s.logger.Error("failed to send job", zap.Error(err), zap.Uint("runnerId", job.ID))
					continue
				}
			} else {
				_ = s.db.UpdateComplianceJob(job.ID, model.ComplianceJobFailed, err.Error())
				s.logger.Error("failed to send audit job", zap.Error(err), zap.Uint("runnerId", job.ID), zap.String("error message", err.Error()))
				continue
			}
		}
		_ = s.db.UpdateComplianceJob(job.ID, model.ComplianceJobRunnersInProgress, "")
	}
	return nil
}
