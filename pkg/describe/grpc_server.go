package describe

import (
	"context"
	"encoding/json"
	"fmt"
	envoyAuth "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/opengovern/og-util/pkg/describe/enums"
	"github.com/opengovern/og-util/pkg/jq"
	opengovernanceTrace "github.com/opengovern/og-util/pkg/trace"
	"github.com/opengovern/og-util/proto/src/golang"
	"github.com/opengovern/opengovernance/pkg/describe/api"
	"github.com/opengovern/opengovernance/pkg/describe/config"
	"github.com/opengovern/opengovernance/pkg/describe/db"
	integration_type "github.com/opengovern/opengovernance/services/integration-v2/integration-type"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

type GRPCDescribeServer struct {
	db                        db.Database
	jq                        *jq.JobQueue
	conf                      config.SchedulerConfig
	topic                     string
	logger                    *zap.Logger
	DoProcessReceivedMessages bool
	authGrpcClient            envoyAuth.AuthorizationClient

	golang.DescribeServiceServer
}

func NewDescribeServer(
	db db.Database,
	jq *jq.JobQueue,
	authGrpcClient envoyAuth.AuthorizationClient,
	logger *zap.Logger,
	conf config.SchedulerConfig,
) *GRPCDescribeServer {
	return &GRPCDescribeServer{
		db:                        db,
		jq:                        jq,
		logger:                    logger,
		DoProcessReceivedMessages: true,
		authGrpcClient:            authGrpcClient,
		conf:                      conf,
	}
}

func (s *GRPCDescribeServer) SetInProgress(ctx context.Context, req *golang.SetInProgressRequest) (*golang.ResponseOK, error) {
	s.logger.Info("changing job to in progress", zap.Uint("jobId", uint(req.JobId)))
	err := s.db.UpdateDescribeConnectionJobToInProgress(uint(req.JobId)) // TODO this is called too much
	if err != nil {
		return nil, err
	}
	return &golang.ResponseOK{}, nil
}

func (s *GRPCDescribeServer) DeliverResult(ctx context.Context, req *golang.DeliverResultRequest) (*golang.ResponseOK, error) {
	ResultsDeliveredCount.WithLabelValues(req.DescribeJob.SourceType).Inc()

	result, err := json.Marshal(DescribeJobResult{
		JobID:       uint(req.JobId),
		ParentJobID: uint(req.ParentJobId),
		Status:      api.DescribeResourceJobStatus(req.Status),
		Error:       req.Error,
		ErrorCode:   req.ErrorCode,
		DescribeJob: DescribeJob{
			JobID:           uint(req.DescribeJob.JobId),
			ScheduleJobID:   uint(req.DescribeJob.ScheduleJobId),
			ParentJobID:     uint(req.DescribeJob.ParentJobId),
			ResourceType:    req.DescribeJob.ResourceType,
			SourceID:        req.DescribeJob.SourceId,
			AccountID:       req.DescribeJob.AccountId,
			DescribedAt:     req.DescribeJob.DescribedAt,
			IntegrationType: integration_type.ParseType(req.DescribeJob.SourceType),
			CipherText:      req.DescribeJob.ConfigReg,
			TriggerType:     enums.DescribeTriggerType(req.DescribeJob.TriggerType),
			RetryCounter:    uint(req.DescribeJob.RetryCounter),
		},
		DescribedResourceIDs: req.DescribedResourceIds,
	})
	if err != nil {
		return nil, err
	}

	s.logger.Info("Result delivered",
		zap.Uint("jobID", uint(req.JobId)),
		zap.String("status", string(req.Status)),
	)

	ctx, span := otel.Tracer(opengovernanceTrace.JaegerTracerName).Start(ctx, opengovernanceTrace.GetCurrentFuncName())
	defer span.End()

	if _, err := s.jq.Produce(ctx, DescribeResultsQueueName, result, fmt.Sprintf("job-result-%d-%d", req.JobId, req.DescribeJob.RetryCounter)); err != nil {
		s.logger.Error("Failed to publish into nats",
			zap.Uint("jobID", uint(req.JobId)),
			zap.Error(err),
		)
		return nil, err
	}

	s.logger.Info("Publish finished",
		zap.Uint("jobID", uint(req.JobId)),
		zap.String("status", string(req.Status)),
	)
	return &golang.ResponseOK{}, nil
}
