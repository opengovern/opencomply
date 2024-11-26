package compliance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/google/uuid"
	"github.com/jackc/pgtype"
	"github.com/labstack/echo/v4"
	authApi "github.com/opengovern/og-util/pkg/api"
	es2 "github.com/opengovern/og-util/pkg/es"
	"github.com/opengovern/og-util/pkg/httpclient"
	httpserver2 "github.com/opengovern/og-util/pkg/httpserver"
	"github.com/opengovern/og-util/pkg/integration"
	"github.com/opengovern/og-util/pkg/model"
	runner "github.com/opengovern/opencomply/jobs/compliance-runner-job"
	"github.com/opengovern/opencomply/jobs/compliance-summarizer-job/types"
	model2 "github.com/opengovern/opencomply/jobs/post-install-job/db/model"
	opengovernanceTypes "github.com/opengovern/opencomply/pkg/types"
	types2 "github.com/opengovern/opencomply/pkg/types"
	"github.com/opengovern/opencomply/pkg/utils"
	"github.com/opengovern/opencomply/services/compliance/api"
	"github.com/opengovern/opencomply/services/compliance/db"
	"github.com/opengovern/opencomply/services/compliance/es"
	model3 "github.com/opengovern/opencomply/services/describe/db/model"
	integrationapi "github.com/opengovern/opencomply/services/integration/api/models"
	integration_type "github.com/opengovern/opencomply/services/integration/integration-type"
	inventoryApi "github.com/opengovern/opencomply/services/inventory/api"
	"github.com/opengovern/opencomply/services/metadata/models"
	"github.com/sashabaranov/go-openai"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"gorm.io/gorm"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	IntegrationIDParam    = "integrationID"
	IntegrationGroupParam = "integrationGroup"
)

func (h *HttpHandler) Register(e *echo.Echo) {
	v1 := e.Group("/api/v1")

	benchmarks := v1.Group("/benchmarks")

	benchmarks.GET("", httpserver2.AuthorizeHandler(h.ListBenchmarks, authApi.ViewerRole))
	benchmarks.GET("/all", httpserver2.AuthorizeHandler(h.ListAllBenchmarks, authApi.AdminRole))
	benchmarks.GET("/:benchmark_id", httpserver2.AuthorizeHandler(h.GetBenchmark, authApi.ViewerRole))
	benchmarks.POST("/:benchmark_id/settings", httpserver2.AuthorizeHandler(h.ChangeBenchmarkSettings, authApi.AdminRole))
	benchmarks.GET("/controls/:control_id", httpserver2.AuthorizeHandler(h.GetControl, authApi.ViewerRole))
	benchmarks.GET("/controls", httpserver2.AuthorizeHandler(h.ListControls, authApi.AdminRole))
	benchmarks.GET("/queries", httpserver2.AuthorizeHandler(h.ListQueries, authApi.AdminRole))

	benchmarks.GET("/summary", httpserver2.AuthorizeHandler(h.ListBenchmarksSummary, authApi.ViewerRole))
	benchmarks.GET("/:benchmark_id/summary", httpserver2.AuthorizeHandler(h.GetBenchmarkSummary, authApi.ViewerRole))
	benchmarks.GET("/:benchmark_id/trend", httpserver2.AuthorizeHandler(h.GetBenchmarkTrend, authApi.ViewerRole))
	benchmarks.GET("/:benchmark_id/controls", httpserver2.AuthorizeHandler(h.GetBenchmarkControlsTree, authApi.ViewerRole))
	benchmarks.GET("/:benchmark_id/controls/:controlId", httpserver2.AuthorizeHandler(h.GetBenchmarkControl, authApi.ViewerRole))

	controls := v1.Group("/controls")
	controls.GET("/summary", httpserver2.AuthorizeHandler(h.ListControlsSummary, authApi.ViewerRole))
	controls.GET("/:controlId/summary", httpserver2.AuthorizeHandler(h.GetControlSummary, authApi.ViewerRole))
	controls.GET("/:controlId/trend", httpserver2.AuthorizeHandler(h.GetControlTrend, authApi.ViewerRole))

	queries := v1.Group("/queries")
	queries.GET("/:query_id", httpserver2.AuthorizeHandler(h.GetQuery, authApi.ViewerRole))
	queries.GET("/sync", httpserver2.AuthorizeHandler(h.SyncQueries, authApi.AdminRole))

	assignments := v1.Group("/assignments")
	assignments.GET("/benchmark/:benchmark_id", httpserver2.AuthorizeHandler(h.ListAssignmentsByBenchmark, authApi.ViewerRole))
	assignments.GET("/integration/:integration_id", httpserver2.AuthorizeHandler(h.ListAssignmentsByIntegration, authApi.ViewerRole))
	assignments.GET("/resource_collection/:resource_collection_id", httpserver2.AuthorizeHandler(h.ListAssignmentsByResourceCollection, authApi.ViewerRole))
	assignments.POST("/:benchmark_id/integration", httpserver2.AuthorizeHandler(h.CreateBenchmarkAssignment, authApi.EditorRole))
	assignments.DELETE("/:benchmark_id/integration", httpserver2.AuthorizeHandler(h.DeleteBenchmarkAssignment, authApi.EditorRole))

	metadata := v1.Group("/metadata")
	metadata.GET("/tag/compliance", httpserver2.AuthorizeHandler(h.ListComplianceTags, authApi.ViewerRole))

	complianceResults := v1.Group("/compliance_result")
	complianceResults.POST("", httpserver2.AuthorizeHandler(h.GetComplianceResults, authApi.ViewerRole))
	complianceResults.POST("/resource", httpserver2.AuthorizeHandler(h.GetSingleResourceFinding, authApi.ViewerRole))
	complianceResults.GET("/single/:id", httpserver2.AuthorizeHandler(h.GetSingleComplianceResultByComplianceResultID, authApi.ViewerRole))
	complianceResults.GET("/events/:id", httpserver2.AuthorizeHandler(h.GetComplianceResultEventsByComplianceResultID, authApi.ViewerRole))
	complianceResults.GET("/count", httpserver2.AuthorizeHandler(h.CountComplianceResults, authApi.ViewerRole))
	complianceResults.POST("/filters", httpserver2.AuthorizeHandler(h.GetComplianceResultFilterValues, authApi.ViewerRole))
	complianceResults.GET("/kpi", httpserver2.AuthorizeHandler(h.GetComplianceResultKPIs, authApi.ViewerRole))
	complianceResults.GET("/top/:field/:count", httpserver2.AuthorizeHandler(h.GetTopFieldByComplianceResultCount, authApi.ViewerRole))
	complianceResults.GET("/:benchmarkId/:field/count", httpserver2.AuthorizeHandler(h.GetComplianceResultsFieldCountByControls, authApi.ViewerRole))
	complianceResults.GET("/:benchmarkId/accounts", httpserver2.AuthorizeHandler(h.GetAccountsComplianceResultsSummary, authApi.ViewerRole))
	complianceResults.GET("/:benchmarkId/services", httpserver2.AuthorizeHandler(h.GetServicesComplianceResultsSummary, authApi.ViewerRole))

	complianceResultEvents := v1.Group("/compliance_result_events")
	complianceResultEvents.POST("", httpserver2.AuthorizeHandler(h.GetComplianceResultDriftEvents, authApi.ViewerRole))
	complianceResultEvents.POST("/filters", httpserver2.AuthorizeHandler(h.GetComplianceResultDriftEventFilterValues, authApi.ViewerRole))
	complianceResultEvents.GET("/count", httpserver2.AuthorizeHandler(h.CountComplianceResultDriftEvents, authApi.ViewerRole))
	complianceResultEvents.GET("/single/:id", httpserver2.AuthorizeHandler(h.GetSingleComplianceResultDriftEvent, authApi.ViewerRole))

	resourceFindings := v1.Group("/resource_findings")
	resourceFindings.POST("", httpserver2.AuthorizeHandler(h.ListResourceFindings, authApi.ViewerRole))
	resourceFindings.GET("/:job_id", httpserver2.AuthorizeHandler(h.ListResourceFindings, authApi.ViewerRole))

	ai := v1.Group("/ai")
	ai.POST("/control/:controlID/remediation", httpserver2.AuthorizeHandler(h.GetControlRemediation, authApi.ViewerRole))

	v3 := e.Group("/api/v3")

	v3.GET("/benchmarks/tags", httpserver2.AuthorizeHandler(h.ListBenchmarksTags, authApi.ViewerRole))
	v3.POST("/benchmarks", httpserver2.AuthorizeHandler(h.ListBenchmarksFiltered, authApi.ViewerRole))
	v3.GET("/benchmarks/filters", httpserver2.AuthorizeHandler(h.ListBenchmarksFilters, authApi.ViewerRole))
	v3.POST("/benchmark/:benchmark_id", httpserver2.AuthorizeHandler(h.GetBenchmarkDetails, authApi.ViewerRole))
	v3.GET("/benchmark/:benchmark_id/assignments", httpserver2.AuthorizeHandler(h.GetBenchmarkAssignments, authApi.ViewerRole))
	v3.POST("/benchmark/:benchmark_id/assign", httpserver2.AuthorizeHandler(h.AssignBenchmarkToIntegration, authApi.ViewerRole))
	v3.POST("/compliance/summary/integration", httpserver2.AuthorizeHandler(h.ComplianceSummaryOfIntegration, authApi.ViewerRole))
	v3.POST("/compliance/summary/benchmark", httpserver2.AuthorizeHandler(h.ComplianceSummaryOfBenchmark, authApi.ViewerRole))
	v3.GET("/compliance/summary/:job_id", httpserver2.AuthorizeHandler(h.ComplianceSummaryOfJob, authApi.ViewerRole))
	v3.POST("/benchmarks/:benchmark_id/trend", httpserver2.AuthorizeHandler(h.GetBenchmarkTrendV3, authApi.ViewerRole))

	v3.POST("/controls", httpserver2.AuthorizeHandler(h.ListControlsFiltered, authApi.ViewerRole))
	v3.GET("/controls/categories", httpserver2.AuthorizeHandler(h.GetControlsResourceCategories, authApi.ViewerRole))
	v3.GET("/categories/controls", httpserver2.AuthorizeHandler(h.GetCategoriesControls, authApi.ViewerRole))
	v3.GET("/parameters/controls", httpserver2.AuthorizeHandler(h.GetParametersControls, authApi.ViewerRole))
	v3.GET("/controls/filters", httpserver2.AuthorizeHandler(h.ListControlsFilters, authApi.ViewerRole))
	v3.POST("/controls/summary", httpserver2.AuthorizeHandler(h.ControlsFilteredSummary, authApi.ViewerRole))
	v3.GET("/control/:control_id", httpserver2.AuthorizeHandler(h.GetControlDetails, authApi.ViewerRole))
	v3.GET("/controls/tags", httpserver2.AuthorizeHandler(h.ListControlsTags, authApi.ViewerRole))
	v3.POST("/compliance_result", httpserver2.AuthorizeHandler(h.GetComplianceResultV2, authApi.ViewerRole))

	v3.GET("/jobs/history", httpserver2.AuthorizeHandler(h.ListComplianceJobsHistory, authApi.ViewerRole))

	v3.GET("/benchmarks/:benchmark_id/nested", httpserver2.AuthorizeHandler(h.ListBenchmarksNestedForBenchmark, authApi.ViewerRole))
}

func bindValidate(ctx echo.Context, i any) error {
	if err := ctx.Bind(i); err != nil {
		return err
	}

	if err := ctx.Validate(i); err != nil {
		return err
	}

	return nil
}

func (h *HttpHandler) getIntegrationIdFilterFromInputs(ctx context.Context, integrationIds []string, integrationGroup []string) ([]string, error) {
	if len(integrationIds) == 0 && len(integrationGroup) == 0 {
		return nil, nil
	}

	if len(integrationIds) > 0 && len(integrationGroup) > 0 {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "integrationId and integrationGroup cannot be used together")
	}

	if len(integrationIds) > 0 {
		return integrationIds, nil
	}

	check := make(map[string]bool)
	var integrationIDSChecked []string

	for i := 0; i < len(integrationGroup); i++ {
		integrationGroupObj, err := h.integrationClient.GetIntegrationGroup(&httpclient.Context{Ctx: ctx, UserRole: authApi.AdminRole}, integrationGroup[i])
		if err != nil {
			return nil, err
		}
		if len(integrationGroupObj.IntegrationIds) == 0 {
			return nil, err
		}

		// Check for duplicate integration groups
		for _, entry := range integrationGroupObj.IntegrationIds {
			if _, value := check[entry]; !value {
				check[entry] = true
				integrationIDSChecked = append(integrationIDSChecked, entry)
			}
		}
	}
	integrationIds = integrationIDSChecked

	return integrationIds, nil
}

func (h *HttpHandler) getIntegrationIdFilterFromParams(echoCtx echo.Context) ([]string, error) {
	integrationIds := httpserver2.QueryArrayParam(echoCtx, IntegrationIDParam)
	//integrationIds, err := httpserver2.ResolveIntegrationIDs(echoCtx, integrationIds)
	//if err != nil {
	//	return nil, err
	//}
	integrationGroup := httpserver2.QueryArrayParam(echoCtx, IntegrationGroupParam)
	return h.getIntegrationIdFilterFromInputs(echoCtx.Request().Context(), integrationIds, integrationGroup)
}

var tracer = otel.Tracer("new_compliance")

// GetComplianceResults godoc
//
//	@Summary		Get compliacne results
//	@Description	Retrieving all compliance run compliacne results with respect to filters.
//	@Tags			compliance
//	@Security		BearerToken
//	@Accept			json
//	@Produce		json
//	@Param			request	body		api.GetComplianceResultsRequest	true	"Request Body"
//	@Success		200		{object}	api.GetComplianceResultsResponse
//	@Router			/compliance/api/v1/compliance_result [post]
func (h *HttpHandler) GetComplianceResults(echoCtx echo.Context) error {
	var err error
	ctx := echoCtx.Request().Context()

	var req api.GetComplianceResultsRequest
	if err := bindValidate(echoCtx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	req.Filters.IntegrationID, err = h.getIntegrationIdFilterFromInputs(echoCtx.Request().Context(), req.Filters.IntegrationID, req.Filters.IntegrationGroup)
	if err != nil {
		return err
	}
	//req.Filters.IntegrationID, err = httpserver2.ResolveIntegrationIDs(echoCtx, req.Filters.IntegrationID)
	//if err != nil {
	//	return err
	//}

	var response api.GetComplianceResultsResponse

	if len(req.Filters.ComplianceStatus) == 0 {
		req.Filters.ComplianceStatus = []api.ComplianceStatus{api.ComplianceStatusFailed}
	}

	esComplianceStatuses := make([]opengovernanceTypes.ComplianceStatus, 0, len(req.Filters.ComplianceStatus))
	for _, status := range req.Filters.ComplianceStatus {
		esComplianceStatuses = append(esComplianceStatuses, status.GetEsComplianceStatuses()...)
	}

	if len(req.Sort) == 0 {
		req.Sort = []api.ComplianceResultsSort{
			{ComplianceStatus: utils.GetPointer(api.SortDirectionDescending)},
		}
	}

	if len(req.AfterSortKey) != 0 {
		expectedLen := len(req.Sort) + 1
		if len(req.AfterSortKey) != expectedLen {
			return echo.NewHTTPError(http.StatusBadRequest, "sort key length should be zero or match a returned sort key from previous response")
		}
	}

	var lastEventFrom, lastEventTo, evaluatedAtFrom, evaluatedAtTo *time.Time
	if req.Filters.LastEvent.From != nil && *req.Filters.LastEvent.From != 0 {
		lastEventFrom = utils.GetPointer(time.Unix(*req.Filters.LastEvent.From, 0))
	}
	if req.Filters.LastEvent.To != nil && *req.Filters.LastEvent.To != 0 {
		lastEventTo = utils.GetPointer(time.Unix(*req.Filters.LastEvent.To, 0))
	}
	if req.Filters.EvaluatedAt.From != nil && *req.Filters.EvaluatedAt.From != 0 {
		evaluatedAtFrom = utils.GetPointer(time.Unix(*req.Filters.EvaluatedAt.From, 0))
	}
	if req.Filters.EvaluatedAt.To != nil && *req.Filters.EvaluatedAt.To != 0 {
		evaluatedAtTo = utils.GetPointer(time.Unix(*req.Filters.EvaluatedAt.To, 0))
	}
	if req.Filters.Interval != nil {
		evaluatedAtFrom, evaluatedAtTo, err = parseTimeInterval(*req.Filters.Interval)
	}

	allIntegrations, err := h.integrationClient.ListIntegrations(&httpclient.Context{UserRole: authApi.AdminRole}, nil)
	if err != nil {
		h.logger.Error("failed to get sources", zap.Error(err))
		return err
	}
	allSourcesMap := make(map[string]*integrationapi.Integration)
	for _, src := range allIntegrations.Integrations {
		src := src
		allSourcesMap[src.IntegrationID] = &src
	}

	res, totalCount, err := es.ComplianceResultsQuery(ctx, h.logger, h.client, req.Filters.ResourceID, req.Filters.IntegrationType,
		req.Filters.IntegrationID, req.Filters.NotIntegrationID, req.Filters.ResourceTypeID, req.Filters.BenchmarkID,
		req.Filters.ControlID, req.Filters.Severity, lastEventFrom, lastEventTo, evaluatedAtFrom, evaluatedAtTo,
		req.Filters.StateActive, esComplianceStatuses, req.Sort, req.Limit, req.AfterSortKey, req.Filters.JobID)
	if err != nil {
		h.logger.Error("failed to get compliacne results", zap.Error(err))
		return err
	}

	controls, err := h.db.ListControls(ctx, nil, nil)
	if err != nil {
		h.logger.Error("failed to get controls", zap.Error(err))
		return err
	}
	controlsMap := make(map[string]*db.Control)
	for _, control := range controls {
		control := control
		controlsMap[control.ID] = &control
	}

	benchmarks, err := h.db.ListBenchmarksBare(ctx)
	if err != nil {
		h.logger.Error("failed to get benchmarks", zap.Error(err))
		return err
	}
	benchmarksMap := make(map[string]*db.Benchmark)
	for _, benchmark := range benchmarks {
		benchmark := benchmark
		benchmarksMap[benchmark.ID] = &benchmark
	}

	resourceTypeMetadata, err := h.inventoryClient.ListResourceTypesMetadata(&httpclient.Context{UserRole: authApi.AdminRole},
		nil, nil, nil, false, nil, 10000, 1)
	if err != nil {
		h.logger.Error("failed to get resource type metadata", zap.Error(err))
		return err
	}
	resourceTypeMetadataMap := make(map[string]*inventoryApi.ResourceType)
	for _, item := range resourceTypeMetadata.ResourceTypes {
		item := item
		resourceTypeMetadataMap[strings.ToLower(item.ResourceType)] = &item
	}

	for _, h := range res {
		finding := api.GetAPIComplianceResultFromESComplianceResult(h.Source)

		for _, parentBenchmark := range h.Source.ParentBenchmarks {
			if benchmark, ok := benchmarksMap[parentBenchmark]; ok {
				finding.ParentBenchmarkNames = append(finding.ParentBenchmarkNames, benchmark.Title)
			}
		}
		if benchmark, ok := benchmarksMap[h.Source.BenchmarkID]; ok {
			finding.ParentBenchmarkNames = append(finding.ParentBenchmarkNames, benchmark.Title)
		}

		if control, ok := controlsMap[finding.ControlID]; ok {
			finding.ControlTitle = control.Title
		}

		if rtMetadata, ok := resourceTypeMetadataMap[strings.ToLower(finding.ResourceType)]; ok {
			finding.ResourceTypeName = rtMetadata.ResourceLabel
		}

		finding.SortKey = h.Sort

		response.ComplianceResults = append(response.ComplianceResults, finding)
	}
	response.TotalCount = totalCount

	platformResourceIDs := make([]string, 0, len(response.ComplianceResults))
	for _, finding := range response.ComplianceResults {
		platformResourceIDs = append(platformResourceIDs, finding.PlatformResourceID)
	}

	lookupResourcesMap, err := es.FetchLookupByResourceIDBatch(ctx, h.client, platformResourceIDs)
	if err != nil {
		h.logger.Error("failed to fetch lookup resources", zap.Error(err))
		return err
	}

	for i, finding := range response.ComplianceResults {
		var lookupResource *es2.LookupResource
		potentialResources := lookupResourcesMap[finding.PlatformResourceID]
		for _, r := range potentialResources {
			r := r
			if strings.ToLower(r.ResourceType) == strings.ToLower(finding.ResourceType) {
				lookupResource = &r
				break
			}
		}
		if lookupResource != nil {
			response.ComplianceResults[i].ResourceName = lookupResource.ResourceName
		} else {
			h.logger.Warn("lookup resource not found",
				zap.String("og_resource_id", finding.PlatformResourceID),
				zap.String("resource_id", finding.ResourceID),
				zap.String("controlId", finding.ControlID),
			)
		}
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// GetComplianceResultEventsByComplianceResultID godoc
//
//	@Summary		Get finding events by finding ID
//	@Description	Retrieving all compliance run finding events with respect to filters.
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"ComplianceResult ID"
//	@Success		200	{object}	api.GetComplianceResultDriftEventsByComplianceResultIDResponse
//	@Router			/compliance/api/v1/compliance_result/events/{id} [get]
func (h *HttpHandler) GetComplianceResultEventsByComplianceResultID(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	findingID := echoCtx.Param("id")

	findingEvents, err := es.FetchComplianceResultDriftEventsByComplianceResultIDs(ctx, h.logger, h.client, []string{findingID})
	if err != nil {
		h.logger.Error("failed to fetch finding by id", zap.Error(err))
		return err
	}

	response := api.GetComplianceResultDriftEventsByComplianceResultIDResponse{
		ComplianceResultDriftEvents: make([]api.ComplianceResultDriftEvent, 0, len(findingEvents)),
	}
	for _, findingEvent := range findingEvents {
		response.ComplianceResultDriftEvents = append(response.ComplianceResultDriftEvents, api.GetAPIComplianceResultDriftEventFromESComplianceResultDriftEvent(findingEvent))
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// GetSingleResourceFinding godoc
//
//	@Summary		Get finding
//	@Description	Retrieving a single finding
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			request	body		api.GetSingleResourceFindingRequest	true	"Request Body"
//	@Success		200		{object}	api.GetSingleResourceFindingResponse
//	@Router			/compliance/api/v1/compliance_result/resource [post]
func (h *HttpHandler) GetSingleResourceFinding(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	var req api.GetSingleResourceFindingRequest
	if err := bindValidate(echoCtx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	platformResourceID := req.PlatformResourceID

	lookupResourceRes, err := es.FetchLookupByResourceIDBatch(ctx, h.client, []string{platformResourceID})
	if err != nil {
		h.logger.Error("failed to fetch lookup resources", zap.Error(err))
		return err
	}
	if len(lookupResourceRes) == 0 || len(lookupResourceRes[req.PlatformResourceID]) == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "resource not found")
	}
	var lookupResource *es2.LookupResource
	if req.ResourceType == nil {
		lookupResource = utils.GetPointer(lookupResourceRes[req.PlatformResourceID][0])
	} else {
		for _, r := range lookupResourceRes[req.PlatformResourceID] {
			r := r
			if strings.ToLower(r.ResourceType) == strings.ToLower(*req.ResourceType) {
				lookupResource = &r
				break
			}
		}
	}
	if lookupResource == nil {
		return echo.NewHTTPError(http.StatusNotFound, "resource not found")
	}

	resource, err := es.FetchResourceByResourceIdAndType(ctx, h.client, lookupResource.PlatformID, lookupResource.ResourceType)
	if err != nil {
		h.logger.Error("failed to fetch resource", zap.Error(err))
		return err
	}
	if resource == nil {
		return echo.NewHTTPError(http.StatusNotFound, "resource not found")
	}

	response := api.GetSingleResourceFindingResponse{
		Resource: *resource,
	}

	controlComplianceResults, err := es.FetchComplianceResultsPerControlForResourceId(ctx, h.logger, h.client, lookupResource.PlatformID)
	if err != nil {
		h.logger.Error("failed to fetch control complianceResults", zap.Error(err))
		return err
	}

	allIntegrations, err := h.integrationClient.ListIntegrations(&httpclient.Context{UserRole: authApi.AdminRole}, nil)
	if err != nil {
		h.logger.Error("failed to get sources", zap.Error(err))
		return err
	}
	allSourcesMap := make(map[string]*integrationapi.Integration)
	for _, src := range allIntegrations.Integrations {
		src := src
		allSourcesMap[src.IntegrationID] = &src
	}

	controls, err := h.db.ListControls(ctx, nil, nil)
	if err != nil {
		h.logger.Error("failed to get controls", zap.Error(err))
		return err
	}
	controlsMap := make(map[string]*db.Control)
	for _, control := range controls {
		control := control
		controlsMap[control.ID] = &control
	}

	benchmarks, err := h.db.ListBenchmarksBare(ctx)
	if err != nil {
		h.logger.Error("failed to get benchmarks", zap.Error(err))
		return err
	}
	benchmarksMap := make(map[string]*db.Benchmark)
	for _, benchmark := range benchmarks {
		benchmark := benchmark
		benchmarksMap[benchmark.ID] = &benchmark
	}

	resourceTypeMetadata, err := h.inventoryClient.ListResourceTypesMetadata(&httpclient.Context{UserRole: authApi.AdminRole},
		nil, nil, nil, false, nil, 10000, 1)
	if err != nil {
		h.logger.Error("failed to get resource type metadata", zap.Error(err))
		return err
	}
	resourceTypeMetadataMap := make(map[string]*inventoryApi.ResourceType)
	for _, item := range resourceTypeMetadata.ResourceTypes {
		item := item
		resourceTypeMetadataMap[strings.ToLower(item.ResourceType)] = &item
	}

	complianceResultIDs := make([]string, 0, len(controlComplianceResults))
	for _, controlFinding := range controlComplianceResults {
		complianceResultIDs = append(complianceResultIDs, controlFinding.EsID)
		controlFinding := controlFinding
		controlFinding.ResourceName = lookupResource.ResourceName
		complianceResult := api.GetAPIComplianceResultFromESComplianceResult(controlFinding)

		for _, parentBenchmark := range controlFinding.ParentBenchmarks {
			if benchmark, ok := benchmarksMap[parentBenchmark]; ok {
				complianceResult.ParentBenchmarkNames = append(complianceResult.ParentBenchmarkNames, benchmark.Title)
			}
		}

		if control, ok := controlsMap[complianceResult.ControlID]; ok {
			complianceResult.ControlTitle = control.Title
		}

		if rtMetadata, ok := resourceTypeMetadataMap[strings.ToLower(complianceResult.ResourceType)]; ok {
			complianceResult.ResourceTypeName = rtMetadata.ResourceLabel
		}

		response.ControlComplianceResults = append(response.ControlComplianceResults, complianceResult)
	}

	findingEvents, err := es.FetchComplianceResultDriftEventsByComplianceResultIDs(ctx, h.logger, h.client, complianceResultIDs)
	if err != nil {
		h.logger.Error("failed to fetch finding events", zap.Error(err))
		return err
	}

	response.ComplianceResultDriftEvents = make([]api.ComplianceResultDriftEvent, 0, len(findingEvents))
	for _, findingEvent := range findingEvents {
		response.ComplianceResultDriftEvents = append(response.ComplianceResultDriftEvents, api.GetAPIComplianceResultDriftEventFromESComplianceResultDriftEvent(findingEvent))
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// GetSingleComplianceResultByComplianceResultID
//
//	@Summary		Get single finding by finding ID
//	@Description	Retrieving a single finding by finding ID
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"ComplianceResult ID"
//	@Success		200	{object}	api.ComplianceResult
//	@Router			/compliance/api/v1/compliance_result/single/{id} [get]
func (h *HttpHandler) GetSingleComplianceResultByComplianceResultID(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	findingID := echoCtx.Param("id")

	finding, err := es.FetchComplianceResultByID(ctx, h.logger, h.client, findingID)
	if err != nil {
		h.logger.Error("failed to fetch finding by id", zap.Error(err))
		return err
	}
	if finding == nil {
		return echo.NewHTTPError(http.StatusNotFound, "finding not found")
	}

	apiFinding := api.GetAPIComplianceResultFromESComplianceResult(*finding)

	integration, err := h.integrationClient.GetIntegration(&httpclient.Context{UserRole: authApi.AdminRole}, finding.IntegrationID)
	if err != nil {
		h.logger.Error("failed to get integration", zap.Error(err), zap.String("integration_id", finding.IntegrationID))
		return err
	}
	apiFinding.ProviderID = integration.ProviderID
	apiFinding.IntegrationName = integration.Name

	if len(finding.ResourceType) > 0 {
		resourceTypeMetadata, err := h.inventoryClient.ListResourceTypesMetadata(&httpclient.Context{UserRole: authApi.AdminRole},
			nil, nil,
			[]string{finding.ResourceType}, false, nil, 10000, 1)
		if err != nil {
			h.logger.Error("failed to get resource type metadata", zap.Error(err))
			return err
		}
		if len(resourceTypeMetadata.ResourceTypes) > 0 {
			apiFinding.ResourceTypeName = resourceTypeMetadata.ResourceTypes[0].ResourceLabel
		}
	}

	control, err := h.db.GetControl(ctx, finding.ControlID)
	if err != nil {
		h.logger.Error("failed to get control", zap.Error(err), zap.String("control_id", finding.ControlID))
		return err
	}
	apiFinding.ControlTitle = control.Title

	parentBenchmarks, err := h.db.GetBenchmarksBare(ctx, finding.ParentBenchmarks)
	if err != nil {
		h.logger.Error("failed to get parent benchmarks", zap.Error(err), zap.Strings("parent_benchmarks", finding.ParentBenchmarks))
		return err
	}
	parentBenchmarksMap := make(map[string]db.Benchmark)
	for _, benchmark := range parentBenchmarks {
		parentBenchmarksMap[benchmark.ID] = benchmark
	}
	for _, parentBenchmark := range finding.ParentBenchmarks {
		if benchmark, ok := parentBenchmarksMap[parentBenchmark]; ok {
			apiFinding.ParentBenchmarkNames = append(apiFinding.ParentBenchmarkNames, benchmark.Title)
		}
	}

	return echoCtx.JSON(http.StatusOK, apiFinding)
}

// CountComplianceResults godoc
//
//	@Summary		Get complianceResult count
//	@Description	Retrieving all compliance run complianceResult count with respect to filters.
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			complianceStatus	query		[]api.ComplianceStatus	false	"ComplianceStatus to filter by defaults to all complianceStatus except passed"
//	@Param			stateActive			query		[]bool					false	"StateActive to filter by defaults to true"
//	@Success		200					{object}	api.CountComplianceResultsResponse
//	@Router			/compliance/api/v1/compliance_result/count [get]
func (h *HttpHandler) CountComplianceResults(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	complianceStatuses := api.ParseComplianceStatuses(httpserver2.QueryArrayParam(echoCtx, "complianceStatus"))
	if len(complianceStatuses) == 0 {
		complianceStatuses = []api.ComplianceStatus{api.ComplianceStatusFailed}
	}

	stateActives := httpserver2.QueryArrayParam(echoCtx, "stateActive")
	if len(stateActives) == 0 {
		stateActives = []string{"true"}
	}
	boolStateActives := make([]bool, 0, len(stateActives))
	for _, stateActive := range stateActives {
		stateActiveBool, err := strconv.ParseBool(stateActive)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid stateActive value")
		}
		boolStateActives = append(boolStateActives, stateActiveBool)
	}

	esComplianceStatuses := make([]opengovernanceTypes.ComplianceStatus, 0, len(complianceStatuses))
	for _, status := range complianceStatuses {
		esComplianceStatuses = append(esComplianceStatuses, status.GetEsComplianceStatuses()...)
	}

	totalCount, err := es.ComplianceResultsCount(ctx, h.client, esComplianceStatuses, boolStateActives)
	if err != nil {
		return err
	}

	response := api.CountComplianceResultsResponse{
		Count: totalCount,
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// GetComplianceResultFilterValues godoc
//
//	@Summary		Get possible values for finding filters
//	@Description	Retrieving possible values for finding filters.
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			request	body		api.ComplianceResultFilters	true	"Request Body"
//	@Success		200		{object}	api.ComplianceResultFiltersWithMetadata
//	@Router			/compliance/api/v1/compliance_result/filters [post]
func (h *HttpHandler) GetComplianceResultFilterValues(echoCtx echo.Context) error {
	var err error
	ctx := echoCtx.Request().Context()

	var req api.ComplianceResultFilters
	if err := bindValidate(echoCtx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	req.IntegrationID, err = h.getIntegrationIdFilterFromInputs(echoCtx.Request().Context(), req.IntegrationID, req.IntegrationGroup)
	if err != nil {
		return err
	}

	//req.IntegrationID, err = httpserver2.ResolveIntegrationIDs(echoCtx, req.IntegrationID)
	//if err != nil {
	//	return err
	//}

	if len(req.ComplianceStatus) == 0 {
		req.ComplianceStatus = []api.ComplianceStatus{api.ComplianceStatusFailed}
	}

	esComplianceStatuses := make([]opengovernanceTypes.ComplianceStatus, 0, len(req.ComplianceStatus))
	for _, status := range req.ComplianceStatus {
		esComplianceStatuses = append(esComplianceStatuses, status.GetEsComplianceStatuses()...)
	}

	resourceTypeMetadata, err := h.inventoryClient.ListResourceTypesMetadata(&httpclient.Context{UserRole: authApi.AdminRole},
		nil, nil, nil, false, nil, 10000, 1)
	if err != nil {
		h.logger.Error("failed to get resource type metadata", zap.Error(err))
		return err
	}
	resourceTypeMetadataMap := make(map[string]*inventoryApi.ResourceType)
	for _, item := range resourceTypeMetadata.ResourceTypes {
		item := item
		resourceTypeMetadataMap[strings.ToLower(item.ResourceType)] = &item
	}

	resourceCollectionMetadata, err := h.inventoryClient.ListResourceCollections(&httpclient.Context{UserRole: authApi.AdminRole})
	if err != nil {
		h.logger.Error("failed to get resource collection metadata", zap.Error(err))
		return err
	}
	resourceCollectionMetadataMap := make(map[string]*inventoryApi.ResourceCollection)
	for _, item := range resourceCollectionMetadata {
		item := item
		resourceCollectionMetadataMap[item.ID] = &item
	}

	integrations, err := h.integrationClient.ListIntegrations(&httpclient.Context{UserRole: authApi.AdminRole}, nil)
	if err != nil {
		h.logger.Error("failed to get integrations", zap.Error(err))
		return err
	}
	integrationMetadataMap := make(map[string]*integrationapi.Integration)
	for _, item := range integrations.Integrations {
		item := item
		integrationMetadataMap[item.IntegrationID] = &item
	}

	benchmarkMetadata, err := h.db.ListBenchmarksBare(ctx)
	if err != nil {
		h.logger.Error("failed to get benchmarks", zap.Error(err))
		return err
	}
	benchmarkMetadataMap := make(map[string]*db.Benchmark)
	for _, item := range benchmarkMetadata {
		item := item
		benchmarkMetadataMap[item.ID] = &item
	}

	controlMetadata, err := h.db.ListControlsBare(ctx)
	if err != nil {
		h.logger.Error("failed to get controls", zap.Error(err))
		return err
	}
	controlMetadataMap := make(map[string]*db.Control)
	for _, item := range controlMetadata {
		item := item
		controlMetadataMap[item.ID] = &item
	}

	var lastEventFrom, lastEventTo, evaluatedAtFrom, evaluatedAtTo *time.Time
	if req.LastEvent.From != nil && *req.LastEvent.From != 0 {
		lastEventFrom = utils.GetPointer(time.Unix(*req.LastEvent.From, 0))
	}
	if req.LastEvent.To != nil && *req.LastEvent.To != 0 {
		lastEventTo = utils.GetPointer(time.Unix(*req.LastEvent.To, 0))
	}
	if req.EvaluatedAt.From != nil && *req.EvaluatedAt.From != 0 {
		evaluatedAtFrom = utils.GetPointer(time.Unix(*req.EvaluatedAt.From, 0))
	}
	if req.EvaluatedAt.To != nil && *req.EvaluatedAt.To != 0 {
		evaluatedAtTo = utils.GetPointer(time.Unix(*req.EvaluatedAt.To, 0))
	}

	possibleFilters, err := es.ComplianceResultsFiltersQuery(ctx, h.logger, h.client,
		req.ResourceID, req.IntegrationType, req.IntegrationID, req.NotIntegrationID,
		req.ResourceTypeID,
		req.BenchmarkID, req.ControlID,
		req.Severity,
		lastEventFrom, lastEventTo,
		evaluatedAtFrom, evaluatedAtTo,
		req.StateActive, esComplianceStatuses)
	if err != nil {
		h.logger.Error("failed to get possible filters", zap.Error(err))
		return err
	}
	response := api.ComplianceResultFiltersWithMetadata{}
	for _, item := range possibleFilters.Aggregations.BenchmarkIDFilter.Buckets {
		if benchmark, ok := benchmarkMetadataMap[item.Key]; ok {
			response.BenchmarkID = append(response.BenchmarkID, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: benchmark.Title,
				Count:       utils.GetPointer(item.DocCount),
			})
		} else {
			response.BenchmarkID = append(response.BenchmarkID, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: item.Key,
				Count:       utils.GetPointer(item.DocCount),
			})
		}
	}
	for _, item := range possibleFilters.Aggregations.ControlIDFilter.Buckets {
		if control, ok := controlMetadataMap[item.Key]; ok {
			response.ControlID = append(response.ControlID, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: control.Title,
				Count:       utils.GetPointer(item.DocCount),
			})
		} else {
			response.ControlID = append(response.ControlID, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: item.Key,
				Count:       utils.GetPointer(item.DocCount),
			})
		}
	}
	if len(possibleFilters.Aggregations.IntegrationTypeFilter.Buckets) > 0 {
		for _, bucket := range possibleFilters.Aggregations.IntegrationTypeFilter.Buckets {
			integrationType := integration_type.ParseType(bucket.Key)
			response.IntegrationType = append(response.IntegrationType, api.FilterWithMetadata{
				Key:         integrationType.String(),
				DisplayName: integrationType.String(),
				Count:       utils.GetPointer(bucket.DocCount),
			})
		}
	}
	for _, item := range possibleFilters.Aggregations.ResourceTypeFilter.Buckets {
		if rtMetadata, ok := resourceTypeMetadataMap[strings.ToLower(item.Key)]; ok {
			response.ResourceTypeID = append(response.ResourceTypeID, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: rtMetadata.ResourceLabel,
				Count:       utils.GetPointer(item.DocCount),
			})
		} else if item.Key == "" {
			response.ResourceTypeID = append(response.ResourceTypeID, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: "Unknown",
			})
		} else {
			response.ResourceTypeID = append(response.ResourceTypeID, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: item.Key,
				Count:       utils.GetPointer(item.DocCount),
			})
		}
	}

	for _, item := range possibleFilters.Aggregations.IntegrationIDFilter.Buckets {
		if integration, ok := integrationMetadataMap[item.Key]; ok {
			response.IntegrationID = append(response.IntegrationID, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: integration.Name,
				Count:       utils.GetPointer(item.DocCount),
			})
		} else {
			response.IntegrationID = append(response.IntegrationID, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: item.Key,
				Count:       utils.GetPointer(item.DocCount),
			})
		}
	}

	for _, item := range possibleFilters.Aggregations.ResourceCollectionFilter.Buckets {
		if resourceCollection, ok := resourceCollectionMetadataMap[item.Key]; ok {
			response.ResourceCollection = append(response.ResourceCollection, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: resourceCollection.Name,
				Count:       utils.GetPointer(item.DocCount),
			})
		} else {
			response.ResourceCollection = append(response.ResourceCollection, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: item.Key,
				Count:       utils.GetPointer(item.DocCount),
			})
		}
	}

	for _, item := range possibleFilters.Aggregations.SeverityFilter.Buckets {
		response.Severity = append(response.Severity, api.FilterWithMetadata{
			Key:         item.Key,
			DisplayName: item.Key,
			Count:       utils.GetPointer(item.DocCount),
		})
	}

	for _, item := range possibleFilters.Aggregations.StateActiveFilter.Buckets {
		response.StateActive = append(response.StateActive, api.FilterWithMetadata{
			Key:         item.KeyAsString,
			DisplayName: item.KeyAsString,
			Count:       utils.GetPointer(item.DocCount),
		})
	}

	apiComplianceStatuses := make(map[api.ComplianceStatus]int)
	for _, item := range possibleFilters.Aggregations.ComplianceStatusFilter.Buckets {
		if opengovernanceTypes.ParseComplianceStatus(item.Key).IsPassed() {
			apiComplianceStatuses[api.ComplianceStatusPassed] += item.DocCount
		} else {
			apiComplianceStatuses[api.ComplianceStatusFailed] += item.DocCount
		}
	}
	for status, count := range apiComplianceStatuses {
		count := count
		response.ComplianceStatus = append(response.ComplianceStatus, api.FilterWithMetadata{
			Key:         string(status),
			DisplayName: string(status),
			Count:       &count,
		})
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// GetComplianceResultKPIs godoc
//
//	@Summary		Get finding KPIs
//	@Description	Retrieving KPIs for complianceResults.
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	api.ComplianceResultKPIResponse
//	@Router			/compliance/api/v1/compliance_result/kpi [get]
func (h *HttpHandler) GetComplianceResultKPIs(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	kpiRes, err := es.ComplianceResultKPIQuery(ctx, h.logger, h.client)
	if err != nil {
		h.logger.Error("failed to get finding kpis", zap.Error(err))
		return err
	}
	response := api.ComplianceResultKPIResponse{
		FailedComplianceResultsCount: kpiRes.Hits.Total.Value,
		FailedResourceCount:          kpiRes.Aggregations.ResourceCount.Value,
		FailedControlCount:           kpiRes.Aggregations.ControlCount.Value,
		FailedIntegrationCount:       kpiRes.Aggregations.IntegrationCount.Value,
	}
	return echoCtx.JSON(http.StatusOK, response)
}

// GetTopFieldByComplianceResultCount godoc
//
//	@Summary		Get top field by finding count
//	@Description	Retrieving the top field by finding count.
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			field				path		string											true	"Field"	Enums(resourceType,integrationID,resourceID,service,controlID)
//	@Param			count				path		int												true	"Count"
//	@Param			integrationId		query		[]string										false	"integration IDs to filter by (inclusive)"
//	@Param			notIntegrationId	query		[]string										false	"integration IDs to filter by (exclusive)"
//	@Param			integrationGroup		query		[]string										false	"integration groups to filter by "
//	@Param			integrationTypes	query		[]integration.Type								false	"integration type to filter by"
//	@Param			benchmarkId			query		[]string										false	"BenchmarkID"
//	@Param			controlId			query		[]string										false	"ControlID"
//	@Param			severities			query		[]opengovernanceTypes.ComplianceResultSeverity	false	"Severities to filter by defaults to all severities except passed"
//	@Param			complianceStatus	query		[]api.ComplianceStatus							false	"ComplianceStatus to filter by defaults to all complianceStatus except passed"
//	@Param			stateActive			query		[]bool											false	"StateActive to filter by defaults to true"
//	@Param			jobId				query		[]string										false	"Job ID to filter"
//	@Param			startTime			query		int64											false	"Start time to filter by"
//	@Param			endTime				query		int64											false	"End time to filter by"
//	@Param			interval			query		string											false	"Time interval to filter by"
//	@Success		200					{object}	api.GetTopFieldResponse
//	@Router			/compliance/api/v1/compliance_result/top/{field}/{count} [get]
func (h *HttpHandler) GetTopFieldByComplianceResultCount(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	field := echoCtx.Param("field")
	esField := field
	countStr := echoCtx.Param("count")
	count, err := strconv.Atoi(countStr)
	if err != nil {
		return err
	}
	esCount := count

	if field == "service" {
		esField = "resourceType"
		esCount = 10000
	}

	integrationIDs, err := h.getIntegrationIdFilterFromParams(echoCtx)
	if err != nil {
		return err
	}
	notIntegrationIDs := httpserver2.QueryArrayParam(echoCtx, "notIntegrationId")
	integrationTypes := httpserver2.QueryArrayParam(echoCtx, "integrationTypes")
	benchmarkIDs := httpserver2.QueryArrayParam(echoCtx, "benchmarkId")
	controlIDs := httpserver2.QueryArrayParam(echoCtx, "controlId")
	jobIDs := httpserver2.QueryArrayParam(echoCtx, "jobId")
	severities := opengovernanceTypes.ParseComplianceResultSeverities(httpserver2.QueryArrayParam(echoCtx, "severities"))
	complianceStatuses := api.ParseComplianceStatuses(httpserver2.QueryArrayParam(echoCtx, "complianceStatus"))
	if len(complianceStatuses) == 0 {
		complianceStatuses = []api.ComplianceStatus{
			api.ComplianceStatusFailed,
		}
	}

	var endTime *time.Time
	var startTime *time.Time

	intervalStr := echoCtx.QueryParam("interval")
	if intervalStr != "" {
		startTime, endTime, err = parseTimeInterval(intervalStr)
		if err != nil {
			h.logger.Error("failed to parse time interval", zap.Error(err))
			return echo.NewHTTPError(http.StatusBadRequest, "failed to parse time interval")
		}
	} else {
		if endTimeStr := echoCtx.QueryParam("endTime"); endTimeStr != "" {
			endTimeInt, err := strconv.ParseInt(endTimeStr, 10, 64)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, "invalid endTime")
			}
			endTime = utils.GetPointer(time.Unix(endTimeInt, 0))
		}
		if startTimeStr := echoCtx.QueryParam("startTime"); startTimeStr != "" {
			startTimeInt, err := strconv.ParseInt(startTimeStr, 10, 64)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, "invalid startTime")
			}
			startTime = utils.GetPointer(time.Unix(startTimeInt, 0))
		}
	}

	esComplianceStatuses := make([]opengovernanceTypes.ComplianceStatus, 0, len(complianceStatuses))
	for _, status := range complianceStatuses {
		esComplianceStatuses = append(esComplianceStatuses, status.GetEsComplianceStatuses()...)
	}

	stateActives := []bool{true}
	if stateActiveStr := httpserver2.QueryArrayParam(echoCtx, "stateActive"); len(stateActiveStr) > 0 {
		stateActives = make([]bool, 0, len(stateActiveStr))
		for _, item := range stateActiveStr {
			stateActive, err := strconv.ParseBool(item)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, "invalid stateActive")
			}
			stateActives = append(stateActives, stateActive)
		}
	}

	var response api.GetTopFieldResponse
	topFieldResponse, err := es.ComplianceResultsTopFieldQuery(ctx, h.logger, h.client, esField, integrationTypes,
		nil, integrationIDs, notIntegrationIDs, jobIDs,
		benchmarkIDs, controlIDs, severities, esComplianceStatuses, stateActives, min(10000, esCount), startTime, endTime)
	if err != nil {
		h.logger.Error("failed to get top field", zap.Error(err))
		return err
	}
	topFieldTotalResponse, err := es.ComplianceResultsTopFieldQuery(ctx, h.logger, h.client, esField, integrationTypes,
		nil, integrationIDs, notIntegrationIDs, jobIDs,
		benchmarkIDs, controlIDs, severities, nil, stateActives, 10000, startTime, endTime)
	if err != nil {
		h.logger.Error("failed to get top field total", zap.Error(err))
		return err
	}

	switch strings.ToLower(field) {
	case "resourcetype":
		resourceTypeList := make([]string, 0, len(topFieldResponse.Aggregations.FieldFilter.Buckets))
		for _, item := range topFieldResponse.Aggregations.FieldFilter.Buckets {
			if item.Key == "" {
				continue
			}
			resourceTypeList = append(resourceTypeList, item.Key)
		}
		resourceTypeMetadata, err := h.inventoryClient.ListResourceTypesMetadata(&httpclient.Context{UserRole: authApi.AdminRole},
			nil, nil, resourceTypeList, false, nil, 10000, 1)
		if err != nil {
			return err
		}
		resourceTypeMetadataMap := make(map[string]*inventoryApi.ResourceType)
		for _, item := range resourceTypeMetadata.ResourceTypes {
			item := item
			resourceTypeMetadataMap[strings.ToLower(item.ResourceType)] = &item
		}
		resourceTypeCountMap := make(map[string]int)
		for _, item := range topFieldResponse.Aggregations.FieldFilter.Buckets {
			if item.Key == "" {
				item.Key = "Unknown"
			}
			resourceTypeCountMap[item.Key] += item.DocCount
		}
		resourceTypeTotalCountMap := make(map[string]int)
		for _, item := range topFieldTotalResponse.Aggregations.FieldFilter.Buckets {
			if item.Key == "" {
				item.Key = "Unknown"
			}
			resourceTypeTotalCountMap[item.Key] += item.DocCount
		}
		resourceTypeCountList := make([]api.TopFieldRecord, 0, len(resourceTypeCountMap))
		for k, v := range resourceTypeCountMap {
			rt, ok := resourceTypeMetadataMap[strings.ToLower(k)]
			if !ok {
				rt = &inventoryApi.ResourceType{
					ResourceType:  k,
					ResourceLabel: k,
				}
			}
			resourceTypeCountList = append(resourceTypeCountList, api.TopFieldRecord{
				ResourceType: rt,
				Count:        v,
				TotalCount:   resourceTypeTotalCountMap[k],
			})
		}
		sort.Slice(resourceTypeCountList, func(i, j int) bool {
			return resourceTypeCountList[i].Count > resourceTypeCountList[j].Count
		})
		if len(resourceTypeCountList) > count {
			response.Records = resourceTypeCountList[:count]
		} else {
			response.Records = resourceTypeCountList
		}
		response.TotalCount = len(resourceTypeCountList)
	case "service":
		resourceTypeList := make([]string, 0, len(topFieldResponse.Aggregations.FieldFilter.Buckets))
		for _, item := range topFieldResponse.Aggregations.FieldFilter.Buckets {
			resourceTypeList = append(resourceTypeList, item.Key)
		}
		resourceTypeMetadata, err := h.inventoryClient.ListResourceTypesMetadata(&httpclient.Context{UserRole: authApi.AdminRole},
			nil, nil, resourceTypeList, false, nil, 10000, 1)
		if err != nil {
			return err
		}
		resourceTypeMetadataMap := make(map[string]inventoryApi.ResourceType)
		for _, item := range resourceTypeMetadata.ResourceTypes {
			resourceTypeMetadataMap[strings.ToLower(item.ResourceType)] = item
		}
		serviceCountMap := make(map[string]int)
		for _, item := range topFieldResponse.Aggregations.FieldFilter.Buckets {
			if rtMetadata, ok := resourceTypeMetadataMap[strings.ToLower(item.Key)]; ok {
				serviceCountMap[rtMetadata.ServiceName] += item.DocCount
			}
		}
		serviceTotalCountMap := make(map[string]int)
		for _, item := range topFieldTotalResponse.Aggregations.FieldFilter.Buckets {
			if rtMetadata, ok := resourceTypeMetadataMap[strings.ToLower(item.Key)]; ok {
				serviceTotalCountMap[rtMetadata.ServiceName] += item.DocCount
			}
		}
		serviceCountList := make([]api.TopFieldRecord, 0, len(serviceCountMap))
		for k, v := range serviceCountMap {
			k := k
			serviceCountList = append(serviceCountList, api.TopFieldRecord{
				Service:    &k,
				Count:      v,
				TotalCount: serviceTotalCountMap[k],
			})
		}
		sort.Slice(serviceCountList, func(i, j int) bool {
			return serviceCountList[i].Count > serviceCountList[j].Count
		})
		if len(serviceCountList) > count {
			response.Records = serviceCountList[:count]
		} else {
			response.Records = serviceCountList
		}
		response.TotalCount = len(serviceCountList)
	case "integrationid":
		resIntegrationIDs := make([]string, 0, len(topFieldTotalResponse.Aggregations.FieldFilter.Buckets))
		for _, item := range topFieldTotalResponse.Aggregations.FieldFilter.Buckets {
			resIntegrationIDs = append(resIntegrationIDs, item.Key)
		}
		integrations, err := h.integrationClient.ListIntegrationsByFilters(&httpclient.Context{UserRole: authApi.AdminRole}, integrationapi.ListIntegrationsRequest{
			IntegrationID: resIntegrationIDs,
		})
		if err != nil {
			h.logger.Error("failed to get integrations", zap.Error(err))
			return err
		}

		integrationsMap := make(map[string]integrationapi.Integration)
		for _, c := range integrations.Integrations {
			integrationsMap[c.IntegrationID] = c
		}

		recordMap := make(map[string]api.TopFieldRecord)

		for _, item := range topFieldTotalResponse.Aggregations.FieldFilter.Buckets {
			record, ok := recordMap[item.Key]
			if !ok {
				id, err := uuid.Parse(item.Key)
				if err != nil {
					h.logger.Error("failed to parse integration id", zap.Error(err))
					return err
				}
				integration, ok := integrationsMap[id.String()]
				if !ok {
					continue
				}
				record = api.TopFieldRecord{
					Integration: &integration,
				}
			}
			record.TotalCount += item.DocCount
			recordMap[item.Key] = record
		}

		for _, item := range topFieldResponse.Aggregations.FieldFilter.Buckets {
			record, ok := recordMap[item.Key]
			if !ok {
				id, err := uuid.Parse(item.Key)
				if err != nil {
					h.logger.Error("failed to parse integration id", zap.Error(err))
					return err
				}
				integration, ok := integrationsMap[id.String()]
				if !ok {
					continue
				}
				record = api.TopFieldRecord{
					Integration: &integration,
				}
			}
			record.Count = item.DocCount
			recordMap[item.Key] = record
		}

		controlsResult, err := es.ComplianceResultsComplianceStatusCountByControlPerIntegration(
			ctx, h.logger, h.client, integrationTypes, nil, resIntegrationIDs, benchmarkIDs, controlIDs, severities, nil,
			startTime, endTime)
		if err != nil {
			h.logger.Error("failed to get controls", zap.Error(err))
			return err
		}
		for _, item := range controlsResult.Aggregations.IntegrationGroup.Buckets {
			record, ok := recordMap[item.Key]
			if !ok {
				continue
			}
			if record.ControlCount == nil {
				record.ControlCount = utils.GetPointer(0)
			}
			if record.ControlTotalCount == nil {
				record.ControlTotalCount = utils.GetPointer(0)
			}
			for _, control := range item.ControlCount.Buckets {
				isFailed := false
				for _, complianceStatus := range control.ComplianceStatuses.Buckets {
					status := opengovernanceTypes.ParseComplianceStatus(complianceStatus.Key)
					if !status.IsPassed() && complianceStatus.DocCount > 0 {
						isFailed = true
						break
					}
				}
				if isFailed {
					record.ControlCount = utils.PAdd(record.ControlCount, utils.GetPointer(1))
				}
				record.ControlTotalCount = utils.PAdd(record.ControlTotalCount, utils.GetPointer(1))
			}
			recordMap[item.Key] = record
		}

		resourcesResult, err := es.GetPerFieldResourceComplianceResult(ctx, h.logger, h.client, "integrationID",
			resIntegrationIDs, notIntegrationIDs, nil, controlIDs, benchmarkIDs, severities, nil, startTime, endTime)
		if err != nil {
			h.logger.Error("failed to get resourcesResult", zap.Error(err))
			return err
		}

		for integrationId, results := range resourcesResult {
			results := results
			record, ok := recordMap[integrationId]
			if !ok {
				continue
			}
			record.ResourceTotalCount = utils.GetPointer(results.TotalCount)
			for _, complianceStatus := range complianceStatuses {
				switch complianceStatus {
				case api.ComplianceStatusFailed:
					record.ResourceCount = utils.PAdd(record.ResourceCount, &results.AlarmCount)
					record.ResourceCount = utils.PAdd(record.ResourceCount, &results.ErrorCount)
					record.ResourceCount = utils.PAdd(record.ResourceCount, &results.InfoCount)
					record.ResourceCount = utils.PAdd(record.ResourceCount, &results.SkipCount)
				case api.ComplianceStatusPassed:
					record.ResourceCount = utils.PAdd(record.ResourceCount, &results.OkCount)
				}
			}
			recordMap[integrationId] = record
		}

		for _, record := range recordMap {
			response.Records = append(response.Records, record)
		}

		//response.TotalCount = topFieldTotalResponse.Aggregations.BucketCount.Value
		response.TotalCount = len(response.Records)
	case "controlid":
		resControlIDs := make([]string, 0, len(topFieldTotalResponse.Aggregations.FieldFilter.Buckets))
		for _, item := range topFieldTotalResponse.Aggregations.FieldFilter.Buckets {
			resControlIDs = append(resControlIDs, item.Key)
		}
		controls, err := h.db.GetControls(ctx, resControlIDs, nil)
		if err != nil {
			h.logger.Error("failed to get controls", zap.Error(err))
			return err
		}

		recordMap := make(map[string]api.TopFieldRecord)

		for _, item := range topFieldTotalResponse.Aggregations.FieldFilter.Buckets {
			record, ok := recordMap[item.Key]
			if !ok {
				record = api.TopFieldRecord{
					Control: &api.Control{ID: item.Key},
				}
			}
			record.TotalCount += item.DocCount
			recordMap[item.Key] = record
		}

		for _, item := range topFieldResponse.Aggregations.FieldFilter.Buckets {
			record, ok := recordMap[item.Key]
			if !ok {
				record = api.TopFieldRecord{
					Control: &api.Control{ID: item.Key},
				}
			}
			record.Count = item.DocCount
			recordMap[item.Key] = record
		}

		for _, control := range controls {
			control := control
			record, ok := recordMap[control.ID]
			if !ok {
				continue
			}
			record.Control = utils.GetPointer(control.ToApi())
			recordMap[control.ID] = record
		}

		resourcesResult, err := es.GetPerFieldResourceComplianceResult(ctx, h.logger, h.client, "controlID",
			integrationIDs, notIntegrationIDs, nil, resControlIDs, benchmarkIDs, severities, nil, startTime, endTime)
		if err != nil {
			h.logger.Error("failed to get resourcesResult", zap.Error(err))
			return err
		}

		for controlId, results := range resourcesResult {
			results := results
			record, ok := recordMap[controlId]
			if !ok {
				continue
			}
			record.ResourceTotalCount = utils.GetPointer(results.TotalCount)
			for _, complianceStatus := range complianceStatuses {
				switch complianceStatus {
				case api.ComplianceStatusFailed:
					record.ResourceCount = utils.PAdd(record.ResourceCount, &results.AlarmCount)
					record.ResourceCount = utils.PAdd(record.ResourceCount, &results.ErrorCount)
					record.ResourceCount = utils.PAdd(record.ResourceCount, &results.InfoCount)
					record.ResourceCount = utils.PAdd(record.ResourceCount, &results.SkipCount)
				case api.ComplianceStatusPassed:
					record.ResourceCount = utils.PAdd(record.ResourceCount, &results.OkCount)
				}
			}
			recordMap[controlId] = record
		}

		for _, record := range recordMap {
			response.Records = append(response.Records, record)
		}

		response.TotalCount = topFieldTotalResponse.Aggregations.BucketCount.Value
	default:
		totalCountMap := make(map[string]int)
		for _, item := range topFieldTotalResponse.Aggregations.FieldFilter.Buckets {
			totalCountMap[item.Key] += item.DocCount
		}

		for _, item := range topFieldResponse.Aggregations.FieldFilter.Buckets {
			item := item
			response.Records = append(response.Records, api.TopFieldRecord{
				Field:      &item.Key,
				Count:      item.DocCount,
				TotalCount: totalCountMap[item.Key],
			})
		}
		//response.TotalCount = topFieldResponse.Aggregations.BucketCount.Value
		response.TotalCount = len(response.Records)
	}

	sort.Slice(response.Records, func(i, j int) bool {
		if response.Records[i].Count != response.Records[j].Count {
			return response.Records[i].Count > response.Records[j].Count
		}
		return response.Records[i].TotalCount > response.Records[j].TotalCount
	})
	if len(response.Records) > 0 {
		response.Records = response.Records[:min(len(response.Records), count)]
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// GetComplianceResultsFieldCountByControls godoc
//
//	@Summary		Get complianceResults field count by controls
//	@Description	Retrieving the number of complianceResults field count by controls.
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			benchmarkId			path		string											true	"BenchmarkID"
//	@Param			field				path		string											true	"Field"	Enums(resourceType,integrationID,resourceID,service)
//	@Param			integrationId		query		[]string										false	"integration IDs to filter by"
//	@Param			integrationGroup		query		[]string										false	"integration groups to filter by "
//	@Param			resourceCollection	query		[]string										false	"Resource collection IDs to filter by"
//	@Param			integrationTypes	query		[]integration.Type								false	"Integration type to filter by"
//	@Param			severities			query		[]opengovernanceTypes.ComplianceResultSeverity	false	"Severities to filter by defaults to all severities except passed"
//	@Param			complianceStatus	query		[]api.ComplianceStatus							false	"ComplianceStatus to filter by defaults to failed"
//	@Success		200					{object}	api.GetTopFieldResponse
//	@Router			/compliance/api/v1/compliance_result/{benchmarkId}/{field}/count [get]
func (h *HttpHandler) GetComplianceResultsFieldCountByControls(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	benchmarkID := echoCtx.Param("benchmarkId")
	field := echoCtx.Param("field")
	var esField string
	if field == "resource" {
		esField = "resourceID"
	} else {
		esField = field
	}

	integrationIDs, err := h.getIntegrationIdFilterFromParams(echoCtx)
	if err != nil {
		return err
	}

	integrationTypes := httpserver2.QueryArrayParam(echoCtx, "integrationTypes")
	severities := opengovernanceTypes.ParseComplianceResultSeverities(httpserver2.QueryArrayParam(echoCtx, "severities"))
	complianceStatuses := api.ParseComplianceStatuses(httpserver2.QueryArrayParam(echoCtx, "complianceStatus"))
	if len(complianceStatuses) == 0 {
		complianceStatuses = []api.ComplianceStatus{
			api.ComplianceStatusFailed,
		}
	}
	esComplianceStatuses := make([]opengovernanceTypes.ComplianceStatus, 0, len(complianceStatuses))
	for _, status := range complianceStatuses {
		esComplianceStatuses = append(esComplianceStatuses, status.GetEsComplianceStatuses()...)
	}
	//tracer :
	ctx, span1 := tracer.Start(ctx, "new_GetBenchmarkTreeIDs", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_GetBenchmarkTreeIDs")
	defer span1.End()

	benchmarkIDs, err := h.GetBenchmarkTreeIDs(ctx, benchmarkID)
	if err != nil {
		span1.RecordError(err)
		span1.SetStatus(codes.Error, err.Error())
		return err
	}
	span1.AddEvent("information", trace.WithAttributes(
		attribute.String("benchmark id", benchmarkID),
	))
	span1.End()

	var response api.GetFieldCountResponse
	res, err := es.ComplianceResultsFieldCountByControl(ctx, h.logger, h.client, esField, integrationTypes, nil, integrationIDs, benchmarkIDs, nil, severities,
		esComplianceStatuses)
	if err != nil {
		return err
	}
	for _, b := range res.Aggregations.ControlCount.Buckets {
		var fieldCounts []api.TopFieldRecord
		for _, bucketField := range b.ComplianceStatuses.Buckets {
			bucketField := bucketField
			fieldCounts = append(fieldCounts, api.TopFieldRecord{Field: &bucketField.Key, Count: bucketField.FieldCount.Value})
		}
		response.Controls = append(response.Controls, struct {
			ControlName string               `json:"controlName"`
			FieldCounts []api.TopFieldRecord `json:"fieldCounts"`
		}{ControlName: b.Key, FieldCounts: fieldCounts})
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// GetAccountsComplianceResultsSummary godoc
//
//	@Summary		Get accounts complianceResults summaries
//	@Description	Retrieving list of accounts with their security score and severities complianceResults count
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			benchmarkId		path		string		true	"BenchmarkID"
//	@Param			integrationId	query		[]string	false	"integration IDs to filter by"
//	@Param			integrationGroup	query		[]string	false	"integration groups to filter by "
//	@Success		200				{object}	api.GetAccountsComplianceResultsSummaryResponse
//	@Router			/compliance/api/v1/compliance_result/{benchmarkId}/accounts [get]
func (h *HttpHandler) GetAccountsComplianceResultsSummary(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	benchmarkID := echoCtx.Param("benchmarkId")
	integrationIDs, err := h.getIntegrationIdFilterFromParams(echoCtx)
	if err != nil {
		return err
	}

	var response api.GetAccountsComplianceResultsSummaryResponse
	res, evaluatedAt, err := es.BenchmarkIntegrationSummary(ctx, h.logger, h.client, benchmarkID)
	if err != nil {
		return err
	}

	if len(integrationIDs) == 0 {
		assignmentsByBenchmarkId, err := h.db.GetBenchmarkAssignmentsByBenchmarkId(ctx, benchmarkID)
		if err != nil {
			return err
		}

		for _, assignment := range assignmentsByBenchmarkId {
			if assignment.IntegrationID != nil {
				integrationIDs = append(integrationIDs, *assignment.IntegrationID)
			}
		}
	}

	srcs, err := h.integrationClient.ListIntegrationsByFilters(&httpclient.Context{UserRole: authApi.AdminRole}, integrationapi.ListIntegrationsRequest{
		IntegrationID: integrationIDs,
	})
	if err != nil {
		return err
	}

	for _, src := range srcs.Integrations {
		summary, ok := res[src.IntegrationID]
		if !ok {
			summary.Result.SeverityResult = map[opengovernanceTypes.ComplianceResultSeverity]int{}
			summary.Result.QueryResult = map[opengovernanceTypes.ComplianceStatus]int{}
		}

		account := api.AccountsComplianceResultsSummary{
			AccountName:   src.Name,
			AccountId:     src.ProviderID,
			SecurityScore: summary.Result.SecurityScore,
			SeveritiesCount: struct {
				Critical int `json:"critical"`
				High     int `json:"high"`
				Medium   int `json:"medium"`
				Low      int `json:"low"`
				None     int `json:"none"`
			}{
				Critical: summary.Result.SeverityResult[opengovernanceTypes.ComplianceResultSeverityCritical],
				High:     summary.Result.SeverityResult[opengovernanceTypes.ComplianceResultSeverityHigh],
				Medium:   summary.Result.SeverityResult[opengovernanceTypes.ComplianceResultSeverityMedium],
				Low:      summary.Result.SeverityResult[opengovernanceTypes.ComplianceResultSeverityLow],
				None:     summary.Result.SeverityResult[opengovernanceTypes.ComplianceResultSeverityNone],
			},
			ComplianceStatusesCount: struct {
				Passed int `json:"passed"`
				Failed int `json:"failed"`
				Error  int `json:"error"`
				Info   int `json:"info"`
				Skip   int `json:"skip"`
			}{
				Passed: summary.Result.QueryResult[opengovernanceTypes.ComplianceStatusOK],
				Failed: summary.Result.QueryResult[opengovernanceTypes.ComplianceStatusALARM],
				Error:  summary.Result.QueryResult[opengovernanceTypes.ComplianceStatusERROR],
				Info:   summary.Result.QueryResult[opengovernanceTypes.ComplianceStatusINFO],
				Skip:   summary.Result.QueryResult[opengovernanceTypes.ComplianceStatusSKIP],
			},
			LastCheckTime: time.Unix(evaluatedAt, 0),
		}

		response.Accounts = append(response.Accounts, account)
	}

	for idx, conn := range response.Accounts {

		response.Accounts[idx] = conn
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// GetServicesComplianceResultsSummary godoc
//
//	@Summary		Get services complianceResults summary
//	@Description	Retrieving list of services with their security score and severities complianceResults count
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			benchmarkId		path		string		true	"BenchmarkID"
//	@Param			integrationId	query		[]string	false	"Integration IDs to filter by"
//	@Param			integrationGroup	query		[]string	false	"Integration groups to filter by "
//	@Success		200				{object}	api.GetServicesComplianceResultsSummaryResponse
//	@Router			/compliance/api/v1/compliance_result/{benchmarkId}/services [get]
func (h *HttpHandler) GetServicesComplianceResultsSummary(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	benchmarkID := echoCtx.Param("benchmarkId")
	integrationIDs, err := h.getIntegrationIdFilterFromParams(echoCtx)
	if err != nil {
		return err
	}

	var response api.GetServicesComplianceResultsSummaryResponse
	resp, err := es.ResourceTypesComplianceResultsSummary(ctx, h.logger, h.client, integrationIDs, benchmarkID)
	if err != nil {
		return err
	}

	resourceTypes, err := h.inventoryClient.ListResourceTypesMetadata(&httpclient.Context{UserRole: authApi.AdminRole},
		nil, nil, nil, false, nil, 10000, 1)
	if err != nil {
		h.logger.Error("failed to get resource types metadata", zap.Error(err))
		return err
	}
	resourceTypeMap := make(map[string]inventoryApi.ResourceType)
	for _, rt := range resourceTypes.ResourceTypes {
		resourceTypeMap[strings.ToLower(rt.ResourceType)] = rt
	}

	for _, resourceType := range resp.Aggregations.Summaries.Buckets {
		sevMap := make(map[string]int)
		for _, severity := range resourceType.Severity.Buckets {
			sevMap[severity.Key] = severity.DocCount
		}
		resMap := make(map[string]int)
		for _, controlResult := range resourceType.ComplianceStatus.Buckets {
			resMap[controlResult.Key] = controlResult.DocCount
		}

		securityScore := float64(resMap[string(opengovernanceTypes.ComplianceStatusOK)]) / float64(resourceType.DocCount) * 100.0

		resourceTypeMetadata := resourceTypeMap[strings.ToLower(resourceType.Key)]
		if resourceTypeMetadata.ResourceType == "" {
			resourceTypeMetadata.ResourceType = resourceType.Key
			if resourceTypeMetadata.ResourceType == "" {
				resourceTypeMetadata.ResourceType = "Unknown"
			}
			resourceTypeMetadata.ResourceLabel = resourceType.Key
			if resourceTypeMetadata.ResourceLabel == "" {
				resourceTypeMetadata.ResourceLabel = "Unknown"
			}
		}
		service := api.ServiceComplianceResultsSummary{
			ServiceName:   resourceTypeMetadata.ResourceType,
			ServiceLabel:  resourceTypeMetadata.ResourceLabel,
			SecurityScore: securityScore,
			SeveritiesCount: struct {
				Critical int `json:"critical"`
				High     int `json:"high"`
				Medium   int `json:"medium"`
				Low      int `json:"low"`
				None     int `json:"none"`
			}{
				Critical: sevMap[string(opengovernanceTypes.ComplianceResultSeverityCritical)],
				High:     sevMap[string(opengovernanceTypes.ComplianceResultSeverityHigh)],
				Medium:   sevMap[string(opengovernanceTypes.ComplianceResultSeverityMedium)],
				Low:      sevMap[string(opengovernanceTypes.ComplianceResultSeverityLow)],
				None:     sevMap[string(opengovernanceTypes.ComplianceResultSeverityNone)],
			},
			ComplianceStatusesCount: struct {
				Passed int `json:"passed"`
				Failed int `json:"failed"`
			}{
				Passed: resMap[string(opengovernanceTypes.ComplianceStatusOK)] +
					resMap[string(opengovernanceTypes.ComplianceStatusINFO)] +
					resMap[string(opengovernanceTypes.ComplianceStatusSKIP)],
				Failed: resMap[string(opengovernanceTypes.ComplianceStatusALARM)] +
					resMap[string(opengovernanceTypes.ComplianceStatusERROR)],
			},
		}
		response.Services = append(response.Services, service)
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// GetComplianceResultDriftEvents godoc
//
//	@Summary		Get finding events
//	@Description	Retrieving all compliance finding events with respect to filters.
//	@Tags			compliance
//	@Security		BearerToken
//	@Accept			json
//	@Produce		json
//	@Param			request	body		api.GetComplianceResultDriftEventsRequest	true	"Request Body"
//	@Success		200		{object}	api.GetComplianceResultDriftEventsResponse
//	@Router			/compliance/api/v1/compliance_result_events [post]
func (h *HttpHandler) GetComplianceResultDriftEvents(echoCtx echo.Context) error {
	var err error
	ctx := echoCtx.Request().Context()

	var req api.GetComplianceResultDriftEventsRequest
	if err := bindValidate(echoCtx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	req.Filters.IntegrationID, err = h.getIntegrationIdFilterFromInputs(ctx, req.Filters.IntegrationID, req.Filters.IntegrationGroup)
	if err != nil {
		return err
	}

	//req.Filters.IntegrationID, err = httpserver2.ResolveIntegrationIDs(echoCtx, req.Filters.IntegrationID)
	//if err != nil {
	//	return err
	//}

	var response api.GetComplianceResultDriftEventsResponse

	if len(req.Filters.ComplianceStatus) == 0 {
		req.Filters.ComplianceStatus = []api.ComplianceStatus{api.ComplianceStatusFailed}
	}

	esComplianceStatuses := make([]opengovernanceTypes.ComplianceStatus, 0, len(req.Filters.ComplianceStatus))
	for _, status := range req.Filters.ComplianceStatus {
		esComplianceStatuses = append(esComplianceStatuses, status.GetEsComplianceStatuses()...)
	}

	if len(req.Sort) == 0 {
		req.Sort = []api.ComplianceResultDriftEventsSort{
			{ComplianceStatus: utils.GetPointer(api.SortDirectionDescending)},
		}
	}

	if len(req.AfterSortKey) != 0 {
		expectedLen := len(req.Sort) + 1
		if len(req.AfterSortKey) != expectedLen {
			return echo.NewHTTPError(http.StatusBadRequest, "sort key length should be zero or match a returned sort key from previous response")
		}
	}

	var evaluatedAtFrom, evaluatedAtTo *time.Time
	if req.Filters.EvaluatedAt.From != nil && *req.Filters.EvaluatedAt.From != 0 {
		evaluatedAtFrom = utils.GetPointer(time.Unix(*req.Filters.EvaluatedAt.From, 0))
	}
	if req.Filters.EvaluatedAt.To != nil && *req.Filters.EvaluatedAt.To != 0 {
		evaluatedAtTo = utils.GetPointer(time.Unix(*req.Filters.EvaluatedAt.To, 0))
	}

	res, totalCount, err := es.ComplianceResultDriftEventsQuery(ctx, h.logger, h.client,
		req.Filters.ComplianceResultID, req.Filters.PlatformResourceID,
		req.Filters.IntegrationType, req.Filters.IntegrationID, req.Filters.NotIntegrationID,
		req.Filters.ResourceType,
		req.Filters.BenchmarkID, req.Filters.ControlID, req.Filters.Severity,
		evaluatedAtFrom, evaluatedAtTo,
		req.Filters.StateActive, esComplianceStatuses, req.Sort, req.Limit, req.AfterSortKey)
	if err != nil {
		h.logger.Error("failed to get complianceResults", zap.Error(err))
		return err
	}

	allSources, err := h.integrationClient.ListIntegrations(&httpclient.Context{UserRole: authApi.AdminRole}, nil)
	if err != nil {
		h.logger.Error("failed to get sources", zap.Error(err))
		return err
	}
	allIntegrationsMap := make(map[string]*integrationapi.Integration)
	for _, src := range allSources.Integrations {
		src := src
		allIntegrationsMap[src.IntegrationID] = &src
	}

	resourceTypeMetadata, err := h.inventoryClient.ListResourceTypesMetadata(&httpclient.Context{UserRole: authApi.AdminRole},
		nil, nil, nil, false, nil, 10000, 1)
	if err != nil {
		h.logger.Error("failed to get resource type metadata", zap.Error(err))
		return err
	}
	resourceTypeMetadataMap := make(map[string]*inventoryApi.ResourceType)
	for _, item := range resourceTypeMetadata.ResourceTypes {
		item := item
		resourceTypeMetadataMap[strings.ToLower(item.ResourceType)] = &item
	}

	var platformResourceIDs []string
	for _, h := range res {
		findingEvent := api.GetAPIComplianceResultDriftEventFromESComplianceResultDriftEvent(h.Source)
		if rtMetadata, ok := resourceTypeMetadataMap[strings.ToLower(h.Source.ResourceType)]; ok {
			findingEvent.ResourceType = rtMetadata.ResourceLabel
		}
		if integration, ok := allIntegrationsMap[h.Source.IntegrationID]; ok {
			findingEvent.ProviderID = integration.ProviderID
			findingEvent.IntegrationName = integration.Name
		}
		findingEvent.SortKey = h.Sort
		platformResourceIDs = append(platformResourceIDs, h.Source.PlatformResourceID)
		response.ComplianceResultDriftEvents = append(response.ComplianceResultDriftEvents, findingEvent)
	}
	response.TotalCount = totalCount

	lookupResourcesMap, err := es.FetchLookupByResourceIDBatch(ctx, h.client, platformResourceIDs)
	if err != nil {
		h.logger.Error("failed to fetch lookup resources", zap.Error(err))
		return err
	}

	for i, findingEvent := range response.ComplianceResultDriftEvents {
		var lookupResource *es2.LookupResource
		potentialResources := lookupResourcesMap[findingEvent.PlatformResourceID]
		for _, r := range potentialResources {
			r := r
			if strings.ToLower(r.ResourceType) == strings.ToLower(findingEvent.ResourceType) {
				lookupResource = &r
				break
			}
		}

		if lookupResource != nil {
			response.ComplianceResultDriftEvents[i].ResourceName = lookupResource.ResourceName
		} else {
			h.logger.Warn("lookup resource not found",
				zap.String("og_resource_id", findingEvent.PlatformResourceID),
				zap.String("resource_id", findingEvent.ResourceID),
				zap.String("controlId", findingEvent.ControlID),
			)
		}
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// CountComplianceResultDriftEvents godoc
//
//	@Summary		Get finding events count
//	@Description	Retrieving all compliance run finding events count with respect to filters.
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			complianceStatus	query		[]api.ComplianceStatus	false	"ComplianceStatus to filter by defaults to all complianceStatus except passed"
//	@Param			benchmarkID			query		[]string				false	"BenchmarkID to filter by"
//	@Param			stateActive			query		[]bool					false	"StateActive to filter by defaults to all stateActives"
//	@Param			startTime			query		int64					false	"Start time to filter by"
//	@Param			endTime				query		int64					false	"End time to filter by"
//	@Success		200					{object}	api.CountComplianceResultDriftEventsResponse
//	@Router			/compliance/api/v1/compliance_result_events/count [get]
func (h *HttpHandler) CountComplianceResultDriftEvents(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	complianceStatuses := api.ParseComplianceStatuses(httpserver2.QueryArrayParam(echoCtx, "complianceStatus"))
	if len(complianceStatuses) == 0 {
		complianceStatuses = []api.ComplianceStatus{api.ComplianceStatusFailed}
	}

	esComplianceStatuses := make([]opengovernanceTypes.ComplianceStatus, 0, len(complianceStatuses))
	for _, status := range complianceStatuses {
		esComplianceStatuses = append(esComplianceStatuses, status.GetEsComplianceStatuses()...)
	}

	benchmarkIDs := httpserver2.QueryArrayParam(echoCtx, "benchmarkID")

	var stateActive []bool
	stateActiveStr := httpserver2.QueryArrayParam(echoCtx, "stateActive")
	for _, s := range stateActiveStr {
		sa, err := strconv.ParseBool(s)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid stateActive")
		}
		stateActive = append(stateActive, sa)
	}

	var endTime *time.Time
	if endTimeStr := echoCtx.QueryParam("endTime"); endTimeStr != "" {
		endTimeInt, err := strconv.ParseInt(endTimeStr, 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid endTime")
		}
		endTime = utils.GetPointer(time.Unix(endTimeInt, 0))
	}
	var startTime *time.Time
	if startTimeStr := echoCtx.QueryParam("startTime"); startTimeStr != "" {
		startTimeInt, err := strconv.ParseInt(startTimeStr, 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid startTime")
		}
		startTime = utils.GetPointer(time.Unix(startTimeInt, 0))
	}

	totalCount, err := es.ComplianceResultDriftEventsCount(ctx, h.client, benchmarkIDs, esComplianceStatuses, stateActive, startTime, endTime)
	if err != nil {
		return err
	}

	response := api.CountComplianceResultDriftEventsResponse{
		Count: totalCount,
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// ChangeBenchmarkSettings godoc
//
//	@Summary		change benchmark settings
//	@Description	Changes benchmark settings.
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			benchmark_id		path	string	false	"BenchmarkID"
//	@Param			tracksDriftEvents	query	bool	false	"tracksDriftEvents"
//	@Success		200
//	@Router			/compliance/api/v1/benchmarks/{benchmark_id}/settings [post]
func (h *HttpHandler) ChangeBenchmarkSettings(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	tracksDriftEvents := echoCtx.QueryParam("tracksDriftEvents") == "true"
	if len(echoCtx.QueryParam("tracksDriftEvents")) > 0 {
		benchmarkID := echoCtx.Param("benchmark_id")
		err := h.db.UpdateBenchmarkTrackDriftEvents(ctx, benchmarkID, tracksDriftEvents)
		if err != nil {
			return err
		}
	}

	return echoCtx.NoContent(http.StatusOK)
}

// GetComplianceResultDriftEventFilterValues godoc
//
//	@Summary		Get possible values for finding event filters
//	@Description	Retrieving possible values for finding event filters.
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			request	body		api.ComplianceResultDriftEventFilters	true	"Request Body"
//	@Success		200		{object}	api.ComplianceResultDriftEventFiltersWithMetadata
//	@Router			/compliance/api/v1/compliance_result_events/filters [post]
func (h *HttpHandler) GetComplianceResultDriftEventFilterValues(echoCtx echo.Context) error {
	var err error
	ctx := echoCtx.Request().Context()

	var req api.ComplianceResultDriftEventFilters
	if err := bindValidate(echoCtx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	req.IntegrationID, err = h.getIntegrationIdFilterFromInputs(ctx, req.IntegrationID, req.IntegrationGroup)
	if err != nil {
		return err
	}

	//req.IntegrationID, err = httpserver2.ResolveIntegrationIDs(echoCtx, req.IntegrationID)
	//if err != nil {
	//	return err
	//}

	if len(req.ComplianceStatus) == 0 {
		req.ComplianceStatus = []api.ComplianceStatus{api.ComplianceStatusFailed}
	}

	esComplianceStatuses := make([]opengovernanceTypes.ComplianceStatus, 0, len(req.ComplianceStatus))
	for _, status := range req.ComplianceStatus {
		esComplianceStatuses = append(esComplianceStatuses, status.GetEsComplianceStatuses()...)
	}

	var evaluatedAtFrom, evaluatedAtTo *time.Time
	if req.EvaluatedAt.From != nil && *req.EvaluatedAt.From != 0 {
		evaluatedAtFrom = utils.GetPointer(time.Unix(*req.EvaluatedAt.From, 0))
	}
	if req.EvaluatedAt.To != nil && *req.EvaluatedAt.To != 0 {
		evaluatedAtTo = utils.GetPointer(time.Unix(*req.EvaluatedAt.To, 0))
	}

	resourceTypeMetadata, err := h.inventoryClient.ListResourceTypesMetadata(&httpclient.Context{UserRole: authApi.AdminRole},
		nil, nil, nil, false, nil, 10000, 1)
	if err != nil {
		h.logger.Error("failed to get resource type metadata", zap.Error(err))
		return err
	}
	resourceTypeMetadataMap := make(map[string]*inventoryApi.ResourceType)
	for _, item := range resourceTypeMetadata.ResourceTypes {
		item := item
		resourceTypeMetadataMap[strings.ToLower(item.ResourceType)] = &item
	}

	resourceCollectionMetadata, err := h.inventoryClient.ListResourceCollections(&httpclient.Context{UserRole: authApi.AdminRole})
	if err != nil {
		h.logger.Error("failed to get resource collection metadata", zap.Error(err))
		return err
	}
	resourceCollectionMetadataMap := make(map[string]*inventoryApi.ResourceCollection)
	for _, item := range resourceCollectionMetadata {
		item := item
		resourceCollectionMetadataMap[item.ID] = &item
	}

	IntegrationMetadata, err := h.integrationClient.ListIntegrations(&httpclient.Context{UserRole: authApi.AdminRole}, nil)
	if err != nil {
		h.logger.Error("failed to get integrations", zap.Error(err))
		return err
	}
	integrationMetadataMap := make(map[string]*integrationapi.Integration)
	for _, item := range IntegrationMetadata.Integrations {
		item := item
		integrationMetadataMap[item.IntegrationID] = &item
	}

	benchmarkMetadata, err := h.db.ListBenchmarksBare(ctx)
	if err != nil {
		h.logger.Error("failed to get benchmarks", zap.Error(err))
		return err
	}
	benchmarkMetadataMap := make(map[string]*db.Benchmark)
	for _, item := range benchmarkMetadata {
		item := item
		benchmarkMetadataMap[item.ID] = &item
	}

	controlMetadata, err := h.db.ListControlsBare(ctx)
	if err != nil {
		h.logger.Error("failed to get controls", zap.Error(err))
		return err
	}
	controlMetadataMap := make(map[string]*db.Control)
	for _, item := range controlMetadata {
		item := item
		controlMetadataMap[item.ID] = &item
	}

	possibleFilters, err := es.ComplianceResultDriftEventsFiltersQuery(ctx, h.logger, h.client,
		req.ComplianceResultID, req.PlatformResourceID, req.IntegrationType, req.IntegrationID, req.NotIntegrationID,
		req.ResourceType,
		req.BenchmarkID, req.ControlID,
		req.Severity,
		evaluatedAtFrom, evaluatedAtTo,
		req.StateActive, esComplianceStatuses)
	if err != nil {
		h.logger.Error("failed to get possible filters", zap.Error(err))
		return err
	}
	response := api.ComplianceResultFiltersWithMetadata{}
	for _, item := range possibleFilters.Aggregations.BenchmarkIDFilter.Buckets {
		if benchmark, ok := benchmarkMetadataMap[item.Key]; ok {
			response.BenchmarkID = append(response.BenchmarkID, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: benchmark.Title,
				Count:       utils.GetPointer(item.DocCount),
			})
		} else {
			response.BenchmarkID = append(response.BenchmarkID, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: item.Key,
				Count:       utils.GetPointer(item.DocCount),
			})
		}
	}
	for _, item := range possibleFilters.Aggregations.ControlIDFilter.Buckets {
		if control, ok := controlMetadataMap[item.Key]; ok {
			response.ControlID = append(response.ControlID, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: control.Title,
				Count:       utils.GetPointer(item.DocCount),
			})
		} else {
			response.ControlID = append(response.ControlID, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: item.Key,
				Count:       utils.GetPointer(item.DocCount),
			})
		}
	}
	if len(possibleFilters.Aggregations.IntegrationTypeFilter.Buckets) > 0 {
		for _, bucket := range possibleFilters.Aggregations.IntegrationTypeFilter.Buckets {
			integrationType := integration_type.ParseType(bucket.Key)
			response.IntegrationType = append(response.IntegrationType, api.FilterWithMetadata{
				Key:         integrationType.String(),
				DisplayName: integrationType.String(),
				Count:       utils.GetPointer(bucket.DocCount),
			})
		}
	}
	for _, item := range possibleFilters.Aggregations.ResourceTypeFilter.Buckets {
		if rtMetadata, ok := resourceTypeMetadataMap[strings.ToLower(item.Key)]; ok {
			response.ResourceTypeID = append(response.ResourceTypeID, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: rtMetadata.ResourceLabel,
				Count:       utils.GetPointer(item.DocCount),
			})
		} else if item.Key == "" {
			response.ResourceTypeID = append(response.ResourceTypeID, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: "Unknown",
			})
		} else {
			response.ResourceTypeID = append(response.ResourceTypeID, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: item.Key,
				Count:       utils.GetPointer(item.DocCount),
			})
		}
	}

	for _, item := range possibleFilters.Aggregations.IntegrationIDFilter.Buckets {
		if integration, ok := integrationMetadataMap[item.Key]; ok {
			response.IntegrationID = append(response.IntegrationID, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: integration.Name,
				Count:       utils.GetPointer(item.DocCount),
			})
		} else {
			response.IntegrationID = append(response.IntegrationID, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: item.Key,
				Count:       utils.GetPointer(item.DocCount),
			})
		}
	}

	for _, item := range possibleFilters.Aggregations.ResourceCollectionFilter.Buckets {
		if resourceCollection, ok := resourceCollectionMetadataMap[item.Key]; ok {
			response.ResourceCollection = append(response.ResourceCollection, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: resourceCollection.Name,
				Count:       utils.GetPointer(item.DocCount),
			})
		} else {
			response.ResourceCollection = append(response.ResourceCollection, api.FilterWithMetadata{
				Key:         item.Key,
				DisplayName: item.Key,
				Count:       utils.GetPointer(item.DocCount),
			})
		}
	}

	for _, item := range possibleFilters.Aggregations.SeverityFilter.Buckets {
		response.Severity = append(response.Severity, api.FilterWithMetadata{
			Key:         item.Key,
			DisplayName: item.Key,
			Count:       utils.GetPointer(item.DocCount),
		})
	}

	for _, item := range possibleFilters.Aggregations.StateActiveFilter.Buckets {
		response.StateActive = append(response.StateActive, api.FilterWithMetadata{
			Key:         item.KeyAsString,
			DisplayName: item.KeyAsString,
			Count:       utils.GetPointer(item.DocCount),
		})
	}

	apiComplianceStatuses := make(map[api.ComplianceStatus]int)
	for _, item := range possibleFilters.Aggregations.ComplianceStatusFilter.Buckets {
		if opengovernanceTypes.ParseComplianceStatus(item.Key).IsPassed() {
			apiComplianceStatuses[api.ComplianceStatusPassed] += item.DocCount
		} else {
			apiComplianceStatuses[api.ComplianceStatusFailed] += item.DocCount
		}
	}
	for status, count := range apiComplianceStatuses {
		count := count
		response.ComplianceStatus = append(response.ComplianceStatus, api.FilterWithMetadata{
			Key:         string(status),
			DisplayName: string(status),
			Count:       &count,
		})
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// GetSingleComplianceResultDriftEvent
//
//	@Summary		Get single finding event
//	@Description	Retrieving single finding event
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			findingID	path		string	true	"ComplianceResultID"
//	@Success		200			{object}	api.ComplianceResultDriftEvent
//	@Router			/compliance/api/v1/compliance_result_events/single/{id} [get]
func (h *HttpHandler) GetSingleComplianceResultDriftEvent(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	findingEventID := echoCtx.Param("id")

	findingEvent, err := es.FetchComplianceResultDriftEventByID(ctx, h.logger, h.client, findingEventID)
	if err != nil {
		h.logger.Error("failed to fetch findingEvent by id", zap.Error(err))
		return err
	}
	if findingEvent == nil {
		return echo.NewHTTPError(http.StatusNotFound, "findingEvent not found")
	}

	apiFindingEvent := api.GetAPIComplianceResultDriftEventFromESComplianceResultDriftEvent(*findingEvent)

	integration, err := h.integrationClient.GetIntegration(&httpclient.Context{UserRole: authApi.AdminRole}, findingEvent.IntegrationID)
	if err != nil {
		h.logger.Error("failed to get integration", zap.Error(err), zap.String("integration_id", findingEvent.IntegrationID))
		return err
	}
	apiFindingEvent.ProviderID = integration.ProviderID
	apiFindingEvent.IntegrationName = integration.Name

	if len(findingEvent.ResourceType) > 0 {
		resourceTypeMetadata, err := h.inventoryClient.ListResourceTypesMetadata(&httpclient.Context{UserRole: authApi.AdminRole},
			nil, nil,
			[]string{findingEvent.ResourceType}, false, nil, 10000, 1)
		if err != nil {
			h.logger.Error("failed to get resource type metadata", zap.Error(err))
			return err
		}
		if len(resourceTypeMetadata.ResourceTypes) > 0 {
			apiFindingEvent.ResourceTypeName = resourceTypeMetadata.ResourceTypes[0].ResourceLabel
		}
	}

	return echoCtx.JSON(http.StatusOK, apiFindingEvent)
}

// ListResourceFindings godoc
//
//	@Summary		List resource findings
//	@Description	Retrieving list of resource findings
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			request	body		api.ListResourceFindingsRequest	true	"Request"
//	@Success		200		{object}	api.ListResourceFindingsResponse
//	@Router			/compliance/api/v1/resource_findings [post]
func (h *HttpHandler) ListResourceFindings(echoCtx echo.Context) error {
	clientCtx := &httpclient.Context{UserRole: authApi.AdminRole}

	var err error
	ctx := echoCtx.Request().Context()

	var req api.ListResourceFindingsRequest
	if err := bindValidate(echoCtx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	integrationGroupId := req.Filters.IntegrationGroup
	if len(req.Filters.IntegrationID) == 0 && len(req.Filters.IntegrationGroup) == 0 {
		integrationGroupId = []string{"active"}
	}

	req.Filters.IntegrationID, err = h.getIntegrationIdFilterFromInputs(ctx, req.Filters.IntegrationID, integrationGroupId)
	if err != nil {
		return err
	}

	//req.Filters.IntegrationID, err = httpserver2.ResolveIntegrationIDs(echoCtx, req.Filters.IntegrationID)
	//if err != nil {
	//	return err
	//}

	if len(req.AfterSortKey) != 0 {
		expectedLen := len(req.Sort) + 1
		if len(req.AfterSortKey) != expectedLen {
			return echo.NewHTTPError(http.StatusBadRequest, "sort key length should be zero or match a returned sort key from previous response")
		}
	}

	var evaluatedAtFrom, evaluatedAtTo *time.Time
	if req.Filters.Interval != nil && *req.Filters.Interval != "" {
		evaluatedAtFrom, evaluatedAtTo, err = parseTimeInterval(*req.Filters.Interval)
	} else {
		if req.Filters.EvaluatedAt.From != nil && *req.Filters.EvaluatedAt.From != 0 {
			evaluatedAtFrom = utils.GetPointer(time.Unix(*req.Filters.EvaluatedAt.From, 0))
		}
		if req.Filters.EvaluatedAt.To != nil && *req.Filters.EvaluatedAt.To != 0 {
			evaluatedAtTo = utils.GetPointer(time.Unix(*req.Filters.EvaluatedAt.To, 0))
		}
	}

	integrations, err := h.integrationClient.ListIntegrations(&httpclient.Context{UserRole: authApi.AdminRole}, nil)
	if err != nil {
		h.logger.Error("failed to get integrations", zap.Error(err))
		return err
	}
	integrationMap := make(map[string]*integrationapi.Integration)
	for _, integration := range integrations.Integrations {
		integration := integration
		integrationMap[integration.IntegrationID] = &integration
	}

	resourceTypes, err := h.inventoryClient.ListResourceTypesMetadata(&httpclient.Context{UserRole: authApi.AdminRole}, nil, nil, nil, false, nil, 10000, 1)
	if err != nil {
		h.logger.Error("failed to get resource types metadata", zap.Error(err))
		return err
	}
	resourceTypeMap := make(map[string]*inventoryApi.ResourceType)
	for _, rt := range resourceTypes.ResourceTypes {
		rt := rt
		resourceTypeMap[strings.ToLower(rt.ResourceType)] = &rt
	}

	if len(req.Filters.ComplianceStatus) == 0 {
		req.Filters.ComplianceStatus = []api.ComplianceStatus{
			api.ComplianceStatusFailed,
		}
	}

	esComplianceStatuses := make([]opengovernanceTypes.ComplianceStatus, 0, len(req.Filters.ComplianceStatus))
	for _, status := range req.Filters.ComplianceStatus {
		esComplianceStatuses = append(esComplianceStatuses, status.GetEsComplianceStatuses()...)
	}

	summaryJobs, err := h.schedulerClient.GetSummaryJobs(clientCtx, req.Filters.ComplianceJobId)
	if err != nil {
		h.logger.Error("could not get Summary Job IDs", zap.Error(err))
		return echoCtx.JSON(http.StatusInternalServerError, "could not get Summary Job IDs")
	}

	resourceFindings, totalCount, err := es.ResourceFindingsQuery(ctx, h.logger, h.client, req.Filters.IntegrationType, req.Filters.IntegrationID,
		req.Filters.NotIntegrationID, req.Filters.ResourceCollection, req.Filters.ResourceTypeID, req.Filters.BenchmarkID,
		req.Filters.ControlID, req.Filters.Severity, evaluatedAtFrom, evaluatedAtTo, esComplianceStatuses, req.Sort, req.Limit, req.AfterSortKey, summaryJobs)
	if err != nil {
		h.logger.Error("failed to get resource findings", zap.Error(err))
		return err
	}

	response := api.ListResourceFindingsResponse{
		TotalCount:       int(totalCount),
		ResourceFindings: nil,
	}

	for _, resourceFinding := range resourceFindings {
		apiRf := api.GetAPIResourceFinding(resourceFinding.Source)
		if integration, ok := integrationMap[apiRf.IntegrationID]; ok {
			apiRf.ProviderID = integration.IntegrationID
			apiRf.IntegrationName = integration.Name
		}
		if resourceType, ok := resourceTypeMap[strings.ToLower(apiRf.ResourceType)]; ok {
			apiRf.ResourceTypeLabel = resourceType.ResourceLabel
		}
		apiRf.SortKey = resourceFinding.Sort
		response.ResourceFindings = append(response.ResourceFindings, apiRf)
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// GetControlRemediation godoc
//
//	@Summary	Get control remediation using AI
//	@Security	BearerToken
//	@Tags		compliance
//	@Accept		json
//	@Produce	json
//	@Param		controlID	path		string	true	"ControlID"
//	@Success	200			{object}	api.BenchmarkRemediation
//	@Router		/compliance/api/v1/ai/control/{controlID}/remediation [post]
func (h *HttpHandler) GetControlRemediation(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	controlID := echoCtx.Param("controlID")

	control, err := h.db.GetControl(ctx, controlID)
	if err != nil {
		return err
	}

	req := openai.ChatCompletionRequest{
		Model: openai.GPT3Dot5Turbo,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: "You will be provided with a problem on AWS, and your task is to create a numbered list of how to fix it using AWS console.",
			},
		},
	}

	req.Messages = append(req.Messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: control.Title,
	})

	resp, err := h.openAIClient.CreateChatCompletion(ctx, req)
	if err != nil {
		return err
	}

	return echoCtx.JSON(http.StatusOK, api.BenchmarkRemediation{Remediation: resp.Choices[0].Message.Content})
}

func addToControlSeverityResult(controlSeverityResult api.BenchmarkControlsSeverityStatus, control *db.Control, controlResult types.ControlResult) api.BenchmarkControlsSeverityStatus {
	if control == nil {
		control = &db.Control{
			Severity: opengovernanceTypes.ComplianceResultSeverityNone,
		}
	}
	switch control.Severity {
	case opengovernanceTypes.ComplianceResultSeverityCritical:
		controlSeverityResult.Total.TotalCount++
		controlSeverityResult.Critical.TotalCount++
		if controlResult.Passed {
			controlSeverityResult.Total.PassedCount++
			controlSeverityResult.Critical.PassedCount++
		}
	case opengovernanceTypes.ComplianceResultSeverityHigh:
		controlSeverityResult.Total.TotalCount++
		controlSeverityResult.High.TotalCount++
		if controlResult.Passed {
			controlSeverityResult.Total.PassedCount++
			controlSeverityResult.High.PassedCount++
		}
	case opengovernanceTypes.ComplianceResultSeverityMedium:
		controlSeverityResult.Total.TotalCount++
		controlSeverityResult.Medium.TotalCount++
		if controlResult.Passed {
			controlSeverityResult.Total.PassedCount++
			controlSeverityResult.Medium.PassedCount++
		}
	case opengovernanceTypes.ComplianceResultSeverityLow:
		controlSeverityResult.Total.TotalCount++
		controlSeverityResult.Low.TotalCount++
		if controlResult.Passed {
			controlSeverityResult.Total.PassedCount++
			controlSeverityResult.Low.PassedCount++
		}
	case opengovernanceTypes.ComplianceResultSeverityNone, "":
		controlSeverityResult.Total.TotalCount++
		controlSeverityResult.None.TotalCount++
		if controlResult.Passed {
			controlSeverityResult.Total.PassedCount++
			controlSeverityResult.None.PassedCount++
		}
	}
	return controlSeverityResult
}

func addToControlSeverityResultV2(controlSeverityResult api.BenchmarkControlsSeverityStatusV2, control *db.Control, controlResult types.ControlResult) api.BenchmarkControlsSeverityStatusV2 {
	if control == nil {
		control = &db.Control{
			Severity: opengovernanceTypes.ComplianceResultSeverityNone,
		}
	}
	switch control.Severity {
	case opengovernanceTypes.ComplianceResultSeverityCritical:
		controlSeverityResult.Total.TotalCount++
		controlSeverityResult.Critical.TotalCount++
		if controlResult.Passed {
			controlSeverityResult.Total.PassedCount++
			controlSeverityResult.Critical.PassedCount++
		} else {
			controlSeverityResult.Total.FailedCount++
			controlSeverityResult.Critical.FailedCount++
		}
	case opengovernanceTypes.ComplianceResultSeverityHigh:
		controlSeverityResult.Total.TotalCount++
		controlSeverityResult.High.TotalCount++
		if controlResult.Passed {
			controlSeverityResult.Total.PassedCount++
			controlSeverityResult.High.PassedCount++
		} else {
			controlSeverityResult.Total.FailedCount++
			controlSeverityResult.High.FailedCount++
		}
	case opengovernanceTypes.ComplianceResultSeverityMedium:
		controlSeverityResult.Total.TotalCount++
		controlSeverityResult.Medium.TotalCount++
		if controlResult.Passed {
			controlSeverityResult.Total.PassedCount++
			controlSeverityResult.Medium.PassedCount++
		} else {
			controlSeverityResult.Total.FailedCount++
			controlSeverityResult.Medium.FailedCount++
		}
	case opengovernanceTypes.ComplianceResultSeverityLow:
		controlSeverityResult.Total.TotalCount++
		controlSeverityResult.Low.TotalCount++
		if controlResult.Passed {
			controlSeverityResult.Total.PassedCount++
			controlSeverityResult.Low.PassedCount++
		} else {
			controlSeverityResult.Total.FailedCount++
			controlSeverityResult.Low.FailedCount++
		}
	case opengovernanceTypes.ComplianceResultSeverityNone, "":
		controlSeverityResult.Total.TotalCount++
		controlSeverityResult.None.TotalCount++
		if controlResult.Passed {
			controlSeverityResult.Total.PassedCount++
			controlSeverityResult.None.PassedCount++
		} else {
			controlSeverityResult.Total.FailedCount++
			controlSeverityResult.None.FailedCount++
		}
	}
	return controlSeverityResult
}

// ListBenchmarksSummary godoc
//
//	@Summary		List benchmarks summaries
//	@Description	Retrieving a summary of all benchmarks and their associated checks and results within a specified time interval.
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			integrationId		query		[]string		false	"Integration IDs to filter by"
//	@Param			integrationGroup		query		[]string		false	"Integration groups to filter by "
//	@Param			resourceCollection	query		[]string		false	"Resource collection IDs to filter by"
//	@Param			integrationTypes	query		[]integration.Type	false	"Integration type to filter by"
//	@Param			tag					query		[]string		false	"Key-Value tags in key=value format to filter by"
//	@Param			timeAt				query		int				false	"timestamp for values in epoch seconds"
//	@Param			topAccountCount		query		int				false	"Top account count"	default(3)
//	@Success		200					{object}	api.ListBenchmarksSummaryResponse
//	@Router			/compliance/api/v1/benchmarks/summary [get]
func (h *HttpHandler) ListBenchmarksSummary(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	tagMap := model.TagStringsToTagMap(httpserver2.QueryArrayParam(echoCtx, "tag"))
	integrationIDs, err := h.getIntegrationIdFilterFromParams(echoCtx)
	if err != nil {
		return err
	}
	if len(integrationIDs) > 20 {
		return echo.NewHTTPError(http.StatusBadRequest, "too many integration IDs")
	}

	integrationTypes := httpserver2.QueryArrayParam(echoCtx, "integrationTypes")
	resourceCollections := httpserver2.QueryArrayParam(echoCtx, "resourceCollection")
	timeAt := time.Now()
	if timeAtStr := echoCtx.QueryParam("timeAt"); timeAtStr != "" {
		timeAtInt, err := strconv.ParseInt(timeAtStr, 10, 64)
		if err != nil {
			return err
		}
		timeAt = time.Unix(timeAtInt, 0)
	}
	topAccountCount := 3
	if topAccountCountStr := echoCtx.QueryParam("topAccountCount"); topAccountCountStr != "" {
		count, err := strconv.ParseInt(topAccountCountStr, 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid topAccountCount")
		}
		topAccountCount = int(count)
	}

	var response api.ListBenchmarksSummaryResponse

	// tracer :
	ctx, span2 := tracer.Start(ctx, "new_ListRootBenchmarks", trace.WithSpanKind(trace.SpanKindServer))
	span2.SetName("new_ListRootBenchmarks")
	defer span2.End()

	benchmarks, err := h.db.ListRootBenchmarks(ctx, tagMap)
	if err != nil {
		span2.RecordError(err)
		span2.SetStatus(codes.Error, err.Error())
		return err
	}
	span2.End()

	controls, err := h.db.ListControlsBare(ctx)
	if err != nil {
		h.logger.Error("failed to get controls", zap.Error(err))
		return err
	}
	controlsMap := make(map[string]*db.Control)
	for _, control := range controls {
		control := control
		controlsMap[strings.ToLower(control.ID)] = &control
	}

	benchmarkIDs := make([]string, 0, len(benchmarks))
	for _, b := range benchmarks {
		benchmarkIDs = append(benchmarkIDs, b.ID)
	}

	summariesAtTime, err := es.ListBenchmarkSummariesAtTime(ctx, h.logger, h.client, benchmarkIDs, integrationIDs, resourceCollections, timeAt, false)
	if err != nil {
		h.logger.Error("failed to fetch benchmark summaries", zap.Error(err))
		return err
	}
	// tracer :
	ctx, span3 := tracer.Start(ctx, "new_PopulateIntegrationTypes(loop)", trace.WithSpanKind(trace.SpanKindServer))
	span3.SetName("new_PopulateIntegrationTypes(loop)")
	defer span3.End()

	passedResourcesResult, err := es.GetPerBenchmarkResourceSeverityResult(ctx, h.logger, h.client, benchmarkIDs, integrationIDs, resourceCollections, nil, opengovernanceTypes.GetPassedComplianceStatuses())
	if err != nil {
		h.logger.Error("failed to fetch per benchmark resource severity result for passed", zap.Error(err))
		return err
	}

	allResourcesResult, err := es.GetPerBenchmarkResourceSeverityResult(ctx, h.logger, h.client, benchmarkIDs, integrationIDs, resourceCollections, nil, nil)
	if err != nil {
		h.logger.Error("failed to fetch per benchmark resource severity result for all", zap.Error(err))
		return err
	}

	for _, b := range benchmarks {
		be := b.ToApi()
		if len(integrationTypes) > 0 && !utils.IncludesAny(be.IntegrationTypes, integrationTypes) {
			continue
		}

		summaryAtTime := summariesAtTime[b.ID]
		csResult := api.ComplianceStatusSummary{}
		sResult := opengovernanceTypes.SeverityResult{}
		controlSeverityResult := api.BenchmarkControlsSeverityStatus{}
		var costImpact *float64
		addToResults := func(resultGroup types.ResultGroup) {
			csResult.AddESComplianceStatusMap(resultGroup.Result.QueryResult)
			sResult.AddResultMap(resultGroup.Result.SeverityResult)
			costImpact = utils.PAdd(costImpact, resultGroup.Result.CostImpact)
			for controlId, controlResult := range resultGroup.Controls {
				control := controlsMap[strings.ToLower(controlId)]
				controlSeverityResult = addToControlSeverityResult(controlSeverityResult, control, controlResult)
			}
		}
		if len(resourceCollections) > 0 {
			for _, resourceCollection := range resourceCollections {
				if len(integrationIDs) > 0 {
					for _, integrationID := range integrationIDs {
						addToResults(summaryAtTime.ResourceCollections[resourceCollection].Integrations[integrationID])
					}
				} else {
					addToResults(summaryAtTime.ResourceCollections[resourceCollection].BenchmarkResult)
				}
			}
		} else if len(integrationIDs) > 0 {
			for _, integrationID := range integrationIDs {
				addToResults(summaryAtTime.Integrations.Integrations[integrationID])
			}
		} else {
			addToResults(summaryAtTime.Integrations.BenchmarkResult)
		}

		topIntegrations := make([]api.TopFieldRecord, 0, topAccountCount)
		if topAccountCount > 0 && (csResult.FailedCount+csResult.PassedCount) > 0 {
			topFieldResponse, err := es.ComplianceResultsTopFieldQuery(ctx, h.logger, h.client, "integrationID", integrationTypes,
				nil, integrationIDs, nil, nil, []string{b.ID}, nil, nil,
				opengovernanceTypes.GetFailedComplianceStatuses(), []bool{true}, topAccountCount, nil, nil)
			if err != nil {
				h.logger.Error("failed to fetch complianceResults top field", zap.Error(err))
				return err
			}
			topFieldTotalResponse, err := es.ComplianceResultsTopFieldQuery(ctx, h.logger, h.client, "integrationID", integrationTypes, nil,
				integrationIDs, nil, nil, []string{b.ID}, nil, nil, opengovernanceTypes.GetComplianceStatuses(),
				[]bool{true}, topAccountCount, nil, nil)
			if err != nil {
				h.logger.Error("failed to fetch complianceResults top field total", zap.Error(err))
				return err
			}
			totalCountMap := make(map[string]int)
			for _, item := range topFieldTotalResponse.Aggregations.FieldFilter.Buckets {
				totalCountMap[item.Key] += item.DocCount
			}

			resIntegrationIDs := make([]string, 0, len(topFieldResponse.Aggregations.FieldFilter.Buckets))
			for _, item := range topFieldResponse.Aggregations.FieldFilter.Buckets {
				resIntegrationIDs = append(resIntegrationIDs, item.Key)
			}
			if len(resIntegrationIDs) > 0 {
				integrations, err := h.integrationClient.ListIntegrationsByFilters(&httpclient.Context{UserRole: authApi.AdminRole}, integrationapi.ListIntegrationsRequest{
					IntegrationID: resIntegrationIDs,
				})
				if err != nil {
					h.logger.Error("failed to get integrations", zap.Error(err))
					return err
				}
				integrationMap := make(map[string]*integrationapi.Integration)
				for _, integration := range integrations.Integrations {
					integration := integration
					integrationMap[integration.IntegrationID] = &integration
				}

				for _, item := range topFieldResponse.Aggregations.FieldFilter.Buckets {
					topIntegrations = append(topIntegrations, api.TopFieldRecord{
						Integration: integrationMap[item.Key],
						Count:       item.DocCount,
						TotalCount:  totalCountMap[item.Key],
					})
				}
			}
		}

		resourcesSeverityResult := api.BenchmarkResourcesSeverityStatus{}
		allResources := allResourcesResult[b.ID]
		resourcesSeverityResult.Total.TotalCount = allResources.TotalCount
		resourcesSeverityResult.Critical.TotalCount = allResources.CriticalCount
		resourcesSeverityResult.High.TotalCount = allResources.HighCount
		resourcesSeverityResult.Medium.TotalCount = allResources.MediumCount
		resourcesSeverityResult.Low.TotalCount = allResources.LowCount
		resourcesSeverityResult.None.TotalCount = allResources.NoneCount
		passedResource := passedResourcesResult[b.ID]
		resourcesSeverityResult.Total.PassedCount = passedResource.TotalCount
		resourcesSeverityResult.Critical.PassedCount = passedResource.CriticalCount
		resourcesSeverityResult.High.PassedCount = passedResource.HighCount
		resourcesSeverityResult.Medium.PassedCount = passedResource.MediumCount
		resourcesSeverityResult.Low.PassedCount = passedResource.LowCount
		resourcesSeverityResult.None.PassedCount = passedResource.NoneCount

		response.BenchmarkSummary = append(response.BenchmarkSummary, api.BenchmarkEvaluationSummary{
			Benchmark:               be,
			ComplianceStatusSummary: csResult,
			Checks:                  sResult,
			ControlsSeverityStatus:  controlSeverityResult,
			ResourcesSeverityStatus: resourcesSeverityResult,
			CostImpact:              costImpact,
			EvaluatedAt:             utils.GetPointer(time.Unix(summaryAtTime.EvaluatedAtEpoch, 0)),
			LastJobStatus:           "",
			TopIntegrations:         topIntegrations,
		})
	}
	span3.End()
	return echoCtx.JSON(http.StatusOK, response)
}

// GetBenchmarkSummary godoc
//
//	@Summary		Get benchmark summary
//	@Description	Retrieving a summary of a benchmark and its associated checks and results.
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			benchmark_id		path		string			true	"Benchmark ID"
//	@Param			integrationId		query		[]string		false	"integration IDs to filter by"
//	@Param			integrationGroup		query		[]string		false	"integration groups to filter by "
//	@Param			resourceCollection	query		[]string		false	"Resource collection IDs to filter by"
//	@Param			integrationTypes	query		[]integration.Type	false	"Integration type to filter by"
//	@Param			timeAt				query		int				false	"timestamp for values in epoch seconds"
//	@Param			topAccountCount		query		int				false	"Top account count"	default(3)
//	@Success		200					{object}	api.BenchmarkEvaluationSummary
//	@Router			/compliance/api/v1/benchmarks/{benchmark_id}/summary [get]
func (h *HttpHandler) GetBenchmarkSummary(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	integrationIDs, err := h.getIntegrationIdFilterFromParams(echoCtx)
	if err != nil {
		return err
	}
	if len(integrationIDs) > 20 {
		return echo.NewHTTPError(http.StatusBadRequest, "too many integration IDs")
	}
	topAccountCount := 3
	if topAccountCountStr := echoCtx.QueryParam("topAccountCount"); topAccountCountStr != "" {
		count, err := strconv.ParseInt(topAccountCountStr, 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid topAccountCount")
		}
		topAccountCount = int(count)
	}

	integrationTypes := httpserver2.QueryArrayParam(echoCtx, "integrationTypes")
	resourceCollections := httpserver2.QueryArrayParam(echoCtx, "resourceCollection")
	timeAt := time.Now()
	if timeAtStr := echoCtx.QueryParam("timeAt"); timeAtStr != "" {
		timeAtInt, err := strconv.ParseInt(timeAtStr, 10, 64)
		if err != nil {
			return err
		}
		timeAt = time.Unix(timeAtInt, 0)
	}
	benchmarkID := echoCtx.Param("benchmark_id")
	// tracer :
	ctx, span1 := tracer.Start(ctx, "new_GetBenchmark", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_GetBenchmark")
	defer span1.End()

	benchmark, err := h.db.GetBenchmark(ctx, benchmarkID)
	if err != nil {
		span1.RecordError(err)
		span1.SetStatus(codes.Error, err.Error())
		return err
	}
	if benchmark == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid benchmarkID")
	}
	span1.AddEvent("information", trace.WithAttributes(
		attribute.String("benchmark ID", benchmark.ID),
	))
	span1.End()

	be := benchmark.ToApi()

	if len(integrationTypes) > 0 && !utils.IncludesAny(be.IntegrationTypes, integrationTypes) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid integration type")
	}

	controls, err := h.db.ListControlsByBenchmarkID(ctx, benchmarkID)
	if err != nil {
		h.logger.Error("failed to get controls", zap.Error(err))
		return err
	}
	controlsMap := make(map[string]*db.Control)
	for _, control := range controls {
		control := control
		controlsMap[strings.ToLower(control.ID)] = &control
	}

	summariesAtTime, err := es.ListBenchmarkSummariesAtTime(ctx, h.logger, h.client,
		[]string{benchmarkID}, integrationIDs, resourceCollections,
		timeAt, true)
	if err != nil {
		return err
	}

	passedResourcesResult, err := es.GetPerBenchmarkResourceSeverityResult(ctx, h.logger, h.client, []string{benchmarkID}, integrationIDs, resourceCollections, nil, opengovernanceTypes.GetPassedComplianceStatuses())
	if err != nil {
		h.logger.Error("failed to fetch per benchmark resource severity result for passed", zap.Error(err))
		return err
	}

	allResourcesResult, err := es.GetPerBenchmarkResourceSeverityResult(ctx, h.logger, h.client, []string{benchmarkID}, integrationIDs, resourceCollections, nil, nil)
	if err != nil {
		h.logger.Error("failed to fetch per benchmark resource severity result for all", zap.Error(err))
		return err
	}

	summaryAtTime := summariesAtTime[benchmarkID]

	csResult := api.ComplianceStatusSummary{}
	sResult := opengovernanceTypes.SeverityResult{}
	controlSeverityResult := api.BenchmarkControlsSeverityStatus{}
	integrationsResult := api.BenchmarkStatusResult{}
	var costImpact *float64
	addToResults := func(resultGroup types.ResultGroup) {
		csResult.AddESComplianceStatusMap(resultGroup.Result.QueryResult)
		sResult.AddResultMap(resultGroup.Result.SeverityResult)
		costImpact = utils.PAdd(costImpact, resultGroup.Result.CostImpact)
		for controlId, controlResult := range resultGroup.Controls {
			control := controlsMap[strings.ToLower(controlId)]
			controlSeverityResult = addToControlSeverityResult(controlSeverityResult, control, controlResult)
		}
	}
	if len(resourceCollections) > 0 {
		for _, resourceCollection := range resourceCollections {
			if len(integrationIDs) > 0 {
				for _, integrationID := range integrationIDs {
					addToResults(summaryAtTime.ResourceCollections[resourceCollection].Integrations[integrationID])
					integrationsResult.TotalCount++
					if summaryAtTime.ResourceCollections[resourceCollection].Integrations[integrationID].Result.IsFullyPassed() {
						integrationsResult.PassedCount++
					}
				}
			} else {
				addToResults(summaryAtTime.ResourceCollections[resourceCollection].BenchmarkResult)
				for _, integrationResult := range summaryAtTime.ResourceCollections[resourceCollection].Integrations {
					integrationsResult.TotalCount++
					if integrationResult.Result.IsFullyPassed() {
						integrationsResult.PassedCount++
					}
				}
			}
		}
	} else if len(integrationIDs) > 0 {
		for _, integrationID := range integrationIDs {
			addToResults(summaryAtTime.Integrations.Integrations[integrationID])
			integrationsResult.TotalCount++
			if summaryAtTime.Integrations.Integrations[integrationID].Result.IsFullyPassed() {
				integrationsResult.PassedCount++
			}
		}
	} else {
		addToResults(summaryAtTime.Integrations.BenchmarkResult)
		for _, integrationResult := range summaryAtTime.Integrations.Integrations {
			integrationsResult.TotalCount++
			if integrationResult.Result.IsFullyPassed() {
				integrationsResult.PassedCount++
			}
		}
	}

	lastJob, err := h.schedulerClient.GetLatestComplianceJobForBenchmark(&httpclient.Context{UserRole: authApi.AdminRole}, benchmarkID)
	if err != nil {
		h.logger.Error("failed to get latest compliance job for benchmark", zap.Error(err), zap.String("benchmarkID", benchmarkID))
		return err
	}

	var lastJobStatus string
	if lastJob != nil {
		lastJobStatus = string(lastJob.Status)
	}

	topIntegrations := make([]api.TopFieldRecord, 0, topAccountCount)
	if topAccountCount > 0 {
		res, err := es.ComplianceResultsTopFieldQuery(ctx, h.logger, h.client, "integrationID", integrationTypes,
			nil, integrationIDs, nil, nil, []string{benchmark.ID}, nil, nil,
			opengovernanceTypes.GetFailedComplianceStatuses(), []bool{true}, topAccountCount, nil, nil)
		if err != nil {
			h.logger.Error("failed to fetch complianceResults top field", zap.Error(err))
			return err
		}

		topFieldTotalResponse, err := es.ComplianceResultsTopFieldQuery(ctx, h.logger, h.client, "integrationID", integrationTypes,
			nil, integrationIDs, nil, nil, []string{benchmark.ID}, nil, nil,
			opengovernanceTypes.GetFailedComplianceStatuses(), []bool{true}, topAccountCount, nil, nil)
		if err != nil {
			h.logger.Error("failed to fetch complianceResults top field total", zap.Error(err))
			return err
		}
		totalCountMap := make(map[string]int)
		for _, item := range topFieldTotalResponse.Aggregations.FieldFilter.Buckets {
			totalCountMap[item.Key] += item.DocCount
		}

		resIntegrationIDs := make([]string, 0, len(res.Aggregations.FieldFilter.Buckets))
		for _, item := range res.Aggregations.FieldFilter.Buckets {
			resIntegrationIDs = append(resIntegrationIDs, item.Key)
		}
		if len(resIntegrationIDs) > 0 {
			integrations, err := h.integrationClient.ListIntegrationsByFilters(&httpclient.Context{UserRole: authApi.AdminRole}, integrationapi.ListIntegrationsRequest{
				IntegrationID: resIntegrationIDs,
			})
			if err != nil {
				h.logger.Error("failed to get integrations", zap.Error(err))
				return err
			}
			integrationMap := make(map[string]*integrationapi.Integration)
			for _, integration := range integrations.Integrations {
				integration := integration
				integrationMap[integration.IntegrationID] = &integration
			}

			for _, item := range res.Aggregations.FieldFilter.Buckets {
				topIntegrations = append(topIntegrations, api.TopFieldRecord{
					Integration: integrationMap[item.Key],
					Count:       item.DocCount,
					TotalCount:  totalCountMap[item.Key],
				})
			}
		}
	}

	resourcesSeverityResult := api.BenchmarkResourcesSeverityStatus{}
	allResources := allResourcesResult[benchmarkID]
	resourcesSeverityResult.Total.TotalCount = allResources.TotalCount
	resourcesSeverityResult.Critical.TotalCount = allResources.CriticalCount
	resourcesSeverityResult.High.TotalCount = allResources.HighCount
	resourcesSeverityResult.Medium.TotalCount = allResources.MediumCount
	resourcesSeverityResult.Low.TotalCount = allResources.LowCount
	resourcesSeverityResult.None.TotalCount = allResources.NoneCount
	passedResource := passedResourcesResult[benchmarkID]
	resourcesSeverityResult.Total.PassedCount = passedResource.TotalCount
	resourcesSeverityResult.Critical.PassedCount = passedResource.CriticalCount
	resourcesSeverityResult.High.PassedCount = passedResource.HighCount
	resourcesSeverityResult.Medium.PassedCount = passedResource.MediumCount
	resourcesSeverityResult.Low.PassedCount = passedResource.LowCount
	resourcesSeverityResult.None.PassedCount = passedResource.NoneCount

	response := api.BenchmarkEvaluationSummary{
		Benchmark:               be,
		ComplianceStatusSummary: csResult,
		Checks:                  sResult,
		ControlsSeverityStatus:  controlSeverityResult,
		ResourcesSeverityStatus: resourcesSeverityResult,
		IntegrationsStatus:      integrationsResult,
		CostImpact:              costImpact,
		EvaluatedAt:             utils.GetPointer(time.Unix(summaryAtTime.EvaluatedAtEpoch, 0)),
		LastJobStatus:           lastJobStatus,
	}

	return echoCtx.JSON(http.StatusOK, response)
}

func (h *HttpHandler) populateBenchmarkControlSummary(ctx context.Context, benchmarkMap map[string]*db.Benchmark, controlSummaryMap map[string]api.ControlSummary, benchmarkId string) (*api.BenchmarkControlSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	benchmark, ok := benchmarkMap[benchmarkId]
	if !ok {
		return nil, errors.New("benchmark not found")
	}

	result := api.BenchmarkControlSummary{
		Benchmark: benchmark.ToApi(),
	}

	for _, control := range benchmark.Controls {
		controlSummary, ok := controlSummaryMap[control.ID]
		if !ok {
			continue
		}
		result.Controls = append(result.Controls, controlSummary)
	}

	for _, child := range benchmark.Children {
		childResult, err := h.populateBenchmarkControlSummary(ctx, benchmarkMap, controlSummaryMap, child.ID)
		if err != nil {
			return nil, err
		}
		result.Children = append(result.Children, *childResult)
	}

	sort.Slice(result.Controls, func(i, j int) bool {
		return result.Controls[i].Control.Title < result.Controls[j].Control.Title
	})

	sort.Slice(result.Children, func(i, j int) bool {
		return result.Children[i].Benchmark.Title < result.Children[j].Benchmark.Title
	})

	return &result, nil
}

// GetBenchmarkControlsTree godoc
//
//	@Summary	Get benchmark controls
//	@Security	BearerToken
//	@Tags		compliance
//	@Accept		json
//	@Produce	json
//	@Param		benchmark_id	path		string		true	"Benchmark ID"
//	@Param		integrationId	query		[]string	false	"integration IDs to filter by"
//	@Param		integrationGroup	query		[]string	false	"integration groups to filter by"
//	@Param		timeAt			query		int			false	"timestamp for values in epoch seconds"
//	@Param		tag				query		[]string	false	"Key-Value tags in key=value format to filter by"
//	@Success	200				{object}	api.BenchmarkControlSummary
//	@Router		/compliance/api/v1/benchmarks/{benchmark_id}/controls [get]
func (h *HttpHandler) GetBenchmarkControlsTree(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	tagMap := model.TagStringsToTagMap(httpserver2.QueryArrayParam(echoCtx, "tag"))
	benchmarkID := echoCtx.Param("benchmark_id")

	integrationIDs, err := h.getIntegrationIdFilterFromParams(echoCtx)
	if err != nil {
		h.logger.Error("failed to get integration IDs", zap.Error(err))
		return err
	}
	timeAt := time.Now()
	if timeAtStr := echoCtx.QueryParam("timeAt"); timeAtStr != "" {
		timeAtInt, err := strconv.ParseInt(timeAtStr, 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid timeAt")
		}
		timeAt = time.Unix(timeAtInt, 0)
	}

	controlsMap := make(map[string]api.Control)
	err = h.populateControlsMap(ctx, benchmarkID, controlsMap, tagMap)
	if err != nil {
		return err
	}

	controlResult, evaluatedAt, err := es.BenchmarkControlSummary(ctx, h.logger, h.client, benchmarkID, integrationIDs, timeAt)
	if err != nil {
		return err
	}

	queryIDs := make([]string, 0, len(controlsMap))
	for _, control := range controlsMap {
		if control.Query == nil {
			continue
		}
		queryIDs = append(queryIDs, control.Query.ID)
	}

	queries, err := h.db.GetQueriesIdAndIntegrationType(ctx, queryIDs)
	if err != nil {
		h.logger.Error("failed to fetch queries", zap.Error(err))
		return err
	}
	queryMap := make(map[string]db.Query)
	for _, query := range queries {
		queryMap[query.ID] = query
	}

	controlSummaryMap := make(map[string]api.ControlSummary)
	for _, control := range controlsMap {
		if control.Query != nil {
			if query, ok := queryMap[control.Query.ID]; ok {
				control.IntegrationType = query.IntegrationType
			}
		}
		result, ok := controlResult[control.ID]
		if !ok {
			result = types.ControlResult{Passed: true}
		}
		controlSummaryMap[control.ID] = api.ControlSummary{
			Control:                control,
			Passed:                 result.Passed,
			FailedResourcesCount:   result.FailedResourcesCount,
			TotalResourcesCount:    result.TotalResourcesCount,
			FailedIntegrationCount: result.FailedIntegrationCount,
			TotalIntegrationCount:  result.TotalIntegrationCount,
			CostImpact:             result.CostImpact,
			EvaluatedAt:            time.Unix(evaluatedAt, 0),
		}
	}

	allBenchmarks, err := h.db.ListBenchmarks(ctx)
	if err != nil {
		h.logger.Error("failed to get benchmarks", zap.Error(err))
		return err
	}
	allBenchmarksMap := make(map[string]*db.Benchmark)
	for _, b := range allBenchmarks {
		b := b
		allBenchmarksMap[b.ID] = &b
	}

	benchmarkControlSummary, err := h.populateBenchmarkControlSummary(ctx, allBenchmarksMap, controlSummaryMap, benchmarkID)
	if err != nil {
		h.logger.Error("failed to populate benchmark control summary", zap.Error(err))
		return err
	}

	return echoCtx.JSON(http.StatusOK, benchmarkControlSummary)
}

// GetBenchmarkControl godoc
//
//	@Summary	Get benchmark controls
//	@Security	BearerToken
//	@Tags		compliance
//	@Accept		json
//	@Produce	json
//	@Param		benchmark_id	path		string		true	"Benchmark ID"
//	@Param		controlId		path		string		true	"Control ID"
//	@Param		integrationId	query		[]string	false	"integration IDs to filter by"
//	@Param		integrationGroup	query		[]string	false	"integration groups to filter by "
//	@Success	200				{object}	api.ControlSummary
//	@Router		/compliance/api/v1/benchmarks/{benchmark_id}/controls/{controlId} [get]
func (h *HttpHandler) GetBenchmarkControl(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	benchmarkID := echoCtx.Param("benchmark_id")
	if benchmarkID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "benchmarkID cannot be empty")
	}
	controlID := echoCtx.Param("controlId")
	if controlID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "controlID cannot be empty")
	}

	integrationIDs, err := h.getIntegrationIdFilterFromParams(echoCtx)
	if err != nil {
		h.logger.Error("failed to get integration IDs", zap.Error(err))
		return err
	}

	controlSummary, err := h.getControlSummary(ctx, controlID, &benchmarkID, integrationIDs)
	if err != nil {
		h.logger.Error("failed to get control summary", zap.Error(err))
		return err
	}

	return echoCtx.JSON(http.StatusOK, controlSummary)
}

func (h *HttpHandler) populateControlsMap(ctx context.Context, benchmarkID string, baseControlsMap map[string]api.Control, tags map[string][]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	benchmark, err := h.db.GetBenchmark(ctx, benchmarkID)
	if err != nil {
		return err
	}
	if benchmark == nil {
		return echo.NewHTTPError(http.StatusNotFound, "invalid benchmarkID")
	}

	if baseControlsMap == nil {
		return errors.New("baseControlsMap cannot be nil")
	}

	for _, child := range benchmark.Children {
		err := h.populateControlsMap(ctx, child.ID, baseControlsMap, tags)
		if err != nil {
			return err
		}
	}

	missingControls := make([]string, 0)
	for _, control := range benchmark.Controls {
		if _, ok := baseControlsMap[control.ID]; !ok {
			missingControls = append(missingControls, control.ID)
		}
	}
	if len(missingControls) > 0 {
		controls, err := h.db.GetControls(ctx, missingControls, tags)
		if err != nil {
			h.logger.Error("failed to get controls", zap.Error(err))
			return err
		}
		for _, control := range controls {
			v := control.ToApi()
			v.IntegrationType = benchmark.IntegrationType
			baseControlsMap[control.ID] = v
		}
	}

	return nil
}

// GetBenchmarkTrend godoc
//
//	@Summary		Get benchmark trend
//	@Description	Retrieving a trend of a benchmark result and checks.
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			benchmark_id		path		string			true	"Benchmark ID"
//	@Param			integrationId		query		[]string		false	"integration IDs to filter by"
//	@Param			integrationGroup		query		[]string		false	"integration groups to filter by "
//	@Param			resourceCollection	query		[]string		false	"Resource collection IDs to filter by"
//	@Param			integrationTypes	query		[]integration.Type	false	"Integration type to filter by"
//	@Param			startTime			query		int				false	"timestamp for start of the chart in epoch seconds"
//	@Param			endTime				query		int				false	"timestamp for end of the chart in epoch seconds"
//	@Success		200					{object}	[]api.BenchmarkTrendDatapoint
//	@Router			/compliance/api/v1/benchmarks/{benchmark_id}/trend [get]
func (h *HttpHandler) GetBenchmarkTrend(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	integrationIDs, err := h.getIntegrationIdFilterFromParams(echoCtx)
	if err != nil {
		return err
	}
	if len(integrationIDs) > 20 {
		return echo.NewHTTPError(http.StatusBadRequest, "too many integration IDs")
	}
	integrationTypes := httpserver2.QueryArrayParam(echoCtx, "integrationTypes")
	resourceCollections := httpserver2.QueryArrayParam(echoCtx, "resourceCollection")
	endTime := time.Now()
	if endTimeStr := echoCtx.QueryParam("endTime"); endTimeStr != "" {
		endTimeInt, err := strconv.ParseInt(endTimeStr, 10, 64)
		if err != nil {
			return err
		}
		endTime = time.Unix(endTimeInt, 0)
	}
	startTime := endTime.AddDate(0, 0, -7)
	if startTimeStr := echoCtx.QueryParam("startTime"); startTimeStr != "" {
		startTimeInt, err := strconv.ParseInt(startTimeStr, 10, 64)
		if err != nil {
			return err
		}
		startTime = time.Unix(startTimeInt, 0)
	}
	benchmarkID := echoCtx.Param("benchmark_id")
	// tracer :
	ctx, span1 := tracer.Start(ctx, "new_GetBenchmark")
	span1.SetName("new_GetBenchmark")
	defer span1.End()

	benchmark, err := h.db.GetBenchmark(ctx, benchmarkID)
	if err != nil {
		span1.RecordError(err)
		span1.SetStatus(codes.Error, err.Error())
		return err
	}

	if benchmark == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid benchmarkID")
	}
	span1.AddEvent("information", trace.WithAttributes(
		attribute.String("benchmark ID", benchmark.ID),
	))
	span1.End()
	be := benchmark.ToApi()

	if len(integrationTypes) > 0 && !utils.IncludesAny(be.IntegrationTypes, integrationTypes) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid integrationType")
	}

	evaluationAcrossTime, err := es.FetchBenchmarkSummaryTrend(ctx, h.logger, h.client,
		[]string{benchmarkID}, integrationIDs, resourceCollections, startTime, endTime)
	if err != nil {
		return err
	}

	controls, err := h.db.ListControlsByBenchmarkID(ctx, benchmarkID)
	if err != nil {
		h.logger.Error("failed to get controls", zap.Error(err))
		return err
	}
	controlsMap := make(map[string]*db.Control)
	for _, control := range controls {
		control := control
		controlsMap[strings.ToLower(control.ID)] = &control
	}

	var response []api.BenchmarkTrendDatapoint
	for _, datapoint := range evaluationAcrossTime[benchmarkID] {
		apiDataPoint := api.BenchmarkTrendDatapoint{
			Timestamp:               time.Unix(datapoint.DateEpoch, 0),
			ComplianceStatusSummary: api.ComplianceStatusSummary{},
			Checks:                  opengovernanceTypes.SeverityResult{},
			ControlsSeverityStatus:  api.BenchmarkControlsSeverityStatus{},
		}
		apiDataPoint.ComplianceStatusSummary.AddESComplianceStatusMap(datapoint.QueryResult)
		apiDataPoint.Checks.AddResultMap(datapoint.SeverityResult)
		for controlId, controlResult := range datapoint.Controls {
			control := controlsMap[strings.ToLower(controlId)]
			apiDataPoint.ControlsSeverityStatus = addToControlSeverityResult(apiDataPoint.ControlsSeverityStatus, control, controlResult)
		}

		response = append(response, apiDataPoint)
	}

	sort.Slice(response, func(i, j int) bool {
		return response[i].Timestamp.Before(response[j].Timestamp)
	})

	return echoCtx.JSON(http.StatusOK, response)
}

// ListControlsTags godoc
//
//	@Summary		List controls tags
//	@Description	Retrieving list of control possible tags
//	@Security		BearerToken
//	@Tags			compliance
//	@Produce		json
//	@Success		200	{object}	[]api.ControlTagsResult
//	@Router			/compliance/api/v3/controls/tags [get]
func (h *HttpHandler) ListControlsTags(ctx echo.Context) error {
	// trace :
	_, span := tracer.Start(ctx.Request().Context(), "new_ListControlsTags", trace.WithSpanKind(trace.SpanKindServer))
	span.SetName("new_ListControlsTags")

	controlsTags, err := h.db.GetControlsTags()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	res := make([]api.ControlTagsResult, 0, len(controlsTags))
	for _, history := range controlsTags {
		res = append(res, history.ToApi())
	}

	span.End()

	return ctx.JSON(200, res)
}

// ListControlsFiltered godoc
//
//	@Summary	List controls filtered by integrationType, benchmark, tags
//	@Security	BearerToken
//	@Tags		compliance
//	@Accept		json
//	@Produce	json
//	@Param		request	body		api.ListControlsFilterRequest	true	"Request Body"
//	@Success	200		{object}	api.ListControlsFilterResponse
//	@Router		/compliance/api/v3/controls [post]
func (h *HttpHandler) ListControlsFiltered(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	var req api.ListControlsFilterRequest
	if err := bindValidate(echoCtx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	var benchmarks []string

	if len(req.RootBenchmark) > 0 {
		var rootBenchmarks []string
		for _, rootBenchmark := range req.RootBenchmark {
			childBenchmarks, err := h.getChildBenchmarks(ctx, rootBenchmark)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			rootBenchmarks = append(rootBenchmarks, childBenchmarks...)
		}
		if len(req.ParentBenchmark) > 0 {
			parentBenchmarks := make(map[string]bool)
			for _, parentBenchmark := range req.ParentBenchmark {
				parentBenchmarks[parentBenchmark] = true
			}
			for _, b := range rootBenchmarks {
				if _, ok := parentBenchmarks[b]; ok {
					benchmarks = append(benchmarks, b)
				}
			}
		} else {
			for _, b := range rootBenchmarks {
				benchmarks = append(benchmarks, b)
			}
		}
	} else if len(req.ParentBenchmark) > 0 {
		benchmarks = req.ParentBenchmark
	}

	var integrationIDs []string
	if req.ComplianceResultFilters != nil {
		integrationIDs = req.ComplianceResultFilters.IntegrationID
	}
	if len(integrationIDs) == 0 {
		integrations, err := h.integrationClient.GetIntegrationGroup(&httpclient.Context{UserRole: authApi.AdminRole}, "active")
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		for _, c := range integrations.Integrations {
			integrationIDs = append(integrationIDs, c.IntegrationID)
		}
	}

	controls, err := h.db.ListControlsByFilter(ctx, nil, req.IntegrationTypes, req.Severity, benchmarks, req.Tags, req.HasParameters,
		req.PrimaryTable, req.ListOfTables, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	var fRes map[string]map[string]int64

	if req.ComplianceResultFilters != nil || req.ComplianceResultSummary {
		var esComplianceStatuses []opengovernanceTypes.ComplianceStatus
		var lastEventFrom, lastEventTo, evaluatedAtFrom, evaluatedAtTo *time.Time

		if req.ComplianceResultFilters != nil {
			esComplianceStatuses = make([]opengovernanceTypes.ComplianceStatus, 0, len(req.ComplianceResultFilters.ComplianceStatus))
			for _, status := range req.ComplianceResultFilters.ComplianceStatus {
				esComplianceStatuses = append(esComplianceStatuses, status.GetEsComplianceStatuses()...)
			}

			if req.ComplianceResultFilters.LastEvent.From != nil && *req.ComplianceResultFilters.LastEvent.From != 0 {
				lastEventFrom = utils.GetPointer(time.Unix(*req.ComplianceResultFilters.LastEvent.From, 0))
			}
			if req.ComplianceResultFilters.LastEvent.To != nil && *req.ComplianceResultFilters.LastEvent.To != 0 {
				lastEventTo = utils.GetPointer(time.Unix(*req.ComplianceResultFilters.LastEvent.To, 0))
			}
			if req.ComplianceResultFilters.EvaluatedAt.From != nil && *req.ComplianceResultFilters.EvaluatedAt.From != 0 {
				evaluatedAtFrom = utils.GetPointer(time.Unix(*req.ComplianceResultFilters.EvaluatedAt.From, 0))
			}
			if req.ComplianceResultFilters.EvaluatedAt.To != nil && *req.ComplianceResultFilters.EvaluatedAt.To != 0 {
				evaluatedAtTo = utils.GetPointer(time.Unix(*req.ComplianceResultFilters.EvaluatedAt.To, 0))
			}
		} else {
			esComplianceStatuses = make([]opengovernanceTypes.ComplianceStatus, 0)
		}

		var controlIDs []string
		for _, c := range controls {
			controlIDs = append(controlIDs, c.ID)
		}
		if req.ComplianceResultFilters != nil {
			benchmarksFilter := benchmarks
			if len(req.ComplianceResultFilters.BenchmarkID) > 0 {
				benchmarksFilter = req.ComplianceResultFilters.BenchmarkID
			}
			fRes, err = es.ComplianceResultsCountByControlID(ctx, h.logger, h.client, req.ComplianceResultFilters.ResourceID,
				req.ComplianceResultFilters.IntegrationType, integrationIDs, req.ComplianceResultFilters.NotIntegrationID,
				req.ComplianceResultFilters.ResourceTypeID, benchmarksFilter, controlIDs, req.ComplianceResultFilters.Severity,
				lastEventFrom, lastEventTo, evaluatedAtFrom, evaluatedAtTo, req.ComplianceResultFilters.StateActive, esComplianceStatuses)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
		} else {
			fRes, err = es.ComplianceResultsCountByControlID(ctx, h.logger, h.client, nil, nil, integrationIDs, nil,
				nil, benchmarks, controlIDs, nil, lastEventFrom, lastEventTo, evaluatedAtFrom,
				evaluatedAtTo, nil, esComplianceStatuses)
		}

		h.logger.Info("ComplianceResult Counts By ControlID", zap.Any("Controls", controlIDs), zap.Any("ComplianceResults Count", fRes))
	}

	var resultControls []api.ListControlsFilterResultControl
	uniqueIntegrationTypes := make(map[string]bool)
	uniqueSeverities := make(map[string]bool)
	uniquePrimaryTables := make(map[string]bool)
	uniqueListOfTables := make(map[string]bool)
	uniqueTags := make(map[string]map[string]bool)
	for _, control := range controls {
		if req.ComplianceResultFilters != nil {
			if count, ok := fRes[control.ID]; ok {
				if len(count) == 0 {
					continue
				}
			} else {
				continue
			}
		}

		apiControl := api.ListControlsFilterResultControl{
			ID:              control.ID,
			Title:           control.Title,
			Description:     control.Description,
			IntegrationType: integration_type.ParseTypes(control.IntegrationType),
			Severity:        control.Severity,
			Tags:            filterTagsByRegex(req.TagsRegex, model.TrimPrivateTags(control.GetTagsMap())),
			Query: struct {
				PrimaryTable *string              `json:"primary_table"`
				ListOfTables []string             `json:"list_of_tables"`
				Parameters   []api.QueryParameter `json:"parameters"`
			}{
				PrimaryTable: control.Query.PrimaryTable,
				ListOfTables: control.Query.ListOfTables,
				Parameters:   make([]api.QueryParameter, 0, len(control.Query.Parameters)),
			},
		}
		for _, p := range control.Query.Parameters {
			apiControl.Query.Parameters = append(apiControl.Query.Parameters, p.ToApi())
		}

		h.logger.Info("ListControlsByFilter", zap.Strings("benchmarks", benchmarks))
		controlResult, _, err := es.BenchmarksControlSummary(ctx, h.logger, h.client, benchmarks, nil)
		if err != nil {
			h.logger.Error("failed to fetch control result", zap.Error(err), zap.String("controlID", control.ID), zap.Any("benchmarkID", benchmarks))
		}

		if req.ComplianceResultSummary {
			var incidentCount, passingComplianceResultCount int64
			if c, ok := fRes[control.ID]["ok"]; ok {
				passingComplianceResultCount = passingComplianceResultCount + c
			}
			if c, ok := fRes[control.ID]["alarm"]; ok {
				incidentCount = incidentCount + c
			}
			if c, ok := fRes[control.ID]["info"]; ok {
				passingComplianceResultCount = passingComplianceResultCount + c
			}
			if c, ok := fRes[control.ID]["skip"]; ok {
				passingComplianceResultCount = passingComplianceResultCount + c
			}
			if c, ok := fRes[control.ID]["error"]; ok {
				incidentCount = incidentCount + c
			}
			apiControl.ComplianceResultsSummary = struct {
				IncidentCount         int64    `json:"incident_count"`
				NonIncidentCount      int64    `json:"non_incident_count"`
				NonCompliantResources int      `json:"noncompliant_resources"`
				CompliantResources    int      `json:"compliant_resources"`
				ImpactedResources     int      `json:"impacted_resources"`
				CostImpact            *float64 `json:"cost_impact"`
			}{
				IncidentCount:         incidentCount,
				NonIncidentCount:      passingComplianceResultCount,
				CompliantResources:    controlResult[control.ID].TotalResourcesCount - controlResult[control.ID].FailedResourcesCount,
				NonCompliantResources: controlResult[control.ID].FailedResourcesCount,
				ImpactedResources:     controlResult[control.ID].TotalResourcesCount,
				CostImpact:            controlResult[control.ID].CostImpact,
			}
		}

		for _, c := range apiControl.IntegrationType {
			uniqueIntegrationTypes[c.String()] = true
		}
		uniqueSeverities[apiControl.Severity.String()] = true
		for _, t := range apiControl.Query.ListOfTables {
			uniqueListOfTables[t] = true
		}
		if apiControl.Query.PrimaryTable != nil {
			uniquePrimaryTables[*apiControl.Query.PrimaryTable] = true
		}
		for k, vs := range apiControl.Tags {
			if _, ok := uniqueTags[k]; !ok {
				uniqueTags[k] = make(map[string]bool)
			}
			for _, v := range vs {
				uniqueTags[k][v] = true
			}
		}

		resultControls = append(resultControls, apiControl)
	}

	totalCount := len(resultControls)

	sortOrder := "asc"
	if strings.ToLower(req.SortOrder) == "asc" || strings.ToLower(req.SortOrder) == "desc" {
		sortOrder = strings.ToLower(req.SortOrder)
	}
	switch sortOrder {
	case "asc":
		switch strings.ToLower(req.SortBy) {
		case "id":
			sort.Slice(resultControls, func(i, j int) bool {
				return resultControls[i].ID < resultControls[j].ID
			})
		case "title":
			sort.Slice(resultControls, func(i, j int) bool {
				return resultControls[i].Title < resultControls[j].Title
			})
		case "severity":
			sort.Slice(resultControls, func(i, j int) bool {
				return resultControls[i].Severity.Level() < resultControls[j].Severity.Level()
			})
		case "incidents":
			sort.Slice(resultControls, func(i, j int) bool {
				return resultControls[i].ComplianceResultsSummary.IncidentCount < resultControls[j].ComplianceResultsSummary.IncidentCount
			})
		case "non-incidents", "nonincidents":
			sort.Slice(resultControls, func(i, j int) bool {
				return resultControls[i].ComplianceResultsSummary.NonIncidentCount < resultControls[j].ComplianceResultsSummary.NonIncidentCount
			})
		case "noncompliant_resources":
			sort.Slice(resultControls, func(i, j int) bool {
				return resultControls[i].ComplianceResultsSummary.NonCompliantResources < resultControls[j].ComplianceResultsSummary.NonCompliantResources
			})
		case "compliant_resources":
			sort.Slice(resultControls, func(i, j int) bool {
				return resultControls[i].ComplianceResultsSummary.CompliantResources < resultControls[j].ComplianceResultsSummary.CompliantResources
			})
		case "impacted_resources":
			sort.Slice(resultControls, func(i, j int) bool {
				return resultControls[i].ComplianceResultsSummary.ImpactedResources < resultControls[j].ComplianceResultsSummary.ImpactedResources
			})
		default:
			sort.Slice(resultControls, func(i, j int) bool {
				return resultControls[i].ID < resultControls[j].ID
			})
		}
	case "desc":
		switch strings.ToLower(req.SortBy) {
		case "id":
			sort.Slice(resultControls, func(i, j int) bool {
				return resultControls[i].ID > resultControls[j].ID
			})
		case "title":
			sort.Slice(resultControls, func(i, j int) bool {
				return resultControls[i].Title > resultControls[j].Title
			})
		case "severity":
			sort.Slice(resultControls, func(i, j int) bool {
				return resultControls[i].Severity.Level() > resultControls[j].Severity.Level()
			})
		case "incidents":
			sort.Slice(resultControls, func(i, j int) bool {
				return resultControls[i].ComplianceResultsSummary.IncidentCount > resultControls[j].ComplianceResultsSummary.IncidentCount
			})
		case "non-incidents", "nonincidents":
			sort.Slice(resultControls, func(i, j int) bool {
				return resultControls[i].ComplianceResultsSummary.NonIncidentCount > resultControls[j].ComplianceResultsSummary.NonIncidentCount
			})
		case "noncompliant_resources":
			sort.Slice(resultControls, func(i, j int) bool {
				return resultControls[i].ComplianceResultsSummary.NonCompliantResources > resultControls[j].ComplianceResultsSummary.NonCompliantResources
			})
		case "compliant_resources":
			sort.Slice(resultControls, func(i, j int) bool {
				return resultControls[i].ComplianceResultsSummary.CompliantResources > resultControls[j].ComplianceResultsSummary.CompliantResources
			})
		case "impacted_resources":
			sort.Slice(resultControls, func(i, j int) bool {
				return resultControls[i].ComplianceResultsSummary.ImpactedResources > resultControls[j].ComplianceResultsSummary.ImpactedResources
			})
		default:
			sort.Slice(resultControls, func(i, j int) bool {
				return resultControls[i].ID > resultControls[j].ID
			})
		}
	}

	if req.PerPage != nil {
		if req.Cursor == nil {
			resultControls = utils.Paginate(1, *req.PerPage, resultControls)
		} else {
			resultControls = utils.Paginate(*req.Cursor, *req.PerPage, resultControls)
		}
	}

	response := api.ListControlsFilterResponse{
		Items:      resultControls,
		TotalCount: totalCount,
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// ControlsFilteredSummary godoc
//
//	@Summary	List controls filtered by integrationType, benchmark, tags
//	@Security	BearerToken
//	@Tags		compliance
//	@Accept		json
//	@Produce	json
//	@Param		request	body		api.ControlsFilterSummaryRequest	true	"Request Body"
//	@Success	200		{object}	api.ControlsFilterSummaryResult
//	@Router		/compliance/api/v3/controls/summary [post]
func (h *HttpHandler) ControlsFilteredSummary(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	var req api.ControlsFilterSummaryRequest
	if err := bindValidate(echoCtx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	var benchmarks []string

	if len(req.RootBenchmark) > 0 {
		var rootBenchmarks []string
		for _, rootBenchmark := range req.RootBenchmark {
			childBenchmarks, err := h.getChildBenchmarks(ctx, rootBenchmark)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			rootBenchmarks = append(rootBenchmarks, childBenchmarks...)
		}
		if len(req.ParentBenchmark) > 0 {
			parentBenchmarks := make(map[string]bool)
			for _, parentBenchmark := range req.ParentBenchmark {
				parentBenchmarks[parentBenchmark] = true
			}
			for _, b := range rootBenchmarks {
				if _, ok := parentBenchmarks[b]; ok {
					benchmarks = append(benchmarks, b)
				}
			}
		} else {
			for _, b := range rootBenchmarks {
				benchmarks = append(benchmarks, b)
			}
		}
	} else if len(req.ParentBenchmark) > 0 {
		benchmarks = req.ParentBenchmark
	}

	controls, err := h.db.ListControlsByFilter(ctx, nil, req.IntegrationTypes, req.Severity, benchmarks, req.Tags, req.HasParameters,
		req.PrimaryTable, req.ListOfTables, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	var fRes map[string]map[string]int64

	if req.ComplianceResultFilters != nil {
		var esComplianceStatuses []opengovernanceTypes.ComplianceStatus
		var lastEventFrom, lastEventTo, evaluatedAtFrom, evaluatedAtTo *time.Time

		if req.ComplianceResultFilters != nil {
			esComplianceStatuses = make([]opengovernanceTypes.ComplianceStatus, 0, len(req.ComplianceResultFilters.ComplianceStatus))
			for _, status := range req.ComplianceResultFilters.ComplianceStatus {
				esComplianceStatuses = append(esComplianceStatuses, status.GetEsComplianceStatuses()...)
			}

			if req.ComplianceResultFilters.LastEvent.From != nil && *req.ComplianceResultFilters.LastEvent.From != 0 {
				lastEventFrom = utils.GetPointer(time.Unix(*req.ComplianceResultFilters.LastEvent.From, 0))
			}
			if req.ComplianceResultFilters.LastEvent.To != nil && *req.ComplianceResultFilters.LastEvent.To != 0 {
				lastEventTo = utils.GetPointer(time.Unix(*req.ComplianceResultFilters.LastEvent.To, 0))
			}
			if req.ComplianceResultFilters.EvaluatedAt.From != nil && *req.ComplianceResultFilters.EvaluatedAt.From != 0 {
				evaluatedAtFrom = utils.GetPointer(time.Unix(*req.ComplianceResultFilters.EvaluatedAt.From, 0))
			}
			if req.ComplianceResultFilters.EvaluatedAt.To != nil && *req.ComplianceResultFilters.EvaluatedAt.To != 0 {
				evaluatedAtTo = utils.GetPointer(time.Unix(*req.ComplianceResultFilters.EvaluatedAt.To, 0))
			}
		} else {
			esComplianceStatuses = make([]opengovernanceTypes.ComplianceStatus, 0)
		}

		var controlIDs []string
		for _, c := range controls {
			controlIDs = append(controlIDs, c.ID)
		}
		if req.ComplianceResultFilters != nil {
			fRes, err = es.ComplianceResultsCountByControlID(ctx, h.logger, h.client, req.ComplianceResultFilters.ResourceID,
				req.ComplianceResultFilters.IntegrationType, req.ComplianceResultFilters.IntegrationID, req.ComplianceResultFilters.NotIntegrationID,
				req.ComplianceResultFilters.ResourceTypeID, req.ComplianceResultFilters.BenchmarkID, controlIDs, req.ComplianceResultFilters.Severity,
				lastEventFrom, lastEventTo, evaluatedAtFrom, evaluatedAtTo, req.ComplianceResultFilters.StateActive, esComplianceStatuses)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
		} else {
			fRes, err = es.ComplianceResultsCountByControlID(ctx, h.logger, h.client, nil, nil, nil, nil,
				nil, nil, controlIDs, nil, lastEventFrom, lastEventTo, evaluatedAtFrom,
				evaluatedAtTo, nil, esComplianceStatuses)
		}

		h.logger.Info("ComplianceResult Counts By ControlID", zap.Any("Controls", controlIDs), zap.Any("ComplianceResults Count", fRes))
	}

	var resultControls []api.ListControlsFilterResultControl
	uniqueIntegrationTypes := make(map[string]bool)
	uniqueSeverities := make(map[string]bool)
	uniquePrimaryTables := make(map[string]bool)
	uniqueListOfTables := make(map[string]bool)
	uniqueTags := make(map[string]map[string]bool)
	for _, control := range controls {
		if req.ComplianceResultFilters != nil {
			if count, ok := fRes[control.ID]; ok {
				if len(count) == 0 {
					continue
				}
			} else {
				continue
			}
		}

		apiControl := api.ListControlsFilterResultControl{
			ID:              control.ID,
			Title:           control.Title,
			Description:     control.Description,
			IntegrationType: integration_type.ParseTypes(control.IntegrationType),
			Severity:        control.Severity,
			Tags:            filterTagsByRegex(req.TagsRegex, model.TrimPrivateTags(control.GetTagsMap())),
			Query: struct {
				PrimaryTable *string              `json:"primary_table"`
				ListOfTables []string             `json:"list_of_tables"`
				Parameters   []api.QueryParameter `json:"parameters"`
			}{
				PrimaryTable: control.Query.PrimaryTable,
				ListOfTables: control.Query.ListOfTables,
				Parameters:   make([]api.QueryParameter, 0, len(control.Query.Parameters)),
			},
		}
		for _, p := range control.Query.Parameters {
			apiControl.Query.Parameters = append(apiControl.Query.Parameters, p.ToApi())
		}

		for _, c := range apiControl.IntegrationType {
			uniqueIntegrationTypes[c.String()] = true
		}
		uniqueSeverities[apiControl.Severity.String()] = true
		for _, t := range apiControl.Query.ListOfTables {
			if t == "" {
				continue
			}
			uniqueListOfTables[t] = true
		}
		if apiControl.Query.PrimaryTable != nil && *apiControl.Query.PrimaryTable != "" {
			uniquePrimaryTables[*apiControl.Query.PrimaryTable] = true
		}
		for k, vs := range apiControl.Tags {
			if _, ok := uniqueTags[k]; !ok {
				uniqueTags[k] = make(map[string]bool)
			}
			for _, v := range vs {
				uniqueTags[k][v] = true
			}
		}

		resultControls = append(resultControls, apiControl)
	}

	uniqueTagsFinal := make(map[string][]string)
	for k, vs := range uniqueTags {
		for v, _ := range vs {
			uniqueTagsFinal[k] = append(uniqueTagsFinal[k], v)
		}
	}
	result := api.ControlsFilterSummaryResult{
		ControlsCount:    int64(len(resultControls)),
		IntegrationTypes: mapToArray(uniqueIntegrationTypes),
		Severity:         mapToArray(uniqueSeverities),
		Tags:             uniqueTagsFinal,
		PrimaryTable:     mapToArray(uniquePrimaryTables),
		ListOfTables:     mapToArray(uniqueListOfTables),
	}

	return echoCtx.JSON(http.StatusOK, result)
}

// GetControlDetails godoc
//
//	@Summary	Get Control Details by control ID
//	@Security	BearerToken
//	@Tags		compliance
//	@Accept		json
//	@Produce	json
//	@Param		control_id	path		string	true	"Control ID"
//	@Success	200			{object}	api.GetControlDetailsResponse
//	@Router		/compliance/api/v3/control/{control_id} [get]
func (h *HttpHandler) GetControlDetails(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	controlId := echoCtx.Param("control_id")

	var showReferences bool
	showReferencesString := echoCtx.QueryParam("showReferences")
	showReferences, err := strconv.ParseBool(showReferencesString)
	if err != nil {
		showReferences = false
	}

	control, err := h.db.GetControl(ctx, controlId)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if control == nil {
		return echo.NewHTTPError(http.StatusNotFound, "control not found")
	}
	var parameters []api.QueryParameter
	for _, qp := range control.Query.Parameters {
		parameters = append(parameters, qp.ToApi())
	}

	response := api.GetControlDetailsResponse{
		ID:              control.ID,
		Title:           control.Title,
		Description:     control.Description,
		IntegrationType: integration_type.ParseTypes(control.IntegrationType),
		Severity:        control.Severity.String(),
		Query: struct {
			Engine         string               `json:"engine"`
			QueryToExecute string               `json:"queryToExecute"`
			PrimaryTable   *string              `json:"primaryTable"`
			ListOfTables   []string             `json:"listOfTables"`
			Parameters     []api.QueryParameter `json:"parameters"`
		}{
			Engine:         control.Query.Engine,
			QueryToExecute: control.Query.QueryToExecute,
			PrimaryTable:   control.Query.PrimaryTable,
			ListOfTables:   control.Query.ListOfTables,
			Parameters:     parameters,
		},
		Tags: model.TrimPrivateTags(control.GetTagsMap()),
	}

	if showReferences {
		benchmarks, err := h.db.GetBenchmarkIdsByControlID(ctx, control.ID)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}

		benchmarkPathsMap := make(map[string]bool)
		rootBenchmarksMap := make(map[string]bool)
		var benchmarkPaths, rootBenchmarks []string
		for _, b := range benchmarks {
			path, err := h.getBenchmarkPath(ctx, b)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			benchmarkPathsMap[path] = true
			root := strings.Split(path, "/")[0]
			rootBenchmarksMap[root] = true
		}
		for k, _ := range benchmarkPathsMap {
			benchmarkPaths = append(benchmarkPaths, k)
		}
		for k, _ := range rootBenchmarksMap {
			rootBenchmarks = append(rootBenchmarks, k)
		}
		response.Benchmarks = &struct {
			Roots    []string `json:"roots"`
			FullPath []string `json:"fullPath"`
		}{
			Roots:    rootBenchmarks,
			FullPath: benchmarkPaths,
		}
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// ListControlsSummary godoc
//
//	@Summary	List controls summaries
//	@Security	BearerToken
//	@Tags		compliance
//	@Accept		json
//	@Produce	json
//	@Param		controlId		query		[]string	false	"Control IDs to filter by"
//	@Param		integrationId	query		[]string	false	"integration IDs to filter by"
//	@Param		integrationGroup	query		[]string	false	"integration groups to filter by "
//	@Param		tag				query		[]string	false	"Key-Value tags in key=value format to filter by"
//	@Success	200				{object}	[]api.ControlSummary
//	@Router		/compliance/api/v1/controls/summary [get]
func (h *HttpHandler) ListControlsSummary(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	tagMap := model.TagStringsToTagMap(httpserver2.QueryArrayParam(echoCtx, "tag"))
	integrationIDs, err := h.getIntegrationIdFilterFromParams(echoCtx)
	if err != nil {
		h.logger.Error("failed to get integration IDs", zap.Error(err))
		return err
	}

	controlIds := httpserver2.QueryArrayParam(echoCtx, "controlId")
	controls, err := h.db.GetControls(ctx, controlIds, tagMap)
	if err != nil {
		h.logger.Error("failed to fetch controls", zap.Error(err))
		return err
	}
	controlIds = make([]string, 0, len(controls))
	for _, control := range controls {
		controlIds = append(controlIds, control.ID)
	}

	benchmarks, err := h.db.ListDistinctRootBenchmarksFromControlIds(ctx, controlIds)
	if err != nil {
		h.logger.Error("failed to fetch benchmarks", zap.Error(err))
		return err
	}
	benchmarkIds := make([]string, 0, len(benchmarks))
	for _, benchmark := range benchmarks {
		benchmarkIds = append(benchmarkIds, benchmark.ID)
	}

	controlResults, evaluatedAts, err := es.BenchmarksControlSummary(ctx, h.logger, h.client, benchmarkIds, integrationIDs)
	if err != nil {
		h.logger.Error("failed to fetch control results", zap.Error(err))
		return err
	}

	resourceTypes, err := h.inventoryClient.ListResourceTypesMetadata(&httpclient.Context{UserRole: authApi.AdminRole},
		nil, nil, nil, false, nil, 10000, 1)
	if err != nil {
		h.logger.Error("failed to get resource types metadata", zap.Error(err))
		return err
	}
	resourceTypeMap := make(map[string]*inventoryApi.ResourceType)
	for _, rt := range resourceTypes.ResourceTypes {
		rt := rt
		resourceTypeMap[strings.ToLower(rt.ResourceType)] = &rt
	}

	results := make([]api.ControlSummary, 0, len(controls))
	for _, control := range controls {
		apiControl := control.ToApi()
		var resourceType *inventoryApi.ResourceType
		if control.Query != nil {
			apiControl.IntegrationType = control.Query.IntegrationType
			if control.Query.PrimaryTable != nil {
				rtName, _, err := runner.GetResourceTypeFromTableName(*control.Query.PrimaryTable, integration_type.ParseTypes(control.Query.IntegrationType))
				if err != nil {
					h.logger.Error("failed to get resource type from table name", zap.Error(err))
					return err
				}
				resourceType = resourceTypeMap[strings.ToLower(rtName)]
			}
		}

		result, ok := controlResults[control.ID]
		if !ok {
			result = types.ControlResult{Passed: true}
		}
		evaluatedAt, ok := evaluatedAts[control.ID]
		if !ok {
			evaluatedAt = -1
		}

		controlSummary := api.ControlSummary{
			Control:                apiControl,
			ResourceType:           resourceType,
			Passed:                 result.Passed,
			FailedResourcesCount:   result.FailedResourcesCount,
			TotalResourcesCount:    result.TotalResourcesCount,
			FailedIntegrationCount: result.FailedIntegrationCount,
			TotalIntegrationCount:  result.TotalIntegrationCount,
			CostImpact:             result.CostImpact,
			EvaluatedAt:            time.Unix(evaluatedAt, 0),
		}
		results = append(results, controlSummary)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].EvaluatedAt.Unix() == -1 {
			return false
		}
		if results[j].EvaluatedAt.Unix() == -1 {
			return true
		}
		return results[i].FailedResourcesCount > results[j].FailedResourcesCount
	})

	return echoCtx.JSON(http.StatusOK, results)
}

// GetControlSummary godoc
//
//	@Summary	Get control summary
//	@Security	BearerToken
//	@Tags		compliance
//	@Accept		json
//	@Produce	json
//	@Param		controlId		path		string		true	"Control ID"
//	@Param		integrationId	query		[]string	false	"integration IDs to filter by"
//	@Param		integrationGroup	query		[]string	false	"integrationion groups to filter by "
//	@Success	200				{object}	api.ControlSummary
//	@Router		/compliance/api/v1/controls/{controlId}/summary [get]
func (h *HttpHandler) GetControlSummary(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	controlID := echoCtx.Param("controlId")
	integrationIds := httpserver2.QueryArrayParam(echoCtx, IntegrationIDParam)
	//integrationIds, err := httpserver2.ResolveIntegrationIDs(echoCtx, integrationIds)
	//if err != nil {
	//	return err
	//}
	integrationGroup := httpserver2.QueryArrayParam(echoCtx, IntegrationGroupParam)

	if len(integrationIds) == 0 && len(integrationGroup) == 0 {
		integrationGroup = []string{"active"}
	}
	integrationIDs, err := h.getIntegrationIdFilterFromInputs(echoCtx.Request().Context(), integrationIds, integrationGroup)
	if err != nil {
		return err
	}

	controlSummary, err := h.getControlSummary(ctx, controlID, nil, integrationIDs)
	if err != nil {
		return err
	}

	return echoCtx.JSON(http.StatusOK, controlSummary)
}

func (h *HttpHandler) getControlSummary(ctx context.Context, controlID string, benchmarkID *string, integrationIDs []string) (*api.ControlSummary, error) {
	control, err := h.db.GetControl(ctx, controlID)
	if err != nil {
		h.logger.Error("failed to fetch control", zap.Error(err), zap.String("controlID", controlID), zap.Stringp("benchmarkID", benchmarkID))
		return nil, err
	}
	if control == nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("control %s not found", controlID))
	}
	apiControl := control.ToApi()
	if benchmarkID != nil {
		benchmark, err := h.db.GetBenchmarkBare(ctx, *benchmarkID)
		if err != nil {
			h.logger.Error("failed to fetch benchmark", zap.Error(err), zap.Stringp("benchmarkID", benchmarkID))
			return nil, err
		}
		apiControl.IntegrationType = benchmark.IntegrationType
	}

	resourceTypes, err := h.inventoryClient.ListResourceTypesMetadata(&httpclient.Context{Ctx: ctx, UserRole: authApi.AdminRole},
		nil, nil, nil, false, nil, 10000, 1)
	if err != nil {
		h.logger.Error("failed to get resource types metadata", zap.Error(err))
		return nil, err
	}
	resourceTypeMap := make(map[string]*inventoryApi.ResourceType)
	for _, rt := range resourceTypes.ResourceTypes {
		rt := rt
		resourceTypeMap[strings.ToLower(rt.ResourceType)] = &rt
	}

	var resourceType *inventoryApi.ResourceType
	if control.Query != nil {
		apiControl.IntegrationType = control.Query.IntegrationType
		if control.Query != nil && control.Query.PrimaryTable != nil {
			rtName, _, err := runner.GetResourceTypeFromTableName(*control.Query.PrimaryTable, integration_type.ParseTypes(control.Query.IntegrationType))
			if err != nil {
				h.logger.Error("failed to get resource type from table name", zap.Error(err))
				return nil, err
			}
			resourceType = resourceTypeMap[strings.ToLower(rtName)]
		}
	}

	benchmarks, err := h.db.ListDistinctRootBenchmarksFromControlIds(ctx, []string{controlID})
	if err != nil {
		h.logger.Error("failed to fetch benchmarks", zap.Error(err), zap.String("controlID", controlID))
		return nil, err
	}
	benchmarkIds := make([]string, 0, len(benchmarks))
	apiBenchmarks := make([]api.Benchmark, 0, len(benchmarks))
	for _, benchmark := range benchmarks {
		benchmarkIds = append(benchmarkIds, benchmark.ID)
		apiBenchmarks = append(apiBenchmarks, benchmark.ToApi())
	}

	var evaluatedAt int64
	var result types.ControlResult
	if benchmarkID != nil {
		controlResult, evAt, err := es.BenchmarkControlSummary(ctx, h.logger, h.client, *benchmarkID, integrationIDs, time.Now())
		if err != nil {
			h.logger.Error("failed to fetch control result", zap.Error(err), zap.String("controlID", controlID), zap.Stringp("benchmarkID", benchmarkID))
			return nil, err
		}
		var ok bool
		result, ok = controlResult[control.ID]
		if !ok {
			result = types.ControlResult{Passed: true}
		}
		evaluatedAt = evAt
	} else {
		controlResult, evaluatedAts, err := es.BenchmarksControlSummary(ctx, h.logger, h.client, benchmarkIds, integrationIDs)
		if err != nil {
			h.logger.Error("failed to fetch control result", zap.Error(err), zap.String("controlID", controlID), zap.Stringp("benchmarkID", benchmarkID))
		}
		var ok bool
		result, ok = controlResult[control.ID]
		if !ok {
			result = types.ControlResult{Passed: true}
		}
		evaluatedAt, ok = evaluatedAts[control.ID]
		if !ok {
			evaluatedAt = -1
		}
	}

	controlSummary := api.ControlSummary{
		Control:                apiControl,
		ResourceType:           resourceType,
		Benchmarks:             apiBenchmarks,
		Passed:                 result.Passed,
		FailedResourcesCount:   result.FailedResourcesCount,
		TotalResourcesCount:    result.TotalResourcesCount,
		FailedIntegrationCount: result.FailedIntegrationCount,
		TotalIntegrationCount:  result.TotalIntegrationCount,
		CostImpact:             result.CostImpact,
		EvaluatedAt:            time.Unix(evaluatedAt, 0),
	}

	return &controlSummary, nil
}

// GetControlTrend godoc
//
//	@Summary	Get control trend
//	@Security	BearerToken
//	@Tags		compliance
//	@Accept		json
//	@Produce	json
//	@Param		controlId		path		string		true	"Control ID"
//	@Param		integrationId	query		[]string	false	"integration IDs to filter by"
//	@Param		integrationGroup	query		[]string	false	"integration groups to filter by "
//	@Param		startTime		query		int			false	"timestamp for start of the chart in epoch seconds"
//	@Param		endTime			query		int			false	"timestamp for end of the chart in epoch seconds"
//	@Param		granularity		query		string		false	"granularity of the chart"	Enums(daily,monthly)	Default(daily)
//	@Success	200				{object}	[]api.ControlTrendDatapoint
//	@Router		/compliance/api/v1/controls/{controlId}/trend [get]
func (h *HttpHandler) GetControlTrend(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	integrationIDs, err := h.getIntegrationIdFilterFromParams(echoCtx)
	if err != nil {
		return err
	}
	if len(integrationIDs) > 20 {
		return echo.NewHTTPError(http.StatusBadRequest, "too many integration IDs")
	}
	endTime := time.Now()
	if endTimeStr := echoCtx.QueryParam("timeAt"); endTimeStr != "" {
		endTimeInt, err := strconv.ParseInt(endTimeStr, 10, 64)
		if err != nil {
			return err
		}
		endTime = time.Unix(endTimeInt, 0)
	}
	startTime := endTime.AddDate(0, 0, -7)
	if startTimeStr := echoCtx.QueryParam("startTime"); startTimeStr != "" {
		startTimeInt, err := strconv.ParseInt(startTimeStr, 10, 64)
		if err != nil {
			return err
		}
		startTime = time.Unix(startTimeInt, 0)
	}

	controlID := echoCtx.Param("controlId")
	control, err := h.db.GetControl(ctx, controlID)
	if err != nil {
		h.logger.Error("failed to fetch control", zap.Error(err), zap.String("controlID", controlID))
		return err
	}
	benchmarks, err := h.db.ListDistinctRootBenchmarksFromControlIds(ctx, []string{controlID})
	if err != nil {
		h.logger.Error("failed to fetch benchmarks", zap.Error(err), zap.String("controlID", controlID))
		return err
	}
	benchmarkIds := make([]string, 0, len(benchmarks))
	for _, benchmark := range benchmarks {
		benchmarkIds = append(benchmarkIds, benchmark.ID)
	}

	stepDuration := 24 * time.Hour
	if granularity := echoCtx.QueryParam("granularity"); granularity == "monthly" {
		stepDuration = 30 * 24 * time.Hour
	}

	dataPoints, err := es.FetchBenchmarkSummaryTrendByIntegrationIDPerControl(ctx, h.logger, h.client,
		benchmarkIds, []string{controlID}, integrationIDs, startTime, endTime, stepDuration)
	if err != nil {
		h.logger.Error("failed to fetch control result", zap.Error(err), zap.String("controlID", controlID))
		return err
	}

	var response []api.ControlTrendDatapoint
	for _, datapoint := range dataPoints[control.ID] {
		response = append(response, api.ControlTrendDatapoint{
			Timestamp:              int(datapoint.DateEpoch),
			FailedResourcesCount:   datapoint.FailedResourcesCount,
			TotalResourcesCount:    datapoint.TotalResourcesCount,
			FailedIntegrationCount: datapoint.FailedIntegrationCount,
			TotalIntegrationCount:  datapoint.TotalIntegrationCount,
		})
	}

	sort.Slice(response, func(i, j int) bool {
		return response[i].Timestamp < response[j].Timestamp
	})

	return echoCtx.JSON(http.StatusOK, response)
}

// CreateBenchmarkAssignment godoc
//
//	@Summary		Create benchmark assignment
//	@Description	Creating a benchmark assignment for a integration.
//	@Security		BearerToken
//	@Tags			benchmarks_assignment
//	@Accept			json
//	@Produce		json
//	@Param			benchmark_id		path		string		true	"Benchmark ID"
//	@Param			auto_assign			query		bool		false	"Auto enable benchmark for integrations"
//	@Param			integrationId		query		[]string	false	"integration ID or 'all' for everything"
//	@Param			integrationGroup		query		[]string	false	"integration group"
//	@Param			resourceCollection	query		[]string	false	"Resource collection"
//	@Success		200					{object}	[]api.BenchmarkAssignment
//	@Router			/compliance/api/v1/assignments/{benchmark_id}/integration [post]
func (h *HttpHandler) CreateBenchmarkAssignment(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	integrationIDs, err := h.getIntegrationIdFilterFromParams(echoCtx)
	if err != nil {
		return err
	}

	resourceCollections := httpserver2.QueryArrayParam(echoCtx, "resourceCollection")
	if len(integrationIDs) > 0 && len(resourceCollections) > 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "cannot specify both integration and resource collection")
	}

	autoAssignStr := echoCtx.QueryParam("auto_assign")

	benchmarkId := echoCtx.Param("benchmark_id")
	if benchmarkId == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "benchmark id is empty")
	}
	// trace :
	ctx, span1 := tracer.Start(ctx, "new_GetBenchmark", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_GetBenchmark")
	defer span1.End()

	benchmark, err := h.db.GetBenchmark(ctx, benchmarkId)

	if err != nil {
		span1.RecordError(err)
		span1.SetStatus(codes.Error, err.Error())
		return err
	}

	if benchmark == nil {
		return echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("benchmark %s not found", benchmarkId))
	}

	span1.AddEvent("information", trace.WithAttributes(
		attribute.String("benchmark Id", benchmark.ID),
	))
	span1.End()

	ca := benchmark.ToApi()
	switch {
	case len(autoAssignStr) > 0:
		autoAssign, err := strconv.ParseBool(autoAssignStr)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid auto_enable value")
		}
		err = h.db.SetBenchmarkAutoAssign(ctx, benchmarkId, autoAssign)
		if err != nil {
			h.logger.Error("failed to set auto assign", zap.Error(err))
			return err
		}
		return echoCtx.JSON(http.StatusOK, []api.BenchmarkAssignment{})
	case len(integrationIDs) > 0:
		integrations := make([]integrationapi.Integration, 0)
		if len(integrationIDs) == 1 && strings.ToLower(integrationIDs[0]) == "all" {
			srcs, err := h.integrationClient.ListIntegrations(&httpclient.Context{UserRole: authApi.AdminRole}, ca.IntegrationTypes)
			if err != nil {
				return err
			}
			for _, src := range srcs.Integrations {
				if src.State == integrationapi.IntegrationStateActive {
					integrations = append(integrations, src)
				}
			}
		} else {
			listIntegrationsResp, err := h.integrationClient.ListIntegrationsByFilters(&httpclient.Context{UserRole: authApi.AdminRole}, integrationapi.ListIntegrationsRequest{
				IntegrationID: integrationIDs,
			})
			if err != nil {
				return err
			}
			integrations = listIntegrationsResp.Integrations
		}

		result := make([]api.BenchmarkAssignment, 0, len(integrations))
		// trace :
		ctx, span4 := tracer.Start(ctx, "new_AddBenchmarkAssignment(loop)", trace.WithSpanKind(trace.SpanKindServer))
		span4.SetName("new_AddBenchmarkAssignment(loop)")
		defer span4.End()

		for _, src := range integrations {
			assignment := &db.BenchmarkAssignment{
				BenchmarkId:   benchmarkId,
				IntegrationID: utils.GetPointer(src.IntegrationID),
				AssignedAt:    time.Now(),
			}
			//trace :
			ctx, span5 := tracer.Start(ctx, "new_AddBenchmarkAssignment", trace.WithSpanKind(trace.SpanKindServer))
			span5.SetName("new_AddBenchmarkAssignment")

			if err := h.db.AddBenchmarkAssignment(ctx, assignment); err != nil {
				span5.RecordError(err)
				span5.SetStatus(codes.Error, err.Error())
				span5.End()
				echoCtx.Logger().Errorf("add benchmark assignment: %v", err)
				return err
			}
			span5.SetAttributes(
				attribute.String("Benchmark ID", assignment.BenchmarkId),
			)
			span5.End()

			for _, integrationId := range integrationIDs {
				result = append(result, api.BenchmarkAssignment{
					BenchmarkId:   benchmarkId,
					IntegrationId: utils.GetPointer(integrationId),
					AssignedAt:    assignment.AssignedAt,
				})
			}
		}
		span4.End()
		return echoCtx.JSON(http.StatusOK, result)
	case len(resourceCollections) > 0:
		result := make([]api.BenchmarkAssignment, 0, len(resourceCollections))

		for _, resourceCollection := range resourceCollections {
			resourceCollection := resourceCollection
			assignment := &db.BenchmarkAssignment{
				BenchmarkId:        benchmarkId,
				ResourceCollection: &resourceCollection,
				AssignedAt:         time.Now(),
			}
			// trace :
			ctx, span6 := tracer.Start(ctx, "new_AddBenchmarkAssignment", trace.WithSpanKind(trace.SpanKindServer))
			span6.SetName("new_AddBenchmarkAssignment")

			if err := h.db.AddBenchmarkAssignment(ctx, assignment); err != nil {
				span6.RecordError(err)
				span6.SetStatus(codes.Error, err.Error())
				span6.End()
				echoCtx.Logger().Errorf("add benchmark assignment: %v", err)
				return err
			}
			span6.SetAttributes(
				attribute.String("benchmark ID", assignment.BenchmarkId),
			)
			span6.End()

			result = append(result, api.BenchmarkAssignment{
				BenchmarkId:          benchmarkId,
				ResourceCollectionId: assignment.ResourceCollection,
				AssignedAt:           assignment.AssignedAt,
			})
		}
		return echoCtx.JSON(http.StatusOK, result)
	}
	return echo.NewHTTPError(http.StatusBadRequest, "auto assign, integration or resource collection is required")
}

// ListAssignmentsByIntegration godoc
//
//	@Summary		Get list of benchmark assignments for a integration
//	@Description	Retrieving all benchmark assigned to a integration with integration id
//	@Security		BearerToken
//	@Tags			benchmarks_assignment
//	@Accept			json
//	@Produce		json
//	@Param			integration_id	path		string	true	"integration ID"
//	@Success		200				{object}	[]api.AssignedBenchmark
//	@Router			/compliance/api/v1/assignments/integration/{integration_id} [get]
func (h *HttpHandler) ListAssignmentsByIntegration(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	integrationId := echoCtx.Param("integration_id")
	if integrationId == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "integration id is empty")
	}

	if err := httpserver2.CheckAccessToConnectionID(echoCtx, integrationId); err != nil {
		return err
	}

	ctx, span1 := tracer.Start(ctx, "new_GetBenchmarkAssignmentsBySourceId", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_GetBenchmarkAssignmentsBySourceId")
	defer span1.End()

	dbAssignments, err := h.db.GetBenchmarkAssignmentsByIntegrationId(ctx, integrationId)
	if err != nil {
		span1.RecordError(err)
		span1.SetStatus(codes.Error, err.Error())
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("benchmark assignments for %s not found", integrationId))
		}
		echoCtx.Logger().Errorf("find benchmark assignments by integration type %s: %v", integrationId, err)
		return err
	}

	span1.AddEvent("information", trace.WithAttributes(
		attribute.String("integration ID", integrationId),
	))
	span1.End()

	benchmarks, err := h.db.ListRootBenchmarks(ctx, nil)
	if err != nil {
		return err
	}

	integrationT, err := h.integrationClient.GetIntegration(&httpclient.Context{UserRole: authApi.AdminRole}, integrationId)
	if err != nil {
		return err
	}

	result := make([]api.AssignedBenchmark, 0, len(dbAssignments))
	for _, benchmark := range benchmarks {
		apiBenchmark := benchmark.ToApi()
		if !utils.Includes(apiBenchmark.IntegrationTypes, integrationT.IntegrationType.String()) {
			continue
		}
		res := api.AssignedBenchmark{
			Benchmark: benchmark.ToApi(),
			Status:    false,
		}
		if benchmark.AutoAssign && integrationT.State == integrationapi.IntegrationStateActive {
			res.Status = true
		} else {
			for _, assignment := range dbAssignments {
				if assignment.IntegrationID != nil && *assignment.IntegrationID == integrationT.IntegrationID && assignment.BenchmarkId == benchmark.ID {
					res.Status = true
					break
				}
			}
		}
		result = append(result, res)
	}

	return echoCtx.JSON(http.StatusOK, result)
}

// ListAssignmentsByResourceCollection godoc
//
//	@Summary		Get list of benchmark assignments for a resource collection
//	@Description	Retrieving all benchmark assigned to a resource collection with resource collection id
//	@Security		BearerToken
//	@Tags			benchmarks_assignment
//	@Accept			json
//	@Produce		json
//	@Param			resource_collection_id	path		string	true	"Resource collection ID"
//	@Success		200						{object}	[]api.AssignedBenchmark
//	@Router			/compliance/api/v1/assignments/resource_collection/{resource_collection_id} [get]
func (h *HttpHandler) ListAssignmentsByResourceCollection(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	resourceCollectionId := echoCtx.Param("resource_collection_id")
	if resourceCollectionId == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "integration id is empty")
	}

	ctx, span1 := tracer.Start(ctx, "new_GetBenchmarkAssignmentsBySourceId", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_GetBenchmarkAssignmentsBySourceId")
	defer span1.End()

	dbAssignments, err := h.db.GetBenchmarkAssignmentsByResourceCollectionId(ctx, resourceCollectionId)
	if err != nil {
		span1.RecordError(err)
		span1.SetStatus(codes.Error, err.Error())
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("benchmark assignments for %s not found", resourceCollectionId))
		}
		h.logger.Error("find benchmark assignments by resource collection", zap.Error(err), zap.String("resourceCollectionId", resourceCollectionId))
		return err
	}

	span1.AddEvent("information", trace.WithAttributes(
		attribute.String("integration ID", resourceCollectionId),
	))
	span1.End()

	benchmarks, err := h.db.ListRootBenchmarks(ctx, nil)
	if err != nil {
		return err
	}

	result := make([]api.AssignedBenchmark, 0, len(dbAssignments))
	for _, benchmark := range benchmarks {
		res := api.AssignedBenchmark{
			Benchmark: benchmark.ToApi(),
			Status:    false,
		}
		for _, assignment := range dbAssignments {
			if assignment.ResourceCollection != nil && *assignment.ResourceCollection == resourceCollectionId && assignment.BenchmarkId == benchmark.ID {
				res.Status = true
				break
			}
		}
		result = append(result, res)
	}

	return echoCtx.JSON(http.StatusOK, result)
}

// ListAssignmentsByBenchmark godoc
//
//	@Summary		Get benchmark assigned sources
//	@Description	Retrieving all benchmark assigned sources with benchmark id
//	@Security		BearerToken
//	@Tags			benchmarks_assignment
//	@Accept			json
//	@Produce		json
//	@Param			benchmark_id	path		string	true	"Benchmark ID"
//	@Success		200				{object}	api.BenchmarkAssignedEntities
//	@Router			/compliance/api/v1/assignments/benchmark/{benchmark_id} [get]
func (h *HttpHandler) ListAssignmentsByBenchmark(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	benchmarkId := echoCtx.Param("benchmark_id")
	if benchmarkId == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "benchmark id is empty")
	}
	// trace :
	ctx, span1 := tracer.Start(ctx, "new_GetBenchmark", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_GetBenchmark")
	defer span1.End()

	benchmark, err := h.db.GetBenchmarkBare(ctx, benchmarkId)
	if err != nil {
		span1.RecordError(err)
		span1.SetStatus(codes.Error, err.Error())
		return err
	}
	span1.AddEvent("information", trace.WithAttributes(
		attribute.String("benchmark ID", benchmark.ID),
	))
	span1.End()

	hctx := &httpclient.Context{UserRole: authApi.AdminRole}

	var assignedIntegrations []api.BenchmarkAssignedIntegration

	for _, c := range benchmark.IntegrationType {
		integrations, err := h.integrationClient.ListIntegrations(hctx, []string{c})
		if err != nil {
			return err
		}

		for _, integration := range integrations.Integrations {
			if integration.State != integrationapi.IntegrationStateActive {
				continue
			}
			if err != nil {
				return err
			}
			ba := api.BenchmarkAssignedIntegration{
				IntegrationID:   integration.IntegrationID,
				ProviderID:      integration.ProviderID,
				IntegrationName: integration.Name,
				IntegrationType: integration_type.ParseType(c),
				Status:          false,
			}
			assignedIntegrations = append(assignedIntegrations, ba)
		}
	}

	// trace :
	ctx, span3 := tracer.Start(ctx, "new_GetBenchmarkAssignmentsByBenchmarkId", trace.WithSpanKind(trace.SpanKindServer))
	span3.SetName("new_GetBenchmarkAssignmentsByBenchmarkId")
	defer span3.End()

	dbAssignments, err := h.db.GetBenchmarkAssignmentsByBenchmarkId(ctx, benchmarkId)
	if err != nil {
		span3.RecordError(err)
		span3.SetStatus(codes.Error, err.Error())
		return err
	}
	span3.AddEvent("information", trace.WithAttributes(
		attribute.String("benchmark ID", benchmarkId),
	))
	span3.End()

	if benchmark.AutoAssign {
		for idx, r := range assignedIntegrations {
			r.Status = true
			assignedIntegrations[idx] = r
		}
	}

	for _, assignment := range dbAssignments {
		if assignment.IntegrationID != nil && !benchmark.AutoAssign {
			for idx, r := range assignedIntegrations {
				if r.IntegrationID == *assignment.IntegrationID {
					r.Status = true
					assignedIntegrations[idx] = r
				}
			}
		}
	}

	resp := api.BenchmarkAssignedEntities{}

	for _, item := range assignedIntegrations {
		if httpserver2.CheckAccessToConnectionID(echoCtx, item.IntegrationID) != nil {
			continue
		}
		resp.Integrations = append(resp.Integrations, item)
	}

	for idx, conn := range resp.Integrations {

		resp.Integrations[idx] = conn
	}

	return echoCtx.JSON(http.StatusOK, resp)
}

// DeleteBenchmarkAssignment godoc
//
//	@Summary		Delete benchmark assignment
//	@Description	Delete benchmark assignment with integration id and benchmark id
//	@Security		BearerToken
//	@Tags			benchmarks_assignment
//	@Accept			json
//	@Produce		json
//	@Param			benchmark_id		path	string		true	"Benchmark ID"
//	@Param			integrationId		query	[]string	false	"integration ID or 'all' for everything"
//	@Param			integrationGroup		query	[]string	false	"integration Group "
//	@Param			resourceCollection	query	[]string	false	"Resource Collection"
//	@Success		200
//	@Router			/compliance/api/v1/assignments/{benchmark_id}/integration [delete]
func (h *HttpHandler) DeleteBenchmarkAssignment(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	integrationIDs, err := h.getIntegrationIdFilterFromParams(echoCtx)
	if err != nil {
		return err
	}
	resourceCollections := httpserver2.QueryArrayParam(echoCtx, "resourceCollection")
	if len(integrationIDs) > 0 && len(resourceCollections) > 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "cannot specify both integration and resource collection")
	}

	benchmarkId := echoCtx.Param("benchmark_id")
	if benchmarkId == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "benchmark id is empty")
	}

	switch {
	case len(integrationIDs) > 0:
		if len(integrationIDs) == 1 && strings.ToLower(integrationIDs[0]) == "all" {
			//trace :
			ctx, span1 := tracer.Start(ctx, "new_DeleteBenchmarkAssignmentByBenchmarkId", trace.WithSpanKind(trace.SpanKindServer))
			span1.SetName("new_DeleteBenchmarkAssignmentByBenchmarkId")
			defer span1.End()

			if err := h.db.DeleteBenchmarkAssignmentByBenchmarkId(ctx, benchmarkId); err != nil {
				span1.RecordError(err)
				span1.SetStatus(codes.Error, err.Error())
				span1.End()
				h.logger.Error("delete benchmark assignment by benchmark id", zap.Error(err))
				return err
			}
			span1.AddEvent("information", trace.WithAttributes(
				attribute.String("benchmark ID", benchmarkId),
			))
			span1.End()
		} else {
			// tracer :
			ctx, span5 := tracer.Start(ctx, "new_GetBenchmarkAssignmentByIds(loop)", trace.WithSpanKind(trace.SpanKindServer))
			span5.SetName("new_GetBenchmarkAssignmentByIds(loop)")
			defer span5.End()

			for _, integrationId := range integrationIDs {
				// trace :
				ctx, span3 := tracer.Start(ctx, "new_GetBenchmarkAssignmentByIds", trace.WithSpanKind(trace.SpanKindServer))
				span3.SetName("new_GetBenchmarkAssignmentByIds")

				if _, err := h.db.GetBenchmarkAssignmentByIds(ctx, benchmarkId, utils.GetPointer(integrationId), nil); err != nil {
					span3.RecordError(err)
					span3.SetStatus(codes.Error, err.Error())
					span3.End()
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return echo.NewHTTPError(http.StatusFound, "benchmark assignment not found")
					}
					echoCtx.Logger().Errorf("find benchmark assignment: %v", err)
					return err
				}
				span3.AddEvent("information", trace.WithAttributes(
					attribute.String("benchmark ID", benchmarkId),
				))
				span3.End()

				// trace :
				ctx, span4 := tracer.Start(ctx, "new_DeleteBenchmarkAssignmentByIds", trace.WithSpanKind(trace.SpanKindServer))
				span4.SetName("new_DeleteBenchmarkAssignmentByIds")

				if err := h.db.DeleteBenchmarkAssignmentByIds(ctx, benchmarkId, utils.GetPointer(integrationId), nil); err != nil {
					span4.RecordError(err)
					span4.SetStatus(codes.Error, err.Error())
					span4.End()
					echoCtx.Logger().Errorf("delete benchmark assignment: %v", err)
					return err
				}
				span4.AddEvent("information", trace.WithAttributes(
					attribute.String("benchmark ID", benchmarkId),
				))
				span4.End()
			}
			span5.End()
		}
		return echoCtx.NoContent(http.StatusOK)
	case len(resourceCollections) > 0:
		// tracer :
		ctx, span6 := tracer.Start(ctx, "new_GetBenchmarkAssignmentByIds(loop)", trace.WithSpanKind(trace.SpanKindServer))
		span6.SetName("new_GetBenchmarkAssignmentByIds(loop)")
		defer span6.End()

		for _, resourceCollection := range resourceCollections {
			// trace :
			resourceCollection := resourceCollection
			ctx, span4 := tracer.Start(ctx, "new_GetBenchmarkAssignmentByIds", trace.WithSpanKind(trace.SpanKindServer))
			span4.SetName("new_GetBenchmarkAssignmentByIds")

			if _, err := h.db.GetBenchmarkAssignmentByIds(ctx, benchmarkId, nil, &resourceCollection); err != nil {
				span4.RecordError(err)
				span4.SetStatus(codes.Error, err.Error())
				span4.End()
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return echo.NewHTTPError(http.StatusFound, "benchmark assignment not found")
				}
				echoCtx.Logger().Errorf("find benchmark assignment: %v", err)
				return err
			}
			span4.AddEvent("information", trace.WithAttributes(
				attribute.String("benchmark ID", benchmarkId),
			))
			span4.End()

			// trace :
			ctx, span5 := tracer.Start(ctx, "new_DeleteBenchmarkAssignmentByIds", trace.WithSpanKind(trace.SpanKindServer))
			span5.SetName("new_DeleteBenchmarkAssignmentByIds")

			if err := h.db.DeleteBenchmarkAssignmentByIds(ctx, benchmarkId, nil, &resourceCollection); err != nil {
				span5.RecordError(err)
				span5.SetStatus(codes.Error, err.Error())
				span5.End()
				echoCtx.Logger().Errorf("delete benchmark assignment: %v", err)
				return err
			}
			span5.AddEvent("information", trace.WithAttributes(
				attribute.String("benchmark ID", benchmarkId),
			))
			span5.End()
		}
		span6.End()
		return echoCtx.NoContent(http.StatusOK)
	}
	return echo.NewHTTPError(http.StatusBadRequest, "integration or resource collection is required")
}

// ListBenchmarksFiltered godoc
//
//	@Summary	List benchmarks filtered by integrations and other filters
//	@Security	BearerToken
//	@Tags		compliance
//	@Accept		json
//	@Produce	json
//	@Param		request	body		api.GetBenchmarkListRequest	true	"Request Body"
//	@Success	200		{object}	[]api.GetBenchmarkListResponse
//	@Router		/compliance/api/v3/benchmarks [post]
func (h *HttpHandler) ListBenchmarksFiltered(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()
	clientCtx := &httpclient.Context{UserRole: authApi.AdminRole}

	var req api.GetBenchmarkListRequest
	if err := bindValidate(echoCtx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	isRoot := true
	if req.Root != nil {
		isRoot = *req.Root
	}

	benchmarkAssignmentsCount, err := h.db.GetBenchmarkAssignmentsCount()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	benchmarkAssignmentsCountMap := make(map[string]int)
	for _, ba := range benchmarkAssignmentsCount {
		benchmarkAssignmentsCountMap[ba.BenchmarkId] = ba.Count
	}
	integrationsCountByType := make(map[string]int)
	integrationsResp, err := h.integrationClient.ListIntegrations(clientCtx, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	for _, s := range integrationsResp.Integrations {
		if _, ok := integrationsCountByType[s.IntegrationType.String()]; ok {
			integrationsCountByType[s.IntegrationType.String()]++
		} else {
			integrationsCountByType[s.IntegrationType.String()] = 1
		}
	}

	var integrations []integrationapi.Integration
	for _, info := range req.Integration {
		if info.IntegrationID != nil {
			integration, err := h.integrationClient.GetIntegration(clientCtx, *info.IntegrationID)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			if integration != nil {
				integrations = append(integrations, *integration)
			}
			continue
		}
		var integrationTypes []string
		if info.IntegrationType != nil {
			integrationTypes = []string{*info.IntegrationType}
		}
		var integrationIDs []string
		if info.IntegrationID != nil {
			integrationIDs = []string{*info.IntegrationID}
		}
		integrationsTmp, err := h.integrationClient.ListIntegrationsByFilters(clientCtx,
			integrationapi.ListIntegrationsRequest{
				IntegrationType: integrationTypes,
				IntegrationID:   integrationIDs,
				NameRegex:       info.Name,
				ProviderIDRegex: info.ProviderID,
			})
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		integrations = append(integrations, integrationsTmp.Integrations...)
	}

	var integrationIDs []string
	for _, c := range integrations {
		integrationIDs = append(integrationIDs, c.IntegrationID)
	}

	benchmarks, err := h.db.ListBenchmarksFiltered(ctx, req.TitleRegex, isRoot, req.Tags, req.ParentBenchmarkID, req.Assigned, req.IsBaseline, integrationIDs)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	var items []api.GetBenchmarkListItem
	for _, b := range benchmarks {
		var incidentCount int
		complianceResults, err := h.getBenchmarkComplianceResultSummary(ctx, b.ID, req.ComplianceResultFilters)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		if req.ComplianceResultFilters != nil {
			if complianceResults == nil || complianceResults.Results == nil || len(complianceResults.Results) == 0 {
				continue
			}
		}
		if c, ok := complianceResults.Results[types2.ComplianceStatusALARM]; ok {
			incidentCount = c
		}

		metadata := db.BenchmarkMetadata{}

		if len(b.Metadata.Bytes) > 0 {
			err := json.Unmarshal(b.Metadata.Bytes, &metadata)
			if err != nil {
				h.logger.Error("failed to unmarshal metadata", zap.Error(err))
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
		}

		primaryTables := metadata.PrimaryTables
		listOfTables := metadata.ListOfTables

		if len(req.PrimaryTable) > 0 {
			if !listContainsList(primaryTables, req.PrimaryTable) {
				continue
			}
		}
		if len(req.ListOfTables) > 0 {
			if !listContainsList(listOfTables, req.ListOfTables) {
				continue
			}
		}
		if len(req.Controls) > 0 {
			if !listContainsList(metadata.Controls, req.Controls) {
				continue
			}
		}

		benchmarkDetails := api.GetBenchmarkListMetadata{
			ID:               b.ID,
			Title:            b.Title,
			Description:      b.Description,
			Enabled:          b.Enabled,
			TrackDriftEvents: b.TracksDriftEvents,
			NumberOfControls: len(metadata.Controls),
			AutoAssigned:     b.AutoAssign,
			PrimaryTables:    primaryTables,
			Tags:             filterTagsByRegex(req.TagsRegex, model.TrimPrivateTags(b.GetTagsMap())),
			CreatedAt:        b.CreatedAt,
			UpdatedAt:        b.UpdatedAt,
		}
		if b.IntegrationType != nil {
			if len(req.IntegrationTypes) > 0 {
				if !listContainsList(b.IntegrationType, req.IntegrationTypes) {
					continue
				}
			}
			benchmarkDetails.IntegrationType = b.IntegrationType
		}
		if b.AutoAssign {
			for _, c := range b.IntegrationType {
				benchmarkDetails.NumberOfAssignments = benchmarkDetails.NumberOfAssignments + integrationsCountByType[c]
			}
		}
		if bac, ok := benchmarkAssignmentsCountMap[b.ID]; ok {
			benchmarkDetails.NumberOfAssignments = benchmarkDetails.NumberOfAssignments + bac
		}

		benchmarkResult := api.GetBenchmarkListItem{
			Benchmark:     benchmarkDetails,
			IncidentCount: incidentCount,
		}
		items = append(items, benchmarkResult)
	}

	totalCount := len(items)

	switch strings.ToLower(req.SortBy) {
	case "assignments", "number_of_assignments":
		sort.Slice(items, func(i, j int) bool {
			return items[i].Benchmark.NumberOfAssignments > items[j].Benchmark.NumberOfAssignments
		})
	case "incidents", "number_of_incidents":
		sort.Slice(items, func(i, j int) bool {
			return items[i].IncidentCount > items[j].IncidentCount
		})
	case "title":
		sort.Slice(items, func(i, j int) bool {
			return items[i].Benchmark.Title < items[j].Benchmark.Title
		})
	}

	if req.PerPage != nil {
		if req.Cursor == nil {
			items = utils.Paginate(1, *req.PerPage, items)
		} else {
			items = utils.Paginate(*req.Cursor, *req.PerPage, items)
		}
	}

	response := api.GetBenchmarkListResponse{
		Items:      items,
		TotalCount: totalCount,
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// GetBenchmarkDetails godoc
//
//	@Summary	Get Benchmark Details by BenchmarkID
//	@Security	BearerToken
//	@Tags		compliance
//	@Accept		json
//	@Produce	json
//	@Param		request			body		api.GetBenchmarkDetailsRequest	true	"Request Body"
//	@Param		benchmark_id	path		string							true	"benchmark id to get the details for"
//	@Success	200				{object}	[]api.GetBenchmarkDetailsResponse
//	@Router		/compliance/api/v3/benchmark/{benchmark_id} [get]
func (h *HttpHandler) GetBenchmarkDetails(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	benchmarkId := echoCtx.Param("benchmark_id")

	var req api.GetBenchmarkDetailsRequest
	if err := bindValidate(echoCtx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// trace :
	ctx, span1 := tracer.Start(ctx, "new_GetBenchmark", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_GetBenchmark")
	defer span1.End()

	benchmark, err := h.db.GetBenchmark(ctx, benchmarkId)
	if err != nil {
		span1.RecordError(err)
		span1.SetStatus(codes.Error, err.Error())
		return err
	}
	if benchmark == nil {
		return echo.NewHTTPError(http.StatusNotFound, "benchmark not found")
	}
	span1.AddEvent("information", trace.WithAttributes(
		attribute.String("benchmark ID", benchmark.ID),
	))
	span1.End()

	complianceResultsResult, err := h.getBenchmarkComplianceResultSummary(ctx, benchmark.ID, req.ComplianceResultFilters)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "complianceResults not found")
	}

	metadata := db.BenchmarkMetadata{}

	if len(benchmark.Metadata.Bytes) > 0 {
		err := json.Unmarshal(benchmark.Metadata.Bytes, &metadata)
		if err != nil {
			h.logger.Error("failed to unmarshal metadata", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}
	primaryTables := metadata.PrimaryTables
	listOfTables := metadata.ListOfTables

	benchmarkMetadata := api.GetBenchmarkDetailsMetadata{
		ID:                benchmark.ID,
		Title:             benchmark.Title,
		Description:       benchmark.Description,
		Enabled:           benchmark.Enabled,
		TrackDriftEvents:  benchmark.TracksDriftEvents,
		SupportedControls: metadata.Controls,
		NumberOfControls:  len(metadata.Controls),
		PrimaryTables:     primaryTables,
		ListOfTables:      listOfTables,
		Tags:              filterTagsByRegex(req.TagsRegex, model.TrimPrivateTags(benchmark.GetTagsMap())),
		CreatedAt:         benchmark.CreatedAt,
		UpdatedAt:         benchmark.UpdatedAt,
	}
	if benchmark.IntegrationType != nil {
		benchmarkMetadata.IntegrationTypes = integration_type.ParseTypes(benchmark.IntegrationType)
	}

	children, err := h.getChildBenchmarksWithDetails(ctx, benchmark.ID, req)

	return echoCtx.JSON(http.StatusOK, api.GetBenchmarkDetailsResponse{
		Metadata:          benchmarkMetadata,
		ComplianceResults: *complianceResultsResult,
		Children:          children,
	})
}

func (h *HttpHandler) ListBenchmarks(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	var response []api.Benchmark
	// trace :
	ctx, span1 := tracer.Start(ctx, "new_ListRootBenchmarks", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_ListRootBenchmarks")
	defer span1.End()
	tagMap := model.TagStringsToTagMap(httpserver2.QueryArrayParam(echoCtx, "tag"))

	benchmarks, err := h.db.ListRootBenchmarks(ctx, tagMap)
	if err != nil {
		span1.RecordError(err)
		span1.SetStatus(codes.Error, err.Error())
		return err
	}
	span1.End()

	// tracer :
	ctx, span2 := tracer.Start(ctx, "new_PopulateIntegrationTypes(loop)", trace.WithSpanKind(trace.SpanKindServer))
	span2.SetName("new_PopulateIntegrationTypes(loop)")
	defer span2.End()

	for _, b := range benchmarks {
		response = append(response, b.ToApi())
	}
	span2.End()

	return echoCtx.JSON(http.StatusOK, response)
}

func (h *HttpHandler) ListAllBenchmarks(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	isBare := true
	if bare := echoCtx.QueryParam("bare"); bare != "" {
		var err error
		isBare, err = strconv.ParseBool(bare)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid bare value")
		}
	}

	var response []api.Benchmark
	// trace :
	ctx, span1 := tracer.Start(ctx, "new_ListRootBenchmarks", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_ListBenchmarks")
	defer span1.End()
	var benchmarks []db.Benchmark
	var err error
	if isBare {
		benchmarks, err = h.db.ListBenchmarksBare(ctx)
	} else {
		benchmarks, err = h.db.ListBenchmarks(ctx)
	}
	if err != nil {
		span1.RecordError(err)
		span1.SetStatus(codes.Error, err.Error())
		return err
	}
	span1.End()

	for _, b := range benchmarks {
		response = append(response, b.ToApi())
	}

	return echoCtx.JSON(http.StatusOK, response)
}

func (h *HttpHandler) GetBenchmark(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	benchmarkId := echoCtx.Param("benchmark_id")
	// trace :
	ctx, span1 := tracer.Start(ctx, "new_GetBenchmark", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_GetBenchmark")
	defer span1.End()

	benchmark, err := h.db.GetBenchmark(ctx, benchmarkId)
	if err != nil {
		span1.RecordError(err)
		span1.SetStatus(codes.Error, err.Error())
		return err
	}
	span1.AddEvent("information", trace.WithAttributes(
		attribute.String("benchmark ID", benchmark.ID),
	))
	span1.End()

	if benchmark == nil {
		return echo.NewHTTPError(http.StatusNotFound, "benchmark not found")
	}

	return echoCtx.JSON(http.StatusOK, benchmark.ToApi())
}

func (h *HttpHandler) getBenchmarkControls(ctx context.Context, benchmarkID string) ([]db.Control, error) {
	//trace :
	ctx, span1 := tracer.Start(ctx, "new_GetBenchmark", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_GetBenchmark")
	defer span1.End()

	b, err := h.db.GetBenchmark(ctx, benchmarkID)
	if err != nil {
		span1.RecordError(err)
		span1.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	span1.AddEvent("information", trace.WithAttributes(
		attribute.String("benchmark ID", b.ID),
	))
	span1.End()

	if b == nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "benchmark not found")
	}

	var controlIDs []string
	for _, p := range b.Controls {
		controlIDs = append(controlIDs, p.ID)
	}
	//trace :
	ctx, span2 := tracer.Start(ctx, "new_GetControls", trace.WithSpanKind(trace.SpanKindServer))
	span2.SetName("new_GetControls")
	defer span2.End()

	controls, err := h.db.GetControls(ctx, controlIDs, nil)
	if err != nil {
		span2.RecordError(err)
		span2.SetStatus(codes.Error, err.Error())
		span2.End()
		return nil, err
	}
	span2.End()

	//tracer :
	ctx, span3 := tracer.Start(ctx, "new_getBenchmarkControls(loop)", trace.WithSpanKind(trace.SpanKindServer))
	span3.SetName("new_getBenchmarkControls(loop)")
	defer span3.End()

	for _, child := range b.Children {
		// tracer :
		ctx, span4 := tracer.Start(ctx, "new_getBenchmarkControls", trace.WithSpanKind(trace.SpanKindServer))
		span4.SetName("new_getBenchmarkControls")

		childControls, err := h.getBenchmarkControls(ctx, child.ID)
		if err != nil {
			span4.RecordError(err)
			span4.SetStatus(codes.Error, err.Error())
			span4.End()
			return nil, err
		}
		span4.SetAttributes(
			attribute.String("benchmark ID", child.ID),
		)
		span4.End()

		controls = append(controls, childControls...)
	}
	span3.End()

	return controls, nil
}

func (h *HttpHandler) GetControl(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	controlId := echoCtx.Param("control_id")
	// trace :
	ctx, span1 := tracer.Start(ctx, "new_GetControl", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_GetControl")
	defer span1.End()

	control, err := h.db.GetControl(ctx, controlId)
	if err != nil {
		span1.RecordError(err)
		span1.SetStatus(codes.Error, err.Error())
		h.logger.Error("failed to fetch control", zap.Error(err), zap.String("controlId", controlId))
		return err
	}
	span1.AddEvent("information", trace.WithAttributes(
		attribute.String("control ID", controlId),
	))
	span1.End()

	if control == nil {
		return echo.NewHTTPError(http.StatusNotFound, "control not found")
	}

	pa := control.ToApi()
	// trace :
	ctx, span2 := tracer.Start(ctx, "new_PopulateIntegrationType", trace.WithSpanKind(trace.SpanKindServer))
	span2.SetName("new_PopulateIntegrationType")
	defer span2.End()

	err = control.PopulateIntegrationType(ctx, h.db, &pa)
	if err != nil {
		span2.RecordError(err)
		span2.SetStatus(codes.Error, err.Error())
		h.logger.Error("failed to populate integration type", zap.Error(err), zap.String("controlId", controlId))
		return err
	}
	span2.End()
	return echoCtx.JSON(http.StatusOK, pa)
}

func (h *HttpHandler) ListControls(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	controlIDs := httpserver2.QueryArrayParam(echoCtx, "control_id")
	tagMap := model.TagStringsToTagMap(httpserver2.QueryArrayParam(echoCtx, "tag"))

	controls, err := h.db.ListControls(ctx, controlIDs, tagMap)
	if err != nil {
		return err
	}

	var resp []api.Control
	for _, control := range controls {
		pa := control.ToApi()
		resp = append(resp, pa)
	}
	return echoCtx.JSON(http.StatusOK, resp)
}

func (h *HttpHandler) ListQueries(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	queries, err := h.db.ListQueries(ctx)
	if err != nil {
		return err
	}

	var resp []api.Query
	for _, query := range queries {
		pa := query.ToApi()
		resp = append(resp, pa)
	}
	return echoCtx.JSON(http.StatusOK, resp)
}

// ListBenchmarksTags godoc
//
//	@Summary		List benchmarks tags
//	@Description	Retrieving list of benchmark possible tags
//	@Security		BearerToken
//	@Tags			compliance
//	@Produce		json
//	@Success		200	{object}	[]api.BenchmarkTagsResult
//	@Router			/compliance/api/v3/benchmarks/tags [get]
func (h *HttpHandler) ListBenchmarksTags(ctx echo.Context) error {
	// trace :
	_, span := tracer.Start(ctx.Request().Context(), "new_ListBenchmarksTags", trace.WithSpanKind(trace.SpanKindServer))
	span.SetName("new_ListBenchmarksTags")

	controlsTags, err := h.db.GetBenchmarksTags()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	res := make([]api.BenchmarkTagsResult, 0, len(controlsTags))
	for _, history := range controlsTags {
		res = append(res, history.ToApi())
	}

	span.End()

	return ctx.JSON(200, res)
}

func (h *HttpHandler) GetQuery(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	queryID := echoCtx.Param("query_id")
	// trace :
	ctx, span1 := tracer.Start(ctx, "new_GetQuery", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_GetQuery")
	defer span1.End()

	q, err := h.db.GetQuery(ctx, queryID)
	if err != nil {
		span1.RecordError(err)
		span1.SetStatus(codes.Error, err.Error())
		return err
	}
	span1.AddEvent("information", trace.WithAttributes(
		attribute.String("query ID", queryID),
	))
	span1.End()

	if q == nil {
		return echo.NewHTTPError(http.StatusNotFound, "query not found")
	}

	return echoCtx.JSON(http.StatusOK, q.ToApi())
}

// SyncQueries godoc
//
//	@Summary		Sync queries
//
//	@Description	Syncs queries with the git backend.
//
//	@Security		BearerToken
//	@Tags			compliance
//	@Param			configzGitURL	query	string	false	"Git URL"
//	@Accept			json
//	@Produce		json
//	@Success		200
//	@Router			/compliance/api/v1/queries/sync [get]
func (h *HttpHandler) SyncQueries(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	var mig *model2.Migration
	tx := h.migratorDb.Orm.Model(&model2.Migration{}).Where("id = ?", "main").First(&mig)
	if tx.Error != nil && !errors.Is(tx.Error, gorm.ErrRecordNotFound) {
		h.logger.Error("failed to get migration", zap.Error(tx.Error))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get migration")
	}
	if mig != nil {
		if mig.Status == "PENDING" || mig.Status == "IN_PROGRESS" {
			return echo.NewHTTPError(http.StatusBadRequest, "sync sample data already in progress")
		}
	}

	enabled, err := h.metadataClient.GetConfigMetadata(&httpclient.Context{UserRole: authApi.AdminRole}, models.MetadataKeyCustomizationEnabled)
	if err != nil {
		h.logger.Error("get config metadata", zap.Error(err))
		return err
	}

	if !enabled.GetValue().(bool) {
		return echo.NewHTTPError(http.StatusForbidden, "customization is not allowed")
	}

	configzGitURL := echoCtx.QueryParam("configzGitURL")
	if configzGitURL != "" {
		// validate url
		_, err := url.ParseRequestURI(configzGitURL)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid url")
		}

		err = h.metadataClient.SetConfigMetadata(&httpclient.Context{UserRole: authApi.AdminRole}, models.MetadataKeyAnalyticsGitURL, configzGitURL)
		if err != nil {
			h.logger.Error("set config metadata", zap.Error(err))
			return err
		}
	}

	currentNamespace, ok := os.LookupEnv("CURRENT_NAMESPACE")
	if !ok {
		return errors.New("current namespace lookup failed")
	}

	var migratorJob batchv1.Job
	err = h.kubeClient.Get(ctx, k8sclient.ObjectKey{
		Namespace: currentNamespace,
		Name:      "post-install-configuration",
	}, &migratorJob)
	if err != nil {
		return err
	}

	err = h.kubeClient.Delete(ctx, &migratorJob)
	if err != nil {
		return err
	}
	envsMap := make(map[string]corev1.EnvVar)
	for _, env := range migratorJob.Spec.Template.Spec.Containers[0].Env {
		envsMap[env.Name] = env
	}
	envsMap["IS_MANUAL"] = corev1.EnvVar{
		Name:  "IS_MANUAL",
		Value: "true",
	}
	var newEnvs []corev1.EnvVar
	for _, v := range envsMap {
		newEnvs = append(newEnvs, v)
	}
	for {
		err = h.kubeClient.Get(ctx, k8sclient.ObjectKey{
			Namespace: currentNamespace,
			Name:      "post-install-configuration",
		}, &migratorJob)
		if err != nil {
			if k8sclient.IgnoreNotFound(err) == nil {
				break
			}
			return err
		}

		time.Sleep(1 * time.Second)
	}

	migratorJob.ObjectMeta = metav1.ObjectMeta{
		Name:      "post-install-configuration",
		Namespace: currentNamespace,
		Annotations: map[string]string{
			"helm.sh/hook":        "post-install,post-upgrade",
			"helm.sh/hook-weight": "0",
		},
	}
	migratorJob.Spec.Selector = nil
	migratorJob.Spec.Suspend = aws.Bool(false)
	migratorJob.Spec.Template.ObjectMeta = metav1.ObjectMeta{}
	migratorJob.Spec.Template.Spec.Containers[0].Env = newEnvs
	migratorJob.Status = batchv1.JobStatus{}

	err = h.kubeClient.Create(ctx, &migratorJob)
	if err != nil {
		return err
	}

	//err := h.syncJobsQueue.Publish([]byte{})
	//if err != nil {
	//	h.logger.Error("publish sync jobs", zap.Error(err))
	//	return err
	//}
	jp := pgtype.JSONB{}
	err = jp.Set([]byte(""))
	if err != nil {
		return err
	}
	tx = h.migratorDb.Orm.Model(&model2.Migration{}).Where("id = ?", "main").Update("status", "Started").Update("jobs_status", jp)
	if tx.Error != nil && !errors.Is(tx.Error, gorm.ErrRecordNotFound) {
		h.logger.Error("failed to update migration", zap.Error(tx.Error))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update migration")
	}

	return echoCtx.JSON(http.StatusOK, struct{}{})
}

// ListComplianceTags godoc
//
//	@Summary		List compliance tag keys
//	@Description	Retrieving a list of compliance tag keys with their possible values.
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string][]string
//	@Router			/compliance/api/v1/metadata/tag/compliance [get]
func (h *HttpHandler) ListComplianceTags(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	// trace :
	ctx, span1 := tracer.Start(ctx, "new_ListComplianceTagKeysWithPossibleValues", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_ListComplianceTagKeysWithPossibleValues")
	defer span1.End()

	tags, err := h.db.ListComplianceTagKeysWithPossibleValues(ctx)
	if err != nil {
		span1.RecordError(err)
		span1.SetStatus(codes.Error, err.Error())
		return err
	}

	tags = model.TrimPrivateTags(tags)
	return echoCtx.JSON(http.StatusOK, tags)
}

// GetBenchmarkAssignments godoc
//
//	@Summary	Get Benchmark Assignments by BenchmarkID
//	@Security	BearerToken
//	@Tags		compliance
//	@Accept		json
//	@Produce	json
//	@Param		assignment_type		query		string	true	"assignment type. options: implicit, explicit, any"
//	@Param		include_potential	query		bool	true	"Include potentials"
//	@Param		benchmark-id		path		string	true	"Benchmark ID"
//	@Success	200					{object}	[]api.IntegrationInfo
//	@Router		/compliance/api/v3/benchmark/{benchmark-id}/assignments [get]
func (h *HttpHandler) GetBenchmarkAssignments(echoCtx echo.Context) error {
	clientCtx := &httpclient.Context{UserRole: authApi.AdminRole}

	ctx := echoCtx.Request().Context()

	benchmarkId := echoCtx.Param("benchmark_id")
	assignmentType := strings.ToLower(echoCtx.QueryParam("assignment_type"))
	if assignmentType == "" {
		assignmentType = "any"
	}

	includePotential := true
	if strings.ToLower(echoCtx.QueryParam("include_potential")) == "false" {
		includePotential = false
	}

	// trace :
	ctx, span1 := tracer.Start(ctx, "new_GetBenchmarkAssignments", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_GetBenchmarkAssignments")
	defer span1.End()

	integrationInfos := make(map[string]api.GetBenchmarkAssignmentsItem)
	benchmark, err := h.db.GetBenchmark(ctx, benchmarkId)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if includePotential {
		integrations, err := h.integrationClient.ListIntegrations(clientCtx, benchmark.IntegrationType)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		for _, integration := range integrations.Integrations {
			if integration.State != integrationapi.IntegrationStateActive {
				continue
			}
			integrationInfos[integration.IntegrationID] = api.GetBenchmarkAssignmentsItem{
				Integration: api.IntegrationInfo{
					IntegrationID:   &integration.IntegrationID,
					IntegrationType: string(integration.IntegrationType),
					ProviderID:      &integration.ProviderID,
					Name:            &integration.Name,
				},
				AssignmentChangePossible: true,
				AssignmentType:           nil,
				Assigned:                 false,
			}
		}
	}

	if assignmentType == "explicit" || assignmentType == "any" {
		assignments, err := h.db.GetBenchmarkAssignmentsByBenchmarkId(ctx, benchmarkId)
		if err != nil {
			h.logger.Error("cannot get explicit assignments", zap.Error(err))
			return echo.NewHTTPError(http.StatusBadRequest, "cannot get explicit assignments")
		}
		assignmentType2 := "explicit"
		for _, assignment := range assignments {
			if assignment.IntegrationID != nil {
				integration, err := h.integrationClient.GetIntegration(clientCtx, *assignment.IntegrationID)
				if err != nil {
					return echo.NewHTTPError(http.StatusBadRequest, err.Error())
				}
				integrationInfos[*assignment.IntegrationID] = api.GetBenchmarkAssignmentsItem{
					Integration: api.IntegrationInfo{
						IntegrationID:   &integration.IntegrationID,
						IntegrationType: string(integration.IntegrationType),
						ProviderID:      &integration.ProviderID,
						Name:            &integration.Name,
					},
					Assigned:                 true,
					AssignmentChangePossible: true,
					AssignmentType:           &assignmentType2,
				}
			}
		}
	}
	if assignmentType == "implicit" || assignmentType == "any" {
		assignmentType2 := "implicit"

		if benchmark.AutoAssign {
			integrations, err := h.integrationClient.ListIntegrations(clientCtx, benchmark.IntegrationType)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			for _, integration := range integrations.Integrations {
				if integration.State != integrationapi.IntegrationStateActive {
					continue
				}
				integrationInfos[integration.IntegrationID] = api.GetBenchmarkAssignmentsItem{
					Integration: api.IntegrationInfo{
						IntegrationID:   &integration.IntegrationID,
						IntegrationType: integration.IntegrationType.String(),
						ProviderID:      &integration.ProviderID,
						Name:            &integration.Name,
					},
					Assigned:                 true,
					AssignmentChangePossible: false,
					AssignmentType:           &assignmentType2,
				}
			}
		}
	}

	assignedCount := 0
	var results []api.GetBenchmarkAssignmentsItem
	for _, info := range integrationInfos {
		results = append(results, info)
		if info.Assigned {
			assignedCount++
		}
	}

	var status api.BenchmarkAssignmentStatus
	if benchmark.AutoAssign {
		status = api.BenchmarkAssignmentStatusAutoEnable
	} else if assignedCount > 0 {
		status = api.BenchmarkAssignmentStatusEnabled
	} else {
		status = api.BenchmarkAssignmentStatusDisabled
	}

	return echoCtx.JSON(http.StatusOK, api.GetBenchmarkAssignmentsResponse{
		Items:  results,
		Status: status,
	})
}

// GetComplianceResultV2 godoc
//
//	@Summary		Get complianceResults
//	@Description	Retrieving all compliance run complianceResults with respect to filters.
//	@Tags			compliance
//	@Security		BearerToken
//	@Accept			json
//	@Produce		json
//	@Param			request	body		api.GetComplianceResultsRequestV2	true	"Request Body"
//	@Success		200		{object}	api.GetComplianceResultsResponse
//	@Router			/compliance/api/v3/compliance_result [post]
func (h *HttpHandler) GetComplianceResultV2(echoCtx echo.Context) error {
	clientCtx := &httpclient.Context{UserRole: authApi.AdminRole}

	ctx := echoCtx.Request().Context()

	var req api.GetComplianceResultsRequestV2
	if err := bindValidate(echoCtx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	var integrations []integrationapi.Integration
	for _, info := range req.Filters.Integration {
		if info.IntegrationID != nil {
			integration, err := h.integrationClient.GetIntegration(clientCtx, *info.IntegrationID)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			if integration != nil {
				integrations = append(integrations, *integration)
			}
			continue
		}
		var integrationTypes []string
		if info.IntegrationType != nil {
			integrationTypes = []string{*info.IntegrationType}
		}
		var integrationIDs []string
		if info.IntegrationID != nil {
			integrationIDs = []string{*info.IntegrationID}
		}
		integrationsTmp, err := h.integrationClient.ListIntegrationsByFilters(clientCtx,
			integrationapi.ListIntegrationsRequest{
				IntegrationType: integrationTypes,
				NameRegex:       info.Name,
				ProviderIDRegex: info.ProviderID,
				IntegrationID:   integrationIDs,
			})
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		integrations = append(integrations, integrationsTmp.Integrations...)
	}

	var integrationIds []string
	for _, c := range integrations {
		integrationIds = append(integrationIds, c.IntegrationID)
	}

	var err error
	//integrationIds, err = httpserver2.ResolveIntegrationIDs(echoCtx, integrationIds)
	//if err != nil {
	//	return err
	//}

	var response api.GetComplianceResultsResponse

	var complianceStatuses []api.ComplianceStatus
	if len(req.Filters.IsCompliant) == 0 {
		complianceStatuses = []api.ComplianceStatus{api.ComplianceStatusFailed}
	} else {
		for _, s := range req.Filters.IsCompliant {
			if s {
				complianceStatuses = append(complianceStatuses, api.ComplianceStatusPassed)
			} else {
				complianceStatuses = append(complianceStatuses, api.ComplianceStatusFailed)
			}
		}
	}

	esComplianceStatuses := make([]opengovernanceTypes.ComplianceStatus, 0, len(complianceStatuses))
	for _, status := range complianceStatuses {
		esComplianceStatuses = append(esComplianceStatuses, status.GetEsComplianceStatuses()...)
	}

	if len(req.Sort) == 0 {
		req.Sort = []api.ComplianceResultsSortV2{
			{ComplianceStatus: utils.GetPointer(api.SortDirectionDescending)},
		}
	}

	if len(req.AfterSortKey) != 0 {
		expectedLen := len(req.Sort) + 1
		if len(req.AfterSortKey) != expectedLen {
			return echo.NewHTTPError(http.StatusBadRequest, "sort key length should be zero or match a returned sort key from previous response")
		}
	}

	var lastEventFrom, lastEventTo, evaluatedAtFrom, evaluatedAtTo *time.Time
	var notLastEventFrom, notLastEventTo *time.Time
	if req.Filters.LastUpdated.From != nil && *req.Filters.LastUpdated.From != 0 {
		lastEventFrom = utils.GetPointer(time.Unix(*req.Filters.LastUpdated.From, 0))
	}
	if req.Filters.LastUpdated.To != nil && *req.Filters.LastUpdated.To != 0 {
		lastEventTo = utils.GetPointer(time.Unix(*req.Filters.LastUpdated.To, 0))
	}
	if req.Filters.NotLastUpdated.From != nil && *req.Filters.NotLastUpdated.From != 0 {
		notLastEventFrom = utils.GetPointer(time.Unix(*req.Filters.NotLastUpdated.From, 0))
	}
	if req.Filters.NotLastUpdated.To != nil && *req.Filters.NotLastUpdated.To != 0 {
		notLastEventTo = utils.GetPointer(time.Unix(*req.Filters.NotLastUpdated.To, 0))
	}

	res, totalCount, err := es.ComplianceResultsQueryV2(ctx, h.logger, h.client, req.Filters.ResourceID, req.Filters.NotResourceID, nil,
		integrationIds, nil, req.Filters.ResourceType, req.Filters.NotResourceType, req.Filters.BenchmarkID,
		req.Filters.NotBenchmarkID, req.Filters.ControlID, req.Filters.NotControlID,
		req.Filters.Severity, req.Filters.NotSeverity, lastEventFrom, lastEventTo, notLastEventFrom, notLastEventTo,
		evaluatedAtFrom, evaluatedAtTo, req.Filters.IsActive, esComplianceStatuses, req.Sort, req.Limit, req.AfterSortKey)
	if err != nil {
		h.logger.Error("failed to get complianceResults", zap.Error(err))
		return err
	}

	allIntegrations, err := h.integrationClient.ListIntegrations(&httpclient.Context{UserRole: authApi.AdminRole}, nil)
	if err != nil {
		h.logger.Error("failed to get sources", zap.Error(err))
		return err
	}
	allIntegrationsMap := make(map[string]*integrationapi.Integration)
	for _, src := range allIntegrations.Integrations {
		src := src
		allIntegrationsMap[src.IntegrationID] = &src
	}

	controls, err := h.db.ListControls(ctx, nil, nil)
	if err != nil {
		h.logger.Error("failed to get controls", zap.Error(err))
		return err
	}
	controlsMap := make(map[string]*db.Control)
	for _, control := range controls {
		control := control
		controlsMap[control.ID] = &control
	}

	benchmarks, err := h.db.ListBenchmarksBare(ctx)
	if err != nil {
		h.logger.Error("failed to get benchmarks", zap.Error(err))
		return err
	}
	benchmarksMap := make(map[string]*db.Benchmark)
	for _, benchmark := range benchmarks {
		benchmark := benchmark
		benchmarksMap[benchmark.ID] = &benchmark
	}

	resourceTypeMetadata, err := h.inventoryClient.ListResourceTypesMetadata(&httpclient.Context{UserRole: authApi.AdminRole},
		nil, nil, nil, false, nil, 10000, 1)
	if err != nil {
		h.logger.Error("failed to get resource type metadata", zap.Error(err))
		return err
	}
	resourceTypeMetadataMap := make(map[string]*inventoryApi.ResourceType)
	for _, item := range resourceTypeMetadata.ResourceTypes {
		item := item
		resourceTypeMetadataMap[strings.ToLower(item.ResourceType)] = &item
	}

	for _, h := range res {
		finding := api.GetAPIComplianceResultFromESComplianceResult(h.Source)

		for _, parentBenchmark := range h.Source.ParentBenchmarks {
			if benchmark, ok := benchmarksMap[parentBenchmark]; ok {
				finding.ParentBenchmarkNames = append(finding.ParentBenchmarkNames, benchmark.Title)
			}
		}

		if control, ok := controlsMap[finding.ControlID]; ok {
			finding.ControlTitle = control.Title
		}

		if rtMetadata, ok := resourceTypeMetadataMap[strings.ToLower(finding.ResourceType)]; ok {
			finding.ResourceTypeName = rtMetadata.ResourceLabel
		}

		finding.SortKey = h.Sort

		response.ComplianceResults = append(response.ComplianceResults, finding)
	}
	response.TotalCount = totalCount

	platformResourceIDs := make([]string, 0, len(response.ComplianceResults))
	for _, finding := range response.ComplianceResults {
		platformResourceIDs = append(platformResourceIDs, finding.PlatformResourceID)
	}

	lookupResourcesMap, err := es.FetchLookupByResourceIDBatch(ctx, h.client, platformResourceIDs)
	if err != nil {
		h.logger.Error("failed to fetch lookup resources", zap.Error(err))
		return err
	}

	for i, finding := range response.ComplianceResults {
		var lookupResource *es2.LookupResource
		potentialResources := lookupResourcesMap[finding.PlatformResourceID]
		for _, r := range potentialResources {
			r := r
			if strings.ToLower(r.ResourceType) == strings.ToLower(finding.ResourceType) {
				lookupResource = &r
				break
			}
		}
		if lookupResource != nil {
			response.ComplianceResults[i].ResourceName = lookupResource.ResourceName
		} else {
			h.logger.Warn("lookup resource not found",
				zap.String("og_resource_id", finding.PlatformResourceID),
				zap.String("resource_id", finding.ResourceID),
				zap.String("controlId", finding.ControlID),
			)
		}
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// AssignBenchmarkToIntegration godoc
//
//	@Summary		Create benchmark assignment
//	@Description	Creating a benchmark assignment for an integration.
//	@Security		BearerToken
//	@Tags			benchmarks_assignment
//	@Accept			json
//	@Produce		json
//	@Param			benchmark_id	path	string							true	"Benchmark ID to assign"
//	@Param			request			body	api.IntegrationFilterRequest	true	"Integrations details to be assigned"
//	@Success		200
//	@Router			/compliance/api/v3/benchmark/{benchmark_id}/assign [post]
func (h *HttpHandler) AssignBenchmarkToIntegration(echoCtx echo.Context) error {
	clientCtx := &httpclient.Context{UserRole: authApi.AdminRole}
	ctx := echoCtx.Request().Context()

	var req api.IntegrationFilterRequest
	if err := bindValidate(echoCtx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	var integrations []integrationapi.Integration
	for _, info := range req.Integration {
		if info.IntegrationID != nil {
			integration, err := h.integrationClient.GetIntegration(clientCtx, *info.IntegrationID)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			if integration != nil {
				integrations = append(integrations, *integration)
			}
			continue
		}
		var integrationTypes []string
		if info.IntegrationType != nil {
			integrationTypes = []string{*info.IntegrationType}
		}
		var integrationIDs []string
		if info.IntegrationID != nil {
			integrationIDs = []string{*info.IntegrationID}
		}
		integrationsTmp, err := h.integrationClient.ListIntegrationsByFilters(clientCtx,
			integrationapi.ListIntegrationsRequest{
				IntegrationType: integrationTypes,
				NameRegex:       info.Name,
				ProviderIDRegex: info.ProviderID,
				IntegrationID:   integrationIDs,
			})
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		integrations = append(integrations, integrationsTmp.Integrations...)
	}

	benchmarkId := echoCtx.Param("benchmark_id")
	if benchmarkId == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "benchmark id is empty")
	}
	// trace :
	ctx, span1 := tracer.Start(ctx, "new_GetBenchmark", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_GetBenchmark")
	defer span1.End()

	benchmark, err := h.db.GetBenchmark(ctx, benchmarkId)

	if err != nil {
		span1.RecordError(err)
		span1.SetStatus(codes.Error, err.Error())
		return err
	}
	if benchmark == nil {
		return echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("benchmark %s not found", benchmarkId))
	}

	span1.AddEvent("information", trace.WithAttributes(
		attribute.String("benchmark Id", benchmark.ID),
	))
	span1.End()

	ctx, span4 := tracer.Start(ctx, "new_AddBenchmarkAssignment(loop)", trace.WithSpanKind(trace.SpanKindServer))
	span4.SetName("new_AddBenchmarkAssignment(loop)")
	defer span4.End()

	for _, src := range integrations {
		assignment := &db.BenchmarkAssignment{
			BenchmarkId:   benchmarkId,
			IntegrationID: utils.GetPointer(src.IntegrationID),
			AssignedAt:    time.Now(),
		}
		//trace :
		ctx, span5 := tracer.Start(ctx, "new_AddBenchmarkAssignment", trace.WithSpanKind(trace.SpanKindServer))
		span5.SetName("new_AddBenchmarkAssignment")

		if err := h.db.AddBenchmarkAssignment(ctx, assignment); err != nil {
			span5.RecordError(err)
			span5.SetStatus(codes.Error, err.Error())
			span5.End()
			echoCtx.Logger().Errorf("add benchmark assignment: %v", err)
			return err
		}
		span5.SetAttributes(
			attribute.String("Benchmark ID", assignment.BenchmarkId),
		)
		span5.End()
	}
	span4.End()
	h.logger.Info("integrations assignments checked")

	if req.AutoEnable {
		err = h.db.SetBenchmarkAutoAssign(ctx, benchmarkId, true)
		if err != nil {
			h.logger.Error("failed to set auto assign", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to set auto assign")
		}
	}
	h.logger.Info("auto enable checked")
	if req.Disable {
		err = h.db.SetBenchmarkAutoAssign(ctx, benchmarkId, false)
		if err != nil {
			h.logger.Error("failed to set auto assign", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to set auto assign")
		}
		err = h.db.DeleteBenchmarkAssignmentByBenchmarkId(ctx, benchmarkId)
		if err != nil {
			h.logger.Error("failed to delete benchmark assignments", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete benchmark assignments")
		}
	}
	h.logger.Info("delete checked")

	return echoCtx.NoContent(http.StatusOK)
}

// ComplianceSummaryOfIntegration godoc
//
//	@Summary		Get benchmark summary
//	@Description	Retrieving a summary of a benchmark and its associated checks and results.
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			request			body		api.ComplianceSummaryOfIntegrationRequest	true	"Integrations filter to get the benchmark summary"
//	@Param			benchmark_id	path		string										true	"Benchmark ID to get the summary"
//	@Success		200				{object}	api.ComplianceSummaryOfIntegrationResponse
//	@Router			/compliance/api/v3/compliance/summary/integration [post]
func (h *HttpHandler) ComplianceSummaryOfIntegration(echoCtx echo.Context) error {
	clientCtx := &httpclient.Context{UserRole: authApi.AdminRole}
	ctx := echoCtx.Request().Context()
	var req api.ComplianceSummaryOfIntegrationRequest
	if err := bindValidate(echoCtx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if req.ShowTop == 0 {
		req.ShowTop = 5
	}

	var integration integrationapi.Integration
	if req.Integration.IntegrationID != nil {
		integrationTmp, err := h.integrationClient.GetIntegration(clientCtx, *req.Integration.IntegrationID)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		if integrationTmp != nil {
			integration = *integrationTmp
		}
	} else {
		var integrationTypes []string
		if req.Integration.IntegrationType != nil {
			integrationTypes = []string{*req.Integration.IntegrationType}
		}
		var integrationIDs []string
		if req.Integration.IntegrationID != nil {
			integrationIDs = []string{*req.Integration.IntegrationID}
		}
		integrationsTmp, err := h.integrationClient.ListIntegrationsByFilters(clientCtx,
			integrationapi.ListIntegrationsRequest{
				IntegrationType: integrationTypes,
				NameRegex:       req.Integration.Name,
				ProviderIDRegex: req.Integration.ProviderID,
				IntegrationID:   integrationIDs,
			})
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		integration = integrationsTmp.Integrations[0]
	}

	integrationInfo := api.IntegrationInfo{
		ProviderID:      &integration.ProviderID,
		Name:            &integration.Name,
		IntegrationType: integration.IntegrationType.String(),
		IntegrationID:   &integration.IntegrationID,
	}

	integrationIDs := []string{integration.IntegrationID}

	benchmarkID := req.BenchmarkId
	// tracer :
	ctx, span1 := tracer.Start(ctx, "new_GetBenchmark", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_GetBenchmark")
	defer span1.End()

	benchmark, err := h.db.GetBenchmark(ctx, benchmarkID)
	if err != nil {
		span1.RecordError(err)
		span1.SetStatus(codes.Error, err.Error())
		return err
	}
	if benchmark == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid benchmarkID")
	}

	span1.AddEvent("information", trace.WithAttributes(
		attribute.String("benchmark ID", benchmark.ID),
	))
	span1.End()

	controls, err := h.db.ListControlsByBenchmarkID(ctx, benchmarkID)
	if err != nil {
		h.logger.Error("failed to get controls", zap.Error(err))
		return err
	}
	controlsMap := make(map[string]*db.Control)
	for _, control := range controls {
		control := control
		controlsMap[strings.ToLower(control.ID)] = &control
	}
	timeAt := time.Now()

	summariesAtTime, err := es.ListBenchmarkSummariesAtTime(ctx, h.logger, h.client,
		[]string{benchmarkID}, integrationIDs, nil,
		timeAt, true)
	if err != nil {
		return err
	}

	passedResourcesResult, err := es.GetPerBenchmarkResourceSeverityResult(ctx, h.logger, h.client, []string{benchmarkID}, integrationIDs, nil, nil, opengovernanceTypes.GetPassedComplianceStatuses())
	if err != nil {
		h.logger.Error("failed to fetch per benchmark resource severity result for passed", zap.Error(err))
		return err
	}

	allResourcesResult, err := es.GetPerBenchmarkResourceSeverityResult(ctx, h.logger, h.client, []string{benchmarkID}, integrationIDs, nil, nil, nil)
	if err != nil {
		h.logger.Error("failed to fetch per benchmark resource severity result for all", zap.Error(err))
		return err
	}

	summaryAtTime := summariesAtTime[benchmarkID]

	csResult := api.ComplianceStatusSummaryV2{}
	sResult := opengovernanceTypes.SeverityResult{}
	controlSeverityResult := api.BenchmarkControlsSeverityStatusV2{}
	var costImpact *float64
	addToResults := func(resultGroup types.ResultGroup) {
		csResult.AddESComplianceStatusMap(resultGroup.Result.QueryResult)
		sResult.AddResultMap(resultGroup.Result.SeverityResult)
		costImpact = utils.PAdd(costImpact, resultGroup.Result.CostImpact)
		for controlId, controlResult := range resultGroup.Controls {
			control := controlsMap[strings.ToLower(controlId)]
			controlSeverityResult = addToControlSeverityResultV2(controlSeverityResult, control, controlResult)
		}
	}

	for _, integrationID := range integrationIDs {
		addToResults(summaryAtTime.Integrations.Integrations[integrationID])
	}

	lastJob, err := h.schedulerClient.GetLatestComplianceJobForBenchmark(&httpclient.Context{UserRole: authApi.AdminRole}, benchmarkID)
	if err != nil {
		h.logger.Error("failed to get latest compliance job for benchmark", zap.Error(err), zap.String("benchmarkID", benchmarkID))
		return err
	}

	var lastJobStatus, lastJobId string
	if lastJob != nil {
		lastJobStatus = string(lastJob.Status)
		lastJobId = strconv.Itoa(int(lastJob.ID))
	}

	var topResourceTypes, topResources, topControls []api.TopFiledRecordV2

	topResourceTypesMap, err := es.GetPerFieldTopWithIssues(ctx, h.logger, h.client, "resourceType", integrationIDs, nil,
		nil, nil, []string{benchmarkID}, nil, req.ShowTop)
	if err != nil {
		h.logger.Error("failed to get top resource types for benchmark", zap.Error(err), zap.String("benchmarkID", benchmarkID))
		return err
	}
	for k, v := range topResourceTypesMap {
		topResourceTypes = append(topResourceTypes, api.TopFiledRecordV2{
			Field:  "ResourceType",
			Key:    k,
			Issues: v.AlarmCount,
		})
	}
	sort.Slice(topResourceTypes, func(i, j int) bool {
		return topResourceTypes[i].Issues > topResourceTypes[j].Issues
	})

	topResourcesMap, err := es.GetPerFieldTopWithIssues(ctx, h.logger, h.client, "resourceID", integrationIDs, nil,
		nil, nil, []string{benchmarkID}, nil, req.ShowTop)
	if err != nil {
		h.logger.Error("failed to get top resources for benchmark", zap.Error(err), zap.String("benchmarkID", benchmarkID))
		return err
	}
	for k, v := range topResourcesMap {
		topResources = append(topResources, api.TopFiledRecordV2{
			Field:  "Resource",
			Key:    k,
			Issues: v.AlarmCount,
		})
	}
	sort.Slice(topResources, func(i, j int) bool {
		return topResources[i].Issues > topResources[j].Issues
	})

	topControlsMap, err := es.GetPerFieldTopWithIssues(ctx, h.logger, h.client, "controlID", integrationIDs, nil,
		nil, nil, []string{benchmarkID}, nil, req.ShowTop)
	if err != nil {
		h.logger.Error("failed to get top resources for benchmark", zap.Error(err), zap.String("benchmarkID", benchmarkID))
		return err
	}
	for k, v := range topControlsMap {
		topControls = append(topControls, api.TopFiledRecordV2{
			Field:  "Control",
			Key:    k,
			Issues: v.AlarmCount,
		})
	}
	sort.Slice(topControls, func(i, j int) bool {
		return topControls[i].Issues > topControls[j].Issues
	})

	resourcesSeverityResult := api.BenchmarkResourcesSeverityStatusV2{}
	allResources := allResourcesResult[benchmarkID]
	resourcesSeverityResult.Total.TotalCount = allResources.TotalCount
	resourcesSeverityResult.Critical.TotalCount = allResources.CriticalCount
	resourcesSeverityResult.High.TotalCount = allResources.HighCount
	resourcesSeverityResult.Medium.TotalCount = allResources.MediumCount
	resourcesSeverityResult.Low.TotalCount = allResources.LowCount
	resourcesSeverityResult.None.TotalCount = allResources.NoneCount
	passedResource := passedResourcesResult[benchmarkID]
	resourcesSeverityResult.Total.PassedCount = passedResource.TotalCount
	resourcesSeverityResult.Critical.PassedCount = passedResource.CriticalCount
	resourcesSeverityResult.High.PassedCount = passedResource.HighCount
	resourcesSeverityResult.Medium.PassedCount = passedResource.MediumCount
	resourcesSeverityResult.Low.PassedCount = passedResource.LowCount
	resourcesSeverityResult.None.PassedCount = passedResource.NoneCount

	resourcesSeverityResult.Total.FailedCount = allResources.TotalCount - passedResource.TotalCount
	resourcesSeverityResult.Critical.FailedCount = allResources.CriticalCount - passedResource.CriticalCount
	resourcesSeverityResult.High.FailedCount = allResources.HighCount - passedResource.HighCount
	resourcesSeverityResult.Medium.FailedCount = allResources.MediumCount - passedResource.MediumCount
	resourcesSeverityResult.Low.FailedCount = allResources.LowCount - passedResource.LowCount
	resourcesSeverityResult.None.FailedCount = allResources.NoneCount - passedResource.NoneCount

	var complianceScore float64
	if controlSeverityResult.Total.TotalCount > 0 {
		complianceScore = float64(controlSeverityResult.Total.PassedCount) / float64(controlSeverityResult.Total.TotalCount)
	} else {
		complianceScore = 0
	}

	response := api.ComplianceSummaryOfIntegrationResponse{
		BenchmarkID:                benchmarkID,
		Integration:                integrationInfo,
		ComplianceScore:            complianceScore,
		SeveritySummaryByControl:   controlSeverityResult,
		SeveritySummaryByResource:  resourcesSeverityResult,
		ComplianceResultsSummary:   csResult,
		IssuesCount:                csResult.FailedCount,
		TopControlsWithIssues:      topControls,
		TopResourcesWithIssues:     topResources,
		TopResourceTypesWithIssues: topResourceTypes,
		LastEvaluatedAt:            utils.GetPointer(time.Unix(summaryAtTime.EvaluatedAtEpoch, 0)),
		LastJobStatus:              lastJobStatus,
		LastJobId:                  lastJobId,
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// ComplianceSummaryOfBenchmark godoc
//
//	@Summary		Get benchmark summary
//	@Description	Retrieving a summary of a benchmark and its associated checks and results.
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			request			body		api.ComplianceSummaryOfBenchmarkRequest	true	"Integrations filter to get the benchmark summary"
//	@Param			benchmark_id	path		string									true	"Benchmark ID to get the summary"
//	@Success		200				{object}	api.ComplianceSummaryOfBenchmarkResponse
//	@Router			/compliance/api/v3/compliance/summary/benchmark [post]
func (h *HttpHandler) ComplianceSummaryOfBenchmark(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()
	var req api.ComplianceSummaryOfBenchmarkRequest
	if err := bindValidate(echoCtx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// tracer :
	ctx, span1 := tracer.Start(ctx, "new_ComplianceSummaryOfBenchmark", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_ComplianceSummaryOfBenchmark")
	defer span1.End()

	if req.ShowTop == 0 {
		req.ShowTop = 5
	}
	if req.IsRoot == nil {
		trueBool := true
		req.IsRoot = &trueBool
	}

	var benchmarks []db.Benchmark
	var err error
	if len(req.Benchmarks) == 0 {
		assigned := false
		benchmarks, err = h.db.ListBenchmarksFiltered(ctx, nil, *req.IsRoot, nil, nil, &assigned, nil, nil)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	} else {
		benchmarks, err = h.db.GetBenchmarksBare(ctx, req.Benchmarks)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}

	var response []api.ComplianceSummaryOfBenchmarkResponse
	for _, benchmark := range benchmarks {

		span1.AddEvent("information", trace.WithAttributes(
			attribute.String("benchmark ID", benchmark.ID),
		))
		span1.End()

		controls, err := h.db.ListControlsByBenchmarkID(ctx, benchmark.ID)
		if err != nil {
			h.logger.Error("failed to get controls", zap.Error(err))
			return err
		}
		controlsMap := make(map[string]*db.Control)
		for _, control := range controls {
			control := control
			controlsMap[strings.ToLower(control.ID)] = &control
		}
		timeAt := time.Now()

		summariesAtTime, err := es.ListBenchmarkSummariesAtTime(ctx, h.logger, h.client,
			[]string{benchmark.ID}, nil, nil,
			timeAt, true)
		if err != nil {
			return err
		}

		passedResourcesResult, err := es.GetPerBenchmarkResourceSeverityResult(ctx, h.logger, h.client, []string{benchmark.ID}, nil, nil, nil, opengovernanceTypes.GetPassedComplianceStatuses())
		if err != nil {
			h.logger.Error("failed to fetch per benchmark resource severity result for passed", zap.Error(err))
			return err
		}

		allResourcesResult, err := es.GetPerBenchmarkResourceSeverityResult(ctx, h.logger, h.client, []string{benchmark.ID}, nil, nil, nil, nil)
		if err != nil {
			h.logger.Error("failed to fetch per benchmark resource severity result for all", zap.Error(err))
			return err
		}

		summaryAtTime := summariesAtTime[benchmark.ID]

		csResult := api.ComplianceStatusSummaryV2{}
		sResult := opengovernanceTypes.SeverityResultV2{}
		controlSeverityResult := api.BenchmarkControlsSeverityStatusV2{}
		var costImpact *float64
		addToResults := func(resultGroup types.ResultGroup) {
			csResult.AddESComplianceStatusMap(resultGroup.Result.QueryResult)
			sResult.AddResultMap(resultGroup.Result.SeverityResult)
			costImpact = utils.PAdd(costImpact, resultGroup.Result.CostImpact)
			for controlId, controlResult := range resultGroup.Controls {
				control := controlsMap[strings.ToLower(controlId)]
				controlSeverityResult = addToControlSeverityResultV2(controlSeverityResult, control, controlResult)
			}
		}

		addToResults(summaryAtTime.Integrations.BenchmarkResult)

		lastJob, err := h.schedulerClient.GetLatestComplianceJobForBenchmark(&httpclient.Context{UserRole: authApi.AdminRole}, benchmark.ID)
		if err != nil {
			h.logger.Error("failed to get latest compliance job for benchmark", zap.Error(err), zap.String("benchmarkID", benchmark.ID))
			return err
		}

		var lastJobStatus, lastJobId string
		if lastJob != nil {
			lastJobStatus = string(lastJob.Status)
			lastJobId = strconv.Itoa(int(lastJob.ID))
		}

		topIntegrationsMap := make([]api.TopFieldRecord, 0, req.ShowTop)
		if req.ShowTop > 0 {
			res, err := es.ComplianceResultsTopFieldQuery(ctx, h.logger, h.client, "integrationID", nil,
				nil, nil, nil, nil, []string{benchmark.ID}, nil, nil,
				opengovernanceTypes.GetFailedComplianceStatuses(), []bool{true}, req.ShowTop, nil, nil)
			if err != nil {
				h.logger.Error("failed to fetch complianceResults top field", zap.Error(err))
				return err
			}

			topFieldTotalResponse, err := es.ComplianceResultsTopFieldQuery(ctx, h.logger, h.client, "integrationID", nil,
				nil, nil, nil, nil, []string{benchmark.ID}, nil, nil,
				opengovernanceTypes.GetFailedComplianceStatuses(), []bool{true}, req.ShowTop, nil, nil)
			if err != nil {
				h.logger.Error("failed to fetch complianceResults top field total", zap.Error(err))
				return err
			}
			totalCountMap := make(map[string]int)
			for _, item := range topFieldTotalResponse.Aggregations.FieldFilter.Buckets {
				totalCountMap[item.Key] += item.DocCount
			}

			resIntegrationIDs := make([]string, 0, len(res.Aggregations.FieldFilter.Buckets))
			for _, item := range res.Aggregations.FieldFilter.Buckets {
				resIntegrationIDs = append(resIntegrationIDs, item.Key)
			}
			if len(resIntegrationIDs) > 0 {
				integrations, err := h.integrationClient.ListIntegrationsByFilters(&httpclient.Context{UserRole: authApi.AdminRole}, integrationapi.ListIntegrationsRequest{
					IntegrationID: resIntegrationIDs,
				})
				if err != nil {
					h.logger.Error("failed to get integrations", zap.Error(err))
					return err
				}
				integrationMap := make(map[string]*integrationapi.Integration)
				for _, integration := range integrations.Integrations {
					integration := integration
					integrationMap[integration.IntegrationID] = &integration
				}

				for _, item := range res.Aggregations.FieldFilter.Buckets {
					if _, ok := integrationMap[item.Key]; !ok {
						continue
					}
					if _, ok := totalCountMap[item.Key]; !ok {
						continue
					}
					topIntegrationsMap = append(topIntegrationsMap, api.TopFieldRecord{
						Integration: integrationMap[item.Key],
						Count:       item.DocCount,
						TotalCount:  totalCountMap[item.Key],
					})
				}
			}
		}

		var topResourceTypes, topResources, topControls []api.TopFiledRecordV2

		topResourceTypesMap, err := es.GetPerFieldTopWithIssues(ctx, h.logger, h.client, "resourceType", nil, nil,
			nil, nil, []string{benchmark.ID}, nil, req.ShowTop)
		if err != nil {
			h.logger.Error("failed to get top resource types for benchmark", zap.Error(err), zap.String("benchmarkID", benchmark.ID))
			return err
		}
		for k, v := range topResourceTypesMap {
			topResourceTypes = append(topResourceTypes, api.TopFiledRecordV2{
				Field:  "ResourceType",
				Key:    k,
				Issues: v.AlarmCount,
			})
		}
		sort.Slice(topResourceTypes, func(i, j int) bool {
			return topResourceTypes[i].Issues > topResourceTypes[j].Issues
		})

		topResourcesMap, err := es.GetPerFieldTopWithIssues(ctx, h.logger, h.client, "resourceID", nil, nil,
			nil, nil, []string{benchmark.ID}, nil, req.ShowTop)
		if err != nil {
			h.logger.Error("failed to get top resources for benchmark", zap.Error(err), zap.String("benchmarkID", benchmark.ID))
			return err
		}
		for k, v := range topResourcesMap {
			topResources = append(topResources, api.TopFiledRecordV2{
				Field:  "Resource",
				Key:    k,
				Issues: v.AlarmCount,
			})
		}
		sort.Slice(topResources, func(i, j int) bool {
			return topResources[i].Issues > topResources[j].Issues
		})

		topControlsMap, err := es.GetPerFieldTopWithIssues(ctx, h.logger, h.client, "controlID", nil, nil,
			nil, nil, []string{benchmark.ID}, nil, req.ShowTop)
		if err != nil {
			h.logger.Error("failed to get top resources for benchmark", zap.Error(err), zap.String("benchmarkID", benchmark.ID))
			return err
		}
		for k, v := range topControlsMap {
			topControls = append(topControls, api.TopFiledRecordV2{
				Field:  "Control",
				Key:    k,
				Issues: v.AlarmCount,
			})
		}
		sort.Slice(topControls, func(i, j int) bool {
			return topControls[i].Issues > topControls[j].Issues
		})

		resourcesSeverityResult := api.BenchmarkResourcesSeverityStatusV2{}
		allResources := allResourcesResult[benchmark.ID]
		resourcesSeverityResult.Total.TotalCount = allResources.TotalCount
		resourcesSeverityResult.Critical.TotalCount = allResources.CriticalCount
		resourcesSeverityResult.High.TotalCount = allResources.HighCount
		resourcesSeverityResult.Medium.TotalCount = allResources.MediumCount
		resourcesSeverityResult.Low.TotalCount = allResources.LowCount
		resourcesSeverityResult.None.TotalCount = allResources.NoneCount
		passedResource := passedResourcesResult[benchmark.ID]
		resourcesSeverityResult.Total.PassedCount = passedResource.TotalCount
		resourcesSeverityResult.Critical.PassedCount = passedResource.CriticalCount
		resourcesSeverityResult.High.PassedCount = passedResource.HighCount
		resourcesSeverityResult.Medium.PassedCount = passedResource.MediumCount
		resourcesSeverityResult.Low.PassedCount = passedResource.LowCount
		resourcesSeverityResult.None.PassedCount = passedResource.NoneCount

		resourcesSeverityResult.Total.FailedCount = allResources.TotalCount - passedResource.TotalCount
		resourcesSeverityResult.Critical.FailedCount = allResources.CriticalCount - passedResource.CriticalCount
		resourcesSeverityResult.High.FailedCount = allResources.HighCount - passedResource.HighCount
		resourcesSeverityResult.Medium.FailedCount = allResources.MediumCount - passedResource.MediumCount
		resourcesSeverityResult.Low.FailedCount = allResources.LowCount - passedResource.LowCount
		resourcesSeverityResult.None.FailedCount = allResources.NoneCount - passedResource.NoneCount

		var topIntegrations []api.TopIntegration
		for _, tf := range topIntegrationsMap {
			if tf.Integration == nil {
				continue
			}
			topIntegrations = append(topIntegrations, api.TopIntegration{
				Issues: tf.Count,
				IntegrationInfo: api.IntegrationInfo{
					ProviderID:      &tf.Integration.ProviderID,
					Name:            &tf.Integration.Name,
					IntegrationType: tf.Integration.IntegrationType.String(),
					IntegrationID:   &tf.Integration.IntegrationID,
				},
			})
		}

		var complianceScore float64
		if controlSeverityResult.Total.TotalCount > 0 {
			complianceScore = float64(controlSeverityResult.Total.PassedCount) / float64(controlSeverityResult.Total.TotalCount)
		} else {
			complianceScore = 0
		}

		var integrationTypes []string
		if benchmark.IntegrationType != nil {
			integrationTypes = benchmark.IntegrationType
		}
		response = append(response, api.ComplianceSummaryOfBenchmarkResponse{
			BenchmarkID:                benchmark.ID,
			BenchmarkTitle:             benchmark.Title,
			IntegrationTypes:           integrationTypes,
			ComplianceScore:            complianceScore,
			SeveritySummaryByControl:   controlSeverityResult,
			SeveritySummaryByResource:  resourcesSeverityResult,
			SeveritySummaryByIncidents: sResult,
			CostImpact:                 costImpact,
			TopIntegrations:            topIntegrations,
			TopResourceTypesWithIssues: topResourceTypes,
			TopResourcesWithIssues:     topResources,
			TopControlsWithIssues:      topControls,
			ComplianceResultsSummary:   csResult,
			IssuesCount:                csResult.FailedCount,
			LastEvaluatedAt:            utils.GetPointer(time.Unix(summaryAtTime.EvaluatedAtEpoch, 0)),
			LastJobStatus:              lastJobStatus,
			LastJobId:                  lastJobId,
		})
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// ComplianceSummaryOfJob godoc
//
//	@Summary		Get benchmark summary for a job
//	@Description	Retrieving a summary of a benchmark and its associated checks and results by given job ID
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			job_id		path		string	true	"Benchmark ID to get the summary"
//	@Param			show_top	query		int		true	"Show top integrations (5 by default)"
//	@Success		200			{object}	api.ComplianceSummaryOfBenchmarkResponse
//	@Router			/compliance/api/v3/compliance/summary/{job_id} [get]
func (h *HttpHandler) ComplianceSummaryOfJob(echoCtx echo.Context) error {
	clientCtx := &httpclient.Context{UserRole: authApi.AdminRole}

	ctx := echoCtx.Request().Context()
	jobId := echoCtx.Param("job_id")
	showTopStr := echoCtx.QueryParam("show_top")

	showTop := int64(5)
	if showTopStr != "" {
		showTopTmp, err := strconv.ParseInt(showTopStr, 10, 64)
		if err == nil {
			showTop = showTopTmp
		}
	}

	summaryJobs, err := h.schedulerClient.GetSummaryJobs(clientCtx, []string{jobId})
	if err != nil {
		h.logger.Error("could not get Summary Job IDs", zap.Error(err))
		return echoCtx.JSON(http.StatusInternalServerError, "could not get Summary Job IDs")
	}

	// tracer :
	ctx, span1 := tracer.Start(ctx, "new_ComplianceSummaryOfBenchmark", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_ComplianceSummaryOfBenchmark")
	defer span1.End()

	var response api.ComplianceSummaryOfBenchmarkResponse

	h.logger.Info("Jobs", zap.Any("jobs", summaryJobs))
	summariesAtTime, err := es.GetComplianceSummaryByJobId(ctx, h.logger, h.client, summaryJobs, true)
	if err != nil {
		return err
	}
	var benchmarkId string
	for k, _ := range summariesAtTime {
		benchmarkId = k
	}
	h.logger.Info("BenchmarkSummary for Job", zap.Any("summary", summariesAtTime), zap.Any("job_is", summaryJobs),
		zap.String("benchmark_id", benchmarkId))

	controls, err := h.db.ListControlsByBenchmarkID(ctx, benchmarkId)
	if err != nil {
		h.logger.Error("failed to get controls", zap.Error(err))
		return err
	}
	controlsMap := make(map[string]*db.Control)
	for _, control := range controls {
		control := control
		controlsMap[strings.ToLower(control.ID)] = &control
	}

	passedResourcesResult, err := es.GetPerBenchmarkResourceSeverityResultByJobId(ctx, h.logger, h.client, []string{benchmarkId},
		nil, nil, nil, opengovernanceTypes.GetPassedComplianceStatuses(), jobId)
	if err != nil {
		h.logger.Error("failed to fetch per benchmark resource severity result for passed", zap.Error(err))
		return err
	}

	allResourcesResult, err := es.GetPerBenchmarkResourceSeverityResultByJobId(ctx, h.logger, h.client, []string{benchmarkId},
		nil, nil, nil, nil, jobId)
	if err != nil {
		h.logger.Error("failed to fetch per benchmark resource severity result for all", zap.Error(err))
		return err
	}

	summaryAtTime := summariesAtTime[benchmarkId]

	csResult := api.ComplianceStatusSummaryV2{}
	sResult := opengovernanceTypes.SeverityResultV2{}
	controlSeverityResult := api.BenchmarkControlsSeverityStatusV2{}
	var costImpact *float64
	addToResults := func(resultGroup types.ResultGroup) {
		csResult.AddESComplianceStatusMap(resultGroup.Result.QueryResult)
		sResult.AddResultMap(resultGroup.Result.SeverityResult)
		costImpact = utils.PAdd(costImpact, resultGroup.Result.CostImpact)
		for controlId, controlResult := range resultGroup.Controls {
			control := controlsMap[strings.ToLower(controlId)]
			controlSeverityResult = addToControlSeverityResultV2(controlSeverityResult, control, controlResult)
		}
	}

	addToResults(summaryAtTime.Integrations.BenchmarkResult)

	topIntegrationsMap := make([]api.TopFieldRecord, 0, showTop)
	if showTop > 0 {
		res, err := es.ComplianceResultsTopFieldQuery(ctx, h.logger, h.client, "integrationID", nil, nil,
			nil, nil, nil, []string{benchmarkId}, nil, nil, opengovernanceTypes.GetFailedComplianceStatuses(),
			[]bool{true}, int(showTop), nil, nil)
		if err != nil {
			h.logger.Error("failed to fetch complianceResults top field", zap.Error(err))
			return err
		}

		topFieldTotalResponse, err := es.ComplianceResultsTopFieldQuery(ctx, h.logger, h.client, "integrationID", nil,
			nil, nil, nil, nil, []string{benchmarkId}, nil, nil,
			opengovernanceTypes.GetFailedComplianceStatuses(), []bool{true}, int(showTop), nil, nil)
		if err != nil {
			h.logger.Error("failed to fetch complianceResults top field total", zap.Error(err))
			return err
		}
		totalCountMap := make(map[string]int)
		for _, item := range topFieldTotalResponse.Aggregations.FieldFilter.Buckets {
			totalCountMap[item.Key] += item.DocCount
		}

		resIntegrationIDs := make([]string, 0, len(res.Aggregations.FieldFilter.Buckets))
		for _, item := range res.Aggregations.FieldFilter.Buckets {
			resIntegrationIDs = append(resIntegrationIDs, item.Key)
		}
		if len(resIntegrationIDs) > 0 {
			integrations, err := h.integrationClient.ListIntegrationsByFilters(&httpclient.Context{UserRole: authApi.AdminRole}, integrationapi.ListIntegrationsRequest{
				IntegrationID: resIntegrationIDs,
			})
			if err != nil {
				h.logger.Error("failed to get integrations", zap.Error(err))
				return err
			}
			integrationMap := make(map[string]*integrationapi.Integration)
			for _, integration := range integrations.Integrations {
				integration := integration
				integrationMap[integration.IntegrationID] = &integration
			}

			for _, item := range res.Aggregations.FieldFilter.Buckets {
				if _, ok := integrationMap[item.Key]; !ok {
					continue
				}
				if _, ok := totalCountMap[item.Key]; !ok {
					continue
				}
				topIntegrationsMap = append(topIntegrationsMap, api.TopFieldRecord{
					Integration: integrationMap[item.Key],
					Count:       item.DocCount,
					TotalCount:  totalCountMap[item.Key],
				})
			}
		}
	}

	var topResourceTypes, topResources, topControls []api.TopFiledRecordV2

	topResourceTypesMap, err := es.GetPerFieldTopWithIssues(ctx, h.logger, h.client, "resourceType", nil, nil,
		nil, nil, []string{benchmarkId}, nil, int(showTop))
	if err != nil {
		h.logger.Error("failed to get top resource types for benchmark", zap.Error(err), zap.String("benchmarkID", benchmarkId))
		return err
	}
	for k, v := range topResourceTypesMap {
		topResourceTypes = append(topResourceTypes, api.TopFiledRecordV2{
			Field:  "ResourceType",
			Key:    k,
			Issues: v.AlarmCount,
		})
	}
	sort.Slice(topResourceTypes, func(i, j int) bool {
		return topResourceTypes[i].Issues > topResourceTypes[j].Issues
	})

	topResourcesMap, err := es.GetPerFieldTopWithIssues(ctx, h.logger, h.client, "resourceID", nil, nil,
		nil, nil, []string{benchmarkId}, nil, int(showTop))
	if err != nil {
		h.logger.Error("failed to get top resources for benchmark", zap.Error(err), zap.String("benchmarkID", benchmarkId))
		return err
	}
	for k, v := range topResourcesMap {
		topResources = append(topResources, api.TopFiledRecordV2{
			Field:  "Resource",
			Key:    k,
			Issues: v.AlarmCount,
		})
	}
	sort.Slice(topResources, func(i, j int) bool {
		return topResources[i].Issues > topResources[j].Issues
	})

	topControlsMap, err := es.GetPerFieldTopWithIssues(ctx, h.logger, h.client, "controlID", nil, nil,
		nil, nil, []string{benchmarkId}, nil, int(showTop))
	if err != nil {
		h.logger.Error("failed to get top resources for benchmark", zap.Error(err), zap.String("benchmarkID", benchmarkId))
		return err
	}
	for k, v := range topControlsMap {
		topControls = append(topControls, api.TopFiledRecordV2{
			Field:  "Control",
			Key:    k,
			Issues: v.AlarmCount,
		})
	}
	sort.Slice(topControls, func(i, j int) bool {
		return topControls[i].Issues > topControls[j].Issues
	})

	resourcesSeverityResult := api.BenchmarkResourcesSeverityStatusV2{}
	allResources := allResourcesResult[benchmarkId]
	resourcesSeverityResult.Total.TotalCount = allResources.TotalCount
	resourcesSeverityResult.Critical.TotalCount = allResources.CriticalCount
	resourcesSeverityResult.High.TotalCount = allResources.HighCount
	resourcesSeverityResult.Medium.TotalCount = allResources.MediumCount
	resourcesSeverityResult.Low.TotalCount = allResources.LowCount
	resourcesSeverityResult.None.TotalCount = allResources.NoneCount
	passedResource := passedResourcesResult[benchmarkId]
	resourcesSeverityResult.Total.PassedCount = passedResource.TotalCount
	resourcesSeverityResult.Critical.PassedCount = passedResource.CriticalCount
	resourcesSeverityResult.High.PassedCount = passedResource.HighCount
	resourcesSeverityResult.Medium.PassedCount = passedResource.MediumCount
	resourcesSeverityResult.Low.PassedCount = passedResource.LowCount
	resourcesSeverityResult.None.PassedCount = passedResource.NoneCount

	resourcesSeverityResult.Total.FailedCount = allResources.TotalCount - passedResource.TotalCount
	resourcesSeverityResult.Critical.FailedCount = allResources.CriticalCount - passedResource.CriticalCount
	resourcesSeverityResult.High.FailedCount = allResources.HighCount - passedResource.HighCount
	resourcesSeverityResult.Medium.FailedCount = allResources.MediumCount - passedResource.MediumCount
	resourcesSeverityResult.Low.FailedCount = allResources.LowCount - passedResource.LowCount
	resourcesSeverityResult.None.FailedCount = allResources.NoneCount - passedResource.NoneCount

	var topIntegrations []api.TopIntegration
	for _, tf := range topIntegrationsMap {
		if tf.Integration == nil {
			continue
		}
		topIntegrations = append(topIntegrations, api.TopIntegration{
			Issues: tf.Count,
			IntegrationInfo: api.IntegrationInfo{
				ProviderID:      &tf.Integration.ProviderID,
				Name:            &tf.Integration.Name,
				IntegrationType: tf.Integration.IntegrationType.String(),
				IntegrationID:   &tf.Integration.IntegrationID,
			},
		})
	}

	var complianceScore float64
	if controlSeverityResult.Total.TotalCount > 0 {
		complianceScore = float64(controlSeverityResult.Total.PassedCount) / float64(controlSeverityResult.Total.TotalCount)
	} else {
		complianceScore = 0
	}

	benchmark, err := h.db.GetBenchmark(ctx, benchmarkId)
	if err != nil {
		h.logger.Error("failed to get benchmark", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get benchmark")
	}

	var integrationTypes []integration.Type
	var title string
	if benchmark != nil {
		if benchmark.IntegrationType != nil {
			integrationTypes = integration_type.ParseTypes(benchmark.IntegrationType)
		}
		title = benchmark.Title
	}
	response = api.ComplianceSummaryOfBenchmarkResponse{
		BenchmarkID:                benchmarkId,
		BenchmarkTitle:             title,
		IntegrationTypes:           integration_type.UnparseTypes(integrationTypes),
		ComplianceScore:            complianceScore,
		SeveritySummaryByControl:   controlSeverityResult,
		SeveritySummaryByResource:  resourcesSeverityResult,
		SeveritySummaryByIncidents: sResult,
		CostImpact:                 costImpact,
		TopIntegrations:            topIntegrations,
		TopResourceTypesWithIssues: topResourceTypes,
		TopResourcesWithIssues:     topResources,
		TopControlsWithIssues:      topControls,
		ComplianceResultsSummary:   csResult,
		IssuesCount:                csResult.FailedCount,
		LastEvaluatedAt:            utils.GetPointer(time.Unix(summaryAtTime.EvaluatedAtEpoch, 0)),
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// ListControlsFilters godoc
//
//	@Summary	List possible values for each filter in List Controls
//	@Security	BearerToken
//	@Tags		compliance
//	@Accept		json
//	@Produce	json
//	@Success	200	{object}	api.ListControlsFiltersResponse
//	@Router		/compliance/api/v3/controls/filters [get]
func (h *HttpHandler) ListControlsFilters(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	integrationTypes, err := h.db.ListControlsUniqueIntegrationTypes(ctx)
	if err != nil {
		h.logger.Error("failed to get integration types list", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get integration types list")
	}

	severities, err := h.db.ListControlsUniqueSeverity(ctx)
	if err != nil {
		h.logger.Error("failed to get severities list", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get severities list")
	}

	rootBenchmarks, err := h.db.ListRootBenchmarks(ctx, nil)
	if err != nil {
		h.logger.Error("failed to get rootBenchmarks", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get rootBenchmarks")
	}
	var rootBenchmarkIds []string
	for _, b := range rootBenchmarks {
		rootBenchmarkIds = append(rootBenchmarkIds, b.ID)
	}

	parentBenchmarks, err := h.db.ListControlsUniqueParentBenchmarks(ctx)
	if err != nil {
		h.logger.Error("failed to get parentBenchmarks", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get parentBenchmarks")
	}

	primaryTables, err := h.db.ListQueriesUniquePrimaryTables(ctx)
	if err != nil {
		h.logger.Error("failed to get primaryTables", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get primaryTables")
	}

	listOfTables, err := h.db.ListQueriesUniqueTables(ctx)
	if err != nil {
		h.logger.Error("failed to get listOfTables", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get listOfTables")
	}

	controlsTags, err := h.db.GetControlsTags()
	if err != nil {
		h.logger.Error("failed to get controlsTags", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get controlsTags")
	}

	tags := make([]api.ControlTagsResult, 0, len(controlsTags))
	for _, history := range controlsTags {
		tags = append(tags, history.ToApi())
	}

	response := api.ListControlsFiltersResponse{
		Provider:        integrationTypes,
		Severity:        severities,
		RootBenchmark:   rootBenchmarkIds,
		ParentBenchmark: parentBenchmarks,
		PrimaryTable:    primaryTables,
		ListOfTables:    listOfTables,
		Tags:            tags,
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// ListBenchmarksFilters godoc
//
//	@Summary	List possible values for each filter in List Benchmarks
//	@Security	BearerToken
//	@Tags		compliance
//	@Accept		json
//	@Produce	json
//	@Success	200	{object}	api.ListBenchmarksFiltersResponse
//	@Router		/compliance/api/v3/benchmarks/filters [get]
func (h *HttpHandler) ListBenchmarksFilters(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()

	benchmarks, err := h.db.ListBenchmarks(ctx)
	if err != nil {
		h.logger.Error("failed to get rootBenchmarks", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get rootBenchmarks")
	}
	var benchmarkIds []string
	for _, b := range benchmarks {
		benchmarkIds = append(benchmarkIds, b.ID)
	}

	primaryTables, err := h.db.ListQueriesUniquePrimaryTables(ctx)
	if err != nil {
		h.logger.Error("failed to get primaryTables", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get primaryTables")
	}

	listOfTables, err := h.db.ListQueriesUniqueTables(ctx)
	if err != nil {
		h.logger.Error("failed to get listOfTables", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get listOfTables")
	}

	benchmarksTags, err := h.db.GetBenchmarksTags()
	if err != nil {
		h.logger.Error("failed to get benchmarksTags", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get benchmarksTags")
	}

	tags := make([]api.BenchmarkTagsResult, 0, len(benchmarksTags))
	for _, history := range benchmarksTags {
		tags = append(tags, history.ToApi())
	}

	response := api.ListBenchmarksFiltersResponse{
		ParentBenchmarkID: benchmarkIds,
		PrimaryTable:      primaryTables,
		ListOfTables:      listOfTables,
		Tags:              tags,
	}

	return echoCtx.JSON(http.StatusOK, response)
}

// GetControlsResourceCategories godoc
//
//	@Summary		Get list of unique resource categories
//	@Description	Get list of unique resource categories for the give controls
//	@Security		BearerToken
//	@Tags			compliance
//	@Param			controls	query	[]string	false	"Controls filter by"
//	@Param			benchmarks	query	[]string	false	"Benchmark filter by"
//	@Accepts		json
//	@Produce		json
//	@Success		200	{object}	[]inventoryApi.GetResourceCategoriesResponse
//	@Router			/compliance/api/v3/controls/categories [get]
func (h *HttpHandler) GetControlsResourceCategories(ctx echo.Context) error {
	controlIds := httpserver2.QueryArrayParam(ctx, "controls")
	benchmarks := httpserver2.QueryArrayParam(ctx, "benchmarks")
	clientCtx := &httpclient.Context{UserRole: authApi.AdminRole}

	controls, err := h.db.ListControlsByFilter(ctx.Request().Context(), controlIds, nil, nil, benchmarks, nil,
		nil, nil, nil, nil)
	if err != nil {
		h.logger.Error("could not find controls", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "could not find controls")
	}
	tablesMap := make(map[string]bool)
	for _, c := range controls {
		for _, t := range c.Query.ListOfTables {
			tablesMap[t] = true
		}
	}
	var tables []string
	for t, _ := range tablesMap {
		tables = append(tables, t)
	}

	categories, err := h.inventoryClient.GetResourceCategories(clientCtx, tables, nil)
	if err != nil {
		h.logger.Error("could not find categories", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "could not find categories")
	}

	return ctx.JSON(200, categories)
}

// GetCategoriesControls godoc
//
//	@Summary		Get list of controls for given categories
//	@Description	Get list of controls for given categories
//	@Security		BearerToken
//	@Tags			compliance
//	@Param			categories	query	[]string	false	"Categories filter by"
//	@Accepts		json
//	@Produce		json
//	@Success		200	{object}	[]api.GetCategoriesControlsResponse
//	@Router			/compliance/api/v3/categories/controls [get]
func (h *HttpHandler) GetCategoriesControls(ctx echo.Context) error {
	categoriesFilter := httpserver2.QueryArrayParam(ctx, "categories")
	clientCtx := &httpclient.Context{UserRole: authApi.AdminRole}

	categories, err := h.inventoryClient.GetResourceCategories(clientCtx, nil, categoriesFilter)
	if err != nil || categories == nil || len(categories.Categories) == 0 {
		h.logger.Error("could not find categories", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "could not find categories")
	}

	var categoriesControls []api.CategoryControls
	for _, c := range categories.Categories {
		tablesFilterMap := make(map[string]string)
		for _, r := range c.Tables {
			tablesFilterMap[r.Table] = r.ResourceType
		}
		var tablesFilter []string
		for t, _ := range tablesFilterMap {
			tablesFilter = append(tablesFilter, t)
		}
		controls, err := h.db.ListControlsByFilter(ctx.Request().Context(), nil, nil, nil, nil,
			nil, nil, nil, tablesFilter, nil)
		if err != nil {
			h.logger.Error("could not find controls", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "could not find controls")
		}

		servicesControls := make(map[string][]api.Control)
		for _, ctrl := range controls {
			for _, t := range ctrl.Query.ListOfTables {
				if t == "" {
					continue
				}
				service := t
				if v, ok := tablesFilterMap[t]; ok {
					service = v
				}
				if _, ok := servicesControls[service]; !ok {
					servicesControls[service] = make([]api.Control, 0)
				}
				servicesControls[service] = append(servicesControls[service], ctrl.ToApi())
			}
		}
		var services []api.ServiceControls
		for k, v := range servicesControls {
			services = append(services, api.ServiceControls{
				Service:  k,
				Controls: v,
			})
		}
		categoriesControls = append(categoriesControls, api.CategoryControls{
			Category: c.Category,
			Services: services,
		})
	}

	return ctx.JSON(200, api.GetCategoriesControlsResponse{
		Categories: categoriesControls,
	})
}

// GetParametersControls godoc
//
//	@Summary		Get list of controls for given parameters
//	@Description	Get list of controls for given parameters
//	@Security		BearerToken
//	@Tags			compliance
//	@Param			parameters	query	[]string	false	"Parameters filter by"
//	@Accepts		json
//	@Produce		json
//	@Success		200	{object}	[]api.GetCategoriesControlsResponse
//	@Router			/compliance/api/v3/parameters/controls [get]
func (h *HttpHandler) GetParametersControls(ctx echo.Context) error {
	parameters := httpserver2.QueryArrayParam(ctx, "parameters")

	var err error
	if len(parameters) == 0 {
		parameters, err = h.db.GetQueryParameters(ctx.Request().Context())
		if err != nil {
			h.logger.Error("failed to get list of parameters", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to get list of parameters")
		}
	}

	var parametersControls []api.ParametersControls
	for _, p := range parameters {
		controls, err := h.db.ListControlsByFilter(ctx.Request().Context(), nil, nil, nil, nil,
			nil, nil, nil, nil, []string{p})
		if err != nil {
			h.logger.Error("failed to get list of controls", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to get list of controls")
		}
		var controlsApi []api.Control
		for _, ctrl := range controls {
			controlsApi = append(controlsApi, ctrl.ToApi())
		}
		parametersControls = append(parametersControls, api.ParametersControls{
			Parameter: p,
			Controls:  controlsApi,
		})
	}

	return ctx.JSON(200, api.GetParametersControlsResponse{
		ParametersControls: parametersControls,
	})
}

// ListComplianceJobsHistory godoc
//
//	@Summary	List jobs by job type and filters
//	@Security	BearerToken
//	@Tags		scheduler
//	@Param		interval		query	string	true	"Time Interval to filter by"
//	@Param		trigger_type	query	string	true	"Trigger Type: (all(default), manual, system)"
//	@Param		created_by		query	string	true	"Created By User ID"
//	@Param		cursor			query	int		true	"cursor"
//	@Param		per_page		query	int		true	"per page"
//	@Produce	json
//	@Success	200	{object}	api.ListComplianceJobsHistoryResponse
//	@Router		/compliance/api/v3/jobs/history [get]
func (h *HttpHandler) ListComplianceJobsHistory(ctx echo.Context) error {
	clientCtx := &httpclient.Context{UserRole: authApi.AdminRole}

	interval := ctx.QueryParam("interval")
	triggerType := ctx.QueryParam("trigger_type")
	createdBy := ctx.QueryParam("created_by")

	var cursor, perPage int64
	var err error
	cursorStr := ctx.QueryParam("cursor")
	if cursorStr != "" {
		cursor, err = strconv.ParseInt(cursorStr, 10, 64)
		if err != nil {
			return err
		}
	}
	perPageStr := ctx.QueryParam("per_page")
	if perPageStr != "" {
		perPage, err = strconv.ParseInt(perPageStr, 10, 64)
		if err != nil {
			return err
		}
	}

	jobs, err := h.schedulerClient.ListComplianceJobsHistory(clientCtx, interval, triggerType, createdBy, int(cursor), int(perPage))
	if err != nil {
		h.logger.Error("could not get list of jobs", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "could not get list of jobs")
	}
	var jobIDs []string
	for _, j := range jobs.Items {
		jobIDs = append(jobIDs, j.SummarizerJobs...)
	}

	timeAt := time.Now()
	summariesAtTime, err := es.ListJobsSummariesAtTime(ctx.Request().Context(), h.logger, h.client,
		nil, jobIDs, nil, nil,
		timeAt, true)
	if err != nil {
		return err
	}

	var items []api.ListComplianceJobsHistoryItem
	for _, j := range jobs.Items {
		var integrations []api.IntegrationInfo
		for _, i := range j.Integrations {
			integrations = append(integrations, api.IntegrationInfo{
				IntegrationType: i.IntegrationType,
				ProviderID:      &i.ProviderID,
				Name:            &i.Name,
				IntegrationID:   &i.IntegrationID,
			})
		}
		item := api.ListComplianceJobsHistoryItem{
			BenchmarkId:  j.BenchmarkId,
			Integrations: integrations,
			JobId:        j.JobId,
			TriggerType:  j.TriggerType,
			CreatedBy:    j.CreatedBy,
			JobStatus:    j.JobStatus,
			CreatedAt:    j.CreatedAt,
			UpdatedAt:    j.UpdatedAt,
		}

		if j.JobStatus == string(model3.ComplianceJobSucceeded) {
			summaryAtTime := summariesAtTime[j.SummarizerJobs[len(j.SummarizerJobs)-1]]

			csResult := api.ComplianceStatusSummaryV2{}
			addToResults := func(resultGroup types.ResultGroup) {
				csResult.AddESComplianceStatusMap(resultGroup.Result.QueryResult)
			}

			addToResults(summaryAtTime.Integrations.BenchmarkResult)
			var complianceScore float64
			if csResult.TotalCount > 0 {
				complianceScore = float64(csResult.PassedCount) / float64(csResult.TotalCount)
			} else {
				complianceScore = 0
			}
			item.ComplianceResultsSummary = csResult
			item.ComplianceScore = complianceScore
		}

		items = append(items, item)
	}

	return ctx.JSON(http.StatusOK, api.ListComplianceJobsHistoryResponse{
		Items:      items,
		TotalCount: jobs.TotalCount,
	})
}

// ListBenchmarksNestedForBenchmark godoc
//
//	@Summary	List benchmarks filtered by integrations and other filters
//	@Security	BearerToken
//	@Tags		compliance
//	@Accept		json
//	@Produce	json
//	@Success	200	{object}	[]api.GetBenchmarkListResponse
//	@Router		/compliance/api/v3/benchmarks/{benchmark_id}/nested [get]
func (h *HttpHandler) ListBenchmarksNestedForBenchmark(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()
	benchmarkId := echoCtx.Param("benchmark_id")
	if benchmarkId == "" {
		return echo.NewHTTPError(http.StatusInternalServerError, "please provide a benchmark id")
	}

	nested, err := h.getBenchmarkTree(ctx, benchmarkId)
	if err != nil {
		h.logger.Error("could not get benchmark tree", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "could not get benchmark tree")
	}

	return echoCtx.JSON(http.StatusOK, nested)
}

// GetBenchmarkTrendV3 godoc
//
//	@Summary		Get benchmark trend
//	@Description	Retrieving a trend of a benchmark result and checks.
//	@Security		BearerToken
//	@Tags			compliance
//	@Accept			json
//	@Produce		json
//	@Param			benchmark_id	path		string							true	"Benchmark ID"
//	@Param			request			body		api.GetBenchmarkTrendV3Request	false	"timestamp for end of the chart in epoch seconds"
//	@Success		200				{object}	api.GetBenchmarkTrendV3Response
//	@Router			/compliance/api/v3/benchmarks/{benchmark_id}/trend [post]
func (h *HttpHandler) GetBenchmarkTrendV3(echoCtx echo.Context) error {
	ctx := echoCtx.Request().Context()
	clientCtx := &httpclient.Context{UserRole: authApi.AdminRole}

	var req api.GetBenchmarkTrendV3Request
	if err := bindValidate(echoCtx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	endTime := time.Now().Truncate(24 * time.Hour).Add(24 * time.Hour).Unix()
	if req.EndTime != nil {
		endTime = *req.EndTime
	}

	startTime := time.Unix(endTime, 0).AddDate(0, 0, -7).Truncate(24 * time.Hour).Unix()
	if req.StartTime != nil {
		startTime = *req.StartTime
	}

	granularity := int64((time.Hour * 24).Seconds())
	if req.Granularity != nil {
		granularity = *req.Granularity
	}
	benchmarkID := echoCtx.Param("benchmark_id")
	// tracer :
	ctx, span1 := tracer.Start(ctx, "new_GetBenchmark")
	span1.SetName("new_GetBenchmark")
	defer span1.End()

	benchmark, err := h.db.GetBenchmark(ctx, benchmarkID)
	if err != nil {
		span1.RecordError(err)
		span1.SetStatus(codes.Error, err.Error())
		return err
	}

	if benchmark == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid benchmarkID")
	}
	span1.AddEvent("information", trace.WithAttributes(
		attribute.String("benchmark ID", benchmark.ID),
	))
	span1.End()

	var integrations []integrationapi.Integration
	for _, info := range req.Integration {
		if info.IntegrationID != nil {
			integration, err := h.integrationClient.GetIntegration(clientCtx, *info.IntegrationID)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			if integration != nil {
				integrations = append(integrations, *integration)
			}
			continue
		}
		var integrationTypes []string
		if info.IntegrationType != nil {
			integrationTypes = []string{*info.IntegrationType}
		}
		var integrationIDs []string
		if info.IntegrationID != nil {
			integrationIDs = []string{*info.IntegrationID}
		}
		integrationsTmp, err := h.integrationClient.ListIntegrationsByFilters(clientCtx,
			integrationapi.ListIntegrationsRequest{
				IntegrationType: integrationTypes,
				NameRegex:       info.Name,
				ProviderIDRegex: info.ProviderID,
				IntegrationID:   integrationIDs,
			})
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		integrations = append(integrations, integrationsTmp.Integrations...)
	}

	var integrationIDs []string
	for _, c := range integrations {
		integrationIDs = append(integrationIDs, c.IntegrationID)
	}

	evaluationAcrossTime, err := es.FetchBenchmarkSummaryTrendByIntegrationIDV3(ctx, h.logger, h.client,
		[]string{benchmarkID}, integrationIDs, startTime, endTime, granularity)
	if err != nil {
		return err
	}

	var datapoints []api.BenchmarkTrendDatapointV3
	var minimumIncidents, minimumNonIncidents, minimumNone, minimumLow, minimumMedium, minimumHigh, minimumCritical int
	var maximumIncidents, maximumNonIncidents, maximumNone, maximumLow, maximumMedium, maximumHigh, maximumCritical int
	for _, datapoint := range evaluationAcrossTime[benchmarkID] {
		apiDataPoint := api.BenchmarkTrendDatapointV3{
			Timestamp:                  time.Unix(datapoint.DateEpoch, 0),
			IncidentsSeverityBreakdown: &opengovernanceTypes.SeverityResult{},
		}
		complianceSummary := api.ComplianceStatusSummary{}
		if len(datapoint.QueryResult) > 0 {
			complianceSummary.AddESComplianceStatusMap(datapoint.QueryResult)
		}
		if len(datapoint.SeverityResult) > 0 {
			apiDataPoint.IncidentsSeverityBreakdown.AddResultMap(datapoint.SeverityResult)
		}
		if complianceSummary.FailedCount == 0 && complianceSummary.PassedCount == 0 {
			apiDataPoint.IncidentsSeverityBreakdown = nil
		} else {
			apiDataPoint.ComplianceResultsSummary = &struct {
				Incidents    int `json:"incidents"`
				NonIncidents int `json:"non_incidents"`
			}{Incidents: complianceSummary.FailedCount, NonIncidents: complianceSummary.PassedCount}
		}

		if apiDataPoint.ComplianceResultsSummary != nil {
			if maximumIncidents < apiDataPoint.ComplianceResultsSummary.Incidents {
				maximumIncidents = apiDataPoint.ComplianceResultsSummary.Incidents
			}
			if maximumNonIncidents < apiDataPoint.ComplianceResultsSummary.NonIncidents {
				maximumNonIncidents = apiDataPoint.ComplianceResultsSummary.NonIncidents
			}

			if minimumIncidents > apiDataPoint.ComplianceResultsSummary.Incidents {
				minimumIncidents = apiDataPoint.ComplianceResultsSummary.Incidents
			}
			if minimumNonIncidents > apiDataPoint.ComplianceResultsSummary.NonIncidents {
				minimumNonIncidents = apiDataPoint.ComplianceResultsSummary.NonIncidents
			}
		}
		if apiDataPoint.IncidentsSeverityBreakdown != nil {
			if maximumNone < apiDataPoint.IncidentsSeverityBreakdown.NoneCount {
				maximumNone = apiDataPoint.IncidentsSeverityBreakdown.NoneCount
			}
			if maximumLow < apiDataPoint.IncidentsSeverityBreakdown.LowCount {
				maximumLow = apiDataPoint.IncidentsSeverityBreakdown.LowCount
			}
			if maximumMedium < apiDataPoint.IncidentsSeverityBreakdown.MediumCount {
				maximumMedium = apiDataPoint.IncidentsSeverityBreakdown.MediumCount
			}
			if maximumHigh < apiDataPoint.IncidentsSeverityBreakdown.HighCount {
				maximumHigh = apiDataPoint.IncidentsSeverityBreakdown.HighCount
			}
			if maximumCritical < apiDataPoint.IncidentsSeverityBreakdown.CriticalCount {
				maximumCritical = apiDataPoint.IncidentsSeverityBreakdown.CriticalCount
			}

			if minimumNone > apiDataPoint.IncidentsSeverityBreakdown.NoneCount {
				minimumNone = apiDataPoint.IncidentsSeverityBreakdown.NoneCount
			}
			if minimumLow > apiDataPoint.IncidentsSeverityBreakdown.LowCount {
				minimumLow = apiDataPoint.IncidentsSeverityBreakdown.LowCount
			}
			if minimumMedium > apiDataPoint.IncidentsSeverityBreakdown.MediumCount {
				minimumMedium = apiDataPoint.IncidentsSeverityBreakdown.MediumCount
			}
			if minimumHigh > apiDataPoint.IncidentsSeverityBreakdown.HighCount {
				minimumHigh = apiDataPoint.IncidentsSeverityBreakdown.HighCount
			}
			if minimumCritical > apiDataPoint.IncidentsSeverityBreakdown.CriticalCount {
				minimumCritical = apiDataPoint.IncidentsSeverityBreakdown.CriticalCount
			}
		}

		datapoints = append(datapoints, apiDataPoint)
	}

	sort.Slice(datapoints, func(i, j int) bool {
		return datapoints[i].Timestamp.Before(datapoints[j].Timestamp)
	})

	response := api.GetBenchmarkTrendV3Response{
		Datapoints: datapoints,
		MaximumValues: api.BenchmarkTrendDatapointV3{
			ComplianceResultsSummary: &struct {
				Incidents    int `json:"incidents"`
				NonIncidents int `json:"non_incidents"`
			}{Incidents: maximumIncidents, NonIncidents: maximumNonIncidents},
			IncidentsSeverityBreakdown: &types2.SeverityResult{
				NoneCount:     maximumNone,
				LowCount:      maximumLow,
				MediumCount:   maximumMedium,
				HighCount:     maximumHigh,
				CriticalCount: maximumCritical,
			},
		},
		MinimumValues: api.BenchmarkTrendDatapointV3{
			ComplianceResultsSummary: &struct {
				Incidents    int `json:"incidents"`
				NonIncidents int `json:"non_incidents"`
			}{Incidents: minimumIncidents, NonIncidents: minimumNonIncidents},
			IncidentsSeverityBreakdown: &types2.SeverityResult{
				NoneCount:     minimumNone,
				LowCount:      minimumLow,
				MediumCount:   minimumMedium,
				HighCount:     minimumHigh,
				CriticalCount: minimumCritical,
			},
		},
	}

	return echoCtx.JSON(http.StatusOK, response)
}

func parseTimeInterval(intervalStr string) (*time.Time, *time.Time, error) {
	// Define regex patterns to extract the time components
	patterns := map[string]*regexp.Regexp{
		"days":    regexp.MustCompile(`(\d+)\s*days?`),
		"hours":   regexp.MustCompile(`(\d+)\s*hours?`),
		"minutes": regexp.MustCompile(`(\d+)\s*minutes?`),
		"seconds": regexp.MustCompile(`(\d+)\s*seconds?`),
	}

	// Variables to store the extracted values
	days, hours, minutes, seconds := 0, 0, 0, 0

	// Extract and convert the values from the string
	for key, pattern := range patterns {
		match := pattern.FindStringSubmatch(intervalStr)
		if len(match) > 1 {
			value, err := strconv.Atoi(match[1])
			if err != nil {
				return nil, nil, fmt.Errorf("error parsing %s: %v", key, err)
			}
			switch key {
			case "days":
				days = value
			case "hours":
				hours = value
			case "minutes":
				minutes = value
			case "seconds":
				seconds = value
			}
		}
	}

	// Calculate total duration based on extracted values
	duration := time.Duration(days)*24*time.Hour +
		time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second

	// Calculate endTime as now and startTime by subtracting the duration
	endTime := time.Now()
	startTime := endTime.Add(-duration)

	return &startTime, &endTime, nil
}
