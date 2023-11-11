package summarizer

import (
	"context"
	"github.com/kaytu-io/kaytu-engine/pkg/compliance/es"
	types2 "github.com/kaytu-io/kaytu-engine/pkg/compliance/summarizer/types"
	"github.com/kaytu-io/kaytu-engine/pkg/types"
	"github.com/kaytu-io/kaytu-util/pkg/kafka"
	"github.com/kaytu-io/kaytu-util/pkg/kaytu-es-sdk"
	"go.uber.org/zap"
	"time"
)

type Job struct {
	ID          uint
	BenchmarkID string
	CreatedAt   time.Time
}

func (w *Worker) RunJob(j Job) error {
	ctx := context.Background()

	w.logger.Info("Running summarizer",
		zap.Uint("job_id", j.ID),
		zap.String("benchmark_id", j.BenchmarkID),
	)

	paginator, err := es.NewFindingPaginator(w.esClient, types.FindingsIndex, []kaytu.BoolFilter{
		kaytu.NewTermFilter("parentBenchmarks", j.BenchmarkID),
	}, nil)
	if err != nil {
		return err
	}

	w.logger.Info("FindingsIndex paginator ready")

	bs := types2.BenchmarkSummary{
		BenchmarkID:      j.BenchmarkID,
		JobID:            j.ID,
		EvaluatedAtEpoch: j.CreatedAt.Unix(),

		Connections: types2.BenchmarkSummaryResult{
			BenchmarkResult: types2.ResultGroup{
				Result: types2.Result{
					QueryResult:    map[types.ComplianceResult]int{},
					SeverityResult: map[types.FindingSeverity]int{},
					SecurityScore:  0,
				},
				ResourceTypes: map[string]types2.Result{},
				Policies:      map[string]types2.PolicyResult{},
			},
			Connections: map[string]types2.ResultGroup{},
		},
		ResourceCollections: map[string]types2.BenchmarkSummaryResult{},
	}

	for paginator.HasNext() {
		w.logger.Info("Next page")
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}

		w.logger.Info("page size", zap.Int("pageSize", len(page)))
		for _, f := range page {
			bs.AddFinding(f)
		}
	}

	paginator, err = es.NewFindingPaginator(w.esClient, types.ResourceCollectionsFindingsIndex, []kaytu.BoolFilter{
		kaytu.NewTermFilter("parentBenchmarks", j.BenchmarkID),
	}, nil)
	if err != nil {
		return err
	}

	w.logger.Info("ResourceCollectionsFindingsIndex paginator ready")

	for paginator.HasNext() {
		w.logger.Info("Next page")
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}

		w.logger.Info("page size", zap.Int("pageSize", len(page)))
		for _, f := range page {
			bs.AddFinding(f)
		}
	}

	w.logger.Info("Starting to summarizer",
		zap.Uint("job_id", j.ID),
		zap.String("benchmark_id", j.BenchmarkID),
	)

	bs.Summarize()

	w.logger.Info("Summarize done")

	err = kafka.DoSend(w.kafkaProducer, w.config.Kafka.Topic, -1, []kafka.Doc{bs}, w.logger, nil)
	if err != nil {
		return err
	}

	w.logger.Info("Finished summarizer",
		zap.Uint("job_id", j.ID),
		zap.String("benchmark_id", j.BenchmarkID),
	)
	return nil
}
