package doppler_account

import (
	"encoding/json"
	"github.com/jackc/pgtype"
	"github.com/opengovern/opencomply/services/integration/integration-type/interfaces"
	tailScaleDescriberLocal "github.com/opengovern/opencomply/services/integration/integration-type/tailscale-account/configs"
	"github.com/opengovern/opencomply/services/integration/integration-type/tailscale-account/discovery"
	"github.com/opengovern/opencomply/services/integration/integration-type/tailscale-account/healthcheck"
	"github.com/opengovern/opencomply/services/integration/models"
)

type TailScaleAccountIntegration struct{}

func (i *TailScaleAccountIntegration) GetConfiguration() interfaces.IntegrationConfiguration {
	return interfaces.IntegrationConfiguration{
		NatsScheduledJobsTopic:   tailScaleDescriberLocal.JobQueueTopic,
		NatsManualJobsTopic:      tailScaleDescriberLocal.JobQueueTopicManuals,
		NatsStreamName:           tailScaleDescriberLocal.StreamName,
		NatsConsumerGroup:        tailScaleDescriberLocal.ConsumerGroup,
		NatsConsumerGroupManuals: tailScaleDescriberLocal.ConsumerGroupManuals,

		SteampipePluginName: "tailscale",

		UISpecFileName: "tailscale-account.json",

		DescriberDeploymentName: tailScaleDescriberLocal.DescriberDeploymentName,
		DescriberRunCommand:     tailScaleDescriberLocal.DescriberRunCommand,
	}
}

func (i *TailScaleAccountIntegration) HealthCheck(jsonData []byte, providerId string, labels map[string]string, annotations map[string]string) (bool, error) {
	var credentials tailScaleDescriberLocal.IntegrationCredentials
	err := json.Unmarshal(jsonData, &credentials)
	if err != nil {
		return false, err
	}

	isHealthy, err := healthcheck.TailScaleIntegrationHealthcheck(healthcheck.Config{
		Token: credentials.Token,
	})
	return isHealthy, err
}

func (i *TailScaleAccountIntegration) DiscoverIntegrations(jsonData []byte) ([]models.Integration, error) {
	var credentials tailScaleDescriberLocal.IntegrationCredentials
	err := json.Unmarshal(jsonData, &credentials)
	if err != nil {
		return nil, err
	}
	var integrations []models.Integration
	user, err := discovery.TailScaleIntegrationDiscovery(discovery.Config{
		Token: credentials.Token,
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
		Name:       user.LoginName,
		Labels:     integrationLabelsJsonb,
	})
	return integrations, nil
}

func (i *TailScaleAccountIntegration) GetResourceTypesByLabels(labels map[string]string) (map[string]*interfaces.ResourceTypeConfiguration, error) {
	resourceTypesMap := make(map[string]*interfaces.ResourceTypeConfiguration)
	for _, resourceType := range tailScaleDescriberLocal.ResourceTypesList {
		resourceTypesMap[resourceType] = nil
	}
	return resourceTypesMap, nil
}

func (i *TailScaleAccountIntegration) GetResourceTypeFromTableName(tableName string) string {
	if v, ok := tailScaleDescriberLocal.TablesToResourceTypes[tableName]; ok {
		return v
	}
	return ""
}

func (i *TailScaleAccountIntegration) GetTablesByLabels(map[string]string) ([]string, error) {
	var tables []string
	for t, _ := range tailScaleDescriberLocal.TablesToResourceTypes {
		tables = append(tables, t)
	}
	return tables, nil
}
