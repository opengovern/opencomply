package doppler_account

import (
	"encoding/json"
	"github.com/jackc/pgtype"
	"github.com/opengovern/og-util/pkg/integration"
	configs "github.com/opengovern/opencomply/services/integration/integration-type/doppler-account/configs"
	"github.com/opengovern/opencomply/services/integration/integration-type/doppler-account/discovery"
	"github.com/opengovern/opencomply/services/integration/integration-type/doppler-account/healthcheck"
	"github.com/opengovern/opencomply/services/integration/integration-type/interfaces"
	"github.com/opengovern/opencomply/services/integration/models"
)

type Integration struct{}

func (i *Integration) GetConfiguration() interfaces.IntegrationConfiguration {
	return interfaces.IntegrationConfiguration{
		NatsScheduledJobsTopic:   configs.JobQueueTopic,
		NatsManualJobsTopic:      configs.JobQueueTopicManuals,
		NatsStreamName:           configs.StreamName,
		NatsConsumerGroup:        configs.ConsumerGroup,
		NatsConsumerGroupManuals: configs.ConsumerGroupManuals,

		SteampipePluginName: "doppler",

		UISpec: configs.UISpec,

		DescriberDeploymentName: configs.DescriberDeploymentName,
		DescriberRunCommand:     configs.DescriberRunCommand,
	}
}

func (i *Integration) HealthCheck(jsonData []byte, providerId string, labels map[string]string, annotations map[string]string) (bool, error) {
	var credentials configs.IntegrationCredentials
	err := json.Unmarshal(jsonData, &credentials)
	if err != nil {
		return false, err
	}

	isHealthy, err := healthcheck.DopplerIntegrationHealthcheck(healthcheck.Config{
		Token: credentials.Token,
	})
	return isHealthy, err
}

func (i *Integration) DiscoverIntegrations(jsonData []byte) ([]models.Integration, error) {
	var credentials configs.IntegrationCredentials
	err := json.Unmarshal(jsonData, &credentials)
	if err != nil {
		return nil, err
	}
	var integrations []models.Integration
	workplace, err := discovery.DopplerIntegrationDiscovery(discovery.Config{
		Token: credentials.Token,
	})
	labels := map[string]string{
		"BillingEmail":  workplace.BillingEmail,
		"SecurityEmail": workplace.SecurityEmail,
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
		ProviderID: workplace.ID,
		Name:       workplace.Name,
		Labels:     integrationLabelsJsonb,
	})
	return integrations, nil
}

func (i *Integration) GetResourceTypesByLabels(labels map[string]string) (map[string]interfaces.ResourceTypeConfiguration, error) {
	resourceTypesMap := make(map[string]interfaces.ResourceTypeConfiguration)
	for _, resourceType := range configs.ResourceTypesList {
		resourceTypesMap[resourceType] = interfaces.ResourceTypeConfiguration{}
	}
	return resourceTypesMap, nil
}

func (i *Integration) GetResourceTypeFromTableName(tableName string) (string, error) {
	if v, ok := configs.TablesToResourceTypes[tableName]; ok {
		return v, nil
	}
	return "", nil
}

func (i *Integration) GetIntegrationType() integration.Type {
	return configs.IntegrationTypeDopplerAccount
}

func (i *Integration) ListAllTables() (map[string][]string, error) {
	return make(map[string][]string), nil
}
