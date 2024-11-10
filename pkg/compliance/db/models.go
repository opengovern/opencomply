package db

import (
	"context"
	"fmt"
	"github.com/jackc/pgtype"
	"github.com/opengovern/opengovernance/pkg/utils"
	integration_type "github.com/opengovern/opengovernance/services/integration/integration-type"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"time"

	"github.com/lib/pq"
	"github.com/opengovern/og-util/pkg/model"
	"github.com/opengovern/opengovernance/pkg/types"

	"github.com/opengovern/opengovernance/pkg/compliance/api"

	"gorm.io/gorm"
)

type BenchmarkAssignment struct {
	gorm.Model
	BenchmarkId        string  `gorm:"index:idx_benchmark_source; index:idx_benchmark_rc; not null"`
	IntegrationID      *string `gorm:"index:idx_benchmark_source"`
	ResourceCollection *string `gorm:"index:idx_benchmark_rc"`
	AssignedAt         time.Time
}

type BenchmarkAssignmentsCount struct {
	BenchmarkId string
	Count       int
}

type BenchmarkMetadata struct {
	IsRoot        bool
	Controls      []string
	PrimaryTables []string
	ListOfTables  []string
	BenchmarkPath string
}

type Benchmark struct {
	ID                string `gorm:"primarykey"`
	Title             string
	DisplayCode       string
	IntegrationType   pq.StringArray `gorm:"type:text[]"`
	Description       string
	LogoURI           string
	Category          string
	DocumentURI       string
	Enabled           bool
	AutoAssign        bool
	TracksDriftEvents bool
	Metadata          pgtype.JSONB

	Tags    []BenchmarkTag      `gorm:"foreignKey:BenchmarkID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	tagsMap map[string][]string `gorm:"-:all"`

	Children  []Benchmark `gorm:"many2many:benchmark_children;"`
	Controls  []Control   `gorm:"many2many:benchmark_controls;"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (b Benchmark) ToApi() api.Benchmark {
	ba := api.Benchmark{
		ID:                b.ID,
		Title:             b.Title,
		ReferenceCode:     b.DisplayCode,
		Description:       b.Description,
		LogoURI:           b.LogoURI,
		Category:          b.Category,
		DocumentURI:       b.DocumentURI,
		AutoAssign:        b.AutoAssign,
		TracksDriftEvents: b.TracksDriftEvents,
		CreatedAt:         b.CreatedAt,
		UpdatedAt:         b.UpdatedAt,
		Tags:              b.GetTagsMap(),
	}
	if b.IntegrationType != nil {
		ba.IntegrationTypes = b.IntegrationType
	}
	for _, child := range b.Children {
		ba.Children = append(ba.Children, child.ID)
	}
	for _, control := range b.Controls {
		ba.Controls = append(ba.Controls, control.ID)
	}
	return ba
}

func (b Benchmark) GetTagsMap() map[string][]string {
	if b.tagsMap == nil {
		tagLikeArr := make([]model.TagLike, 0, len(b.Tags))
		for _, tag := range b.Tags {
			tagLikeArr = append(tagLikeArr, tag)
		}
		b.tagsMap = model.GetTagsMap(tagLikeArr)
	}
	return b.tagsMap
}

type BenchmarkChild struct {
	BenchmarkID string
	ChildID     string
}

type BenchmarkTag struct {
	model.Tag
	BenchmarkID string `gorm:"primaryKey"`
}

type ControlTagsResult struct {
	Key          string
	UniqueValues pq.StringArray `gorm:"type:text[]"`
}

func (s ControlTagsResult) ToApi() api.ControlTagsResult {
	return api.ControlTagsResult{
		Key:          s.Key,
		UniqueValues: s.UniqueValues,
	}
}

type BenchmarkTagsResult struct {
	Key          string
	UniqueValues pq.StringArray `gorm:"type:text[]"`
}

func (s BenchmarkTagsResult) ToApi() api.BenchmarkTagsResult {
	return api.BenchmarkTagsResult{
		Key:          s.Key,
		UniqueValues: s.UniqueValues,
	}
}

type Control struct {
	ID          string `gorm:"primaryKey"`
	Title       string
	Description string

	Tags    []ControlTag        `gorm:"foreignKey:ControlID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	tagsMap map[string][]string `gorm:"-:all"`

	IntegrationType    pq.StringArray `gorm:"type:text[]"`
	DocumentURI        string
	Enabled            bool
	QueryID            *string
	Query              *Query      `gorm:"foreignKey:QueryID;references:ID;constraint:OnDelete:SET NULL"`
	Benchmarks         []Benchmark `gorm:"many2many:benchmark_controls;"`
	Severity           types.ComplianceResultSeverity
	ManualVerification bool
	Managed            bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (p Control) ToApi() api.Control {
	pa := api.Control{
		ID:                 p.ID,
		Title:              p.Title,
		Description:        p.Description,
		Tags:               model.TrimPrivateTags(p.GetTagsMap()),
		Explanation:        "",
		NonComplianceCost:  "",
		UsefulExample:      "",
		IntegrationType:    nil,
		Enabled:            p.Enabled,
		DocumentURI:        p.DocumentURI,
		Severity:           p.Severity,
		ManualVerification: p.ManualVerification,
		Managed:            p.Managed,
		CreatedAt:          p.CreatedAt,
		UpdatedAt:          p.UpdatedAt,
	}

	if p.QueryID != nil {
		pa.Query = &api.Query{
			ID: *p.QueryID,
		}
	}
	if p.Query != nil {
		pa.Query = utils.GetPointer(p.Query.ToApi())
	}

	if v, ok := p.GetTagsMap()[model.OpenGovernancePrivateTagPrefix+"explanation"]; ok && len(v) > 0 {
		pa.Explanation = v[0]
	}
	if v, ok := p.GetTagsMap()[model.OpenGovernancePrivateTagPrefix+"noncompliance-cost"]; ok && len(v) > 0 {
		pa.NonComplianceCost = v[0]
	}
	if v, ok := p.GetTagsMap()[model.OpenGovernancePrivateTagPrefix+"usefulness-example"]; ok && len(v) > 0 {
		pa.UsefulExample = v[0]
	}
	if v, ok := p.GetTagsMap()[model.OpenGovernancePrivateTagPrefix+"manual-remediation"]; ok && len(v) > 0 {
		pa.ManualRemediation = v[0]
	}
	if v, ok := p.GetTagsMap()[model.OpenGovernancePrivateTagPrefix+"cli-remediation"]; ok && len(v) > 0 {
		pa.CliRemediation = v[0]
	}
	if v, ok := p.GetTagsMap()[model.OpenGovernancePrivateTagPrefix+"programmatic-remediation"]; ok && len(v) > 0 {
		pa.ProgrammaticRemediation = v[0]
	}
	if v, ok := p.GetTagsMap()[model.OpenGovernancePrivateTagPrefix+"guardrail-remediation"]; ok && len(v) > 0 {
		pa.GuardrailRemediation = v[0]
	}

	return pa
}

func (p Control) GetTagsMap() map[string][]string {
	if p.tagsMap == nil {
		tagLikeArr := make([]model.TagLike, 0, len(p.Tags))
		for _, tag := range p.Tags {
			tagLikeArr = append(tagLikeArr, tag)
		}
		p.tagsMap = model.GetTagsMap(tagLikeArr)
	}
	return p.tagsMap
}

func (p *Control) PopulateConnector(ctx context.Context, db Database, api *api.Control) error {
	tracer := otel.Tracer("PopulateConnector")
	if api.IntegrationType == nil || len(api.IntegrationType) > 0 {
		return nil
	}

	if p.QueryID == nil {
		return nil
	}
	// tracer :
	_, span1 := tracer.Start(ctx, "new_GetQuery", trace.WithSpanKind(trace.SpanKindServer))
	span1.SetName("new_GetQuery")

	query, err := db.GetQuery(ctx, *p.QueryID)
	if err != nil {
		span1.RecordError(err)
		span1.SetStatus(codes.Error, err.Error())
		return err
	}
	span1.AddEvent("information", trace.WithAttributes(
		attribute.String("control id", p.ID),
	))
	span1.End()

	if query == nil {
		return fmt.Errorf("query %s not found", *p.QueryID)
	}

	ty := query.IntegrationType

	api.IntegrationType = ty
	return nil
}

type ControlTag struct {
	model.Tag
	ControlID string `gorm:"primaryKey"`
}

type BenchmarkControls struct {
	BenchmarkID string
	ControlID   string
}

type QueryParameter struct {
	QueryID  string `gorm:"primaryKey"`
	Key      string `gorm:"primaryKey"`
	Required bool   `gorm:"not null"`
}

func (qp QueryParameter) ToApi() api.QueryParameter {
	return api.QueryParameter{
		Key:      qp.Key,
		Required: qp.Required,
	}
}

type Query struct {
	ID              string `gorm:"primaryKey"`
	QueryToExecute  string
	IntegrationType pq.StringArray `gorm:"type:text[]"`
	PrimaryTable    *string
	ListOfTables    pq.StringArray `gorm:"type:text[]"`
	Engine          string
	Controls        []Control        `gorm:"foreignKey:QueryID"`
	Parameters      []QueryParameter `gorm:"foreignKey:QueryID"`
	Global          bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (q Query) ToApi() api.Query {
	query := api.Query{
		ID:              q.ID,
		QueryToExecute:  q.QueryToExecute,
		IntegrationType: integration_type.ParseTypes(q.IntegrationType),
		ListOfTables:    q.ListOfTables,
		PrimaryTable:    q.PrimaryTable,
		Engine:          q.Engine,
		Parameters:      make([]api.QueryParameter, 0, len(q.Parameters)),
		Global:          q.Global,
		CreatedAt:       q.CreatedAt,
		UpdatedAt:       q.UpdatedAt,
	}
	for _, p := range q.Parameters {
		query.Parameters = append(query.Parameters, p.ToApi())
	}
	return query
}
