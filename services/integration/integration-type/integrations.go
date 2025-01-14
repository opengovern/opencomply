package integration_type

import (
	"strings"

	"github.com/opengovern/og-util/pkg/integration"
	"github.com/opengovern/opencomply/services/integration/integration-type/aws-account"
	awsConfigs "github.com/opengovern/opencomply/services/integration/integration-type/aws-account/configs"
	"github.com/opengovern/opencomply/services/integration/integration-type/azure-subscription"
	azureConfigs "github.com/opengovern/opencomply/services/integration/integration-type/azure-subscription/configs"
	cloudflareaccount "github.com/opengovern/opencomply/services/integration/integration-type/cloudflare-account"
	cloudflareConfigs "github.com/opengovern/opencomply/services/integration/integration-type/cloudflare-account/configs"
	cohereaiproject "github.com/opengovern/opencomply/services/integration/integration-type/cohereai-project"
	cohereaiConfigs "github.com/opengovern/opencomply/services/integration/integration-type/cohereai-project/configs"
	"github.com/opengovern/opencomply/services/integration/integration-type/digitalocean-team"
	digitalOceanConfigs "github.com/opengovern/opencomply/services/integration/integration-type/digitalocean-team/configs"
	doppler "github.com/opengovern/opencomply/services/integration/integration-type/doppler-account"
	dopplerConfigs "github.com/opengovern/opencomply/services/integration/integration-type/doppler-account/configs"
	"github.com/opengovern/opencomply/services/integration/integration-type/entra-id-directory"
	entraidConfigs "github.com/opengovern/opencomply/services/integration/integration-type/entra-id-directory/configs"
	fly "github.com/opengovern/opencomply/services/integration/integration-type/fly-account"
	flyConfigs "github.com/opengovern/opencomply/services/integration/integration-type/fly-account/configs"
	githubaccount "github.com/opengovern/opencomply/services/integration/integration-type/github-account"
	githubConfigs "github.com/opengovern/opencomply/services/integration/integration-type/github-account/configs"
	google_workspace_account "github.com/opengovern/opencomply/services/integration/integration-type/google-workspace-account"
	googleConfig "github.com/opengovern/opencomply/services/integration/integration-type/google-workspace-account/configs"
	"github.com/opengovern/opencomply/services/integration/integration-type/interfaces"
	linodeaccount "github.com/opengovern/opencomply/services/integration/integration-type/linode-account"
	linodeConfigs "github.com/opengovern/opencomply/services/integration/integration-type/linode-account/configs"
	oci "github.com/opengovern/opencomply/services/integration/integration-type/oci-repository"
	ociConfigs "github.com/opengovern/opencomply/services/integration/integration-type/oci-repository/configs"
	openaiproject "github.com/opengovern/opencomply/services/integration/integration-type/openai-integration"
	openaiConfigs "github.com/opengovern/opencomply/services/integration/integration-type/openai-integration/configs"
	render "github.com/opengovern/opencomply/services/integration/integration-type/render-account"
	renderConfigs "github.com/opengovern/opencomply/services/integration/integration-type/render-account/configs"
	tailscale "github.com/opengovern/opencomply/services/integration/integration-type/tailscale-account"
	tailscaleConfigs "github.com/opengovern/opencomply/services/integration/integration-type/tailscale-account/configs"
)

const (
	IntegrationTypeAWSAccount             = awsConfigs.IntegrationTypeAwsCloudAccount
	IntegrationTypeAzureSubscription      = azureConfigs.IntegrationTypeAzureSubscription
	IntegrationTypeEntraIdDirectory       = entraidConfigs.IntegrationTypeEntraidDirectory
	IntegrationTypeGithubAccount          = githubConfigs.IntegrationTypeGithubAccount
	IntegrationTypeDigitalOceanTeam       = digitalOceanConfigs.IntegrationTypeDigitalOceanTeam
	IntegrationTypeCloudflareAccount      = cloudflareConfigs.IntegrationNameCloudflareAccount
	IntegrationTypeOpenAIIntegration      = openaiConfigs.IntegrationTypeOpenaiIntegration
	IntegrationTypeLinodeProject          = linodeConfigs.IntegrationTypeLinodeProject
	IntegrationTypeCohereAIProject        = cohereaiConfigs.IntegrationTypeCohereaiProject
	IntegrationTypeGoogleWorkspaceAccount = googleConfig.IntegrationTypeGoogleWorkspaceAccount
	IntegrationTypeOCIRepository          = ociConfigs.IntegrationTypeOciRepository
	IntegrationTypeRenderAccount          = renderConfigs.IntegrationTypeRenderAccount
	IntegrationTypeDopplerAccount         = dopplerConfigs.IntegrationTypeDopplerAccount
	IntegrationTypeTailScaleAccount       = tailscaleConfigs.IntegrationTypeTailScaleAccount
	IntegrationTypeFlyAccount             = flyConfigs.IntegrationTypeFlyAccount
)

var AllIntegrationTypes = []integration.Type{
	IntegrationTypeAWSAccount,
	IntegrationTypeAzureSubscription,
	IntegrationTypeEntraIdDirectory,
	IntegrationTypeGithubAccount,
	IntegrationTypeDigitalOceanTeam,
	IntegrationTypeCloudflareAccount,
	IntegrationTypeOpenAIIntegration,
	IntegrationTypeLinodeProject,
	IntegrationTypeCohereAIProject,
	IntegrationTypeGoogleWorkspaceAccount,
	IntegrationTypeOCIRepository,
	IntegrationTypeRenderAccount,
	IntegrationTypeDopplerAccount,
	IntegrationTypeTailScaleAccount,
	IntegrationTypeFlyAccount,
}

var IntegrationTypes = map[integration.Type]interfaces.IntegrationType{
	IntegrationTypeAWSAccount:             &aws_account.AwsCloudAccountIntegration{},
	IntegrationTypeAzureSubscription:      &azure_subscription.AzureSubscriptionIntegration{},
	IntegrationTypeEntraIdDirectory:       &entra_id_directory.EntraIdDirectoryIntegration{},
	IntegrationTypeGithubAccount:          &githubaccount.GithubAccountIntegration{},
	IntegrationTypeDigitalOceanTeam:       &digitalocean_team.DigitaloceanTeamIntegration{},
	IntegrationTypeCloudflareAccount:      &cloudflareaccount.CloudFlareAccountIntegration{},
	IntegrationTypeOpenAIIntegration:      &openaiproject.OpenAIIntegration{},
	IntegrationTypeLinodeProject:          &linodeaccount.LinodeAccountIntegration{},
	IntegrationTypeCohereAIProject:        &cohereaiproject.CohereAIProjectIntegration{},
	IntegrationTypeGoogleWorkspaceAccount: &google_workspace_account.GoogleWorkspaceAccountIntegration{},
	IntegrationTypeOCIRepository:          &oci.Integration{},
	IntegrationTypeRenderAccount:          &render.RenderAccountIntegration{},
	IntegrationTypeDopplerAccount:         &doppler.DopplerAccountIntegration{},
	IntegrationTypeTailScaleAccount:       &tailscale.TailScaleAccountIntegration{},
	IntegrationTypeFlyAccount:             &fly.FlyAccountIntegration{},
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
