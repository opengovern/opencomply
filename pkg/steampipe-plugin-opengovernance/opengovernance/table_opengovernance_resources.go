package opengovernance

import (
	"context"
	og_client "github.com/opengovern/opengovernance/pkg/steampipe-plugin-opengovernance/opengovernance-client"
	"github.com/turbot/steampipe-plugin-sdk/v5/grpc/proto"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin/transform"
	"time"
)

func tablePlatformResources(_ context.Context) *plugin.Table {
	return &plugin.Table{
		Name:        "og_resources",
		Description: "OpenGovernance Resources",
		Cache: &plugin.TableCacheOptions{
			Enabled: false,
		},
		List: &plugin.ListConfig{
			Hydrate: og_client.ListResources,
			KeyColumns: []*plugin.KeyColumn{
				{
					Name:    "resource_type",
					Require: "required",
				},
			},
		},
		Columns: []*plugin.Column{
			{Name: "platform_id", Type: proto.ColumnType_STRING},
			{Name: "resource_id", Transform: transform.FromField("ID"), Type: proto.ColumnType_STRING},
			{Name: "integration_id", Transform: transform.FromField("IntegrationID"), Type: proto.ColumnType_STRING},
			{Name: "integration_type", Transform: transform.FromField("IntegrationType"), Type: proto.ColumnType_STRING},
			{Name: "resource_type", Type: proto.ColumnType_STRING},
			{Name: "resource_name", Transform: transform.FromField("ResourceName"), Type: proto.ColumnType_STRING},
			{Name: "described_by", Transform: transform.FromField("DescribedBy"), Type: proto.ColumnType_STRING},
			{Name: "described_at", Transform: transform.From(fixTime), Type: proto.ColumnType_TIMESTAMP},
			{Name: "description", Type: proto.ColumnType_JSON},
		},
	}
}

func fixTime(ctx context.Context, d *transform.TransformData) (interface{}, error) {
	resource := d.HydrateItem.(og_client.Resource)
	t := time.UnixMilli(resource.CreatedAt)
	return t.Format("2006-01-02T15:04:05"), nil
}

func getResourceRegion(ctx context.Context, d *transform.TransformData) (interface{}, error) {
	resource := d.HydrateItem.(og_client.Resource)
	if len(resource.Metadata.Region) > 0 {
		return resource.Metadata.Region, nil
	}
	return resource.Metadata.Location, nil
}
