package es

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kaytu-io/kaytu-util/pkg/source"
)

const (
	ConnectionSummaryIndex = "connection_summary"
	CostSummeryIndex       = "cost_summary"
)

type ConnectionReportType string

const (
	ServiceSummary                     ConnectionReportType = "ServicePerSource"
	CategorySummary                    ConnectionReportType = "CategoryPerSource"
	ResourceSummary                    ConnectionReportType = "ResourcesPerSource"
	ResourceTypeSummary                ConnectionReportType = "ResourceTypePerSource"
	LocationConnectionSummary          ConnectionReportType = "LocationPerSource"
	LocationConnectionSummaryHistory   ConnectionReportType = "LocationPerSourceHistory"
	ServiceLocationConnectionSummary   ConnectionReportType = "ServiceLocationPerSource"
	TrendConnectionSummary             ConnectionReportType = "TrendPerSourceHistory"
	ResourceTypeTrendConnectionSummary ConnectionReportType = "ResourceTypeTrendPerSourceHistory"
	CostConnectionSummaryMonthly       ConnectionReportType = "CostPerSource"
	CostConnectionSummaryDaily         ConnectionReportType = "CostPerSourceDaily"
)

type ConnectionResourcesSummary struct {
	SummarizeJobID   uint                 `json:"summarize_job_id"`
	ScheduleJobID    uint                 `json:"schedule_job_id"`
	SourceID         string               `json:"source_id"`
	SourceType       source.Type          `json:"source_type"`
	SourceJobID      uint                 `json:"source_job_id"`
	DescribedAt      int64                `json:"described_at"`
	ResourceCount    int                  `json:"resource_count"`
	LastDayCount     *int                 `json:"last_day_count"`
	LastWeekCount    *int                 `json:"last_week_count"`
	LastQuarterCount *int                 `json:"last_quarter_count"`
	LastYearCount    *int                 `json:"last_year_count"`
	ReportType       ConnectionReportType `json:"report_type"`
}

func (r ConnectionResourcesSummary) KeysAndIndex() ([]string, string) {
	keys := []string{
		r.SourceID,
		string(ResourceSummary),
	}
	if strings.HasSuffix(string(r.ReportType), "History") {
		keys = append(keys, fmt.Sprintf("%d", r.SummarizeJobID))
	}
	return keys, ConnectionSummaryIndex
}

type ConnectionLocationSummary struct {
	SummarizeJobID       uint                 `json:"summarize_job_id"`
	SummarizedAt         int64                `json:"summarized_at"`
	SourceID             string               `json:"source_id"`
	SourceType           source.Type          `json:"source_type"`
	SourceJobID          uint                 `json:"source_job_id"`
	LocationDistribution map[string]int       `json:"location_distribution"`
	ReportType           ConnectionReportType `json:"report_type"`
}

func (r ConnectionLocationSummary) KeysAndIndex() ([]string, string) {
	keys := []string{
		r.SourceID,
		string(LocationConnectionSummary),
	}
	if strings.HasSuffix(string(r.ReportType), "History") {
		keys = append(keys, fmt.Sprintf("%d", r.SummarizeJobID))
	}
	return keys, ConnectionSummaryIndex
}

type ConnectionServiceLocationSummary struct {
	SummarizeJobID       uint                 `json:"summarize_job_id"`
	ScheduleJobID        uint                 `json:"schedule_job_id"`
	SourceID             string               `json:"source_id"`
	SourceType           source.Type          `json:"source_type"`
	SourceJobID          uint                 `json:"source_job_id"`
	ServiceName          string               `json:"service_name"`
	LocationDistribution map[string]int       `json:"location_distribution"`
	ReportType           ConnectionReportType `json:"report_type"`
}

func (r ConnectionServiceLocationSummary) KeysAndIndex() ([]string, string) {
	keys := []string{
		r.ServiceName,
		r.SourceID,
		string(ServiceLocationConnectionSummary),
	}
	if strings.HasSuffix(string(r.ReportType), "History") {
		keys = append(keys, fmt.Sprintf("%d", r.SummarizeJobID))
	}
	return keys, ConnectionSummaryIndex
}

type ConnectionTrendSummary struct {
	SummarizeJobID uint                 `json:"summarize_job_id"`
	ScheduleJobID  uint                 `json:"schedule_job_id"`
	SourceID       string               `json:"source_id"`
	SourceType     source.Type          `json:"source_type"`
	SourceJobID    uint                 `json:"source_job_id"`
	DescribedAt    int64                `json:"described_at"`
	ResourceCount  int                  `json:"resource_count"`
	ReportType     ConnectionReportType `json:"report_type"`
}

func (r ConnectionTrendSummary) KeysAndIndex() ([]string, string) {
	keys := []string{
		strconv.FormatInt(r.DescribedAt, 10),
		r.SourceID,
		string(TrendConnectionSummary),
	}
	if strings.HasSuffix(string(r.ReportType), "History") {
		keys = append(keys, fmt.Sprintf("%d", r.SummarizeJobID))
	}
	return keys, ConnectionSummaryIndex
}

type ConnectionResourceTypeTrendSummary struct {
	SummarizeJobID uint                 `json:"summarize_job_id"`
	ScheduleJobID  uint                 `json:"schedule_job_id"`
	SourceID       string               `json:"source_id"`
	SourceType     source.Type          `json:"source_type"`
	SourceJobID    uint                 `json:"source_job_id"`
	DescribedAt    int64                `json:"described_at"`
	ResourceType   string               `json:"resource_type"`
	ResourceCount  int                  `json:"resource_count"`
	ReportType     ConnectionReportType `json:"report_type"`
}

func (r ConnectionResourceTypeTrendSummary) KeysAndIndex() ([]string, string) {
	keys := []string{
		strconv.FormatInt(r.DescribedAt, 10),
		r.SourceID,
		r.ResourceType,
		string(ResourceTypeTrendConnectionSummary),
	}
	if strings.HasSuffix(string(r.ReportType), "History") {
		keys = append(keys, fmt.Sprintf("%d", r.SummarizeJobID))
	}
	return keys, ConnectionSummaryIndex
}

type ConnectionResourceTypeSummary struct {
	SummarizeJobID   uint                 `json:"summarize_job_id"`
	ScheduleJobID    uint                 `json:"schedule_job_id"`
	SourceID         string               `json:"source_id"`
	SourceJobID      uint                 `json:"source_job_id"`
	ResourceType     string               `json:"resource_type"`
	SourceType       source.Type          `json:"source_type"`
	ResourceCount    int                  `json:"resource_count"`
	LastDayCount     *int                 `json:"last_day_count"`
	LastWeekCount    *int                 `json:"last_week_count"`
	LastQuarterCount *int                 `json:"last_quarter_count"`
	LastYearCount    *int                 `json:"last_year_count"`
	ReportType       ConnectionReportType `json:"report_type"`
}

func (r ConnectionResourceTypeSummary) KeysAndIndex() ([]string, string) {
	keys := []string{
		r.ResourceType,
		r.SourceID,
		string(ResourceTypeSummary),
	}
	if strings.HasSuffix(string(r.ReportType), "History") {
		keys = append(keys, fmt.Sprintf("%d", r.SummarizeJobID))
	}
	return keys, ConnectionSummaryIndex
}

type ConnectionServiceSummary struct {
	SummarizeJobID   uint                 `json:"summarize_job_id"`
	ScheduleJobID    uint                 `json:"schedule_job_id"`
	SourceID         string               `json:"source_id"`
	SourceJobID      uint                 `json:"source_job_id"`
	ServiceName      string               `json:"service_name"`
	ResourceType     string               `json:"resource_type"`
	SourceType       source.Type          `json:"source_type"`
	DescribedAt      int64                `json:"described_at"`
	ResourceCount    int                  `json:"resource_count"`
	LastDayCount     *int                 `json:"last_day_count"`
	LastWeekCount    *int                 `json:"last_week_count"`
	LastQuarterCount *int                 `json:"last_quarter_count"`
	LastYearCount    *int                 `json:"last_year_count"`
	ReportType       ConnectionReportType `json:"report_type"`
}

func (r ConnectionServiceSummary) KeysAndIndex() ([]string, string) {
	keys := []string{
		r.ServiceName,
		r.SourceID,
		string(ServiceSummary),
	}
	if strings.HasSuffix(string(r.ReportType), "History") {
		keys = append(keys, fmt.Sprintf("%d", r.SummarizeJobID))
	}
	return keys, ConnectionSummaryIndex
}

type ConnectionCategorySummary struct {
	SummarizeJobID   uint                 `json:"summarize_job_id"`
	ScheduleJobID    uint                 `json:"schedule_job_id"`
	SourceID         string               `json:"source_id"`
	SourceJobID      uint                 `json:"source_job_id"`
	CategoryName     string               `json:"category_name"`
	ResourceType     string               `json:"resource_type"`
	SourceType       source.Type          `json:"source_type"`
	DescribedAt      int64                `json:"described_at"`
	ResourceCount    int                  `json:"resource_count"`
	LastDayCount     *int                 `json:"last_day_count"`
	LastWeekCount    *int                 `json:"last_week_count"`
	LastQuarterCount *int                 `json:"last_quarter_count"`
	LastYearCount    *int                 `json:"last_year_count"`
	ReportType       ConnectionReportType `json:"report_type"`
}

func (r ConnectionCategorySummary) KeysAndIndex() ([]string, string) {
	keys := []string{
		r.CategoryName,
		r.SourceID,
		string(CategorySummary),
	}
	if strings.HasSuffix(string(r.ReportType), "History") {
		keys = append(keys, fmt.Sprintf("%d", r.SummarizeJobID))
	}
	return keys, ConnectionSummaryIndex
}
