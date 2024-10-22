package services

import (
	"context"
	"errors"
	complianceClient "github.com/opengovern/opengovernance/pkg/compliance/client"
	"github.com/opengovern/opengovernance/pkg/steampipe-plugin-kaytu/kaytu-sdk/config"
	"github.com/turbot/steampipe-plugin-sdk/v5/connection"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin"
)

func NewComplianceClientCached(c config.ClientConfig, cache *connection.ConnectionCache, ctx context.Context) (complianceClient.ComplianceServiceClient, error) {
	value, ok := cache.Get(ctx, "opengovernance-compliance-service-client")
	if ok {
		return value.(complianceClient.ComplianceServiceClient), nil
	}

	plugin.Logger(ctx).Warn("compliance service client is not cached, creating a new one")

	if c.ComplianceServiceBaseURL == nil {
		plugin.Logger(ctx).Error("compliance service base url is not set")
		return nil, errors.New("compliance service base url is not set")
	}
	client := complianceClient.NewComplianceClient(*c.ComplianceServiceBaseURL)

	cache.Set(ctx, "opengovernance-compliance-service-client", client)

	return client, nil
}
