package entra_id_directory

import (
	"encoding/json"
	"github.com/opengovern/og-util/pkg/integration"
	entraidDescriberLocal "github.com/opengovern/opengovernance/services/integration/integration-type/entra-id-directory/configs"
	"github.com/opengovern/opengovernance/services/integration/integration-type/entra-id-directory/discovery"
	"github.com/opengovern/opengovernance/services/integration/integration-type/entra-id-directory/healthcheck"
	"github.com/opengovern/opengovernance/services/integration/integration-type/interfaces"
	"github.com/opengovern/opengovernance/services/integration/models"
)

const (
	IntegrationTypeEntraIdDirectory integration.Type = "ENTRA_ID_DIRECTORY"
)

type EntraIdDirectoryIntegration struct{}

func CreateAzureSubscriptionIntegration() (interfaces.IntegrationType, error) {
	return &EntraIdDirectoryIntegration{}, nil
}

func (i *EntraIdDirectoryIntegration) GetDescriberConfiguration() interfaces.DescriberConfiguration {
	return interfaces.DescriberConfiguration{
		NatsScheduledJobsTopic: entraidDescriberLocal.JobQueueTopic,
		NatsManualJobsTopic:    entraidDescriberLocal.JobQueueTopicManuals,
		NatsStreamName:         entraidDescriberLocal.StreamName,
	}
}

func (i *EntraIdDirectoryIntegration) GetAnnotations(jsonData []byte) (map[string]string, error) {
	annotations := make(map[string]string)

	return annotations, nil
}

func (i *EntraIdDirectoryIntegration) GetLabels(jsonData []byte) (map[string]string, error) {
	annotations := make(map[string]string)

	return annotations, nil
}

func (i *EntraIdDirectoryIntegration) HealthCheck(jsonData []byte, providerId string, labels map[string]string) (bool, error) {
	var configs entraidDescriberLocal.IntegrationCredentials
	err := json.Unmarshal(jsonData, &configs)
	if err != nil {
		return false, err
	}

	return healthcheck.EntraidIntegrationHealthcheck(healthcheck.Config{
		TenantID:     providerId,
		ClientID:     configs.ClientID,
		ClientSecret: configs.ClientSecret,
		CertPath:     configs.CertificatePath,
		CertContent:  configs.CertificatePath,
		CertPassword: configs.CertificatePass,
	})
}

func (i *EntraIdDirectoryIntegration) DiscoverIntegrations(jsonData []byte) ([]models.Integration, error) {
	var configs entraidDescriberLocal.IntegrationCredentials
	err := json.Unmarshal(jsonData, &configs)
	if err != nil {
		return nil, err
	}

	var integrations []models.Integration
	directories, err := discovery.EntraidIntegrationDiscovery(discovery.Config{
		TenantID:     configs.TenantID,
		ClientID:     configs.ClientID,
		ClientSecret: configs.ClientSecret,
		CertPath:     configs.CertificatePath,
		CertContent:  configs.CertificatePath,
		CertPassword: configs.CertificatePass,
	})
	if err != nil {
		return nil, err
	}
	for _, s := range directories {
		integrations = append(integrations, models.Integration{
			ProviderID: s.TenantID,
			Name:       s.Name,
		})
	}

	return integrations, nil
}

func (i *EntraIdDirectoryIntegration) GetResourceTypesByLabels(map[string]string) ([]string, error) {
	return entraidDescriberLocal.ResourceTypesList, nil
}

func (i *EntraIdDirectoryIntegration) GetResourceTypeFromTableName(tableName string) string {
	return ""
}