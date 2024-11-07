package client

import (
	"encoding/json"
	"fmt"
	"github.com/labstack/echo/v4"
	authApi "github.com/opengovern/og-util/pkg/api"
	"github.com/opengovern/og-util/pkg/httpclient"
	"github.com/opengovern/opengovernance/services/integration/api/models"
	"net/http"
)

type IntegrationServiceClient interface {
	GetIntegration(ctx *httpclient.Context, integrationID string) (*models.Integration, error)
	ListIntegrations(ctx *httpclient.Context, integrationTypes []string) (*models.ListIntegrationsResponse, error)
	ListIntegrationsByFilters(ctx *httpclient.Context, req models.ListIntegrationsRequest) (*models.ListIntegrationsResponse, error)
	IntegrationHealthcheck(ctx *httpclient.Context, integrationID string) (*models.Integration, error)
	GetCredential(ctx *httpclient.Context, credentialID string) (*models.Credential, error)
}

type integrationClient struct {
	baseURL string
}

func NewIntegrationServiceClient(baseURL string) IntegrationServiceClient {
	return &integrationClient{baseURL: baseURL}
}

func (c *integrationClient) GetIntegration(ctx *httpclient.Context, integrationID string) (*models.Integration, error) {
	url := fmt.Sprintf("%s/api/v1/integrations/%s", c.baseURL, integrationID)
	var response *models.Integration

	if statusCode, err := httpclient.DoRequest(ctx.Ctx, http.MethodGet, url, ctx.ToHeaders(), nil, &response); err != nil {
		if 400 <= statusCode && statusCode < 500 {
			return nil, echo.NewHTTPError(statusCode, err.Error())
		}
		return nil, err
	}
	return response, nil
}

func (c *integrationClient) ListIntegrations(ctx *httpclient.Context, integrationTypes []string) (*models.ListIntegrationsResponse, error) {
	ctx.UserRole = authApi.AdminRole
	url := fmt.Sprintf("%s/api/v1/integrations", c.baseURL)
	for i, v := range integrationTypes {
		if i == 0 {
			url += "?"
		} else {
			url += "&"
		}
		url += "integration_type=" + v
	}

	var response models.ListIntegrationsResponse
	if statusCode, err := httpclient.DoRequest(ctx.Ctx, http.MethodGet, url, ctx.ToHeaders(), nil, &response); err != nil {
		if 400 <= statusCode && statusCode < 500 {
			return nil, echo.NewHTTPError(statusCode, err.Error())
		}
		return nil, err
	}
	return &response, nil
}

func (c *integrationClient) ListIntegrationsByFilters(ctx *httpclient.Context, req models.ListIntegrationsRequest) (*models.ListIntegrationsResponse, error) {
	ctx.UserRole = authApi.AdminRole
	url := fmt.Sprintf("%s/api/v1/integrations/list", c.baseURL)

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var response models.ListIntegrationsResponse
	if statusCode, err := httpclient.DoRequest(ctx.Ctx, http.MethodGet, url, ctx.ToHeaders(), payload, &response); err != nil {
		if 400 <= statusCode && statusCode < 500 {
			return nil, echo.NewHTTPError(statusCode, err.Error())
		}
		return nil, err
	}
	return &response, nil
}

func (c *integrationClient) GetCredential(ctx *httpclient.Context, credentialID string) (*models.Credential, error) {
	url := fmt.Sprintf("%s/api/v1/credentials/%s", c.baseURL, credentialID)
	var response *models.Credential

	if statusCode, err := httpclient.DoRequest(ctx.Ctx, http.MethodGet, url, ctx.ToHeaders(), nil, &response); err != nil {
		if 400 <= statusCode && statusCode < 500 {
			return nil, echo.NewHTTPError(statusCode, err.Error())
		}
		return nil, err
	}
	return response, nil
}

func (c *integrationClient) IntegrationHealthcheck(ctx *httpclient.Context, integrationID string) (*models.Integration, error) {
	url := fmt.Sprintf("%s/api/v1/integrations/%s/healthcheck", c.baseURL, integrationID)
	var response *models.Integration

	if statusCode, err := httpclient.DoRequest(ctx.Ctx, http.MethodPut, url, ctx.ToHeaders(), nil, &response); err != nil {
		if 400 <= statusCode && statusCode < 500 {
			return nil, echo.NewHTTPError(statusCode, err.Error())
		}
		return nil, err
	}
	return response, nil
}
