package transactions

import (
	"fmt"
	api5 "github.com/kaytu-io/kaytu-engine/pkg/analytics/api"
	"github.com/kaytu-io/kaytu-engine/pkg/describe/api"
	client2 "github.com/kaytu-io/kaytu-engine/pkg/describe/client"
	api3 "github.com/kaytu-io/kaytu-engine/pkg/insight/api"
	client3 "github.com/kaytu-io/kaytu-engine/pkg/onboard/client"
	api4 "github.com/kaytu-io/kaytu-engine/pkg/workspace/api"
	"github.com/kaytu-io/kaytu-engine/pkg/workspace/config"
	"github.com/kaytu-io/kaytu-engine/pkg/workspace/db"
	api2 "github.com/kaytu-io/kaytu-util/pkg/api"
	"github.com/kaytu-io/kaytu-util/pkg/httpclient"
	"github.com/kaytu-io/kaytu-util/pkg/source"
	"golang.org/x/net/context"
	"strconv"
	"strings"
)

type EnsureJobsFinished struct {
	cfg config.Config
}

func NewEnsureJobsFinished(
	cfg config.Config,
) *EnsureJobsFinished {
	return &EnsureJobsFinished{
		cfg: cfg,
	}
}

func (t *EnsureJobsFinished) Requirements() []api4.TransactionID {
	return []api4.TransactionID{api4.Transaction_EnsureJobsRunning}
}

func (t *EnsureJobsFinished) ApplyIdempotent(ctx context.Context, workspace db.Workspace) error {
	hctx := &httpclient.Context{UserRole: api2.InternalRole}
	schedulerURL := strings.ReplaceAll(t.cfg.Scheduler.BaseURL, "%NAMESPACE%", workspace.ID)
	schedulerClient := client2.NewSchedulerServiceClient(schedulerURL)
	onboardURL := strings.ReplaceAll(t.cfg.Onboard.BaseURL, "%NAMESPACE%", workspace.ID)
	onboardClient := client3.NewOnboardServiceClient(onboardURL)

	job, err := schedulerClient.GetAnalyticsJob(hctx, workspace.AnalyticsJobID)
	if err != nil {
		return fmt.Errorf("getting analytics job failed: %v", err)
	}
	if job == nil {
		return ErrTransactionNeedsTime
	}

	if job.Status == api5.JobCreated || job.Status == api5.JobInProgress {
		return ErrTransactionNeedsTime
	}

	isInProgress := false
	for _, insJobIDStr := range strings.Split(workspace.InsightJobsID, ",") {
		insJobID, err := strconv.ParseUint(insJobIDStr, 10, 64)
		if err != nil {
			return err
		}

		job, err := schedulerClient.GetInsightJob(hctx, uint(insJobID))
		if err != nil {
			return err
		}
		if job == nil {
			return ErrTransactionNeedsTime
		}

		if job.Status == api3.InsightJobSucceeded {
			isInProgress = false
			break
		}

		if job.Status == api3.InsightJobCreated || job.Status == api3.InsightJobInProgress {
			isInProgress = true
		}
	}

	if isInProgress {
		return ErrTransactionNeedsTime
	}

	awsSrcs, err := onboardClient.ListSources(hctx, []source.Type{source.CloudAWS})
	if err != nil {
		return err
	}
	if len(awsSrcs) > 0 {
		complianceJob, err := schedulerClient.GetLatestComplianceJobForBenchmark(hctx, "aws_cis_v200")
		if err != nil {
			return err
		}

		if complianceJob.Status != api.ComplianceJobSucceeded && complianceJob.Status != api.ComplianceJobFailed {
			return ErrTransactionNeedsTime
		}
	}

	azureSrcs, err := onboardClient.ListSources(hctx, []source.Type{source.CloudAzure})
	if err != nil {
		return err
	}
	if len(azureSrcs) > 0 {
		complianceJob, err := schedulerClient.GetLatestComplianceJobForBenchmark(hctx, "azure_cis_v200")
		if err != nil {
			return err
		}

		if complianceJob.Status != api.ComplianceJobSucceeded && complianceJob.Status != api.ComplianceJobFailed {
			return ErrTransactionNeedsTime
		}
	}

	return nil
}

func (t *EnsureJobsFinished) RollbackIdempotent(ctx context.Context, workspace db.Workspace) error {
	return nil
}
