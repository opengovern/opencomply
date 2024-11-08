package interfaces

import "github.com/opengovern/opengovernance/services/integration/models"

type DescriberConfiguration struct {
	NatsScheduledJobsTopic string
	NatsManualJobsTopic    string
	NatsStreamName         string
}

type IntegrationType interface {
	GetDescriberConfiguration() DescriberConfiguration
	GetAnnotations(jsonData []byte) (map[string]string, error)
	GetLabels(jsonData []byte) (map[string]string, error)
	GetResourceTypesByLabels(map[string]string) ([]string, error)
	HealthCheck(jsonData []byte, providerId string, labels map[string]string) (bool, error)
	DiscoverIntegrations(jsonData []byte) ([]models.Integration, error)
	GetResourceTypeFromTableName(tableName string) string
}

// IntegrationCreator IntegrationType interface, credentials, error
type IntegrationCreator func() (IntegrationType, error)
