package integration_type

import (
	"github.com/opengovern/og-util/pkg/integration"
	"github.com/opengovern/opengovernance/services/integration/integration-type/aws-account"
	awsConfigs "github.com/opengovern/opengovernance/services/integration/integration-type/aws-account/configs"
	"github.com/opengovern/opengovernance/services/integration/integration-type/azure-subscription"
	azureConfigs "github.com/opengovern/opengovernance/services/integration/integration-type/azure-subscription/configs"
	"github.com/opengovern/opengovernance/services/integration/integration-type/entra-id-directory"
	entraidConfigs "github.com/opengovern/opengovernance/services/integration/integration-type/entra-id-directory/configs"
	"github.com/opengovern/opengovernance/services/integration/integration-type/interfaces"
	"strings"
)

const (
	IntegrationTypeAWSAccount        = awsConfigs.IntegrationTypeAwsCloudAccount
	IntegrationTypeAzureSubscription = azureConfigs.IntegrationTypeAzureSubscription
	IntegrationTypeEntraIdDirectory  = entraidConfigs.IntegrationTypeEntraidDirectory
)

var AllIntegrationTypes = []integration.Type{
	IntegrationTypeAWSAccount,
	IntegrationTypeAzureSubscription,
	IntegrationTypeEntraIdDirectory,
}

var IntegrationTypes = map[integration.Type]interfaces.IntegrationCreator{
	IntegrationTypeAWSAccount:        aws_account.CreateAwsCloudAccountIntegration,
	IntegrationTypeAzureSubscription: azure_subscription.CreateAzureSubscriptionIntegration,
	IntegrationTypeEntraIdDirectory:  entra_id_directory.CreateEntraidSubscriptionIntegration,
}

func ParseType(str string) integration.Type {
	str = strings.ToLower(str)
	for _, t := range AllIntegrationTypes {
		if str == t.String() {
			return t
		}
	}
	return ""
}

func ParseTypes(str []string) []integration.Type {
	result := make([]integration.Type, 0, len(str))
	for _, s := range str {
		t := ParseType(s)
		if t == "" {
			continue
		}
		result = append(result, t)
	}
	return result
}

func UnparseTypes(types []integration.Type) []string {
	result := make([]string, 0, len(types))
	for _, t := range types {
		result = append(result, t.String())
	}
	return result
}
