package onboard

import (
	"context"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/subscription/armsubscription"
	absauth "github.com/microsoft/kiota-abstractions-go/authentication"
	authentication "github.com/microsoft/kiota-authentication-azure-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/opengovern/opengovernance/pkg/describe/connectors"
	"github.com/opengovern/opengovernance/services/integration/model"
	"go.uber.org/zap"
)

type azureSubscription struct {
	SubscriptionID string
	SubModel       armsubscription.Subscription
	SubTags        []armresources.TagDetails
}

func discoverAzureSubscriptions(ctx context.Context, logger *zap.Logger, authConfig any) ([]azureSubscription, error) {
	//identity, err := azidentity.NewClientSecretCredential(
	//	authConfig.TenantID,
	//	authConfig.ClientID,
	//	authConfig.ClientSecret,
	//	nil)
	//if err != nil {
	//	return nil, err
	//}
	//client, err := armsubscription.NewSubscriptionsClient(identity, nil)
	//if err != nil {
	//	return nil, err
	//}

	//it := client.NewListPager(nil)
	//subs := make([]azureSubscription, 0)
	//for it.More() {
	//	page, err := it.NextPage(ctx)
	//	if err != nil {
	//		logger.Error("failed to get subscription page", zap.Error(err))
	//		return nil, err
	//	}
	//	for _, v := range page.Value {
	//		if v == nil || v.State == nil {
	//			continue
	//		}
	//		tagsClient, err := armresources.NewTagsClient(*v.SubscriptionID, identity, nil)
	//		if err != nil {
	//			logger.Error("failed to create tags client", zap.Error(err))
	//			return nil, err
	//		}
	//		tagIt := tagsClient.NewListPager(nil)
	//		tagList := make([]armresources.TagDetails, 0)
	//		for tagIt.More() {
	//			tagPage, err := tagIt.NextPage(ctx)
	//			if err != nil {
	//				logger.Error("failed to get tag page", zap.Error(err))
	//				return nil, err
	//			}
	//			for _, tag := range tagPage.Value {
	//				tagList = append(tagList, *tag)
	//			}
	//		}
	//		localV := v
	//		subs = append(subs, azureSubscription{
	//			SubscriptionID: *v.SubscriptionID,
	//			SubModel:       *localV,
	//			SubTags:        tagList,
	//		})
	//	}
	//}

	return nil, nil
}

func currentAzureSubscription(ctx context.Context, logger *zap.Logger, subId string, authConfig any) (*azureSubscription, error) {
	//identity, err := azidentity.NewClientSecretCredential(
	//	authConfig.TenantID,
	//	authConfig.ClientID,
	//	authConfig.ClientSecret,
	//	nil)
	//if err != nil {
	//	return nil, err
	//}
	//client, err := armsubscription.NewSubscriptionsClient(identity, nil)
	//if err != nil {
	//	return nil, err
	//}
	//sub, err := client.Get(ctx, subId, nil)
	//if err != nil {
	//	return nil, err
	//}
	//tagsClient, err := armresources.NewTagsClient(*sub.SubscriptionID, identity, nil)
	//if err != nil {
	//	logger.Error("failed to create tags client", zap.Error(err))
	//	return nil, err
	//}
	//tagIt := tagsClient.NewListPager(nil)
	//tagList := make([]armresources.TagDetails, 0)
	//for tagIt.More() {
	//	tagPage, err := tagIt.NextPage(ctx)
	//	if err != nil {
	//		logger.Error("failed to get tag page", zap.Error(err))
	//		return nil, err
	//	}
	//	for _, tag := range tagPage.Value {
	//		tagList = append(tagList, *tag)
	//	}
	//}

	return nil, nil
}

func getAzureCredentialsMetadata(ctx context.Context, config connectors.AzureSubscriptionConfig, credType model.CredentialType) (*model.AzureCredentialMetadata, error) {
	identity, err := azidentity.NewClientSecretCredential(
		config.TenantID,
		config.ClientID,
		config.ClientSecret,
		nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create identity: %v", err)
	}

	tokenProvider, err := authentication.NewAzureIdentityAccessTokenProvider(identity)
	if err != nil {
		return nil, fmt.Errorf("failed to create tokenProvider: %v", err)
	}

	authProvider := absauth.NewBaseBearerTokenAuthenticationProvider(tokenProvider)
	requestAdaptor, err := msgraphsdk.NewGraphRequestAdapter(authProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create requestAdaptor: %v", err)
	}

	graphClient := msgraphsdk.NewGraphServiceClient(requestAdaptor)

	metadata := model.AzureCredentialMetadata{}
	if config.ObjectID == "" {
		return &metadata, nil
	}

	result, err := graphClient.Applications().ByApplicationId(config.ObjectID).Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get Applications: %v", err)
	}

	metadata.SpnName = *result.GetDisplayName()
	metadata.ObjectId = *result.GetId()
	metadata.SecretId = config.SecretID
	metadata.TenantId = config.TenantID
	metadata.ClientId = config.ClientID
	for _, passwd := range result.GetPasswordCredentials() {
		if passwd.GetKeyId() != nil && passwd.GetKeyId().String() == config.SecretID {
			metadata.SecretId = config.SecretID
			metadata.SecretExpirationDate = *passwd.GetEndDateTime()
		}
	}

	//entraExtraData, err := azure.CheckEntraIDPermission(azure.AuthConfig{
	//	TenantID:     config.TenantID,
	//	ClientID:     config.ClientID,
	//	ClientSecret: config.ClientSecret,
	//})
	//if err == nil {
	//	metadata.DefaultDomain = entraExtraData.DefaultDomain
	//}

	//
	//if metadata.SecretId == "" {
	//	return nil, fmt.Errorf("failed to find the secret in application's credential list")
	//}

	return &metadata, nil
}
