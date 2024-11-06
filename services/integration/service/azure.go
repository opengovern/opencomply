package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/golang-jwt/jwt/v4"
	"github.com/opengovern/opengovernance/pkg/describe/connectors"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/google/uuid"
	absauth "github.com/microsoft/kiota-abstractions-go/authentication"
	authentication "github.com/microsoft/kiota-authentication-azure-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/opengovern/og-util/pkg/fp"
	"github.com/opengovern/og-util/pkg/source"
	"github.com/opengovern/opengovernance/services/integration/api/entity"
	"github.com/opengovern/opengovernance/services/integration/model"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

// NewAzure create a credential instance for azure SPN
func (h Credential) NewAzure(
	ctx context.Context,
	credType model.CredentialType,
	config entity.AzureCredentialConfig,
) (*model.Credential, error) {
	azureCnf, err := connectors.AzureSubscriptionConfigFromMap(config.AsMap())
	if err != nil {
		return nil, err
	}

	metadata, err := h.AzureMetadata(ctx, azureCnf)
	if err != nil {
		return nil, fmt.Errorf("failed to get credential metadata: %w", err)
	}

	cred, err := model.NewAzureCredential(credType, metadata)
	if err != nil {
		return nil, err
	}

	secretBytes, err := h.vault.Encrypt(ctx, config.AsMap())
	if err != nil {
		return nil, err
	}
	cred.Secret = secretBytes
	return cred, nil
}

func (h Credential) NewAzureConnection(
	ctx context.Context,
	sub model.AzureSubscription,
	creationMethod source.SourceCreationMethod,
	description string,
	creds model.Credential,
	tenantID string,
) model.Connection {
	id := uuid.New()

	name := sub.SubscriptionID
	if sub.SubModel.DisplayName != nil {
		name = *sub.SubModel.DisplayName
	}

	metadata := model.NewAzureConnectionMetadata(&sub, tenantID)
	jsonMetadata, err := json.Marshal(metadata)
	if err != nil {
		jsonMetadata = []byte("{}")
	}

	s := model.Connection{
		ID:                   id,
		SourceId:             sub.SubscriptionID,
		Name:                 name,
		Description:          description,
		Type:                 source.CloudAzure,
		CredentialID:         creds.ID,
		Credential:           creds,
		LifecycleState:       model.ConnectionLifecycleStateInProgress,
		AssetDiscoveryMethod: source.AssetDiscoveryMethodTypeScheduled,
		CreationMethod:       creationMethod,
		Metadata:             datatypes.JSON(jsonMetadata),
	}

	return s
}

// ExtractObjectID parses the token and extracts the object ID (oid claim).
func ExtractObjectID(tokenString string) (string, error) {
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return "", fmt.Errorf("failed to parse token: %v", err)
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		if oid, ok := claims["oid"].(string); ok {
			return oid, nil
		}
		return "", fmt.Errorf("oid claim not found in token")
	}
	return "", fmt.Errorf("failed to parse claims")
}

// ValidateAzureSPN validates the Azure Service Principal credentials and extracts the object ID.
func ValidateAzureSPN(clientID, clientSecret, tenantID string) (azcore.TokenCredential, string, error) {
	cred, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create ClientSecretCredential: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	scopes := []string{"https://management.azure.com/.default"}

	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: scopes,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to acquire token: %v", err)
	}

	objectID, err := ExtractObjectID(token.Token)
	if err != nil {
		return nil, "", fmt.Errorf("failed to extract object ID from token: %v", err)
	}

	return cred, objectID, nil
}

func (h Credential) AzureMetadata(ctx context.Context, config connectors.AzureSubscriptionConfig) (*model.AzureCredentialMetadata, error) {
	identity, err := azidentity.NewClientSecretCredential(
		config.TenantID,
		config.ClientID,
		config.ClientSecret,
		nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create identity: %w", err)
	}

	tokenProvider, err := authentication.NewAzureIdentityAccessTokenProvider(identity)
	if err != nil {
		return nil, fmt.Errorf("failed to create tokenProvider: %w", err)
	}

	authProvider := absauth.NewBaseBearerTokenAuthenticationProvider(tokenProvider)
	requestAdaptor, err := msgraphsdk.NewGraphRequestAdapter(authProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create requestAdaptor: %w", err)
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

	return &metadata, nil
}

// AzureHealthCheck checks the credential health.
func (h Credential) AzureHealthCheck(ctx context.Context, cred *model.Credential) (bool, error) {
	config, err := h.vault.Decrypt(ctx, cred.Secret)
	if err != nil {
		return false, err
	}

	var azureConfig connectors.AzureSubscriptionConfig
	azureConfig, err = connectors.AzureSubscriptionConfigFromMap(config)
	if err != nil {
		return false, err
	}

	//authConfig := azure.AuthConfig{
	//	TenantID:            azureConfig.TenantID,
	//	ObjectID:            azureConfig.ObjectID,
	//	SecretID:            azureConfig.SecretID,
	//	ClientID:            azureConfig.ClientID,
	//	ClientSecret:        azureConfig.ClientSecret,
	//	CertificatePath:     azureConfig.CertificatePath,
	//	CertificatePassword: azureConfig.CertificatePass,
	//	Username:            azureConfig.Username,
	//	Password:            azureConfig.Password,
	//}

	//if err := azure.CheckSPNAccessPermission(authConfig); err != nil {
	//	return false, err
	//}

	if _, _, err := ValidateAzureSPN(azureConfig.ClientID, azureConfig.ClientSecret, azureConfig.TenantID); err != nil {
		return false, err
	}

	//entraExtra, err := azure.CheckEntraIDPermission(authConfig)
	//if err == nil && entraExtra.DefaultDomain != nil {
	//	cred.Name = entraExtra.DefaultDomain
	//}

	metadata, err := h.AzureMetadata(ctx, azureConfig)
	if err != nil {
		return false, err
	}
	jsonMetadata, err := json.Marshal(metadata)
	if err != nil {
		return false, err
	}
	cred.Metadata = jsonMetadata

	return true, nil
}

func (h Credential) AzureOnboard(ctx context.Context, credential model.Credential) ([]model.Connection, error) {
	connections := make([]model.Connection, 0)

	cnf, err := h.vault.Decrypt(ctx, credential.Secret)
	if err != nil {
		return nil, err
	}

	azureCnf, err := connectors.AzureSubscriptionConfigFromMap(cnf)
	if err != nil {
		return nil, err
	}

	h.logger.Info("discovering azure subscriptions", zap.String("credential-id", credential.ID.String()))

	//subs, err := h.AzureDiscoverSubscriptions(ctx, azure.AuthConfig{
	//	TenantID:     azureCnf.TenantID,
	//	ObjectID:     azureCnf.ObjectID,
	//	SecretID:     azureCnf.SecretID,
	//	ClientID:     azureCnf.ClientID,
	//	ClientSecret: azureCnf.ClientSecret,
	//})
	//if err != nil {
	//	h.logger.Error("failed to discover subscriptions", zap.Error(err))
	//
	//	return nil, err
	//}

	//h.logger.Info("discovered azure subscriptions", zap.Int("count", len(subs)))

	existingConnections, err := h.connSvc.List(ctx, []source.Type{credential.ConnectorType})
	if err != nil {
		return nil, err
	}

	existingConnectionSubIDs := make([]string, 0, len(existingConnections))
	subsToOnboard := make([]model.AzureSubscription, 0)
	for _, conn := range existingConnections {
		existingConnectionSubIDs = append(existingConnectionSubIDs, conn.SourceId)
	}

	//for _, sub := range subs {
	//	if sub.SubModel.State != nil && *sub.SubModel.State == armsubscription.SubscriptionStateEnabled && !utils.Includes(existingConnectionSubIDs, sub.SubscriptionID) {
	//		subsToOnboard = append(subsToOnboard, sub)
	//	} else {
	//		for _, conn := range existingConnections {
	//			if conn.SourceId == sub.SubscriptionID {
	//				name := sub.SubscriptionID
	//				if sub.SubModel.DisplayName != nil {
	//					name = *sub.SubModel.DisplayName
	//				}
	//				localConn := conn
	//				if conn.Name != name {
	//					localConn.Name = name
	//				}
	//				if sub.SubModel.State != nil && *sub.SubModel.State != armsubscription.SubscriptionStateEnabled {
	//					localConn.LifecycleState = model.ConnectionLifecycleStateDisabled
	//				}
	//				if conn.Name != name || localConn.LifecycleState != conn.LifecycleState {
	//					if err := h.connSvc.Update(ctx, localConn); err != nil {
	//						h.logger.Error("failed to update source", zap.Error(err))
	//						return nil, err
	//					}
	//				}
	//			}
	//		}
	//	}
	//}

	h.logger.Info("onboarding subscriptions", zap.Int("count", len(subsToOnboard)))

	for _, sub := range subsToOnboard {
		h.logger.Info("onboarding subscription", zap.String("subscriptionId", sub.SubscriptionID))

		count, err := h.connSvc.Count(ctx, nil, nil)
		if err != nil {
			return nil, err
		}

		maxConnections, err := h.connSvc.MaxConnections(ctx)
		if err != nil {
			return nil, err
		}

		if count >= maxConnections {
			return nil, ErrMaxConnectionsExceeded
		}

		//isAttached, err := azure.CheckRole(azure.AuthConfig{
		//	TenantID:     azureCnf.TenantID,
		//	ObjectID:     azureCnf.ObjectID,
		//	SecretID:     azureCnf.SecretID,
		//	ClientID:     azureCnf.ClientID,
		//	ClientSecret: azureCnf.ClientSecret,
		//}, sub.SubscriptionID, azure.DefaultReaderRoleDefinitionIDTemplate)
		//if err != nil {
		//	h.logger.Warn("failed to check role", zap.Error(err))
		//	continue
		//}
		//if !isAttached {
		//	h.logger.Warn("role not attached", zap.String("subscriptionId", sub.SubscriptionID))
		//	continue
		//}

		src := h.NewAzureConnection(
			ctx,
			sub,
			source.SourceCreationMethodAutoOnboard,
			fmt.Sprintf("Auto on-boarded subscription %s", sub.SubscriptionID),
			credential,
			azureCnf.TenantID,
		)

		if err := h.connSvc.Create(ctx, src); err != nil {
			return nil, err
		}

		metadata := make(map[string]any)
		if src.Metadata.String() != "" {
			err := json.Unmarshal(src.Metadata, &metadata)
			if err != nil {
				return nil, err
			}
		}

		connections = append(connections, src)
	}

	return connections, nil
}

func (h Credential) AzureDiscoverSubscriptions(ctx context.Context, authConfig any) ([]model.AzureSubscription, error) {
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
	//subs := make([]model.AzureSubscription, 0)
	//for it.More() {
	//	page, err := it.NextPage(ctx)
	//	if err != nil {
	//		h.logger.Error("failed to get subscription page", zap.Error(err))
	//		return nil, err
	//	}
	//	for _, v := range page.Value {
	//		if v == nil || v.State == nil {
	//			continue
	//		}
	//		tagsClient, err := armresources.NewTagsClient(*v.SubscriptionID, identity, nil)
	//		if err != nil {
	//			h.logger.Error("failed to create tags client", zap.Error(err))
	//
	//			return nil, err
	//		}
	//		tagIt := tagsClient.NewListPager(nil)
	//		tagList := make([]armresources.TagDetails, 0)
	//		for tagIt.More() {
	//			tagPage, err := tagIt.NextPage(ctx)
	//			if err != nil {
	//				h.logger.Error("failed to get tag page", zap.Error(err))
	//
	//				return nil, err
	//			}
	//			for _, tag := range tagPage.Value {
	//				tagList = append(tagList, *tag)
	//			}
	//		}
	//		localV := v
	//		subs = append(subs, model.AzureSubscription{
	//			SubscriptionID: *v.SubscriptionID,
	//			SubModel:       *localV,
	//			SubTags:        tagList,
	//		})
	//	}
	//}

	return nil, nil
}

func (h Credential) AzureUpdate(ctx context.Context, id string, req entity.UpdateAzureCredentialRequest) error {
	ctx, span := h.tracer.Start(ctx, "update-aws-credential")
	defer span.End()

	cred, err := h.Get(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		return err
	}
	span.AddEvent("information", trace.WithAttributes(
		attribute.String("credential name", *cred.Name),
	))

	if req.Name != nil {
		cred.Name = req.Name
	}

	cnf, err := h.vault.Decrypt(ctx, cred.Secret)
	if err != nil {
		return err
	}
	config, err := fp.FromMap[connectors.AzureSubscriptionConfig](cnf)
	if err != nil {
		return err
	}

	if req.Config != nil {
		if req.Config.TenantId != "" {
			config.TenantID = req.Config.TenantId
		}

		if req.Config.ObjectId != "" {
			config.ObjectID = req.Config.ObjectId
		}

		if req.Config.ClientId != "" {
			config.ClientID = req.Config.ClientId
		}

		if req.Config.ClientSecret != "" {
			config.ClientSecret = req.Config.ClientSecret
		}
	}

	metadata, err := h.AzureMetadata(ctx, *config)
	if err != nil {
		return err
	}

	jsonMetadata, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	cred.Metadata = jsonMetadata

	secretBytes, err := h.vault.Encrypt(ctx, config.ToMap())
	if err != nil {
		return err
	}

	cred.Secret = secretBytes
	if metadata.SpnName != "" {
		cred.Name = &metadata.SpnName
	}

	if err := h.repo.Update(ctx, cred); err != nil {
		return err
	}

	if err := h.repo.Update(ctx, cred); err != nil {
		return err
	}

	return nil
}

// AzureCredentialConfig reads credentials configuration from azure credential secret and return it.
func (h Credential) AzureCredentialConfig(ctx context.Context, credential model.Credential) (*connectors.AzureSubscriptionConfig, error) {
	raw, err := h.vault.Decrypt(ctx, credential.Secret)
	if err != nil {
		return nil, err
	}

	cnf, err := connectors.AzureSubscriptionConfigFromMap(raw)
	if err != nil {
		return nil, err
	}

	return &cnf, nil
}

func (h Connection) AzureHealth(ctx context.Context, connection model.Connection, updateMetadata bool) (model.Connection, error) {
	//var cnf map[string]any

	_, err := h.vault.Decrypt(ctx, connection.Credential.Secret)
	if err != nil {
		h.logger.Error("failed to decrypt credential", zap.Error(err), zap.String("sourceId", connection.SourceId))
		return connection, err
	}

	var assetDiscoveryAttached, spendAttached bool

	//subscriptionConfig, err := connectors.AzureSubscriptionConfigFromMap(cnf)
	//if err != nil {
	//	h.logger.Error("failed to get azure config", zap.Error(err), zap.String("sourceId", connection.SourceId))
	//	return connection, err
	//}

	//authCnf := azure.AuthConfig{
	//	TenantID:            subscriptionConfig.TenantID,
	//	ClientID:            subscriptionConfig.ClientID,
	//	ObjectID:            subscriptionConfig.ObjectID,
	//	SecretID:            subscriptionConfig.SecretID,
	//	ClientSecret:        subscriptionConfig.ClientSecret,
	//	CertificatePath:     subscriptionConfig.CertificatePath,
	//	CertificatePassword: subscriptionConfig.CertificatePass,
	//	Username:            subscriptionConfig.Username,
	//	Password:            subscriptionConfig.Password,
	//}
	//
	//azureAssetDiscovery, err := h.meta.Client.GetConfigMetadata(&httpclient.Context{UserRole: api.AdminRole}, models.MetadataKeyAssetDiscoveryAzureRoleIDs)
	//if err != nil {
	//	return connection, err
	//}
	//
	//assetDiscoveryAttached = true
	//for _, ruleID := range strings.Split(azureAssetDiscovery.GetValue().(string), ",") {
	//	isAttached, err := azure.CheckRole(authCnf, connection.SourceId, ruleID)
	//	if err != nil {
	//		return connection, err
	//	}
	//
	//	if !isAttached {
	//		h.logger.Error("rule is not there", zap.String("ruleID", ruleID))
	//		assetDiscoveryAttached = false
	//	}
	//}
	//
	//azureSpendDiscovery, err := h.meta.Client.GetConfigMetadata(&httpclient.Context{UserRole: api.AdminRole}, models.MetadataKeySpendDiscoveryAzureRoleIDs)
	//if err != nil {
	//	return connection, err
	//}
	//
	//spendAttached = true
	//for _, ruleID := range strings.Split(azureSpendDiscovery.GetValue().(string), ",") {
	//	isAttached, err := azure.CheckRole(authCnf, connection.SourceId, ruleID)
	//	if err != nil {
	//		return connection, err
	//	}
	//
	//	if !isAttached {
	//		h.logger.Error("rule is not there", zap.String("ruleID", ruleID))
	//		spendAttached = false
	//	}
	//}
	//
	//if (assetDiscoveryAttached || spendAttached) && updateMetadata {
	//	var subscription *model.AzureSubscription
	//
	//	subscription, err = CurrentAzureSubscription(ctx, connection.SourceId, authCnf)
	//	if err != nil {
	//		h.logger.Error("failed to get current azure subscription", zap.Error(err), zap.String("connectionId", connection.SourceId))
	//
	//		return connection, err
	//	}
	//
	//	metadata := model.NewAzureConnectionMetadata(subscription, subscriptionConfig.TenantID)
	//	var jsonMetadata []byte
	//	jsonMetadata, err = json.Marshal(metadata)
	//	if err != nil {
	//		h.logger.Error("failed to marshal azure metadata", zap.Error(err), zap.String("connectionId", connection.SourceId))
	//		return connection, err
	//	}
	//	connection.Metadata = jsonMetadata
	//}

	assetDiscoveryAttached = true
	spendAttached = connection.Credential.SpendDiscovery != nil && *connection.Credential.SpendDiscovery // LIKE AWS (CHECK LATER)
	if !assetDiscoveryAttached && !spendAttached {
		var healthMessage string
		if err == nil {
			healthMessage = "failed to find read permission"
		} else {
			healthMessage = err.Error()
		}

		connection, err = h.UpdateHealth(ctx, connection, source.HealthStatusUnhealthy, &healthMessage, fp.Optional(false), fp.Optional(false), true)
		if err != nil {
			h.logger.Warn("failed to update source health", zap.Error(err), zap.String("connectionId", connection.SourceId))

			return connection, err
		}
	} else {
		connection, err = h.UpdateHealth(ctx, connection, source.HealthStatusHealthy, fp.Optional(""), &spendAttached, &assetDiscoveryAttached, true)
		if err != nil {
			h.logger.Warn("failed to update source health", zap.Error(err), zap.String("connectionId", connection.SourceId))

			return connection, err
		}
	}

	return connection, nil
}

//
//func CurrentAzureSubscription(ctx context.Context, subId string, authConfig azure.AuthConfig) (*model.AzureSubscription, error) {
//	identity, err := azidentity.NewClientSecretCredential(
//		authConfig.TenantID,
//		authConfig.ClientID,
//		authConfig.ClientSecret,
//		nil)
//	if err != nil {
//		return nil, err
//	}
//
//	client, err := armsubscription.NewSubscriptionsClient(identity, nil)
//	if err != nil {
//		return nil, err
//	}
//
//	sub, err := client.Get(ctx, subId, nil)
//	if err != nil {
//		return nil, err
//	}
//
//	tagsClient, err := armresources.NewTagsClient(*sub.SubscriptionID, identity, nil)
//	if err != nil {
//		return nil, err
//	}
//
//	tagIt := tagsClient.NewListPager(nil)
//	tagList := make([]armresources.TagDetails, 0)
//	for tagIt.More() {
//		tagPage, err := tagIt.NextPage(ctx)
//		if err != nil {
//			return nil, err
//		}
//		for _, tag := range tagPage.Value {
//			tagList = append(tagList, *tag)
//		}
//	}
//
//	return &model.AzureSubscription{
//		SubscriptionID: subId,
//		SubModel:       sub.Subscription,
//		SubTags:        tagList,
//	}, nil
//}

func (h Credential) UpdateHealth(
	ctx context.Context,
	credential model.Credential,
	healthStatus source.HealthStatus,
	reason *string,
	updateDatabase bool,
) (model.Credential, error) {
	credential.HealthStatus = healthStatus
	credential.HealthReason = reason
	credential.LastHealthCheckTime = time.Now()

	if updateDatabase == true {
		ctx, span := h.tracer.Start(ctx, "update-health")
		defer span.End()

		if err := h.repo.Update(ctx, &credential); err != nil {
			return model.Credential{}, err
		}
	}

	return credential, nil
}
