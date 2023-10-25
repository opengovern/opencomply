package resource

import (
	"github.com/google/uuid"
	"github.com/kaytu-io/kaytu-util/pkg/source"
	"strconv"
)

const (
	AnalyticsConnectionSummaryIndex                    = "analytics_connection_summary"
	ResourceCollectionsAnalyticsConnectionSummaryIndex = "rc_analytics_connection_summary"
)

type PerConnectionMetricTrendSummary struct {
	Connector       source.Type `json:"connector"`
	ConnectionID    string      `json:"connection_id"`
	ConnectionName  string      `json:"connection_name"`
	ResourceCount   int         `json:"resource_count"`
	IsJobSuccessful bool        `json:"is_job_successful"`
}

type ConnectionMetricTrendSummaryResult struct {
	TotalResourceCount int                                        `json:"total_resource_count"`
	Connections        map[string]PerConnectionMetricTrendSummary `json:"connections"`
}

type ConnectionMetricTrendSummary struct {
	EvaluatedAt int64  `json:"evaluated_at"`
	Date        string `json:"date"`
	Month       string `json:"month"`
	Year        string `json:"year"`
	MetricID    string `json:"metric_id"`
	MetricName  string `json:"metric_name"`

	Connections         *ConnectionMetricTrendSummaryResult           `json:"connections,omitempty"`
	ResourceCollections map[string]ConnectionMetricTrendSummaryResult `json:"resource_collections,omitempty"`

	// Deprecated
	Connector source.Type `json:"connector"`
	// Deprecated
	ConnectionID uuid.UUID `json:"connection_id"`
	// Deprecated
	ConnectionName string `json:"connection_name"`
	// Deprecated
	ResourceCount int `json:"resource_count"`
	// Deprecated
	IsJobSuccessful bool `json:"is_job_successful"`
	// Deprecated
	ResourceCollection *string `json:"resource_collection"`
}

func (r ConnectionMetricTrendSummary) KeysAndIndex() ([]string, string) {
	keys := []string{
		strconv.FormatInt(r.EvaluatedAt, 10),
		r.MetricID,
	}
	idx := AnalyticsConnectionSummaryIndex
	if r.ResourceCollections != nil {
		idx = ResourceCollectionsAnalyticsConnectionSummaryIndex
	}
	return keys, idx
}
