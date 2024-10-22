package client

import (
	"encoding/json"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/opengovern/og-util/pkg/httpclient"
	"github.com/opengovern/opengovernance/services/integration/api/entity"
	"net/http"
)

type IntegrationServiceClient interface {
	CreateAzure(ctx *httpclient.Context, request entity.CreateAzureCredentialRequest) (*entity.CreateCredentialResponse, error)
	CreateAws(ctx *httpclient.Context, request entity.CreateAWSCredentialRequest) (*entity.CreateCredentialResponse, error)
}

type integrationClient struct {
	baseURL string
}

func NewIntegrationServiceClient(baseURL string) IntegrationServiceClient {
	return &integrationClient{baseURL: baseURL}
}

func (c *integrationClient) CreateAzure(ctx *httpclient.Context, request entity.CreateAzureCredentialRequest) (*entity.CreateCredentialResponse, error) {
	url := fmt.Sprintf("%s/api/v1/credentials/azure", c.baseURL)
	var response *entity.CreateCredentialResponse

	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}

	//headers := map[string]string{
	//	httpserver.XKaytuUserIDHeader:        ctx.UserID,
	//	httpserver.XKaytuUserRoleHeader:      string(ctx.UserRole),
	//}
	if statusCode, err := httpclient.DoRequest(ctx.Ctx, http.MethodPost, url, ctx.ToHeaders(), payload, &response); err != nil {
		if 400 <= statusCode && statusCode < 500 {
			return nil, echo.NewHTTPError(statusCode, err.Error())
		}
		return nil, err
	}
	return response, nil
}

func (c *integrationClient) CreateAws(ctx *httpclient.Context, request entity.CreateAWSCredentialRequest) (*entity.CreateCredentialResponse, error) {
	url := fmt.Sprintf("%s/api/v1/credentials/aws", c.baseURL)
	var response *entity.CreateCredentialResponse

	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}

	//headers := map[string]string{
	//	httpserver.XKaytuUserIDHeader:        ctx.UserID,
	//	httpserver.XKaytuUserRoleHeader:      string(ctx.UserRole),
	//}
	if statusCode, err := httpclient.DoRequest(ctx.Ctx, http.MethodPost, url, ctx.ToHeaders(), payload, &response); err != nil {
		if 400 <= statusCode && statusCode < 500 {
			return nil, echo.NewHTTPError(statusCode, err.Error())
		}
		return nil, err
	}
	return response, nil
}
