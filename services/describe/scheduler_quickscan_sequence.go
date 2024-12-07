package describe

import (
	"fmt"
	flow "github.com/Azure/go-workflow"
	"github.com/aws/aws-sdk-go-v2/aws"
	apiAuth "github.com/opengovern/og-util/pkg/api"
	"github.com/opengovern/og-util/pkg/httpclient"
	"github.com/opengovern/og-util/pkg/ticker"
	"github.com/opengovern/opencomply/services/describe/db/model"
	"github.com/opengovern/opencomply/services/integration/api/models"
	integration_type "github.com/opengovern/opencomply/services/integration/integration-type"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"time"
)

func (s *Scheduler) ScheduleQuickScanSequence(ctx context.Context) {
	s.logger.Info("Scheduling quick scan sequencer")

	t := ticker.NewTicker(time.Second*10, time.Second*10)
	defer t.Stop()

	for ; ; <-t.C {
		err := s.checkQuickScanSequence(ctx)
		if err != nil {
			s.logger.Error("failed to run checkJobSequences", zap.Error(err))
			continue
		}
	}
}

func (s *Scheduler) checkQuickScanSequence(ctx context.Context) error {
	jobs, err := s.db.FetchCreatedQuickScanSequences()
	if err != nil {
		return err
	}

	for _, job := range jobs {
		go s.RunQuickScanSequence(ctx, job)
	}
	return nil
}

func (s *Scheduler) RunQuickScanSequence(ctx context.Context, job model.QuickScanSequence) {
	s.logger.Info("Started Quick Scan Sequence", zap.Uint("job_id", job.ID))

	var err error
	defer func() {
		if err != nil {
			job.FailureMessage = err.Error()
			job.Status = model.QuickScanSequenceFailed
		} else {
			job.Status = model.QuickScanSequenceFinished
		}

		err = s.db.UpdateQuickScanSequenceStatus(job.ID, job.Status, job.FailureMessage)
		if err != nil {
			s.logger.Error("failed to update quick scan sequence status", zap.Error(err))
		}
	}()

	err = s.db.UpdateQuickScanSequenceStatus(job.ID, model.QuickScanSequenceStarted, "")
	if err != nil {
		s.logger.Error("failed to update quick scan sequence status", zap.Error(err))
		return
	}

	describeDependencies := &DescribeDependencies{
		s:   s,
		job: job,
	}
	complianceQuickRun := &RunQuickComplianceScan{
		s:   s,
		job: job,
	}

	w := new(flow.Workflow)
	w.Add(
		flow.Step(complianceQuickRun).DependsOn(describeDependencies),
		flow.Step(describeDependencies).
			Timeout(10*time.Minute),
	)

	// execute the workflow and block until all steps are terminated
	err = w.Do(context.Background())
	if err != nil {
		s.logger.Error("failed to run quick scan sequence", zap.Error(err))
	}
}

type RunQuickComplianceScan struct {
	s *Scheduler

	job model.QuickScanSequence
}

func (s *RunQuickComplianceScan) Do(ctx context.Context) error {
	jobId, err := s.s.db.CreateComplianceQuickRun(&model.ComplianceQuickRun{
		FrameworkID:    s.job.FrameworkID,
		IntegrationIDs: s.job.IntegrationIDs,
		IncludeResults: s.job.IncludeResults,
		Status:         model.ComplianceQuickRunStatusCreated,
		CreatedBy:      "QuickScanSequencer",
		ParentJobId:    &s.job.ID,
	})
	if err != nil {
		return err
	}

	s.s.logger.Info("Waiting for quick scan", zap.Uint("JobID", s.job.ID))
	err = s.s.db.UpdateQuickScanSequenceStatus(s.job.ID, model.QuickScanSequenceComplianceRunning, "")
	if err != nil {
		return err
	}

	t := ticker.NewTicker(time.Second*5, time.Second*10)
	defer t.Stop()

	for ; ; <-t.C {
		run, err := s.s.db.GetComplianceQuickRunByID(jobId)
		if err != nil {
			return err
		}
		if run.Status == model.ComplianceQuickRunStatusSucceeded || run.Status == model.ComplianceQuickRunStatusFailed ||
			run.Status == model.ComplianceQuickRunStatusCanceled || run.Status == model.ComplianceQuickRunStatusTimeOut {
			break
		}
	}

	return nil
}

type DescribeDependencies struct {
	s *Scheduler

	job model.QuickScanSequence
}

func (s *DescribeDependencies) Do(ctx context.Context) error {
	var clientCtx = &httpclient.Context{UserRole: apiAuth.AdminRole}

	resourceTypes, err := s.s.getFrameworkDependencies(s.job.FrameworkID)
	if err != nil {
		return err
	}

	resp, err := s.s.integrationClient.ListIntegrationsByFilters(clientCtx, models.ListIntegrationsRequest{
		IntegrationID: s.job.IntegrationIDs,
		Cursor:        aws.Int64(1),
		PerPage:       aws.Int64(int64(len(s.job.IntegrationIDs))),
	})
	if err != nil {
		return err
	}

	for _, integration := range resp.Integrations {
		for _, resourceType := range resourceTypes {
			_, err = s.s.describe(integration, resourceType, false, false, false, &s.job.ID, "QuickScanSequencer")
			if err != nil {
				return err
			}
		}
	}

	s.s.logger.Info("Waiting for job dependencies", zap.Uint("JobID", s.job.ID))
	err = s.s.db.UpdateQuickScanSequenceStatus(s.job.ID, model.QuickScanSequenceFetchingDependencies, "")
	if err != nil {
		return err
	}

	t := ticker.NewTicker(time.Second*5, time.Second*10)
	defer t.Stop()

	for ; ; <-t.C {
		jobsNotDone, err := s.s.db.CheckJobsDoneByParentID(s.job.ID)
		if err != nil {
			return err
		}
		if len(jobsNotDone) == 0 {
			break
		}
	}

	return nil
}

func (s *Scheduler) getFrameworkDependencies(frameworkID string) ([]string, error) {
	var clientCtx = &httpclient.Context{UserRole: apiAuth.AdminRole}
	framework, err := s.complianceClient.GetBenchmark(clientCtx, frameworkID)
	if err != nil {
		return nil, err
	}
	controls, err := s.complianceClient.ListControl(clientCtx, framework.Controls, nil)
	if err != nil {
		return nil, err
	}

	tables := make(map[string]bool)
	integrationTypesMap := make(map[string]bool)
	var resourceTypes []string
	for _, control := range controls {
		for _, i := range control.IntegrationType {
			integrationTypesMap[i] = true
		}
		for _, i := range control.Query.IntegrationType {
			integrationTypesMap[i.String()] = true
		}
		for _, table := range control.Query.ListOfTables {
			tables[table] = true
		}
	}

	var integrationTypes []string
	for i, _ := range integrationTypesMap {
		integrationTypes = append(integrationTypes, i)
	}

	for table, _ := range tables {
		resourceType, err := s.findTableResourceTypeInIntegrations(integrationTypes, table)
		if err != nil {
			s.logger.Error("failed to find table resource type",
				zap.Strings("integration_types", integrationTypes),
				zap.String("table", table), zap.Error(err))
		}
		resourceTypes = append(resourceTypes, resourceType)
	}

	return resourceTypes, nil
}

func (s *Scheduler) findTableResourceTypeInIntegrations(integrations []string, table string) (string, error) {
	for _, i := range integrations {
		if value, ok := integration_type.IntegrationTypes[integration_type.ParseType(i)]; ok {
			resourceType := value.GetResourceTypeFromTableName(table)
			if resourceType != "" {
				return resourceType, nil
			}
		} else {
			return "", fmt.Errorf("integration type not found, integration-type: %s", value)
		}
	}
	for _, integrationType := range integration_type.AllIntegrationTypes {
		if value, ok := integration_type.IntegrationTypes[integrationType]; ok {
			resourceType := value.GetResourceTypeFromTableName(table)
			if resourceType != "" {
				return resourceType, nil
			}
		} else {
			return "", fmt.Errorf("integration type not found, integration-type: %s", value)
		}
	}
	return "", fmt.Errorf("resource type not found in integrations, table: %s, integration-types: %v",
		table, integrations)
}
