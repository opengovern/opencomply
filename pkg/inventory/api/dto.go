package api

import (
	"time"

	"github.com/opengovern/og-util/pkg/es"
	"github.com/opengovern/og-util/pkg/opengovernance-es-sdk"
	"github.com/opengovern/og-util/pkg/source"
)

type DirectionType string

const (
	DirectionAscending  DirectionType = "asc"
	DirectionDescending DirectionType = "desc"
)

type SortFieldType string

const (
	SortFieldResourceID    SortFieldType = "resourceID"
	SortFieldConnector     SortFieldType = "connector"
	SortFieldResourceType  SortFieldType = "resourceType"
	SortFieldResourceGroup SortFieldType = "resourceGroup"
	SortFieldLocation      SortFieldType = "location"
	SortFieldConnectionID  SortFieldType = "connectionID"
)

type CostWithUnit struct {
	Cost float64 `json:"cost"` // Value
	Unit string  `json:"unit"` // Currency
}

type Page struct {
	No   int `json:"no,omitempty"`
	Size int `json:"size,omitempty"`
}

// ResourceFilters model
//
//	@Description	if you provide two values for same filter OR operation would be used
//	@Description	if you provide value for two filters AND operation would be used
type ResourceFilters struct {
	// if you dont need to use this filter, leave them empty. (e.g. [])
	ResourceType []string `json:"resourceType"`
	// if you dont need to use this filter, leave them empty. (e.g. [])
	Category []string `json:"category"`
	// if you dont need to use this filter, leave them empty. (e.g. [])
	Service []string `json:"service"`
	// if you dont need to use this filter, leave them empty. (e.g. [])
	Location []string `json:"location"`
	// if you dont need to use this filter, leave them empty. (e.g. [])
	Provider []string `json:"provider"`
	// if you dont need to use this filter, leave them empty. (e.g. [])
	Connections []string `json:"connections"`
	// if you dont need to use this filter, leave them empty. (e.g. [])
	TagKeys []string `json:"tagKeys"`
	// if you dont need to use this filter, leave them empty. (e.g. [])
	TagValues map[string][]string `json:"tagValues"`
}

type ResourceSortItem struct {
	Field     SortFieldType `json:"field" enums:"resourceID,connector,resourceType,resourceGroup,location,connectionID"`
	Direction DirectionType `json:"direction" enums:"asc,desc"`
}

type NamedQuerySortItem struct {
	// fill this with column name
	Field     string        `json:"field"`
	Direction DirectionType `json:"direction" enums:"asc,desc"`
}

type AllResource struct {
	ResourceName           string      `json:"resourceName"`           // Resource Name
	ResourceID             string      `json:"resourceID"`             // Resource Id
	ResourceType           string      `json:"resourceType"`           // Resource Type
	ResourceTypeLabel      string      `json:"resourceTypeLabel"`      // Resource Type Label
	Connector              source.Type `json:"connector"`              // Resource Provider
	Location               string      `json:"location"`               // The Region of the resource
	ConnectionID           string      `json:"connectionID"`           // Platform Connection Id of the resource
	ProviderConnectionID   string      `json:"providerConnectionID"`   // Platform Connection Id
	ProviderConnectionName string      `json:"providerConnectionName"` // Provider Connection Name

	Attributes map[string]string `json:"attributes"`
}

type AzureResource struct {
	ResourceName           string `json:"resourceName"`           // Resource Name
	ResourceID             string `json:"resourceID"`             // Resource Id
	ResourceType           string `json:"resourceType"`           // Resource Type
	ResourceTypeLabel      string `json:"resourceTypeLabel"`      // Resource Type Label
	ResourceGroup          string `json:"resourceGroup"`          // Resource Group
	Location               string `json:"location"`               // The Region of the resource
	ConnectionID           string `json:"connectionID"`           // Kaytu Connection Id of the resource
	ProviderConnectionID   string `json:"providerConnectionID"`   // Provider Connection Id
	ProviderConnectionName string `json:"providerConnectionName"` // Provider Connection Name

	Attributes map[string]string `json:"attributes"`
}

type AWSResource struct {
	ResourceName           string `json:"resourceName"`
	ResourceID             string `json:"resourceID"`
	ResourceType           string `json:"resourceType"`
	ResourceTypeLabel      string `json:"resourceTypeLabel"`
	Location               string `json:"location"`
	ConnectionID           string `json:"connectionID"`
	ProviderConnectionID   string `json:"ProviderConnectionID"`
	ProviderConnectionName string `json:"providerConnectionName"`

	Attributes map[string]string `json:"attributes"`
}

type SummaryQueryResponse struct {
	Hits SummaryQueryHits `json:"hits"`
}
type SummaryQueryHits struct {
	Total opengovernance.SearchTotal `json:"total"`
	Hits  []SummaryQueryHit          `json:"hits"`
}
type SummaryQueryHit struct {
	ID      string            `json:"_id"`
	Score   float64           `json:"_score"`
	Index   string            `json:"_index"`
	Type    string            `json:"_type"`
	Version int64             `json:"_version,omitempty"`
	Source  es.LookupResource `json:"_source"`
	Sort    []any             `json:"sort"`
}

type NamedQueryItem struct {
	ID         string            `json:"id"`         // Query Id
	Connectors []source.Type     `json:"connectors"` // Provider
	Title      string            `json:"title"`      // Title
	Category   string            `json:"category"`   // Category (Tags[category])
	Query      string            `json:"query"`      // Query
	Tags       map[string]string `json:"tags"`       // Tags
}

type ListQueriesV2Response struct {
	Items      []NamedQueryItemV2 `json:"items"`
	TotalCount int                `json:"total_count"`
}

type NamedQueryItemV2 struct {
	ID          string              `json:"id"`    // Query Id
	Title       string              `json:"title"` // Title
	Description string              `json:"description"`
	Connectors  []source.Type       `json:"connectors"` // Provider
	Query       Query               `json:"query"`      // Query
	Tags        map[string][]string `json:"tags"`       // Tags
}

type QueryParameter struct {
	Key      string `json:"key" example:"key"`
	Required bool   `json:"required" example:"true"`
}

type Query struct {
	ID             string           `json:"id" example:"azure_ad_manual_control"`
	QueryToExecute string           `json:"queryToExecute" example:"select\n  -- Required Columns\n  'active_directory' as resource,\n  'info' as status,\n  'Manual verification required.' as reason;\n"`
	PrimaryTable   *string          `json:"primaryTable" example:"null"`
	ListOfTables   []string         `json:"listOfTables" example:"null"`
	Engine         string           `json:"engine" example:"steampipe-v0.5"`
	Parameters     []QueryParameter `json:"parameters"`
	Global         bool             `json:"Global"`
	CreatedAt      time.Time        `json:"createdAt" example:"2023-06-07T14:00:15.677558Z"`
	UpdatedAt      time.Time        `json:"updatedAt" example:"2023-06-16T14:58:08.759554Z"`
}

type ListQueryV2Request struct {
	TitleFilter   string              `json:"titleFilter"`
	Providers     []string            `json:"providers"`
	HasParameters *bool               `json:"has_parameters"`
	PrimaryTable  []string            `json:"primary_table"`
	ListOfTables  []string            `json:"list_of_tables"`
	Tags          map[string][]string `json:"tags"`
	TagsRegex     *string             `json:"tags_regex"`
	Cursor        *int64              `json:"cursor"`
	PerPage       *int64              `json:"per_page"`
}

type ListQueryRequest struct {
	TitleFilter string `json:"titleFilter"` // Specifies the Title
}

type ConnectionData struct {
	ConnectionID         string     `json:"connectionID"`
	Count                *int       `json:"count"`
	OldCount             *int       `json:"oldCount"`
	LastInventory        *time.Time `json:"lastInventory"`
	TotalCost            *float64   `json:"cost"`
	DailyCostAtStartTime *float64   `json:"dailyCostAtStartTime"`
	DailyCostAtEndTime   *float64   `json:"dailyCostAtEndTime"`
}

type CountAnalyticsMetricsResponse struct {
	ConnectionCount int `json:"connectionCount"`
	MetricCount     int `json:"metricCount"`
}

type CountAnalyticsSpendResponse struct {
	ConnectionCount int `json:"connectionCount"`
	MetricCount     int `json:"metricCount"`
}

type ParametersQueries struct {
	Parameter string             `json:"parameter"`
	Queries   []NamedQueryItemV2 `json:"queries"`
}

type GetParametersQueriesResponse struct {
	ParametersQueries []ParametersQueries `json:"parameters"`
}
