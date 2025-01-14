package doppler_account

import (
	"encoding/json"
	"github.com/jackc/pgtype"
	flyDescriberLocal "github.com/opengovern/opencomply/services/integration/integration-type/fly-account/configs"
	"github.com/opengovern/opencomply/services/integration/integration-type/fly-account/discovery"
	"github.com/opengovern/opencomply/services/integration/integration-type/fly-account/healthcheck"
	"github.com/opengovern/opencomply/services/integration/integration-type/interfaces"
	"github.com/opengovern/opencomply/services/integration/models"
)

type FlyAccountIntegration struct{}

func (i *FlyAccountIntegration) GetConfiguration() interfaces.IntegrationConfiguration {
	return interfaces.IntegrationConfiguration{
		NatsScheduledJobsTopic:   flyDescriberLocal.JobQueueTopic,
		NatsManualJobsTopic:      flyDescriberLocal.JobQueueTopicManuals,
		NatsStreamName:           flyDescriberLocal.StreamName,
		NatsConsumerGroup:        flyDescriberLocal.ConsumerGroup,
		NatsConsumerGroupManuals: flyDescriberLocal.ConsumerGroupManuals,

		SteampipePluginName: "fly",

		UISpecFileName: "fly-account.json",

		DescriberDeploymentName: flyDescriberLocal.DescriberDeploymentName,
		DescriberRunCommand:     flyDescriberLocal.DescriberRunCommand,
	}
}

func (i *FlyAccountIntegration) HealthCheck(jsonData []byte, providerId string, labels map[string]string, annotations map[string]string) (bool, error) {
	var credentials flyDescriberLocal.IntegrationCredentials
	err := json.Unmarshal(jsonData, &credentials)
	if err != nil {
		return false, err
	}

	var appName string
	if v, ok := labels["AppName"]; ok {
		appName = v
	}
	isHealthy, err := healthcheck.FlyIntegrationHealthcheck(healthcheck.Config{
		Token:   credentials.Token,
		AppName: appName,
	})
	return isHealthy, err
}

func (i *FlyAccountIntegration) DiscoverIntegrations(jsonData []byte) ([]models.Integration, error) {
	var credentials flyDescriberLocal.IntegrationCredentials
	err := json.Unmarshal(jsonData, &credentials)
	if err != nil {
		return nil, err
	}
	var integrations []models.Integration
	apps, err := discovery.FlyIntegrationDiscovery(discovery.Config{
		Token: credentials.Token,
	})
	for _, app := range apps {
		labels := map[string]string{
			"AppName": app.Name,
			"Status":  app.Status,
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
			ProviderID: app.ID,
			Name:       app.Name,
			Labels:     integrationLabelsJsonb,
		})
	}
	return integrations, nil
}

func (i *FlyAccountIntegration) GetResourceTypesByLabels(labels map[string]string) (map[string]*interfaces.ResourceTypeConfiguration, error) {
	resourceTypesMap := make(map[string]*interfaces.ResourceTypeConfiguration)
	for _, resourceType := range flyDescriberLocal.ResourceTypesList {
		resourceTypesMap[resourceType] = nil
	}
	return resourceTypesMap, nil
}

func (i *FlyAccountIntegration) GetResourceTypeFromTableName(tableName string) string {
	if v, ok := flyDescriberLocal.TablesToResourceTypes[tableName]; ok {
		return v
	}
	return ""
}

func (i *FlyAccountIntegration) GetTablesByLabels(map[string]string) ([]string, error) {
	var tables []string
	for t, _ := range flyDescriberLocal.TablesToResourceTypes {
		tables = append(tables, t)
	}
	return tables, nil
}
