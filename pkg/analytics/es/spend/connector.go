package spend

import (
	"github.com/kaytu-io/kaytu-util/pkg/source"
)

const (
	AnalyticsSpendConnectorSummaryIndex = "analytics_spend_connector_summary"
)

type PerConnectorMetricTrendSummary struct {
	DateEpoch                  int64       `json:"date_epoch"`
	Connector                  source.Type `json:"connector"`
	CostValue                  float64     `json:"cost_value"`
	TotalConnections           int64       `json:"total_connections"`
	TotalSuccessfulConnections int64       `json:"total_successful_connections"`
}

type ConnectorMetricTrendSummary struct {
	MetricID       string  `json:"metric_id"`
	MetricName     string  `json:"metric_name"`
	TotalCostValue float64 `json:"total_cost_value"`

	Date        string `json:"date"`
	DateEpoch   int64  `json:"date_epoch"`
	Month       string `json:"month"`
	Year        string `json:"year"`
	PeriodStart int64  `json:"period_start"`
	PeriodEnd   int64  `json:"period_end"`
	EvaluatedAt int64  `json:"evaluated_at"`

	Connectors map[string]PerConnectorMetricTrendSummary `json:"connectors"`

	// Deprecated
	Connector source.Type `json:"connector"`
	// Deprecated
	CostValue float64 `json:"cost_value"`
	// Deprecated
	TotalConnections int64 `json:"total_connections"`
	// Deprecated
	TotalSuccessfulConnections int64 `json:"total_successful_connections"`
}

func (r ConnectorMetricTrendSummary) KeysAndIndex() ([]string, string) {
	keys := []string{
		r.Date,
		r.MetricID,
	}
	return keys, AnalyticsSpendConnectorSummaryIndex
}
