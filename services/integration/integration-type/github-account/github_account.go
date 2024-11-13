package github_account

import (
	"encoding/json"
	"fmt"
	"github.com/jackc/pgtype"
	awsDescriberLocal "github.com/opengovern/opengovernance/services/integration/integration-type/aws-account/configs"
	"github.com/opengovern/opengovernance/services/integration/integration-type/aws-account/discovery"
	githubDescriberLocal "github.com/opengovern/opengovernance/services/integration/integration-type/github-account/configs"
	"github.com/opengovern/opengovernance/services/integration/integration-type/github-account/healthcheck"
	"github.com/opengovern/opengovernance/services/integration/integration-type/interfaces"
	"github.com/opengovern/opengovernance/services/integration/models"
)

type GithubAccountIntegration struct{}

func (i *GithubAccountIntegration) GetConfiguration() interfaces.IntegrationConfiguration {
	return interfaces.IntegrationConfiguration{
		NatsScheduledJobsTopic: githubDescriberLocal.JobQueueTopic,
		NatsManualJobsTopic:    githubDescriberLocal.JobQueueTopicManuals,
		NatsStreamName:         githubDescriberLocal.StreamName,

		UISpecFileName: "github-account.json",
	}
}

func (i *GithubAccountIntegration) HealthCheck(jsonData []byte, providerId string, labels map[string]string, annotations map[string]string) (bool, error) {
	var credentials githubDescriberLocal.IntegrationCredentials
	err := json.Unmarshal(jsonData, &credentials)
	if err != nil {
		return false, err
	}

	return healthcheck.GithubIntegrationHealthcheck(healthcheck.Config{
		Token:          credentials.Token,
		BaseURL:        credentials.BaseURL,
		AppId:          credentials.AppId,
		InstallationId: credentials.InstallationId,
		PrivateKeyPath: credentials.PrivateKeyPath,
	})
}

func (i *GithubAccountIntegration) DiscoverIntegrations(jsonData []byte) ([]models.Integration, error) {
	var credentials awsDescriberLocal.IntegrationCredentials
	err := json.Unmarshal(jsonData, &credentials)
	if err != nil {
		return nil, err
	}

	var integrations []models.Integration
	accounts := discovery.AWSIntegrationDiscovery(discovery.Config{
		AWSAccessKeyID:                credentials.AwsAccessKeyID,
		AWSSecretAccessKey:            credentials.AwsSecretAccessKey,
		RoleNameToAssumeInMainAccount: credentials.RoleToAssumeInMainAccount,
		CrossAccountRoleName:          credentials.CrossAccountRoleName,
		ExternalID:                    credentials.ExternalID,
	})
	for _, a := range accounts {
		if a.Details.Error != "" {
			return nil, fmt.Errorf(a.Details.Error)
		}

		labels := map[string]string{
			"RoleNameInMainAccount": a.Labels.RoleNameInMainAccount,
			"AccountType":           a.Labels.AccountType,
			"CrossAccountRoleARN":   a.Labels.CrossAccountRoleARN,
			"ExternalID":            a.Labels.ExternalID,
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
			ProviderID: a.AccountID,
			Name:       a.AccountName,
			Labels:     integrationLabelsJsonb,
		})
	}

	return integrations, nil
}

func (i *GithubAccountIntegration) GetResourceTypesByLabels(map[string]string) ([]string, error) {
	return githubDescriberLocal.ResourceTypesList, nil
}

func (i *GithubAccountIntegration) GetResourceTypeFromTableName(tableName string) string {
	return ""
}
