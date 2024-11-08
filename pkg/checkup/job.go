package checkup

import (
	"fmt"
	authAPI "github.com/opengovern/og-util/pkg/api"
	"github.com/opengovern/og-util/pkg/httpclient"
	"strconv"
	"time"

	"github.com/go-errors/errors"
	"github.com/opengovern/opengovernance/pkg/checkup/api"
	"github.com/opengovern/opengovernance/services/integration/client"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

var DoCheckupJobsCount = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "opengovernance",
	Subsystem: "checkup_worker",
	Name:      "do_checkup_jobs_total",
	Help:      "Count of done checkup jobs in checkup-worker service",
}, []string{"queryid", "status"})

var DoCheckupJobsDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: "opengovernance",
	Subsystem: "checkup_worker",
	Name:      "do_checkup_jobs_duration_seconds",
	Help:      "Duration of done checkup jobs in checkup-worker service",
	Buckets:   []float64{5, 60, 300, 600, 1800, 3600, 7200, 36000},
}, []string{"queryid", "status"})

type Job struct {
	JobID      uint
	ExecutedAt int64
}

type JobResult struct {
	JobID  uint
	Status api.CheckupJobStatus
	Error  string
}

func (j Job) Do(integrationClient client.IntegrationServiceClient, logger *zap.Logger) (r JobResult) {
	startTime := time.Now().Unix()
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("paniced with error:", err)
			fmt.Println(errors.Wrap(err, 2).ErrorStack())

			DoCheckupJobsDuration.WithLabelValues(strconv.Itoa(int(j.JobID)), "failure").Observe(float64(time.Now().Unix() - startTime))
			DoCheckupJobsCount.WithLabelValues(strconv.Itoa(int(j.JobID)), "failure").Inc()
			r = JobResult{
				JobID:  j.JobID,
				Status: api.CheckupJobFailed,
				Error:  fmt.Sprintf("paniced: %s", err),
			}
		}
	}()

	// Assume it succeeded unless it fails somewhere
	var (
		status         = api.CheckupJobSucceeded
		firstErr error = nil
	)

	fail := func(err error) {
		DoCheckupJobsDuration.WithLabelValues(strconv.Itoa(int(j.JobID)), "failure").Observe(float64(time.Now().Unix() - startTime))
		DoCheckupJobsCount.WithLabelValues(strconv.Itoa(int(j.JobID)), "failure").Inc()
		status = api.CheckupJobFailed
		if firstErr == nil {
			firstErr = err
		}
	}

	// Healthcheck
	logger.Info("starting healthcheck")
	integrations, err := integrationClient.ListIntegrations(&httpclient.Context{
		UserRole: authAPI.EditorRole,
	}, nil)
	if err != nil {
		logger.Error("failed to get connections list from onboard service", zap.Error(err))
		fail(fmt.Errorf("failed to get connections list from onboard service: %w", err))
	} else {
		for _, integrationObj := range integrations.Integrations {
			if integrationObj.LastCheck != nil && integrationObj.LastCheck.Add(8*time.Hour).After(time.Now()) {
				logger.Info("skipping integration health check", zap.String("integration_id", integrationObj.IntegrationID))
				continue
			}
			logger.Info("checking integration health", zap.String("integration_id", integrationObj.IntegrationID))
			_, err := integrationClient.IntegrationHealthcheck(&httpclient.Context{
				UserRole: authAPI.EditorRole,
			}, integrationObj.IntegrationID)
			if err != nil {
				logger.Error("failed to check integration health", zap.String("integration_id", integrationObj.IntegrationID), zap.Error(err))
				fail(fmt.Errorf("failed to check source health %s: %w", integrationObj.IntegrationID, err))
			}
		}
	}

	errMsg := ""
	if firstErr != nil {
		errMsg = firstErr.Error()
	}
	if status == api.CheckupJobSucceeded {
		DoCheckupJobsDuration.WithLabelValues(strconv.Itoa(int(j.JobID)), "successful").Observe(float64(time.Now().Unix() - startTime))
		DoCheckupJobsCount.WithLabelValues(strconv.Itoa(int(j.JobID)), "successful").Inc()
	}

	return JobResult{
		JobID:  j.JobID,
		Status: status,
		Error:  errMsg,
	}
}
