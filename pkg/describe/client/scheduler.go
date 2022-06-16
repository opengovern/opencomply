package client

import (
	"fmt"
	"net/http"

	compliance "gitlab.com/keibiengine/keibi-engine/pkg/compliance-report/api"
	"gitlab.com/keibiengine/keibi-engine/pkg/internal/httpclient"
	inventory "gitlab.com/keibiengine/keibi-engine/pkg/inventory/api"
	"gitlab.com/keibiengine/keibi-engine/pkg/onboard/api"
)

type SchedulerServiceClient interface {
	GetSource(ctx *httpclient.Context, sourceID string) (*api.Source, error)
	ListComplianceReportJobs(ctx *httpclient.Context, sourceID string, filter *inventory.TimeRangeFilter) ([]*compliance.ComplianceReport, error)
	GetLastComplianceReportID(ctx *httpclient.Context) (uint, error)
}

type schedulerClient struct {
	baseURL string
}

func NewSchedulerServiceClient(baseURL string) SchedulerServiceClient {
	return &schedulerClient{baseURL: baseURL}
}

func (s *schedulerClient) GetSource(ctx *httpclient.Context, sourceID string) (*api.Source, error) {
	url := fmt.Sprintf("%s/api/v1/sources/%s", s.baseURL, sourceID)

	var source api.Source
	if err := httpclient.DoRequest(http.MethodGet, url, ctx.ToHeaders(), nil, &source); err != nil {
		return nil, err
	}
	return &source, nil
}

func (s *schedulerClient) ListComplianceReportJobs(ctx *httpclient.Context, sourceID string, filter *inventory.TimeRangeFilter) ([]*compliance.ComplianceReport, error) {
	url := fmt.Sprintf("%s/api/v1/sources/%s/jobs/compliance", s.baseURL, sourceID)
	if filter != nil {
		url = fmt.Sprintf("%s?from=%d&to=%d", url, filter.From, filter.To)
	}

	reports := []*compliance.ComplianceReport{}
	if err := httpclient.DoRequest(http.MethodGet, url, nil, nil, &reports); err != nil {
		return nil, err
	}
	return reports, nil
}

func (s *schedulerClient) GetLastComplianceReportID(ctx *httpclient.Context) (uint, error) {
	url := fmt.Sprintf("%s/api/v1/compliance/report/last/completed", s.baseURL)

	var id uint
	if err := httpclient.DoRequest(http.MethodGet, url, ctx.ToHeaders(), nil, &id); err != nil {
		return 0, err
	}
	return id, nil
}
