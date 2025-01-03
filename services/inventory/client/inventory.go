package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/opengovern/og-util/pkg/httpclient"
	"github.com/opengovern/og-util/pkg/integration"

	"github.com/labstack/echo/v4"
	"github.com/opengovern/opencomply/services/inventory/api"
)

type InventoryServiceClient interface {
	RunQuery(ctx *httpclient.Context, req api.RunQueryRequest) (*api.RunQueryResponse, error)
	GetQuery(ctx *httpclient.Context, id string) (*api.NamedQueryItemV2, error)
	ListQueriesV2(ctx *httpclient.Context, req *api.ListQueryV2Request) (*api.ListQueriesV2Response, error)
	CountResources(ctx *httpclient.Context) (int64, error)
	ListIntegrationsData(ctx *httpclient.Context, integrationIds []string, resourceCollections []string, startTime, endTime *time.Time, metricIDs []string, needCost, needResourceCount bool) (map[string]api.ConnectionData, error)
	ListResourceTypesMetadata(ctx *httpclient.Context, integrationTypes []integration.Type, services []string, resourceTypes []string, summarized bool, tags map[string]string, pageSize, pageNumber int) (*api.ListResourceTypeMetadataResponse, error)
	ListResourceCollections(ctx *httpclient.Context) ([]api.ResourceCollection, error)
	GetResourceCollectionMetadata(ctx *httpclient.Context, id string) (*api.ResourceCollection, error)
	ListResourceCollectionsMetadata(ctx *httpclient.Context, ids []string) ([]api.ResourceCollection, error)
	GetTablesResourceCategories(ctx *httpclient.Context, tables []string) ([]api.CategoriesTables, error)
	GetResourceCategories(ctx *httpclient.Context, tables []string, categories []string) (*api.GetResourceCategoriesResponse, error)
	RunQueryByID(ctx *httpclient.Context, req api.RunQueryByIDRequest) (*api.RunQueryResponse, error)
}

type inventoryClient struct {
	baseURL string
}

func NewInventoryServiceClient(baseURL string) InventoryServiceClient {
	return &inventoryClient{baseURL: baseURL}
}

func (s *inventoryClient) RunQuery(ctx *httpclient.Context, req api.RunQueryRequest) (*api.RunQueryResponse, error) {
	url := fmt.Sprintf("%s/api/v1/query/run", s.baseURL)

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var resp api.RunQueryResponse
	if statusCode, err := httpclient.DoRequest(ctx.Ctx, http.MethodPost, url, ctx.ToHeaders(), reqBytes, &resp); err != nil {
		if 400 <= statusCode && statusCode < 500 {
			return nil, echo.NewHTTPError(statusCode, err.Error())
		}
		return nil, err
	}
	return &resp, nil
}

func (s *inventoryClient) CountResources(ctx *httpclient.Context) (int64, error) {
	url := fmt.Sprintf("%s/api/v2/resources/count", s.baseURL)

	var count int64
	if statusCode, err := httpclient.DoRequest(ctx.Ctx, http.MethodGet, url, ctx.ToHeaders(), nil, &count); err != nil {
		if 400 <= statusCode && statusCode < 500 {
			return 0, echo.NewHTTPError(statusCode, err.Error())
		}
		return 0, err
	}
	return count, nil
}

func (s *inventoryClient) GetQuery(ctx *httpclient.Context, id string) (*api.NamedQueryItemV2, error) {
	url := fmt.Sprintf("%s/api/v3/queries/%s", s.baseURL, id)

	var namedQuery api.NamedQueryItemV2
	if statusCode, err := httpclient.DoRequest(ctx.Ctx, http.MethodGet, url, ctx.ToHeaders(), nil, &namedQuery); err != nil {
		if statusCode == http.StatusNotFound {
			return nil, nil
		}
	}
	return &namedQuery, nil
}

func (s *inventoryClient) ListQueriesV2(ctx *httpclient.Context, req *api.ListQueryV2Request) (*api.ListQueriesV2Response, error) {
	url := fmt.Sprintf("%s/api/v3/queries", s.baseURL)

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var namedQuery api.ListQueriesV2Response
	if statusCode, err := httpclient.DoRequest(ctx.Ctx, http.MethodPost, url, ctx.ToHeaders(), reqBytes, &namedQuery); err != nil {
		if statusCode == http.StatusNotFound {
			return nil, nil
		}
	}
	return &namedQuery, nil
}

func (s *inventoryClient) GetTablesResourceCategories(ctx *httpclient.Context, tables []string) ([]api.CategoriesTables, error) {
	url := fmt.Sprintf("%s/api/v3/tables/categories", s.baseURL)

	firstParamAttached := false
	if len(tables) > 0 {
		for _, t := range tables {
			if !firstParamAttached {
				url += "?"
				firstParamAttached = true
			} else {
				url += "&"
			}
			url += fmt.Sprintf("tables=%s", t)
		}
	}

	var resp []api.CategoriesTables
	if statusCode, err := httpclient.DoRequest(ctx.Ctx, http.MethodGet, url, ctx.ToHeaders(), nil, &resp); err != nil {
		if 400 <= statusCode && statusCode < 500 {
			return nil, echo.NewHTTPError(statusCode, err.Error())
		}
		return nil, err
	}
	return resp, nil
}

func (s *inventoryClient) GetResourceCategories(ctx *httpclient.Context, tables []string, categories []string) (*api.GetResourceCategoriesResponse, error) {
	url := fmt.Sprintf("%s/api/v3/resources/categories", s.baseURL)

	firstParamAttached := false
	if len(tables) > 0 {
		for _, t := range tables {
			if !firstParamAttached {
				url += "?"
				firstParamAttached = true
			} else {
				url += "&"
			}
			url += fmt.Sprintf("tables=%s", t)
		}
	}
	if len(categories) > 0 {
		for _, t := range categories {
			if !firstParamAttached {
				url += "?"
				firstParamAttached = true
			} else {
				url += "&"
			}
			url += fmt.Sprintf("categories=%s", t)
		}
	}

	var resp api.GetResourceCategoriesResponse
	if statusCode, err := httpclient.DoRequest(ctx.Ctx, http.MethodGet, url, ctx.ToHeaders(), nil, &resp); err != nil {
		if 400 <= statusCode && statusCode < 500 {
			return nil, echo.NewHTTPError(statusCode, err.Error())
		}
		return nil, err
	}
	return &resp, nil
}

func (s *inventoryClient) ListIntegrationsData(
	ctx *httpclient.Context,
	integrationIds, resourceCollections []string,
	startTime, endTime *time.Time, metricIDs []string,
	needCost, needResourceCount bool,
) (map[string]api.ConnectionData, error) {
	params := url.Values{}

	url := fmt.Sprintf("%s/api/v2/integrations/data", s.baseURL)

	if len(integrationIds) > 0 {
		for _, integrationId := range integrationIds {
			params.Add("integrationId", integrationId)
		}
	}
	if len(resourceCollections) > 0 {
		for _, resourceCollection := range resourceCollections {
			params.Add("resourceCollection", resourceCollection)
		}
	}
	if len(metricIDs) > 0 {
		for _, metricID := range metricIDs {
			params.Add("metricId", metricID)
		}

	}
	if startTime != nil {
		params.Set("startTime", strconv.FormatInt(startTime.Unix(), 10))
	}
	if endTime != nil {
		params.Set("endTime", strconv.FormatInt(endTime.Unix(), 10))
	}
	if !needCost {
		params.Set("needCost", "false")
	}
	if !needResourceCount {
		params.Set("needResourceCount", "false")
	}
	if len(params) > 0 {
		url += "?" + params.Encode()
	}
	var response map[string]api.ConnectionData
	if statusCode, err := httpclient.DoRequest(ctx.Ctx, http.MethodGet, url, ctx.ToHeaders(), nil, &response); err != nil {
		if 400 <= statusCode && statusCode < 500 {
			return nil, echo.NewHTTPError(statusCode, err.Error())
		}
		return nil, err
	}
	return response, nil
}

func (s *inventoryClient) ListResourceTypesMetadata(ctx *httpclient.Context, integrationTypes []integration.Type, services []string, resourceTypes []string, summarized bool, tags map[string]string, pageSize, pageNumber int) (*api.ListResourceTypeMetadataResponse, error) {
	url := fmt.Sprintf("%s/api/v2/metadata/resourcetype", s.baseURL)
	firstParamAttached := false
	if len(integrationTypes) > 0 {
		for _, integrationType := range integrationTypes {
			if !firstParamAttached {
				url += "?"
				firstParamAttached = true
			} else {
				url += "&"
			}
			url += fmt.Sprintf("integrationType=%s", integrationType)
		}
	}
	if len(services) > 0 {
		for _, service := range services {
			if !firstParamAttached {
				url += "?"
				firstParamAttached = true
			} else {
				url += "&"
			}
			url += fmt.Sprintf("service=%s", service)
		}
	}
	if len(resourceTypes) > 0 {
		for _, resourceType := range resourceTypes {
			if !firstParamAttached {
				url += "?"
				firstParamAttached = true
			} else {
				url += "&"
			}
			url += fmt.Sprintf("resourceType=%s", resourceType)
		}
	}
	if summarized {
		if !firstParamAttached {
			url += "?"
			firstParamAttached = true
		} else {
			url += "&"
		}
		url += "summarized=true"
	}
	if len(tags) > 0 {
		for key, value := range tags {
			if !firstParamAttached {
				url += "?"
				firstParamAttached = true
			} else {
				url += "&"
			}
			url += fmt.Sprintf("tags=%s=%s", key, value)
		}
	}
	if pageSize > 0 {
		if !firstParamAttached {
			url += "?"
			firstParamAttached = true
		} else {
			url += "&"
		}
		url += fmt.Sprintf("pageSize=%d", pageSize)
	}
	if pageNumber > 0 {
		if !firstParamAttached {
			url += "?"
			firstParamAttached = true
		} else {
			url += "&"
		}
		url += fmt.Sprintf("pageNumber=%d", pageNumber)
	}

	var response api.ListResourceTypeMetadataResponse
	if statusCode, err := httpclient.DoRequest(ctx.Ctx, http.MethodGet, url, ctx.ToHeaders(), nil, &response); err != nil {
		if 400 <= statusCode && statusCode < 500 {
			return nil, echo.NewHTTPError(statusCode, err.Error())
		}
		return nil, err
	}
	return &response, nil
}

func (s *inventoryClient) GetResourceCollectionMetadata(ctx *httpclient.Context, id string) (*api.ResourceCollection, error) {
	url := fmt.Sprintf("%s/api/v2/metadata/resource-collection/%s", s.baseURL, id)

	var response api.ResourceCollection
	if statusCode, err := httpclient.DoRequest(ctx.Ctx, http.MethodGet, url, ctx.ToHeaders(), nil, &response); err != nil {
		if 400 <= statusCode && statusCode < 500 {
			return nil, echo.NewHTTPError(statusCode, err.Error())
		}
		return nil, err
	}
	return &response, nil
}

func (s *inventoryClient) ListResourceCollectionsMetadata(ctx *httpclient.Context, ids []string) ([]api.ResourceCollection, error) {
	url := fmt.Sprintf("%s/api/v2/metadata/resource-collection", s.baseURL)

	firstParamAttached := false
	for _, id := range ids {
		if !firstParamAttached {
			url += "?"
			firstParamAttached = true
		} else {
			url += "&"
		}
		url += fmt.Sprintf("id=%s", id)
	}

	var response []api.ResourceCollection
	if statusCode, err := httpclient.DoRequest(ctx.Ctx, http.MethodGet, url, ctx.ToHeaders(), nil, &response); err != nil {
		if 400 <= statusCode && statusCode < 500 {
			return nil, echo.NewHTTPError(statusCode, err.Error())
		}
		return nil, err
	}
	return response, nil
}

func (s *inventoryClient) ListResourceCollections(ctx *httpclient.Context) ([]api.ResourceCollection, error) {
	url := fmt.Sprintf("%s/api/v2/metadata/resource-collection", s.baseURL)

	var response []api.ResourceCollection
	if statusCode, err := httpclient.DoRequest(ctx.Ctx, http.MethodGet, url, ctx.ToHeaders(), nil, &response); err != nil {
		if 400 <= statusCode && statusCode < 500 {
			return nil, echo.NewHTTPError(statusCode, err.Error())
		}
		return nil, err
	}
	return response, nil
}

func (s *inventoryClient) RunQueryByID(ctx *httpclient.Context, req api.RunQueryByIDRequest) (*api.RunQueryResponse, error) {
	url := fmt.Sprintf("%s/api/v3/query/run", s.baseURL)

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var resp api.RunQueryResponse
	if statusCode, err := httpclient.DoRequest(ctx.Ctx, http.MethodPost, url, ctx.ToHeaders(), reqBytes, &resp); err != nil {
		if 400 <= statusCode && statusCode < 500 {
			return nil, echo.NewHTTPError(statusCode, err.Error())
		}
		return nil, err
	}
	return &resp, nil
}
