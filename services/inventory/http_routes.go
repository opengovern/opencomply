package inventory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/opengovern/og-util/pkg/api"
	"github.com/opengovern/og-util/pkg/httpclient"
	"github.com/opengovern/og-util/pkg/httpserver"
	"github.com/opengovern/og-util/pkg/integration"
	queryrunner "github.com/opengovern/opencomply/jobs/query-runner-job"
	"github.com/opengovern/opencomply/pkg/types"
	integration_type "github.com/opengovern/opencomply/services/integration/integration-type"
	"github.com/opengovern/opencomply/services/inventory/rego_runner"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin"

	"github.com/labstack/echo/v4"
	"github.com/open-policy-agent/opa/rego"
	"github.com/opengovern/og-util/pkg/model"
	esSdk "github.com/opengovern/og-util/pkg/opengovernance-es-sdk"
	"github.com/opengovern/og-util/pkg/steampipe"
	"github.com/opengovern/opencomply/pkg/utils"
	integrationApi "github.com/opengovern/opencomply/services/integration/api/models"
	inventoryApi "github.com/opengovern/opencomply/services/inventory/api"
	"github.com/opengovern/opencomply/services/inventory/es"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	EsFetchPageSize = 10000
	MaxConns        = 100
	KafkaPageSize   = 5000
)

const (
	AWSLogoURI   = "https://raw.githubusercontent.com/kaytu-io/awsicons/master/svg-export/icons/AWS.svg"
	AzureLogoURI = "https://raw.githubusercontent.com/kaytu-io/Azure-Design/master/SVG_Azure_All/Azure.svg"
)

const (
	IntegrationIdParam    = "integrationId"
	IntegrationGroupParam = "integrationGroup"
)

func (h *HttpHandler) Register(e *echo.Echo) {
	v1 := e.Group("/api/v1")

	queryV1 := v1.Group("/query")
	queryV1.GET("", httpserver.AuthorizeHandler(h.ListQueries, api.ViewerRole))
	queryV1.POST("/run", httpserver.AuthorizeHandler(h.RunQuery, api.ViewerRole))
	queryV1.GET("/run/history", httpserver.AuthorizeHandler(h.GetRecentRanQueries, api.ViewerRole))

	v2 := e.Group("/api/v2")

	metadata := v2.Group("/metadata")
	metadata.GET("/resourcetype", httpserver.AuthorizeHandler(h.ListResourceTypeMetadata, api.ViewerRole))

	resourceCollectionMetadata := metadata.Group("/resource-collection")
	resourceCollectionMetadata.GET("", httpserver.AuthorizeHandler(h.ListResourceCollectionsMetadata, api.ViewerRole))
	resourceCollectionMetadata.GET("/:resourceCollectionId", httpserver.AuthorizeHandler(h.GetResourceCollectionMetadata, api.ViewerRole))

	v3 := e.Group("/api/v3")
	v3.POST("/queries", httpserver.AuthorizeHandler(h.ListQueriesV2, api.ViewerRole))
	v3.GET("/queries/filters", httpserver.AuthorizeHandler(h.ListQueriesFilters, api.ViewerRole))
	v3.GET("/queries/:query_id", httpserver.AuthorizeHandler(h.GetQuery, api.ViewerRole))
	v3.GET("/queries/tags", httpserver.AuthorizeHandler(h.ListQueriesTags, api.ViewerRole))
	v3.POST("/query/run", httpserver.AuthorizeHandler(h.RunQueryByID, api.ViewerRole))
	v3.GET("/query/async/run/:run_id/result", httpserver.AuthorizeHandler(h.GetAsyncQueryRunResult, api.ViewerRole))
	v3.GET("/resources/categories", httpserver.AuthorizeHandler(h.GetResourceCategories, api.ViewerRole))
	v3.GET("/queries/categories", httpserver.AuthorizeHandler(h.GetQueriesResourceCategories, api.ViewerRole))
	v3.GET("/tables/categories", httpserver.AuthorizeHandler(h.GetTablesResourceCategories, api.ViewerRole))
	v3.GET("/categories/queries", httpserver.AuthorizeHandler(h.GetCategoriesQueries, api.ViewerRole))
	v3.GET("/parameters/queries", httpserver.AuthorizeHandler(h.GetParametersQueries, api.ViewerRole))
}

var tracer = otel.Tracer("new_inventory")

func (h *HttpHandler) getConnectionIdFilterFromParams(ctx echo.Context) ([]string, error) {
	integrationIds := httpserver.QueryArrayParam(ctx, IntegrationIdParam)
	integrationIds, err := httpserver.ResolveConnectionIDs(ctx, integrationIds)
	if err != nil {
		return nil, err
	}

	integrationGroup := httpserver.QueryArrayParam(ctx, IntegrationGroupParam)
	if len(integrationIds) == 0 && len(integrationGroup) == 0 {
		return nil, nil
	}

	if len(integrationIds) > 0 && len(integrationGroup) > 0 {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "integrationId and integrationGroup cannot be used together")
	}

	if len(integrationIds) > 0 {
		return integrationIds, nil
	}

	integrationMap := map[string]bool{}
	for _, integrationGroupID := range integrationGroup {
		integrationGroupObj, err := h.integrationClient.GetIntegrationGroup(&httpclient.Context{UserRole: api.AdminRole}, integrationGroupID)
		if err != nil {
			return nil, err
		}
		for _, integrationId := range integrationGroupObj.IntegrationIds {
			integrationMap[integrationId] = true
		}
	}
	integrationIds = make([]string, 0, len(integrationMap))
	for integrationId := range integrationMap {
		integrationIds = append(integrationIds, integrationId)
	}
	if len(integrationIds) == 0 {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "integrationGroup(s) do not have any integrations")
	}

	return integrationIds, nil
}

func bindValidate(ctx echo.Context, i interface{}) error {
	if err := ctx.Bind(i); err != nil {
		return err
	}

	if err := ctx.Validate(i); err != nil {
		return err
	}

	return nil
}

func (h *HttpHandler) getIntegrationTypesFromIntegrationIDs(ctx echo.Context, integrationTypes []integration.Type, integrationIDs []string) ([]integration.Type, error) {
	if len(integrationIDs) == 0 {
		return integrationTypes, nil
	}
	if len(integrationTypes) != 0 {
		return integrationTypes, nil
	}
	integrations, err := h.integrationClient.ListIntegrationsByFilters(&httpclient.Context{UserRole: api.AdminRole}, integrationApi.ListIntegrationsRequest{
		IntegrationID: integrationIDs,
	})
	if err != nil {
		return nil, err
	}

	enabledIntegrations := make(map[integration.Type]bool)
	for _, integration := range integrations.Integrations {
		enabledIntegrations[integration.IntegrationType] = true
	}
	integrationTypes = make([]integration.Type, 0, len(enabledIntegrations))
	for integrationType := range enabledIntegrations {
		integrationTypes = append(integrationTypes, integrationType)
	}

	return integrationTypes, nil
}

// ListQueries godoc
//
//	@Summary		List named queries
//	@Description	Retrieving list of named queries by specified filters
//	@Security		BearerToken
//	@Tags			named_query
//	@Produce		json
//	@Param			request	body		inventoryApi.ListQueryRequest	true	"Request Body"
//	@Success		200		{object}	[]inventoryApi.NamedQueryItem
//	@Router			/inventory/api/v1/query [get]
func (h *HttpHandler) ListQueries(ctx echo.Context) error {
	var req inventoryApi.ListQueryRequest
	if err := bindValidate(ctx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	var search *string
	if len(req.TitleFilter) > 0 {
		search = &req.TitleFilter
	}
	// trace :
	_, span := tracer.Start(ctx.Request().Context(), "new_GetQueriesWithFilters", trace.WithSpanKind(trace.SpanKindServer))
	span.SetName("new_GetQueriesWithFilters")

	queries, err := h.db.GetQueriesWithFilters(search)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	span.End()

	var result []inventoryApi.NamedQueryItem
	for _, item := range queries {
		category := ""

		tags := map[string]string{}
		if item.IsBookmarked {
			tags["platform_queries_bookmark"] = "true"
		}
		result = append(result, inventoryApi.NamedQueryItem{
			ID:               item.ID,
			IntegrationTypes: integration_type.ParseTypes(item.IntegrationTypes),
			Title:            item.Title,
			Category:         category,
			Query:            item.Query.QueryToExecute,
			Tags:             tags,
		})
	}
	return ctx.JSON(200, result)
}

// ListQueriesV2 godoc
//
//	@Summary		List named queries
//	@Description	Retrieving list of named queries by specified filters and tags filters
//	@Security		BearerToken
//	@Tags			named_query
//	@Produce		json
//	@Param			request	body		inventoryApi.ListQueryV2Request	true	"List Queries Filters"
//	@Success		200		{object}	inventoryApi.ListQueriesV2Response
//	@Router			/inventory/api/v3/queries [post]
func (h *HttpHandler) ListQueriesV2(ctx echo.Context) error {
	var req inventoryApi.ListQueryV2Request
	if err := bindValidate(ctx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	var search *string
	if len(req.TitleFilter) > 0 {
		search = &req.TitleFilter
	}

	integrationTypes := make(map[string]bool)
	integrations, err := h.integrationClient.ListIntegrations(&httpclient.Context{UserRole: api.AdminRole}, nil)
	if err != nil {
		h.logger.Error("failed to get integrations list", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get integrations list")
	}
	for _, i := range integrations.Integrations {
		integrationTypes[i.IntegrationType.String()] = true
	}

	// trace :
	_, span := tracer.Start(ctx.Request().Context(), "new_GetQueriesWithTagsFilters", trace.WithSpanKind(trace.SpanKindServer))
	span.SetName("new_GetQueriesWithTagsFilters")

	var tablesFilter []string
	if len(req.Categories) > 0 {

		categories, err := h.db.ListUniqueCategoriesAndTablesForTables(nil)
		if err != nil {
			h.logger.Error("failed to list resource categories", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to list resource categories")
		}
		categoriesFilterMap := make(map[string]bool)
		for _, c := range req.Categories {
			categoriesFilterMap[c] = true
		}

		var categoriesApi []inventoryApi.ResourceCategory
		for _, c := range categories {
			if _, ok := categoriesFilterMap[c.Category]; !ok && len(req.Categories) > 0 {
				continue
			}
			resourceTypes, err := h.db.ListCategoryResourceTypes(c.Category)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "list category resource types")
			}
			var resourceTypesApi []inventoryApi.ResourceTypeV2
			for _, r := range resourceTypes {
				resourceTypesApi = append(resourceTypesApi, r.ToApi())
			}
			categoriesApi = append(categoriesApi, inventoryApi.ResourceCategory{
				Category:  c.Category,
				Resources: resourceTypesApi,
			})
		}

		tablesFilterMap := make(map[string]string)

		for _, c := range categoriesApi {
			for _, r := range c.Resources {
				tablesFilterMap[r.SteampipeTable] = r.ResourceID
			}
		}
		if len(req.ListOfTables) > 0 {
			for _, t := range req.ListOfTables {
				if _, ok := tablesFilterMap[t]; ok {
					tablesFilter = append(tablesFilter, t)
				}
			}
		} else {
			for t, _ := range tablesFilterMap {
				tablesFilter = append(tablesFilter, t)
			}
		}
	} else {
		tablesFilter = req.ListOfTables
	}

	queries, err := h.db.ListQueriesByFilters(req.QueryIDs, search, req.Tags, req.IntegrationTypes, req.HasParameters, req.PrimaryTable,
		tablesFilter, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	span.End()

	var items []inventoryApi.NamedQueryItemV2
	for _, item := range queries {
		if req.IsBookmarked {
			if !item.IsBookmarked {
				continue
			}
		}
		if req.IntegrationExists {
			integrationExists := false
			for _, i := range item.IntegrationTypes {
				if _, ok := integrationTypes[i]; ok {
					integrationExists = true
				}
			}
			if !integrationExists {
				continue
			}
		}

		tags := item.GetTagsMap()
		if tags == nil || len(tags) == 0 {
			tags = make(map[string][]string)
		}
		if item.IsBookmarked {
			tags["platform_queries_bookmark"] = []string{"true"}
		}
		items = append(items, inventoryApi.NamedQueryItemV2{
			ID:               item.ID,
			Title:            item.Title,
			Description:      item.Description,
			IntegrationTypes: integration_type.ParseTypes(item.IntegrationTypes),
			Query:            item.Query.ToApi(),
			Tags:             filterTagsByRegex(req.TagsRegex, tags),
		})
	}

	totalCount := len(items)

	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
	if req.PerPage != nil {
		if req.Cursor == nil {
			items = utils.Paginate(1, *req.PerPage, items)
		} else {
			items = utils.Paginate(*req.Cursor, *req.PerPage, items)
		}
	}

	result := inventoryApi.ListQueriesV2Response{
		Items:      items,
		TotalCount: totalCount,
	}

	return ctx.JSON(http.StatusOK, result)
}

// GetQuery godoc
//
//	@Summary		Get named query by ID
//	@Description	Retrieving list of named queries by specified filters and tags filters
//	@Security		BearerToken
//	@Tags			named_query
//	@Produce		json
//	@Param			query_id	path		string	true	"QueryID"
//	@Success		200			{object}	inventoryApi.NamedQueryItemV2
//	@Router			/inventory/api/v3/queries/{query_id} [get]
func (h *HttpHandler) GetQuery(ctx echo.Context) error {
	queryID := ctx.Param("query_id")

	// trace :
	_, span := tracer.Start(ctx.Request().Context(), "new_GetQuery", trace.WithSpanKind(trace.SpanKindServer))
	span.SetName("new_GetQuery")

	query, err := h.db.GetQuery(queryID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if query == nil {
		return echo.NewHTTPError(http.StatusNotFound, "query not found")
	}
	span.End()
	tags := query.GetTagsMap()
	if query.IsBookmarked {
		tags["platform_queries_bookmark"] = []string{"true"}
	}
	result := inventoryApi.NamedQueryItemV2{
		ID:               query.ID,
		Title:            query.Title,
		Description:      query.Description,
		IntegrationTypes: integration_type.ParseTypes(query.IntegrationTypes),
		Query:            query.Query.ToApi(),
		Tags:             tags,
	}

	return ctx.JSON(http.StatusOK, result)
}

func filterTagsByRegex(regexPattern *string, tags map[string][]string) map[string][]string {
	if regexPattern == nil {
		return tags
	}
	re := regexp.MustCompile(*regexPattern)

	resultsMap := make(map[string][]string)
	for k, v := range tags {
		if re.MatchString(k) {
			resultsMap[k] = v
		}
	}
	return resultsMap
}

// ListQueriesTags godoc
//
//	@Summary		List named queries tags
//	@Description	Retrieving list of named queries by specified filters
//	@Security		BearerToken
//	@Tags			named_query
//	@Produce		json
//	@Success		200	{object}	[]inventoryApi.NamedQueryTagsResult
//	@Router			/inventory/api/v3/query/tags [get]
func (h *HttpHandler) ListQueriesTags(ctx echo.Context) error {
	// trace :
	_, span := tracer.Start(ctx.Request().Context(), "new_GetQueriesWithFilters", trace.WithSpanKind(trace.SpanKindServer))
	span.SetName("new_GetQueriesWithFilters")

	namedQueriesTags, err := h.db.GetQueriesTags()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	res := make([]inventoryApi.NamedQueryTagsResult, 0, len(namedQueriesTags))
	for _, history := range namedQueriesTags {
		res = append(res, history.ToApi())
	}

	span.End()

	return ctx.JSON(200, res)
}

// RunQuery godoc
//
//	@Summary		Run query
//	@Description	Run provided named query and returns the result.
//	@Security		BearerToken
//	@Tags			named_query
//	@Accepts		json
//	@Produce		json
//	@Param			request	body		inventoryApi.RunQueryRequest	true	"Request Body"
//	@Param			accept	header		string							true	"Accept header"	Enums(application/json,text/csv)
//	@Success		200		{object}	inventoryApi.RunQueryResponse
//	@Router			/inventory/api/v1/query/run [post]
func (h *HttpHandler) RunQuery(ctx echo.Context) error {
	var req inventoryApi.RunQueryRequest
	if err := bindValidate(ctx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Query == nil || *req.Query == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Query is required")
	}
	// tracer :
	outputS, span := tracer.Start(ctx.Request().Context(), "new_RunQuery", trace.WithSpanKind(trace.SpanKindServer))
	span.SetName("new_RunQuery")

	queryParams, err := h.metadataClient.ListQueryParameters(&httpclient.Context{UserRole: api.AdminRole})
	if err != nil {
		return err
	}
	queryParamMap := make(map[string]string)
	for _, qp := range queryParams.Items {
		queryParamMap[qp.Key] = qp.Value
	}

	queryTemplate, err := template.New("query").Parse(*req.Query)
	if err != nil {
		return err
	}
	var queryOutput bytes.Buffer
	if err := queryTemplate.Execute(&queryOutput, queryParamMap); err != nil {
		return fmt.Errorf("failed to execute query template: %w", err)
	}

	var resp *inventoryApi.RunQueryResponse
	if req.Engine == nil || *req.Engine == inventoryApi.QueryEngineCloudQL {
		resp, err = h.RunSQLNamedQuery(outputS, *req.Query, queryOutput.String(), &req)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	} else if *req.Engine == inventoryApi.QueryEngineCloudQLRego {
		resp, err = h.RunRegoNamedQuery(outputS, *req.Query, queryOutput.String(), &req)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	} else {
		return fmt.Errorf("invalid query engine: %s", *req.Engine)
	}

	span.AddEvent("information", trace.WithAttributes(
		attribute.String("query title ", resp.Title),
	))
	span.End()
	return ctx.JSON(200, resp)
}

// GetRecentRanQueries godoc
//
//	@Summary		List recently ran queries
//	@Description	List queries which have been run recently
//	@Security		BearerToken
//	@Tags			named_query
//	@Accepts		json
//	@Produce		json
//	@Success		200	{object}	[]inventoryApi.NamedQueryHistory
//	@Router			/inventory/api/v1/query/run/history [get]
func (h *HttpHandler) GetRecentRanQueries(ctx echo.Context) error {
	// trace :
	_, span := tracer.Start(ctx.Request().Context(), "new_GetQueryHistory", trace.WithSpanKind(trace.SpanKindServer))
	span.SetName("new_GetQueryHistory")

	namedQueryHistories, err := h.db.GetQueryHistory()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.logger.Error("Failed to get query history", zap.Error(err))
		return err
	}
	span.End()

	res := make([]inventoryApi.NamedQueryHistory, 0, len(namedQueryHistories))
	for _, history := range namedQueryHistories {
		res = append(res, history.ToApi())
	}

	return ctx.JSON(200, res)
}

func (h *HttpHandler) RunSQLNamedQuery(ctx context.Context, title, query string, req *inventoryApi.RunQueryRequest) (*inventoryApi.RunQueryResponse, error) {
	var err error
	lastIdx := (req.Page.No - 1) * req.Page.Size

	direction := inventoryApi.DirectionType("")
	orderBy := ""
	if req.Sorts != nil && len(req.Sorts) > 0 {
		direction = req.Sorts[0].Direction
		orderBy = req.Sorts[0].Field
	}
	if len(req.Sorts) > 1 {
		return nil, errors.New("multiple sort items not supported")
	}

	for i := 0; i < 10; i++ {
		err = h.steampipeConn.Conn().Ping(ctx)
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		h.logger.Error("failed to ping steampipe", zap.Error(err))
		return nil, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	h.logger.Info("executing named query", zap.String("query", query))
	res, err := h.steampipeConn.Query(ctx, query, &lastIdx, &req.Page.Size, orderBy, steampipe.DirectionType(direction))
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// tracer :
	integrations, err := h.integrationClient.ListIntegrations(&httpclient.Context{UserRole: api.AdminRole}, nil)
	if err != nil {
		return nil, err
	}
	integrationToNameMap := make(map[string]string)
	for _, integration := range integrations.Integrations {
		integrationToNameMap[integration.IntegrationID] = integration.Name
	}

	accountIDExists := false
	for _, header := range res.Headers {
		if header == "platform_account_id" {
			accountIDExists = true
		}
	}

	if accountIDExists {
		// Add account name
		res.Headers = append(res.Headers, "account_name")
		for colIdx, header := range res.Headers {
			if strings.ToLower(header) != "platform_account_id" {
				continue
			}
			for rowIdx, row := range res.Data {
				if len(row) <= colIdx {
					continue
				}
				if row[colIdx] == nil {
					continue
				}
				if accountID, ok := row[colIdx].(string); ok {
					if accountName, ok := integrationToNameMap[accountID]; ok {
						res.Data[rowIdx] = append(res.Data[rowIdx], accountName)
					} else {
						res.Data[rowIdx] = append(res.Data[rowIdx], "null")
					}
				}
			}
		}
	}

	_, span := tracer.Start(ctx, "new_UpdateQueryHistory", trace.WithSpanKind(trace.SpanKindServer))
	span.SetName("new_UpdateQueryHistory")

	err = h.db.UpdateQueryHistory(query)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.logger.Error("failed to update query history", zap.Error(err))
		return nil, err
	}
	span.End()

	resp := inventoryApi.RunQueryResponse{
		Title:   title,
		Query:   query,
		Headers: res.Headers,
		Result:  res.Data,
	}
	return &resp, nil
}

type resourceFieldItem struct {
	fieldName string
	value     interface{}
}

func (h *HttpHandler) RunRegoNamedQuery(ctx context.Context, title, query string, req *inventoryApi.RunQueryRequest) (*inventoryApi.RunQueryResponse, error) {
	var err error
	lastIdx := (req.Page.No - 1) * req.Page.Size

	reqoQuery, err := rego.New(
		rego.Query("x = data.cloudql.query.allow; resource_type = data.cloudql.query.resource_type"),
		rego.Module("cloudql.query", query),
	).PrepareForEval(ctx)
	if err != nil {
		return nil, err
	}
	results, err := reqoQuery.Eval(ctx, rego.EvalInput(map[string]interface{}{}))
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("undefined result")
	}
	resourceType, ok := results[0].Bindings["resource_type"].(string)
	if !ok {
		return nil, errors.New("resource_type not defined")
	}
	h.logger.Info("reqo runner", zap.String("resource_type", resourceType))

	var filters []esSdk.BoolFilter
	if req.AccountId != nil {
		if len(*req.AccountId) > 0 && *req.AccountId != "all" {
			var accountFieldName string
			// TODO: removed for integration dependencies
			//awsRTypes := onboardApi.GetAWSSupportedResourceTypeMap()
			//if _, ok := awsRTypes[strings.ToLower(resourceType)]; ok {
			//	accountFieldName = "AccountID"
			//}
			//azureRTypes := onboardApi.GetAzureSupportedResourceTypeMap()
			//if _, ok := azureRTypes[strings.ToLower(resourceType)]; ok {
			//	accountFieldName = "SubscriptionID"
			//}

			filters = append(filters, esSdk.NewTermFilter("metadata."+accountFieldName, *req.AccountId))
		}
	}

	if req.SourceId != nil {
		filters = append(filters, esSdk.NewTermFilter("source_id", *req.SourceId))
	}

	jsonFilters, _ := json.Marshal(filters)
	plugin.Logger(ctx).Trace("reqo runner", "filters", filters, "jsonFilters", string(jsonFilters))

	paginator, err := rego_runner.Client{ES: h.client}.NewResourcePaginator(filters, nil, types.ResourceTypeToESIndex(resourceType))
	if err != nil {
		return nil, err
	}
	defer paginator.Close(ctx)

	ignore := lastIdx
	size := req.Page.Size

	h.logger.Info("reqo runner page", zap.Int("ignoreInit", ignore), zap.Int("sizeInit", size), zap.Bool("hasPage", paginator.HasNext()))
	var header []string
	var result [][]any
	for paginator.HasNext() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, v := range page {
			if ignore > 0 {
				h.logger.Info("rego ignoring resource", zap.Int("ignore", ignore))
				ignore--
				continue
			}

			if size <= 0 {
				h.logger.Info("rego pagination finished", zap.Int("size", size))
				break
			}

			evalResults, err := reqoQuery.Eval(ctx, rego.EvalInput(v))
			if err != nil {
				return nil, err
			}
			if len(evalResults) == 0 {
				return nil, fmt.Errorf("undefined result")
			}

			allowed, ok := evalResults[0].Bindings["x"].(bool)
			if !ok {
				return nil, errors.New("x not defined")
			}

			if !allowed {
				h.logger.Info("rego resource not allowed", zap.Any("resource", v))
				continue
			}

			var cells []resourceFieldItem
			for k, vv := range v {
				cells = append(cells, resourceFieldItem{
					fieldName: k,
					value:     vv,
				})
			}
			sort.Slice(cells, func(i, j int) bool {
				return cells[i].fieldName < cells[j].fieldName
			})

			if len(header) == 0 {
				for _, c := range cells {
					header = append(header, c.fieldName)
				}
			}

			size--
			var res []any
			for _, va := range cells {
				res = append(res, va.value)
			}
			result = append(result, res)
		}
	}

	_, span := tracer.Start(ctx, "new_UpdateQueryHistory", trace.WithSpanKind(trace.SpanKindServer))
	span.SetName("new_UpdateQueryHistory")

	err = h.db.UpdateQueryHistory(query)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		h.logger.Error("failed to update query history", zap.Error(err))
		return nil, err
	}
	span.End()

	resp := inventoryApi.RunQueryResponse{
		Title:   title,
		Query:   query,
		Headers: header,
		Result:  result,
	}
	return &resp, nil
}

func (h *HttpHandler) ListResourceTypeMetadata(ctx echo.Context) error {
	tagMap := model.TagStringsToTagMap(httpserver.QueryArrayParam(ctx, "tag"))
	integrationTypes := integration_type.ParseTypes(httpserver.QueryArrayParam(ctx, "integrationType"))
	serviceNames := httpserver.QueryArrayParam(ctx, "service")
	resourceTypeNames := httpserver.QueryArrayParam(ctx, "resourceType")
	summarized := strings.ToLower(ctx.QueryParam("summarized")) == "true"
	pageNumber, pageSize, err := utils.PageConfigFromStrings(ctx.QueryParam("pageNumber"), ctx.QueryParam("pageSize"))
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, err.Error())
	}
	// trace :
	_, span := tracer.Start(ctx.Request().Context(), "new_ListFilteredResourceTypes", trace.WithSpanKind(trace.SpanKindServer))
	span.SetName("new_ListFilteredResourceTypes")

	resourceTypes, err := h.db.ListFilteredResourceTypes(tagMap, resourceTypeNames, serviceNames, integrationTypes, summarized)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	span.End()

	var resourceTypeMetadata []inventoryApi.ResourceType

	for _, resourceType := range resourceTypes {
		apiResourceType := resourceType.ToApi()
		resourceTypeMetadata = append(resourceTypeMetadata, apiResourceType)
	}

	sort.Slice(resourceTypeMetadata, func(i, j int) bool {
		return resourceTypeMetadata[i].ResourceType < resourceTypeMetadata[j].ResourceType
	})

	result := inventoryApi.ListResourceTypeMetadataResponse{
		TotalResourceTypeCount: len(resourceTypeMetadata),
		ResourceTypes:          utils.Paginate(pageNumber, pageSize, resourceTypeMetadata),
	}

	return ctx.JSON(http.StatusOK, result)
}

// ListResourceCollectionsMetadata godoc
//
//	@Summary		List resource collections
//	@Description	Retrieving list of resource collections by specified filters
//	@Security		BearerToken
//	@Tags			resource_collection
//	@Produce		json
//	@Param			id		query		[]string								false	"Resource collection IDs"
//	@Param			status	query		[]inventoryApi.ResourceCollectionStatus	false	"Resource collection status"
//	@Success		200		{object}	[]inventoryApi.ResourceCollection
//	@Router			/inventory/api/v2/metadata/resource-collection [get]
func (h *HttpHandler) ListResourceCollectionsMetadata(ctx echo.Context) error {
	ids := httpserver.QueryArrayParam(ctx, "id")

	statuesString := httpserver.QueryArrayParam(ctx, "status")
	var statuses []ResourceCollectionStatus
	for _, statusString := range statuesString {
		statuses = append(statuses, ResourceCollectionStatus(statusString))
	}

	resourceCollections, err := h.db.ListResourceCollections(ids, nil)
	if err != nil {
		return err
	}

	res := make([]inventoryApi.ResourceCollection, 0, len(resourceCollections))
	for _, collection := range resourceCollections {
		res = append(res, collection.ToApi())
	}

	return ctx.JSON(http.StatusOK, res)
}

// GetResourceCollectionMetadata godoc
//
//	@Summary		Get resource collection
//	@Description	Retrieving resource collection by specified ID
//	@Security		BearerToken
//	@Tags			resource_collection
//	@Produce		json
//	@Param			resourceCollectionId	path		string	true	"Resource collection ID"
//	@Success		200						{object}	inventoryApi.ResourceCollection
//	@Router			/inventory/api/v2/metadata/resource-collection/{resourceCollectionId} [get]
func (h *HttpHandler) GetResourceCollectionMetadata(ctx echo.Context) error {
	collectionID := ctx.Param("resourceCollectionId")
	resourceCollection, err := h.db.GetResourceCollection(collectionID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return echo.NewHTTPError(http.StatusNotFound, "resource collection not found")
		}
		return err
	}
	return ctx.JSON(http.StatusOK, resourceCollection.ToApi())
}

func (h *HttpHandler) connectionsFilter(filter map[string]interface{}) ([]string, error) {
	var integrations []string
	allIntegrations, err := h.integrationClient.ListIntegrations(&httpclient.Context{UserRole: api.AdminRole}, nil)
	if err != nil {
		return nil, err
	}
	var allIntegrationsSrt []string
	for _, c := range allIntegrations.Integrations {
		allIntegrationsSrt = append(allIntegrationsSrt, c.IntegrationID)
	}
	for key, value := range filter {
		if key == "Match" {
			dimFilter := value.(map[string]interface{})
			if dimKey, ok := dimFilter["Key"]; ok {
				if dimKey == "IntegrationID" {
					integrations, err = dimFilterFunction(dimFilter, allIntegrationsSrt)
					if err != nil {
						return nil, err
					}
					h.logger.Warn(fmt.Sprintf("===Dim Filter Function on filter %v, result: %v", dimFilter, integrations))
				} else if dimKey == "Provider" {
					providers, err := dimFilterFunction(dimFilter, []string{"AWS", "Azure"})
					if err != nil {
						return nil, err
					}
					h.logger.Warn(fmt.Sprintf("===Dim Filter Function on filter %v, result: %v", dimFilter, providers))
					for _, c := range allIntegrations.Integrations {
						if arrayContains(providers, c.IntegrationType.String()) {
							integrations = append(integrations, c.IntegrationID)
						}
					}
				} else if dimKey == "ConnectionGroup" {
					allGroups, err := h.integrationClient.ListIntegrationGroups(&httpclient.Context{UserRole: api.AdminRole})
					if err != nil {
						return nil, err
					}
					allGroupsMap := make(map[string][]string)
					var allGroupsStr []string
					for _, g := range allGroups {
						allGroupsMap[g.Name] = make([]string, 0, len(g.IntegrationIds))
						for _, cid := range g.IntegrationIds {
							allGroupsMap[g.Name] = append(allGroupsMap[g.Name], cid)
							allGroupsStr = append(allGroupsStr, cid)
						}
					}
					groups, err := dimFilterFunction(dimFilter, allGroupsStr)
					if err != nil {
						return nil, err
					}
					h.logger.Warn(fmt.Sprintf("===Dim Filter Function on filter %v, result: %v", dimFilter, groups))

					for _, g := range groups {
						for _, conn := range allGroupsMap[g] {
							if !arrayContains(integrations, conn) {
								integrations = append(integrations, conn)
							}
						}
					}
				} else if dimKey == "ConnectionName" {
					var allIntegrationsNames []string
					for _, c := range allIntegrations.Integrations {
						allIntegrationsNames = append(allIntegrationsNames, c.Name)
					}
					integrationNames, err := dimFilterFunction(dimFilter, allIntegrationsNames)
					if err != nil {
						return nil, err
					}
					h.logger.Warn(fmt.Sprintf("===Dim Filter Function on filter %v, result: %v", dimFilter, integrationNames))
					for _, conn := range allIntegrations.Integrations {
						if arrayContains(integrationNames, conn.Name) {
							integrations = append(integrations, conn.IntegrationID)
						}
					}

				}
			} else {
				return nil, fmt.Errorf("missing key")
			}
		} else if key == "AND" {
			var andFilters []map[string]interface{}
			for _, v := range value.([]interface{}) {
				andFilter := v.(map[string]interface{})
				andFilters = append(andFilters, andFilter)
			}
			counter := make(map[string]int)
			for _, f := range andFilters {
				values, err := h.connectionsFilter(f)
				if err != nil {
					return nil, err
				}
				for _, v := range values {
					if c, ok := counter[v]; ok {
						counter[v] = c + 1
					} else {
						counter[v] = 1
					}
					if counter[v] == len(andFilters) {
						integrations = append(integrations, v)
					}
				}
			}
		} else if key == "OR" {
			var orFilters []map[string]interface{}
			for _, v := range value.([]interface{}) {
				orFilter := v.(map[string]interface{})
				orFilters = append(orFilters, orFilter)
			}
			for _, f := range orFilters {
				values, err := h.connectionsFilter(f)
				if err != nil {
					return nil, err
				}
				for _, v := range values {
					if !arrayContains(integrations, v) {
						integrations = append(integrations, v)
					}
				}
			}
		} else {
			return nil, fmt.Errorf("invalid key: %s", key)
		}
	}
	return integrations, nil
}

func dimFilterFunction(dimFilter map[string]interface{}, allValues []string) ([]string, error) {
	var values []string
	for _, v := range dimFilter["Values"].([]interface{}) {
		values = append(values, fmt.Sprintf("%v", v))
	}
	var output []string
	if matchOption, ok := dimFilter["MatchOption"]; ok {
		switch {
		case strings.Contains(matchOption.(string), "EQUAL"):
			output = values
		case strings.Contains(matchOption.(string), "STARTS_WITH"):
			for _, v := range values {
				for _, conn := range allValues {
					if strings.HasPrefix(conn, v) {
						if !arrayContains(output, conn) {
							output = append(output, conn)
						}
					}
				}
			}
		case strings.Contains(matchOption.(string), "ENDS_WITH"):
			for _, v := range values {
				for _, conn := range allValues {
					if strings.HasSuffix(conn, v) {
						if !arrayContains(output, conn) {
							output = append(output, conn)
						}
					}
				}
			}
		case strings.Contains(matchOption.(string), "CONTAINS"):
			for _, v := range values {
				for _, conn := range allValues {
					if strings.Contains(conn, v) {
						if !arrayContains(output, conn) {
							output = append(output, conn)
						}
					}
				}
			}
		default:
			return nil, fmt.Errorf("invalid option")
		}
		if strings.HasPrefix(matchOption.(string), "~") {
			var notOutput []string
			for _, v := range allValues {
				if !arrayContains(output, v) {
					notOutput = append(notOutput, v)
				}
			}
			return notOutput, nil
		}
	} else {
		output = values
	}
	return output, nil
}

func arrayContains(array []string, key string) bool {
	for _, v := range array {
		if v == key {
			return true
		}
	}
	return false
}

// RunQueryByID godoc
//
//	@Summary		Run query by named query or compliance ID
//	@Description	Run provided named query or compliance and returns the result.
//	@Security		BearerToken
//	@Tags			named_query
//	@Accepts		json
//	@Produce		json
//	@Param			request	body		inventoryApi.RunQueryByIDRequest	true	"Request Body"
//	@Param			accept	header		string								true	"Accept header"	Enums(application/json,text/csv)
//	@Success		200		{object}	inventoryApi.RunQueryResponse
//	@Router			/inventory/api/v3/query/run [post]
func (h *HttpHandler) RunQueryByID(ctx echo.Context) error {
	var req inventoryApi.RunQueryByIDRequest
	if err := bindValidate(ctx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.ID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Runnable Type and ID should be provided")
	}
	if req.Type == "" {
		req.Type = "namedquery"
	}

	newCtx, cancel := context.WithTimeout(ctx.Request().Context(), 30*time.Second)
	defer cancel()

	// tracer :
	newCtx, span := tracer.Start(newCtx, "new_RunNamedQuery", trace.WithSpanKind(trace.SpanKindServer))
	span.SetName("new_RunNamedQuery")

	var query, engineStr string
	if strings.ToLower(req.Type) == "namedquery" || strings.ToLower(req.Type) == "named_query" {
		namedQuery, err := h.db.GetQuery(req.ID)
		if err != nil || namedQuery == nil {
			h.logger.Error("failed to get named query", zap.Error(err))
			return echo.NewHTTPError(http.StatusBadRequest, "Could not find named query")
		}
		query = namedQuery.Query.QueryToExecute
		engineStr = namedQuery.Query.Engine
	} else if strings.ToLower(req.Type) == "control" {
		control, err := h.complianceClient.GetControl(&httpclient.Context{UserRole: api.AdminRole}, req.ID)
		if err != nil || control == nil {
			h.logger.Error("failed to get compliance", zap.Error(err))
			return echo.NewHTTPError(http.StatusBadRequest, "Could not find named query")
		}
		if control.Query == nil {
			return echo.NewHTTPError(http.StatusBadRequest, "Compliance query is empty")
		}
		query = control.Query.QueryToExecute
		engineStr = string(control.Query.Engine)
	} else {
		return echo.NewHTTPError(http.StatusBadRequest, "Runnable Type is not valid. Options: named_query, control")
	}
	var engine inventoryApi.QueryEngine
	if engineStr == "" {
		engine = inventoryApi.QueryEngineCloudQL
	} else {
		engine = inventoryApi.QueryEngine(engineStr)
	}

	queryParams, err := h.metadataClient.ListQueryParameters(&httpclient.Context{UserRole: api.AdminRole})
	if err != nil {
		h.logger.Error("failed to get query parameters", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get query parameters")
	}
	queryParamMap := make(map[string]string)
	for _, qp := range queryParams.Items {
		queryParamMap[qp.Key] = qp.Value
	}

	for k, v := range req.QueryParams {
		queryParamMap[k] = v
	}

	queryTemplate, err := template.New("query").Parse(query)
	if err != nil {
		return err
	}
	var queryOutput bytes.Buffer
	if err := queryTemplate.Execute(&queryOutput, queryParamMap); err != nil {
		return fmt.Errorf("failed to execute query template: %w", err)
	}

	var resp *inventoryApi.RunQueryResponse
	if engine == inventoryApi.QueryEngineCloudQL {
		resp, err = h.RunSQLNamedQuery(newCtx, query, queryOutput.String(), &inventoryApi.RunQueryRequest{
			Page:   req.Page,
			Query:  &query,
			Engine: &engine,
			Sorts:  req.Sorts,
		})
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	} else if engine == inventoryApi.QueryEngineCloudQLRego {
		resp, err = h.RunRegoNamedQuery(newCtx, query, queryOutput.String(), &inventoryApi.RunQueryRequest{
			Page:   req.Page,
			Query:  &query,
			Engine: &engine,
			Sorts:  req.Sorts,
		})
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	} else {
		resp, err = h.RunSQLNamedQuery(newCtx, query, queryOutput.String(), &inventoryApi.RunQueryRequest{
			Page:   req.Page,
			Query:  &query,
			Engine: &engine,
			Sorts:  req.Sorts,
		})
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}

	span.AddEvent("information", trace.WithAttributes(
		attribute.String("query title ", resp.Title),
	))
	span.End()
	select {
	case <-newCtx.Done():
		job, err := h.schedulerClient.RunQuery(&httpclient.Context{UserRole: api.AdminRole}, req.ID)
		if err != nil {
			h.logger.Error("failed to run async query run", zap.Error(err))
			return echo.NewHTTPError(http.StatusRequestTimeout, "Query execution timed out and failed to create async query run")
		}
		msg := fmt.Sprintf("Query execution timed out, created an async query run instead: jobid = %v", job.ID)
		return echo.NewHTTPError(http.StatusRequestTimeout, msg)
	default:
		return ctx.JSON(200, resp)
	}
}

// ListQueriesFilters godoc
//
//	@Summary	List possible values for each filter in List Controls
//	@Security	BearerToken
//	@Tags		compliance
//	@Accept		json
//	@Produce	json
//	@Success	200	{object}	inventoryApi.ListQueriesFiltersResponse
//	@Router		/inventory/api/v3/queries/filters [get]
func (h *HttpHandler) ListQueriesFilters(echoCtx echo.Context) error {
	providers, err := h.db.ListNamedQueriesUniqueProviders()
	if err != nil {
		h.logger.Error("failed to get providers list", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get providers list")
	}

	namedQueriesTags, err := h.db.GetQueriesTags()
	if err != nil {
		h.logger.Error("failed to get namedQueriesTags", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get namedQueriesTags")
	}

	tags := make([]inventoryApi.NamedQueryTagsResult, 0, len(namedQueriesTags))
	for _, history := range namedQueriesTags {
		tags = append(tags, history.ToApi())
	}

	response := inventoryApi.ListQueriesFiltersResponse{
		Providers: providers,
		Tags:      tags,
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// GetAsyncQueryRunResult godoc
//
//	@Summary		Run async query run result by run id
//	@Description	Run async query run result by run id.
//	@Security		BearerToken
//	@Tags			named_query
//	@Accepts		json
//	@Produce		json
//	@Param			run_id	path		string	true	"Run ID to get the result for"
//	@Success		200		{object}	inventoryApi.GetAsyncQueryRunResultResponse
//	@Router			/inventory/api/v3/query/async/run/:run_id/result [get]
func (h *HttpHandler) GetAsyncQueryRunResult(ctx echo.Context) error {
	runId := ctx.Param("run_id")
	// tracer :
	newCtx, span := tracer.Start(ctx.Request().Context(), "new_GetAsyncQueryRunResult", trace.WithSpanKind(trace.SpanKindServer))
	span.SetName("new_GetAsyncQueryRunResult")

	job, err := h.schedulerClient.GetAsyncQueryRunJobStatus(&httpclient.Context{UserRole: api.AdminRole}, runId)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "failed to find async query run job status")
	}
	if job.JobStatus == queryrunner.QueryRunnerCreated || job.JobStatus == queryrunner.QueryRunnerQueued || job.JobStatus == queryrunner.QueryRunnerInProgress {
		return echo.NewHTTPError(http.StatusOK, "Job is still in progress")
	} else if job.JobStatus == queryrunner.QueryRunnerFailed {
		return echo.NewHTTPError(http.StatusOK, fmt.Sprintf("Job has been failed: %s", job.FailureMessage))
	} else if job.JobStatus == queryrunner.QueryRunnerTimeOut {
		return echo.NewHTTPError(http.StatusOK, "Job has been timed out")
	} else if job.JobStatus == queryrunner.QueryRunnerCanceled {
		return echo.NewHTTPError(http.StatusOK, "Job has been canceled")
	}

	runResult, err := es.GetAsyncQueryRunResult(newCtx, h.logger, h.client, runId)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "failed to find async query run result")
	}

	resp := inventoryApi.GetAsyncQueryRunResultResponse{
		RunId:       runResult.RunId,
		QueryID:     runResult.QueryID,
		Parameters:  runResult.Parameters,
		ColumnNames: runResult.ColumnNames,
		CreatedBy:   runResult.CreatedBy,
		TriggeredAt: runResult.TriggeredAt,
		EvaluatedAt: runResult.EvaluatedAt,
		Result:      runResult.Result,
	}

	span.End()
	return ctx.JSON(200, resp)
}

// GetResourceCategories godoc
//
//	@Summary		Get list of unique resource categories
//	@Description	Get list of unique resource categories
//	@Security		BearerToken
//	@Tags			named_query
//	@Param			tables		query	[]string	false	"Tables filter"
//	@Param			categories	query	[]string	false	"Categories filter"
//	@Accepts		json
//	@Produce		json
//	@Success		200	{object}	inventoryApi.GetResourceCategoriesResponse
//	@Router			/inventory/api/v3/resources/categories [get]
func (h *HttpHandler) GetResourceCategories(ctx echo.Context) error {
	tablesFilter := httpserver.QueryArrayParam(ctx, "tables")
	categoriesFilter := httpserver.QueryArrayParam(ctx, "categories")

	resourceTypes, err := h.db.ListResourceTypes(tablesFilter, categoriesFilter)
	if err != nil {
		h.logger.Error("could not find resourceTypes", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "could not find resourceTypes")
	}

	categories := make(map[string][]ResourceTypeV2)
	for _, rt := range resourceTypes {
		if _, ok := categories[rt.Category]; !ok {
			categories[rt.Category] = make([]ResourceTypeV2, 0)
		}
		categories[rt.Category] = append(categories[rt.Category], rt)
	}
	var categoriesResponse []inventoryApi.GetResourceCategoriesCategory
	for c, rts := range categories {
		var responseTables []inventoryApi.GetResourceCategoriesTables
		for _, rt := range rts {
			responseTables = append(responseTables, inventoryApi.GetResourceCategoriesTables{
				Name:         rt.ResourceName,
				Table:        rt.SteampipeTable,
				ResourceType: rt.ResourceID,
			})
		}
		categoriesResponse = append(categoriesResponse, inventoryApi.GetResourceCategoriesCategory{
			Category: c,
			Tables:   responseTables,
		})
	}

	return ctx.JSON(200, inventoryApi.GetResourceCategoriesResponse{
		Categories: categoriesResponse,
	})
}

// GetQueriesResourceCategories godoc
//
//	@Summary		Get list of unique resource categories
//	@Description	Get list of unique resource categories for the give queries
//	@Security		BearerToken
//	@Tags			named_query
//	@Param			queries	query	[]string	false	"Connection group to filter by - mutually exclusive with connectionId"
//	 	@Param 			is_bookmarked 	query 	bool 		false	"is bookmarked filter"
//	@Accepts		json
//	@Produce		json
//	@Success		200	{object}	inventoryApi.GetResourceCategoriesResponse
//	@Router			/inventory/api/v3/queries/categories [get]
func (h *HttpHandler) GetQueriesResourceCategories(ctx echo.Context) error {
	queryIds := httpserver.QueryArrayParam(ctx, "queries")
	isBookmarkedStr := ctx.Param("is_bookmarked")
	var isBookmarked *bool
	if isBookmarkedStr == "true" {
		isBookmarked = aws.Bool(true)
	} else if isBookmarkedStr == "false" {
		isBookmarked = aws.Bool(false)
	}

	queries, err := h.db.ListQueries(queryIds, nil, nil, nil, isBookmarked)
	if err != nil {
		h.logger.Error("could not find queries", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "could not find queries")
	}
	tablesMap := make(map[string]bool)
	for _, q := range queries {
		for _, t := range q.Query.ListOfTables {
			tablesMap[t] = true
		}
	}
	var tables []string
	for t, _ := range tablesMap {
		tables = append(tables, t)
	}

	resourceTypes, err := h.db.ListResourceTypes(tables, nil)
	if err != nil {
		h.logger.Error("could not find resourceTypes", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "could not find resourceTypes")
	}

	categories := make(map[string][]ResourceTypeV2)
	for _, rt := range resourceTypes {
		if _, ok := categories[rt.Category]; !ok {
			categories[rt.Category] = make([]ResourceTypeV2, 0)
		}
		categories[rt.Category] = append(categories[rt.Category], rt)
	}
	var categoriesResponse []inventoryApi.GetResourceCategoriesCategory
	for c, rts := range categories {
		var responseTables []inventoryApi.GetResourceCategoriesTables
		for _, rt := range rts {
			responseTables = append(responseTables, inventoryApi.GetResourceCategoriesTables{
				Name:         rt.ResourceName,
				Table:        rt.SteampipeTable,
				ResourceType: rt.ResourceID,
			})
		}
		categoriesResponse = append(categoriesResponse, inventoryApi.GetResourceCategoriesCategory{
			Category: c,
			Tables:   responseTables,
		})
	}

	return ctx.JSON(200, inventoryApi.GetResourceCategoriesResponse{
		Categories: categoriesResponse,
	})
}

// GetTablesResourceCategories godoc
//
//	@Summary		Get list of unique resource categories
//	@Description	Get list of unique resource categories for the give queries
//	@Security		BearerToken
//	@Tags			named_query
//	@Param			tables	query	[]string	false	"Connection group to filter by - mutually exclusive with connectionId"
//	@Accepts		json
//	@Produce		json
//	@Success		200	{object}	[]inventoryApi.CategoriesTables
//	@Router			/inventory/api/v3/tables/categories [get]
func (h *HttpHandler) GetTablesResourceCategories(ctx echo.Context) error {
	tables := httpserver.QueryArrayParam(ctx, "tables")

	categories, err := h.db.ListUniqueCategoriesAndTablesForTables(tables)
	if err != nil {
		h.logger.Error("could not find categories", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "could not find categories")
	}

	return ctx.JSON(200, categories)
}

// GetCategoriesQueries godoc
//
//		@Summary		Get list of controls for given categories
//		@Description	Get list of controls for given categories
//		@Security		BearerToken
//		@Tags			named_query
//		@Param			categories		query	[]string	false	"Connection group to filter by - mutually exclusive with connectionId"
//	 	@Param 			is_bookmarked 	query 	bool 		false	"is bookmarked filter"
//		@Accepts		json
//		@Produce		json
//		@Success		200	{object}	[]string
//		@Router			/inventory/api/v3/categories/queries [get]
func (h *HttpHandler) GetCategoriesQueries(ctx echo.Context) error {
	categories, err := h.db.ListUniqueCategoriesAndTablesForTables(nil)
	if err != nil {
		h.logger.Error("failed to list resource categories", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list resource categories")
	}

	isBookmarkedStr := ctx.Param("is_bookmarked")
	var isBookmarked *bool
	if isBookmarkedStr == "true" {
		isBookmarked = aws.Bool(true)
	} else if isBookmarkedStr == "false" {
		isBookmarked = aws.Bool(false)
	}

	categoriesFilter := httpserver.QueryArrayParam(ctx, "categories")
	categoriesFilterMap := make(map[string]bool)
	for _, c := range categoriesFilter {
		categoriesFilterMap[c] = true
	}

	var categoriesApi []inventoryApi.ResourceCategory
	for _, c := range categories {
		if _, ok := categoriesFilterMap[c.Category]; !ok && len(categoriesFilter) > 0 {
			continue
		}
		resourceTypes, err := h.db.ListCategoryResourceTypes(c.Category)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "list category resource types")
		}
		var resourceTypesApi []inventoryApi.ResourceTypeV2
		for _, r := range resourceTypes {
			resourceTypesApi = append(resourceTypesApi, r.ToApi())
		}
		categoriesApi = append(categoriesApi, inventoryApi.ResourceCategory{
			Category:  c.Category,
			Resources: resourceTypesApi,
		})
	}

	tablesFilterMap := make(map[string]string)
	var categoryQueries []inventoryApi.CategoryQueries
	for _, c := range categoriesApi {
		for _, r := range c.Resources {
			tablesFilterMap[r.SteampipeTable] = r.ResourceID
		}
		var tablesFilter []string
		for k, _ := range tablesFilterMap {
			tablesFilter = append(tablesFilter, k)
		}

		queries, err := h.db.ListQueries(nil, nil, tablesFilter, nil, isBookmarked)
		if err != nil {
			h.logger.Error("could not find queries", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "could not find queries")
		}
		servicesQueries := make(map[string][]inventoryApi.NamedQueryItemV2)
		for _, query := range queries {
			tags := query.GetTagsMap()
			if query.IsBookmarked {
				tags["platform_queries_bookmark"] = []string{"true"}
			}
			result := inventoryApi.NamedQueryItemV2{
				ID:               query.ID,
				Title:            query.Title,
				Description:      query.Description,
				IntegrationTypes: integration_type.ParseTypes(query.IntegrationTypes),
				Query:            query.Query.ToApi(),
				Tags:             tags,
			}
			for _, t := range query.Query.ListOfTables {
				if t == "" {
					continue
				}
				if _, ok := servicesQueries[tablesFilterMap[t]]; !ok {
					servicesQueries[tablesFilterMap[t]] = make([]inventoryApi.NamedQueryItemV2, 0)
				}
				servicesQueries[tablesFilterMap[t]] = append(servicesQueries[tablesFilterMap[t]], result)
			}
		}
		var services []inventoryApi.ServiceQueries
		for k, v := range servicesQueries {
			services = append(services, inventoryApi.ServiceQueries{
				Service: k,
				Queries: v,
			})
		}
		categoryQueries = append(categoryQueries, inventoryApi.CategoryQueries{
			Category: c.Category,
			Services: services,
		})
	}
	return ctx.JSON(200, inventoryApi.GetCategoriesControlsResponse{
		Categories: categoryQueries,
	})
}

// GetParametersQueries godoc
//
//	@Summary		Get list of queries for given parameters
//	@Description	Get list of queries for given parameters
//	@Security		BearerToken
//	@Tags			compliance
//	@Param			parameters	query	[]string	false	"Parameters filter by"
//	 	@Param 			is_bookmarked 	query 	bool 		false	"is bookmarked filter"
//	@Accepts		json
//	@Produce		json
//	@Success		200	{object}	inventoryApi.GetParametersQueriesResponse
//	@Router			/compliance/api/v3/parameters/controls [get]
func (h *HttpHandler) GetParametersQueries(ctx echo.Context) error {
	parameters := httpserver.QueryArrayParam(ctx, "parameters")
	isBookmarkedStr := ctx.Param("is_bookmarked")
	var isBookmarked *bool
	if isBookmarkedStr == "true" {
		isBookmarked = aws.Bool(true)
	} else if isBookmarkedStr == "false" {
		isBookmarked = aws.Bool(false)
	}

	var err error
	if len(parameters) == 0 {
		parameters, err = h.db.GetQueryParameters()
		if err != nil {
			h.logger.Error("failed to get list of parameters", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to get list of parameters")
		}
	}

	var parametersQueries []inventoryApi.ParametersQueries
	for _, p := range parameters {
		queries, err := h.db.ListQueries(nil, nil, nil, []string{p}, isBookmarked)
		if err != nil {
			h.logger.Error("failed to get list of controls", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to get list of controls")
		}
		var items []inventoryApi.NamedQueryItemV2
		for _, item := range queries {
			tags := item.GetTagsMap()
			if tags == nil || len(tags) == 0 {
				tags = make(map[string][]string)
			}
			if item.IsBookmarked {
				tags["platform_queries_bookmark"] = []string{"true"}
			}
			items = append(items, inventoryApi.NamedQueryItemV2{
				ID:               item.ID,
				Title:            item.Title,
				Description:      item.Description,
				IntegrationTypes: integration_type.ParseTypes(item.IntegrationTypes),
				Query:            item.Query.ToApi(),
				Tags:             tags,
			})
		}

		parametersQueries = append(parametersQueries, inventoryApi.ParametersQueries{
			Parameter: p,
			Queries:   items,
		})
	}

	return ctx.JSON(200, inventoryApi.GetParametersQueriesResponse{
		ParametersQueries: parametersQueries,
	})
}
