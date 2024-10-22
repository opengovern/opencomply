package opengovernance

import (
	"context"
	kaytu_client "github.com/opengovern/opengovernance/pkg/steampipe-plugin-opengovernance/opengovernance-client"
	"github.com/turbot/steampipe-plugin-sdk/v5/grpc/proto"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin"
)

func tablePlatformCost(_ context.Context) *plugin.Table {
	return &plugin.Table{
		Name:        "kaytu_cost",
		Description: "Account-level cost of connections onboarded into Kaytu platform",
		Cache: &plugin.TableCacheOptions{
			Enabled: false,
		},
		List: &plugin.ListConfig{
			Hydrate: kaytu_client.ListCostSummary,
		},
		Columns: []*plugin.Column{
			{Name: "connection_id", Type: proto.ColumnType_STRING},
			{Name: "connection_name", Type: proto.ColumnType_STRING},
			{Name: "connector", Type: proto.ColumnType_STRING},
			{Name: "date", Type: proto.ColumnType_STRING},
			{Name: "date_epoch", Type: proto.ColumnType_INT},
			{Name: "month", Type: proto.ColumnType_STRING},
			{Name: "year", Type: proto.ColumnType_STRING},
			{Name: "metric_id", Type: proto.ColumnType_STRING},
			{Name: "metric_name", Type: proto.ColumnType_STRING},
			{Name: "cost_value", Type: proto.ColumnType_DOUBLE},
			{Name: "period_start", Type: proto.ColumnType_TIMESTAMP},
			{Name: "period_end", Type: proto.ColumnType_TIMESTAMP},
		},
	}
}
