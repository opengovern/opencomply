package spend

import (
	"github.com/kaytu-io/kaytu-util/pkg/source"
)

const (
	AnalyticsSpendConnectorSummaryIndex = "analytics_spend_connector_summary"
)

type ConnectorMetricTrendSummary struct {
	Connector source.Type `json:"connector"`
	Date      string      `json:"date"`
	MetricID  string      `json:"metric_id"`
	CostValue float64     `json:"cost_value"`
}

func (r ConnectorMetricTrendSummary) KeysAndIndex() ([]string, string) {
	keys := []string{
		r.Date,
		r.Connector.String(),
		r.MetricID,
	}
	return keys, AnalyticsSpendConnectorSummaryIndex
}
