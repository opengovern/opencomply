package cloudflare_account

import (
	"encoding/json"
	"github.com/jackc/pgtype"
	cloudflareDescriberLocal "github.com/opengovern/opengovernance/services/integration/integration-type/cloudflare-account/configs"
	"github.com/opengovern/opengovernance/services/integration/integration-type/cloudflare-account/discovery"
	"github.com/opengovern/opengovernance/services/integration/integration-type/cloudflare-account/healthcheck"
	"github.com/opengovern/opengovernance/services/integration/integration-type/interfaces"
	"github.com/opengovern/opengovernance/services/integration/models"
)

type CloudFlareAccountIntegration struct{}

func (i *CloudFlareAccountIntegration) GetConfiguration() interfaces.IntegrationConfiguration {
	return interfaces.IntegrationConfiguration{
		NatsScheduledJobsTopic: cloudflareDescriberLocal.JobQueueTopic,
		NatsManualJobsTopic:    cloudflareDescriberLocal.JobQueueTopicManuals,
		NatsStreamName:         cloudflareDescriberLocal.StreamName,

		SteampipePluginName: "cloudflare",

		UISpecFileName: "cloudflare-account.json",
	}
}

func (i *CloudFlareAccountIntegration) HealthCheck(jsonData []byte, providerId string, labels map[string]string, annotations map[string]string) (bool, error) {
	var credentials cloudflareDescriberLocal.IntegrationCredentials
	err := json.Unmarshal(jsonData, &credentials)
	if err != nil {
		return false, err
	}

	isHealthy, err := healthcheck.CloudflareIntegrationHealthcheck(healthcheck.Config{
		Token:    credentials.Token,
		MemberID: credentials.MemberID,
	})
	return isHealthy, err
}

func (i *CloudFlareAccountIntegration) DiscoverIntegrations(jsonData []byte) ([]models.Integration, error) {
	var credentials cloudflareDescriberLocal.IntegrationCredentials
	err := json.Unmarshal(jsonData, &credentials)
	if err != nil {
		return nil, err
	}
	var integrations []models.Integration
	user, err := discovery.CloudflareIntegrationDiscovery(discovery.Config{
		Token:    credentials.Token,
		MemberID: credentials.MemberID,
	})
	labels := map[string]string{
		"Status": user.Status,
	}
	labelsJsonData, err := json.Marshal(labels)
	if err != nil {
		return nil, err
	}
	integrationLabelsJsonb := pgtype.JSONB{}
	err = integrationLabelsJsonb.Set(labelsJsonData)
	if err != nil {
		return nil, err
	}
	integrations = append(integrations, models.Integration{
		ProviderID: user.ID,
		Name:       user.Name,
		Labels:     integrationLabelsJsonb,
	})
	return integrations, nil
}

func (i *CloudFlareAccountIntegration) GetResourceTypesByLabels(map[string]string) ([]string, error) {
	return cloudflareDescriberLocal.ResourceTypesList, nil
}

func (i *CloudFlareAccountIntegration) GetResourceTypeFromTableName(tableName string) string {
	if v, ok := cloudflareDescriberLocal.TablesToResourceTypes[tableName]; ok {
		return v
	}

	return ""
}