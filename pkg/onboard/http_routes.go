package onboard

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gitlab.com/keibiengine/keibi-engine/pkg/source"

	api3 "gitlab.com/keibiengine/keibi-engine/pkg/auth/api"

	keibiaws "gitlab.com/keibiengine/keibi-engine/pkg/aws"

	"gitlab.com/keibiengine/keibi-engine/pkg/internal/httpserver"

	"gitlab.com/keibiengine/keibi-engine/pkg/onboard/connector"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gitlab.com/keibiengine/keibi-engine/pkg/describe"
	"gitlab.com/keibiengine/keibi-engine/pkg/onboard/api"
	"gorm.io/gorm"
)

const (
	paramSourceId = "sourceId"
)

func (h HttpHandler) Register(r *echo.Echo) {
	v1 := r.Group("/api/v1")

	v1.GET("/sources", httpserver.AuthorizeHandler(h.ListSources, api3.ViewerRole))
	v1.POST("/sources", httpserver.AuthorizeHandler(h.GetSources, api3.ViewerRole))
	v1.GET("/sources/count", httpserver.AuthorizeHandler(h.CountSources, api3.ViewerRole))

	v1.GET("/providers", httpserver.AuthorizeHandler(h.GetProviders, api3.ViewerRole))
	v1.GET("/providers/types", httpserver.AuthorizeHandler(h.GetProviderTypes, api3.ViewerRole))

	v1.GET("/connectors/categories", httpserver.AuthorizeHandler(h.GetConnectorCategories, api3.ViewerRole))
	v1.GET("/connectors", httpserver.AuthorizeHandler(h.GetConnector, api3.ViewerRole))

	source := v1.Group("/source")

	source.POST("/aws", httpserver.AuthorizeHandler(h.PostSourceAws, api3.EditorRole))
	source.POST("/azure", httpserver.AuthorizeHandler(h.PostSourceAzure, api3.EditorRole))
	source.POST("/azure/spn", httpserver.AuthorizeHandler(h.PostSourceAzureSPN, api3.EditorRole))
	source.GET("/:sourceId", httpserver.AuthorizeHandler(h.GetSource, api3.ViewerRole))
	source.GET("/:sourceId/healthcheck", httpserver.AuthorizeHandler(h.GetSourceHealth, api3.EditorRole))
	source.GET("/:sourceId/credentials", httpserver.AuthorizeHandler(h.GetSourceCred, api3.ViewerRole))
	source.PUT("/:sourceId/credentials", httpserver.AuthorizeHandler(h.PutSourceCred, api3.EditorRole))
	source.PUT("/:sourceId", httpserver.AuthorizeHandler(h.PutSource, api3.EditorRole))
	source.POST("/:sourceId/disable", httpserver.AuthorizeHandler(h.DisableSource, api3.EditorRole))
	source.POST("/:sourceId/enable", httpserver.AuthorizeHandler(h.EnableSource, api3.EditorRole))
	source.DELETE("/:sourceId", httpserver.AuthorizeHandler(h.DeleteSource, api3.EditorRole))

	spn := v1.Group("/spn")

	spn.POST("/azure", httpserver.AuthorizeHandler(h.PostSPN, api3.EditorRole))
	spn.DELETE("/:spnId", httpserver.AuthorizeHandler(h.DeleteSPN, api3.EditorRole))
	spn.GET("/:spnId", httpserver.AuthorizeHandler(h.GetSPNCred, api3.ViewerRole))
	spn.GET("/list", httpserver.AuthorizeHandler(h.ListSPNs, api3.ViewerRole))
	spn.PUT("/:spnId", httpserver.AuthorizeHandler(h.PutSPNCred, api3.EditorRole))

	disc := v1.Group("/discover")

	disc.POST("/aws/accounts", httpserver.AuthorizeHandler(h.DiscoverAwsAccounts, api3.EditorRole))
	disc.POST("/azure/subscriptions", httpserver.AuthorizeHandler(h.DiscoverAzureSubscriptions, api3.EditorRole))
	disc.POST("/azure/subscriptions/spn", httpserver.AuthorizeHandler(h.DiscoverAzureSubscriptionsWithSPN, api3.EditorRole))
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

// GetProviders godoc
// @Summary     Get providers
// @Description Getting cloud providers
// @Tags        onboard
// @Produce     json
// @Success     200 {object} api.ProvidersResponse
// @Router      /onboard/api/v1/providers [get]
func (h HttpHandler) GetProviders(ctx echo.Context) error {
	return ctx.JSON(http.StatusOK, api.ProvidersResponse{
		{Name: "Sumo Logic", ID: "sumologic", Type: "IT Operations", State: api.ProviderStateDisabled},
		{Name: "Akamai", ID: "akamai", Type: "Content Delivery (CDN)", State: api.ProviderStateDisabled},
		{Name: "Box", ID: "boxnet", Type: "Cloud Storage", State: api.ProviderStateDisabled},
		{Name: "DropBox", ID: "dropbox", Type: "Cloud Storage", State: api.ProviderStateDisabled},
		{Name: "Microsoft OneDrive", ID: "onedrive", Type: "Cloud Storage", State: api.ProviderStateDisabled},
		{Name: "Kubernetes", ID: "kubernetes", Type: "Cointainer Orchestrator", State: api.ProviderStateComingSoon},
		{Name: "Box", ID: "boxnet", Type: "Collaboration & Productivity", State: api.ProviderStateDisabled},
		{Name: "DocuSign", ID: "docusign", Type: "Collaboration & Productivity", State: api.ProviderStateDisabled},
		{Name: "Google Workspace", ID: "googleworkspace", Type: "Collaboration & Productivity", State: api.ProviderStateDisabled},
		{Name: "Microsoft Office 365", ID: "o365", Type: "Collaboration & Productivity", State: api.ProviderStateDisabled},
		{Name: "Microsoft SharePoint", ID: "sharepoint", Type: "Collaboration & Productivity", State: api.ProviderStateDisabled},
		{Name: "Microsoft Teams", ID: "teams", Type: "Collaboration & Productivity", State: api.ProviderStateDisabled},
		{Name: "Slack", ID: "slack", Type: "Collaboration & Productivity", State: api.ProviderStateDisabled},
		{Name: "Trello", ID: "trello", Type: "Collaboration & Productivity", State: api.ProviderStateDisabled},
		{Name: "Zoom", ID: "zoom", Type: "Collaboration & Productivity", State: api.ProviderStateDisabled},
		{Name: "Mailchimp", ID: "mailchimp", Type: "Communications", State: api.ProviderStateDisabled},
		{Name: "PagerDuty", ID: "pagerduty", Type: "Communications", State: api.ProviderStateDisabled},
		{Name: "RingCentral", ID: "ringcentral", Type: "Communications", State: api.ProviderStateDisabled},
		{Name: "Twilio SendGrid", ID: "sendgrid", Type: "Communications", State: api.ProviderStateDisabled},
		{Name: "Mailchimp", ID: "mailchimp", Type: "Communications", State: api.ProviderStateDisabled},
		{Name: "Mailgun", ID: "mailgun", Type: "Communications", State: api.ProviderStateDisabled},
		{Name: "Rubrik", ID: "rubrik", Type: "Data Management", State: api.ProviderStateDisabled},
		{Name: "Snowflake", ID: "snowflake", Type: "Data Management", State: api.ProviderStateDisabled},
		{Name: "talend.com", ID: "talend", Type: "Data Management", State: api.ProviderStateDisabled},
		{Name: "MongoDB Atlas", ID: "mongodbatlast", Type: "Databases", State: api.ProviderStateDisabled},
		{Name: "Elastic Cloud", ID: "elasticcloud", Type: "Databases", State: api.ProviderStateDisabled},
		{Name: "Okta", ID: "okta", Type: "Identity", State: api.ProviderStateDisabled},
		{Name: "JumpCloud", ID: "jumpcloud", Type: "Identity", State: api.ProviderStateDisabled},
		{Name: "Ping Identity", ID: "pingidentity", Type: "Identity", State: api.ProviderStateDisabled},
		{Name: "Auth0.com", ID: "auth0", Type: "Identity", State: api.ProviderStateDisabled},
		{Name: "Microsoft Azure Active Directory", ID: "azuread", Type: "Identity", State: api.ProviderStateComingSoon},
		{Name: "OneLogin", ID: "onelogin", Type: "Identity", State: api.ProviderStateDisabled},
		{Name: "Expensify", ID: "expensify", Type: "Enterprise Applications", State: api.ProviderStateDisabled},
		{Name: "Salesforce", ID: "salesforce", Type: "Enterprise Applications", State: api.ProviderStateDisabled},
		{Name: "Xero", ID: "xero", Type: "Enterprise Applications", State: api.ProviderStateDisabled},
		{Name: "AppViewX", ID: "appviewx", Type: "Enterprise Security", State: api.ProviderStateDisabled},
		{Name: "Rapid7", ID: "rapid7", Type: "Enterprise Security", State: api.ProviderStateDisabled},
		{Name: "Akamai", ID: "akamai", Type: "Edge Compute", State: api.ProviderStateDisabled},
		{Name: "Akamai", ID: "akamai", Type: "Enterprise Security", State: api.ProviderStateDisabled},
		{Name: "Imperva", ID: "imperva", Type: "Enterprise Security", State: api.ProviderStateDisabled},
		{Name: "Cloudflare", ID: "cloudfare", Type: "Enterprise Security", State: api.ProviderStateDisabled},
		{Name: "CyberArk", ID: "cuberark", Type: "Enterprise Security", State: api.ProviderStateDisabled},
		{Name: "Blackberry CylanceProtect", ID: "cylance", Type: "Enterprise Security", State: api.ProviderStateDisabled},
		{Name: "Cisco Duo", ID: "duo", Type: "Enterprise Security", State: api.ProviderStateDisabled},
		{Name: "OneLogin", ID: "onelogin", Type: "Enterprise Security", State: api.ProviderStateDisabled},
		{Name: "OneTrust", ID: "onetrust", Type: "Enterprise Security", State: api.ProviderStateDisabled},
		{Name: "PaloAlto Networks Prisma", ID: "prismacloud", Type: "Enterprise Security", State: api.ProviderStateDisabled},
		{Name: "Ping Identity", ID: "pingidentity", Type: "Enterprise Security", State: api.ProviderStateDisabled},
		{Name: "SignalSciences", ID: "signalscience", Type: "Enterprise Security", State: api.ProviderStateDisabled},
		{Name: "StrongDM", ID: "strongdm", Type: "Enterprise Security", State: api.ProviderStateDisabled},
		{Name: "Sumo Logic", ID: "sumologic", Type: "Enterprise Security", State: api.ProviderStateDisabled},
		{Name: "Tenable", ID: "tenable", Type: "Enterprise Security", State: api.ProviderStateDisabled},
		{Name: "Atlassian", ID: "atlassian", Type: "IT Operations", State: api.ProviderStateDisabled},
		{Name: "DataDog", ID: "datadog", Type: "IT Operations", State: api.ProviderStateDisabled},
		{Name: "PagerDuty", ID: "pagerduty", Type: "IT Operations", State: api.ProviderStateDisabled},
		{Name: "RingCentral", ID: "ringcentral", Type: "IT Operations", State: api.ProviderStateDisabled},
		{Name: "ServiceNow", ID: "servicenow", Type: "IT Operations", State: api.ProviderStateDisabled},
		{Name: "Zendesk", ID: "zendesk", Type: "IT Operations", State: api.ProviderStateDisabled},
		{Name: "Splunk", ID: "splunk", Type: "IT Operations", State: api.ProviderStateDisabled},
		{Name: "Confluent", ID: "confluence", Type: "Messaging", State: api.ProviderStateDisabled},
		{Name: "Splunk", ID: "splunk", Type: "Observability", State: api.ProviderStateDisabled},
		{Name: "DataDog", ID: "datadog", Type: "Observability", State: api.ProviderStateDisabled},
		{Name: "OpenStack", ID: "openstack", Type: "Private Cloud", State: api.ProviderStateDisabled},
		{Name: "VMWare", ID: "vmware", Type: "Private Cloud", State: api.ProviderStateComingSoon},
		{Name: "HPE Helion", ID: "hpehelion", Type: "Private Cloud", State: api.ProviderStateDisabled},
		{Name: "Amazon Web Services", ID: "aws", Type: "Public Cloud", State: api.ProviderStateEnabled},
		{Name: "Google Cloud Platform", ID: "gcp", Type: "Public Cloud", State: api.ProviderStateComingSoon},
		{Name: "Oracle Cloud Infrastructure", ID: "oci", Type: "Public Cloud", State: api.ProviderStateDisabled},
		{Name: "Alibaba Cloud", ID: "alibabacloud", Type: "Public Cloud", State: api.ProviderStateDisabled},
		{Name: "Tencent Cloud", ID: "tencentcloud", Type: "Public Cloud", State: api.ProviderStateDisabled},
		{Name: "IBM Cloud", ID: "ibmcloud", Type: "Public Cloud", State: api.ProviderStateDisabled},
		{Name: "Microsoft Azure", ID: "azure", Type: "Public Cloud", State: api.ProviderStateEnabled},
		{Name: "Salesforce Tableau", ID: "tableau", Type: "Reporting", State: api.ProviderStateDisabled},
		{Name: "Google Looker", ID: "looker", Type: "Reporting", State: api.ProviderStateDisabled},
		{Name: "Gitlab.com", ID: "gitlab", Type: "Source Code Management", State: api.ProviderStateComingSoon},
		{Name: "GitHub", ID: "github", Type: "Source Code Management", State: api.ProviderStateComingSoon},
		{Name: "Azure DevOps", ID: "azuredevops", Type: "Source Code Management", State: api.ProviderStateDisabled},
		{Name: "Jfrog", ID: "jfrog", Type: "Source Code Management", State: api.ProviderStateDisabled},
		{Name: "NewRelic", ID: "newrelic", Type: "Observability", State: api.ProviderStateDisabled},
		{Name: "DynaTrace", ID: "dynatrace", Type: "Observability", State: api.ProviderStateDisabled},
	})
}

// GetConnectorCategories godoc
// @Summary     Get connector categories
// @Description Getting connector categories
// @Tags        onboard
// @Produce     json
// @Success     200 {object} []connector.Category
// @Router      /onboard/api/v1/connectors/categories [get]
func (h HttpHandler) GetConnectorCategories(ctx echo.Context) error {
	return ctx.JSON(http.StatusOK, connector.CategoryList)
}

// GetConnector godoc
// @Summary     Get connectors
// @Description Getting connectors
// @Tags        onboard
// @Produce     json
// @Success     200      {object} []connector.ConnectorCount
// @Param       category query    string false "category"
// @Router      /onboard/api/v1/connectors [get]
func (h HttpHandler) GetConnector(ctx echo.Context) error {
	categoryID := ctx.QueryParam("categoryID")

	var res []connector.ConnectorCount
	for _, c := range connector.Connectors {
		ok := false
		if len(categoryID) > 0 {
			for _, m := range connector.CategoryConnectorMapping {
				if m.CategoryID == categoryID && m.ConnectorID == c.ID {
					ok = true
				}
			}
		} else {
			ok = true
		}

		if ok {
			typ, _ := source.ParseType(c.SourceType)
			count, err := h.db.CountSourcesOfType(typ)
			if err != nil {
				return err
			}

			res = append(res, connector.ConnectorCount{
				Connector:       c,
				ConnectionCount: count,
			})
		}
	}
	return ctx.JSON(http.StatusOK, res)
}

// GetProviderTypes godoc
// @Summary     Get provider types
// @Description Getting provider types
// @Tags        onboard
// @Produce     json
// @Success     200 {object} api.ProviderTypesResponse
// @Router      /onboard/api/v1/providers/types [get]
func (h HttpHandler) GetProviderTypes(ctx echo.Context) error {
	return ctx.JSON(http.StatusOK, api.ProviderTypesResponse{
		{ID: "1", TypeName: "Public Cloud", State: api.ProviderTypeStateEnabled},
		{ID: "2", TypeName: "Cointainer Orchestrator", State: api.ProviderTypeStateComingSoon},
		{ID: "3", TypeName: "Private Cloud", State: api.ProviderTypeStateComingSoon},
		{ID: "4", TypeName: "Source Code Management", State: api.ProviderTypeStateComingSoon},
		{ID: "5", TypeName: "Identity", State: api.ProviderTypeStateComingSoon},
		{ID: "6", TypeName: "Enterprise Security", State: api.ProviderTypeStateDisabled},
		{ID: "7", TypeName: "Observability", State: api.ProviderTypeStateDisabled},
		{ID: "8", TypeName: "Messaging", State: api.ProviderTypeStateDisabled},
		{ID: "9", TypeName: "Communications", State: api.ProviderTypeStateDisabled},
		{ID: "10", TypeName: "IT Operations", State: api.ProviderTypeStateDisabled},
		{ID: "11", TypeName: "Enterprise Applications", State: api.ProviderTypeStateDisabled},
		{ID: "12", TypeName: "Databases", State: api.ProviderTypeStateDisabled},
		{ID: "13", TypeName: "Data Management", State: api.ProviderTypeStateDisabled},
		{ID: "14", TypeName: "Cloud Storage", State: api.ProviderTypeStateDisabled},
		{ID: "15", TypeName: "Content Delivery (CDN)", State: api.ProviderTypeStateDisabled},
		{ID: "16", TypeName: "Collaboration & Productivity", State: api.ProviderTypeStateDisabled},
		{ID: "17", TypeName: "Edge Compute", State: api.ProviderTypeStateDisabled},
		{ID: "18", TypeName: "Reporting", State: api.ProviderTypeStateDisabled},
	})
}

// PostSourceAws godoc
// @Summary     Create AWS source
// @Description Creating AWS source
// @Tags        onboard
// @Produce     json
// @Success     200         {object} api.CreateSourceResponse
// @Param       name        body     string              true "name"
// @Param       description body     string              true "description"
// @Param       config      body     api.SourceConfigAWS true "config"
// @Router      /onboard/api/v1/source/aws [post]
func (h HttpHandler) PostSourceAws(ctx echo.Context) error {
	var req api.SourceAwsRequest
	if err := bindValidate(ctx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	// Check creds section
	err := keibiaws.CheckDescribeRegionsPermission(req.Config.AccessKey, req.Config.SecretKey)
	if err != nil {
		return PermissionError
	}

	err = keibiaws.CheckSecurityAuditPermission(req.Config.AccessKey, req.Config.SecretKey)
	if err != nil {
		return PermissionError
	}

	// Create source section
	cfg, err := keibiaws.GetConfig(context.Background(), req.Config.AccessKey, req.Config.SecretKey, "", "")
	if err != nil {
		return err
	}

	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	acc, err := currentAwsAccount(context.Background(), cfg)
	if err != nil {
		return err
	}
	if acc.Name == "" {
		acc.Name = acc.AccountID
	}

	req.Name = acc.Name
	req.Email = acc.Email
	req.Config.AccountId = acc.AccountID

	count, err := h.db.CountSources()
	if err != nil {
		return err
	}
	if count >= httpserver.GetMaxConnections(ctx) {
		return echo.NewHTTPError(http.StatusBadRequest, "maximum number of connections reached")
	}

	src := NewAWSSource(req.Config.AccountId, req.Name, req.Description, req.Email, acc.OrganizationID)
	err = h.db.orm.Transaction(func(tx *gorm.DB) error {
		err := h.db.CreateSource(&src)
		if err != nil {
			return err
		}

		// TODO: Handle edge case where writing to Vault succeeds and writing to event queue fails.
		if err := h.vault.Write(src.ConfigRef, req.Config.AsMap()); err != nil {
			return err
		}

		if err := h.sourceEventsQueue.Publish(api.SourceEvent{
			Action:     api.SourceCreated,
			SourceID:   src.ID,
			AccountID:  src.SourceId,
			SourceType: src.Type,
			ConfigRef:  src.ConfigRef,
		}); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	return ctx.JSON(http.StatusOK, src.ToSourceResponse())
}

// PostSourceAzure godoc
// @Summary     Create Azure source
// @Description Creating Azure source
// @Tags        onboard
// @Produce     json
// @Success     200         {object} api.CreateSourceResponse
// @Param       name        body     string                true "name"
// @Param       description body     string                true "description"
// @Param       config      body     api.SourceConfigAzure true "config"
// @Router      /onboard/api/v1/source/azure [post]
func (h HttpHandler) PostSourceAzure(ctx echo.Context) error {
	var req api.SourceAzureRequest

	if err := bindValidate(ctx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	count, err := h.db.CountSources()
	if err != nil {
		return err
	}
	if count >= httpserver.GetMaxConnections(ctx) {
		return echo.NewHTTPError(http.StatusBadRequest, "maximum number of connections reached")
	}

	src := NewAzureSource(req)
	err = h.db.orm.Transaction(func(tx *gorm.DB) error {
		err := h.db.CreateSource(&src)
		if err != nil {
			return err
		}

		// TODO: Handle edge case where writing to Vault succeeds and writing to event queue fails.
		if err := h.vault.Write(src.ConfigRef, req.Config.AsMap()); err != nil {
			return err
		}

		if err := h.sourceEventsQueue.Publish(api.SourceEvent{
			Action:     api.SourceCreated,
			SourceID:   src.ID,
			AccountID:  src.SourceId,
			SourceType: src.Type,
			ConfigRef:  src.ConfigRef,
		}); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	return ctx.JSON(http.StatusOK, src.ToSourceResponse())
}

// PostSourceAzureSPN godoc
// @Summary     Create Azure source with SPN
// @Description Creating Azure source with SPN
// @Tags        onboard
// @Produce     json
// @Success     200         {object} api.CreateSourceResponse
// @Param       name        body     string                true "name"
// @Param       description body     string                true "description"
// @Param       config      body     api.SourceConfigAzure true "config"
// @Router      /onboard/api/v1/source/azure/spn [post]
func (h HttpHandler) PostSourceAzureSPN(ctx echo.Context) error {
	var req api.SourceAzureSPNRequest

	if err := bindValidate(ctx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	count, err := h.db.CountSources()
	if err != nil {
		return err
	}
	if count >= httpserver.GetMaxConnections(ctx) {
		return echo.NewHTTPError(http.StatusBadRequest, "maximum number of connections reached")
	}

	src := NewAzureSourceWithSPN(req)
	_, err = h.db.GetSPN(req.SPNId)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "SPN not found")
	}

	err = h.db.orm.Transaction(func(tx *gorm.DB) error {
		err := h.db.CreateSource(&src)
		if err != nil {
			return err
		}

		if err := h.sourceEventsQueue.Publish(api.SourceEvent{
			Action:     api.SourceCreated,
			SourceID:   src.ID,
			AccountID:  src.SourceId,
			SourceType: src.Type,
			ConfigRef:  src.ConfigRef,
		}); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	return ctx.JSON(http.StatusOK, src.ToSourceResponse())
}

// PostSPN godoc
// @Summary     Create Azure SPN
// @Description Creating Azure SPN
// @Tags        onboard
// @Produce     json
// @Success     200    {object} api.CreateSPNResponse
// @Param       name   body     string                true "name"
// @Param       config body     api.SourceConfigAzure true "config"
// @Router      /onboard/api/v1/spn/azure [post]
func (h HttpHandler) PostSPN(ctx echo.Context) error {
	var req api.CreateSPNRequest

	if err := bindValidate(ctx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	src := NewSPN(req)
	err := h.db.orm.Transaction(func(tx *gorm.DB) error {
		if err := h.db.CreateSPN(&src); err != nil {
			return err
		}

		// TODO: Handle edge case where writing to Vault succeeds and writing to event queue fails.
		if err := h.vault.Write(src.ConfigRef, req.Config.AsMap()); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "id conflict") {
			spn, err := h.db.GetSPNByTenantClientID(src.TenantId, src.ClientId)
			if err != nil {
				return err
			}

			return ctx.JSON(http.StatusBadRequest, api.DuplicateSPNResponse{ErrorMessage: "SPN is already created",
				SpnID: spn.ID.String()})
		}
		return err
	}

	return ctx.JSON(http.StatusOK, src.ToSPNResponse())
}

// GetSPNCred godoc
// @Summary Get SPN credential
// @Tags    onboard
// @Produce json
// @Param   spnId query string true "SPN ID"
// @Router  /onboard/api/v1/spn/{spnId} [post]
func (h HttpHandler) GetSPNCred(ctx echo.Context) error {
	spnUUID, err := uuid.Parse(ctx.Param("spnId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid SPN uuid")
	}

	src, err := h.db.GetSPN(spnUUID)
	if err != nil {
		return err
	}

	cnf, err := h.vault.Read(src.ConfigRef)
	if err != nil {
		return err
	}

	azureCnf, err := describe.AzureSubscriptionConfigFromMap(cnf)
	if err != nil {
		return err
	}

	return ctx.JSON(http.StatusOK, api.SPNCredential{
		SPNName:  fmt.Sprintf("SPN-%s", src.ID.String()),
		ClientID: azureCnf.ClientID,
		TenantID: azureCnf.TenantID,
	})
}

// ListSPNs godoc
// @Summary List SPN credentials
// @Tags    onboard
// @Produce json
// @Router  /onboard/api/v1/spn/list [get]
func (h HttpHandler) ListSPNs(ctx echo.Context) error {
	src, err := h.db.GetAllSPNs()
	if err != nil {
		return err
	}

	var res []api.SPNRecord
	for _, r := range src {
		res = append(res, api.SPNRecord{
			SPNID:    r.ID.String(),
			SPNName:  fmt.Sprintf("SPN-%s", r.ID.String()),
			ClientID: r.ClientId,
			TenantID: r.TenantId,
		})
	}
	return ctx.JSON(http.StatusOK, res)
}

// PutSPNCred godoc
// @Summary Put SPN credential
// @Tags    onboard
// @Produce json
// @Param   spnId query string true "SPN ID"
// @Router  /onboard/api/v1/spn/{spnId} [put]
func (h HttpHandler) PutSPNCred(ctx echo.Context) error {
	spnUUID, err := uuid.Parse(ctx.Param("spnId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid SPN uuid")
	}

	src, err := h.db.GetSPN(spnUUID)
	if err != nil {
		return err
	}

	cnf, err := h.vault.Read(src.ConfigRef)
	if err != nil {
		return err
	}
	azureCnf, err := describe.AzureSubscriptionConfigFromMap(cnf)
	if err != nil {
		return err
	}

	var req api.AzureCredential
	if err := bindValidate(ctx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	newCnf := api.SPNConfigAzure{
		TenantId:     azureCnf.TenantID,
		ClientId:     req.ClientID,
		ClientSecret: req.ClientSecret,
	}
	if err := h.vault.Write(src.ConfigRef, newCnf.AsMap()); err != nil {
		return err
	}
	return ctx.NoContent(http.StatusOK)
}

// DeleteSPN godoc
// @Summary Delete SPN credential
// @Tags    onboard
// @Produce json
// @Param   spnId query string true "SPN ID"
// @Router  /onboard/api/v1/spn/{spnId} [delete]
func (h HttpHandler) DeleteSPN(ctx echo.Context) error {
	spnUUID, err := uuid.Parse(ctx.Param("spnId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid SPN uuid")
	}

	src, err := h.db.DeleteSPN(spnUUID)
	if err != nil {
		return err
	}

	err = h.vault.Delete(src.ConfigRef)
	if err != nil {
		return err
	}
	return ctx.NoContent(http.StatusOK)
}

// GetSourceCred godoc
// @Summary Get source credential
// @Tags    onboard
// @Produce json
// @Param   sourceId query string true "Source ID"
// @Router  /onboard/api/v1/source/{sourceId}/credentials [post]
func (h HttpHandler) GetSourceCred(ctx echo.Context) error {
	sourceUUID, err := uuid.Parse(ctx.Param("sourceId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid source uuid")
	}

	src, err := h.db.GetSource(sourceUUID)
	if err != nil {
		return err
	}

	cnf, err := h.vault.Read(src.ConfigRef)
	if err != nil {
		return err
	}

	switch src.Type {
	case source.CloudAWS:
		awsCnf, err := describe.AWSAccountConfigFromMap(cnf)
		if err != nil {
			return err
		}
		return ctx.JSON(http.StatusOK, api.AWSCredential{
			AccessKey: awsCnf.AccessKey,
		})
	case source.CloudAzure:
		azureCnf, err := describe.AzureSubscriptionConfigFromMap(cnf)
		if err != nil {
			return err
		}
		return ctx.JSON(http.StatusOK, api.AzureCredential{
			ClientID: azureCnf.ClientID,
			TenantID: azureCnf.TenantID,
		})
	default:
		return errors.New("invalid provider")
	}
}

// GetSourceHealth godoc
// @Summary Get live source health status
// @Tags    onboard
// @Produce json
// @Param   sourceId query string true "Source ID"
// @Router  /onboard/api/v1/source/{sourceId}/healthcheck [post]
func (h HttpHandler) GetSourceHealth(ctx echo.Context) error {
	sourceUUID, err := uuid.Parse(ctx.Param("sourceId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid source uuid")
	}

	src, err := h.db.GetSource(sourceUUID)
	if err != nil {
		return err
	}

	cnf, err := h.vault.Read(src.ConfigRef)
	if err != nil {
		return err
	}

	switch src.Type {
	case source.CloudAWS:
		awsCnf, err := describe.AWSAccountConfigFromMap(cnf)
		if err != nil {
			return err
		}
		err = keibiaws.CheckSecurityAuditPermission(awsCnf.AccessKey, awsCnf.SecretKey)
		if err != nil {
			src.HealthState = source.SourceHealthStateUnhealthy
			healthMessage := err.Error()
			src.HealthReason = &healthMessage
			src.LastHeathCheckTime = time.Now()
			_, err = h.db.UpdateSource(&src)
			if err != nil {
				return err
			}
			//TODO Mahan: record state change in elastic search
		} else {
			src.HealthState = source.SourceHealthStateHealthy
			src.HealthReason = nil
			src.LastHeathCheckTime = time.Now()
			_, err = h.db.UpdateSource(&src)
			if err != nil {
				return err
			}
			//TODO Mahan: record state change in elastic search
		}

	}

	return ctx.JSON(http.StatusOK, &api.Source{
		ID:                   src.ID,
		ConnectionID:         src.SourceId,
		ConnectionName:       src.Name,
		Email:                src.Email,
		Type:                 src.Type,
		Description:          src.Description,
		OnboardDate:          src.CreatedAt,
		Enabled:              src.Enabled,
		AssetDiscoveryMethod: src.AssetDiscoveryMethod,
		HealthState:          src.HealthState,
		LastHealthCheckTime:  src.LastHeathCheckTime,
		HealthReason:         src.HealthReason,
	})
}

// PutSourceCred godoc
// @Summary Put source credential
// @Tags    onboard
// @Produce json
// @Param   sourceId query string true "Source ID"
// @Router  /onboard/api/v1/source/{sourceId}/credentials [put]
func (h HttpHandler) PutSourceCred(ctx echo.Context) error {
	sourceUUID, err := uuid.Parse(ctx.Param("sourceId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid source uuid")
	}

	src, err := h.db.GetSource(sourceUUID)
	if err != nil {
		return err
	}

	cnf, err := h.vault.Read(src.ConfigRef)
	if err != nil {
		return err
	}

	switch src.Type {
	case source.CloudAWS:
		awsCnf, err := describe.AWSAccountConfigFromMap(cnf)
		if err != nil {
			return err
		}

		var req api.AWSCredential
		if err := bindValidate(ctx, &req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
		}

		newCnf := api.SourceConfigAWS{
			AccountId: awsCnf.AccountID,
			Regions:   awsCnf.Regions,
			AccessKey: req.AccessKey,
			SecretKey: req.SecretKey,
		}
		if err := h.vault.Write(src.ConfigRef, newCnf.AsMap()); err != nil {
			return err
		}
		return ctx.NoContent(http.StatusOK)
	case source.CloudAzure:
		azureCnf, err := describe.AzureSubscriptionConfigFromMap(cnf)
		if err != nil {
			return err
		}

		var req api.AzureCredential
		if err := bindValidate(ctx, &req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
		}

		newCnf := api.SourceConfigAzure{
			SubscriptionId: azureCnf.SubscriptionID,
			TenantId:       azureCnf.TenantID,
			ClientId:       req.ClientID,
			ClientSecret:   req.ClientSecret,
		}
		if err := h.vault.Write(src.ConfigRef, newCnf.AsMap()); err != nil {
			return err
		}
		return ctx.NoContent(http.StatusOK)
	default:
		return errors.New("invalid provider")
	}
}

// GetSource godoc
// @Summary     Returns a single source
// @Description Returning single source either AWS / Azure.
// @Tags        onboard
// @Produce     json
// @Success     200      {object} api.Source
// @Param       sourceId path     integer true "SourceID"
// @Router      /onboard/api/v1/source/{sourceId} [get]
func (h HttpHandler) GetSource(ctx echo.Context) error {
	srcId, err := uuid.Parse(ctx.Param(paramSourceId))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	src, err := h.db.GetSource(srcId)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return echo.NewHTTPError(http.StatusBadRequest, "source not found")
		}
		return err
	}

	return ctx.JSON(http.StatusOK, &api.Source{
		ID:                   src.ID,
		ConnectionID:         src.SourceId,
		ConnectionName:       src.Name,
		Email:                src.Email,
		Type:                 src.Type,
		Description:          src.Description,
		OnboardDate:          src.CreatedAt,
		Enabled:              src.Enabled,
		AssetDiscoveryMethod: src.AssetDiscoveryMethod,
		HealthState:          src.HealthState,
		LastHealthCheckTime:  src.LastHeathCheckTime,
		HealthReason:         src.HealthReason,
	})
}

// DeleteSource godoc
// @Summary     Delete a single source
// @Description Deleting a single source either AWS / Azure.
// @Tags        onboard
// @Produce     json
// @Success     200
// @Param       sourceId path integer true "SourceID"
// @Router      /onboard/api/v1/source/{sourceId} [delete]
func (h HttpHandler) DeleteSource(ctx echo.Context) error {
	srcId, err := uuid.Parse(ctx.Param(paramSourceId))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	src, err := h.db.GetSource(srcId)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return echo.NewHTTPError(http.StatusBadRequest, "source not found")
		}
		return err
	}

	err = h.db.orm.Transaction(func(tx *gorm.DB) error {
		if err := h.db.DeleteSource(srcId); err != nil {
			return err
		}

		if !strings.HasPrefix(src.ConfigRef, "sources/azure/spn") {
			// TODO: Handle edge case where deleting from Vault succeeds and writing to event queue fails.
			err = h.vault.Delete(src.ConfigRef)
			if err != nil {
				return err
			}
		}

		if err := h.sourceEventsQueue.Publish(api.SourceEvent{
			Action:     api.SourceDeleted,
			SourceID:   src.ID,
			SourceType: src.Type,
			ConfigRef:  src.ConfigRef,
		}); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	return ctx.NoContent(http.StatusOK)
}

// DisableSource godoc
// @Summary Disable a single source
// @Tags    onboard
// @Produce json
// @Success 200
// @Param   sourceId path integer true "SourceID"
// @Router  /onboard/api/v1/source/{sourceId}/disable [post]
func (h HttpHandler) DisableSource(ctx echo.Context) error {
	srcId, err := uuid.Parse(ctx.Param(paramSourceId))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	src, err := h.db.GetSource(srcId)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return echo.NewHTTPError(http.StatusBadRequest, "source not found")
		}
		return err
	}

	err = h.db.orm.Transaction(func(tx *gorm.DB) error {
		if err := h.db.UpdateSourceEnabled(srcId, false); err != nil {
			return err
		}

		if err := h.sourceEventsQueue.Publish(api.SourceEvent{
			Action:     api.SourceDeleted,
			SourceID:   src.ID,
			SourceType: src.Type,
			ConfigRef:  src.ConfigRef,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	return ctx.NoContent(http.StatusOK)
}

// EnableSource godoc
// @Summary Enable a single source
// @Tags    onboard
// @Produce json
// @Success 200
// @Param   sourceId path integer true "SourceID"
// @Router  /onboard/api/v1/source/{sourceId}/enable [post]
func (h HttpHandler) EnableSource(ctx echo.Context) error {
	srcId, err := uuid.Parse(ctx.Param(paramSourceId))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	src, err := h.db.GetSource(srcId)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return echo.NewHTTPError(http.StatusBadRequest, "source not found")
		}
		return err
	}

	err = h.db.orm.Transaction(func(tx *gorm.DB) error {
		if err := h.db.UpdateSourceEnabled(srcId, true); err != nil {
			return err
		}

		if err := h.sourceEventsQueue.Publish(api.SourceEvent{
			Action:     api.SourceCreated,
			SourceID:   src.ID,
			AccountID:  src.SourceId,
			SourceType: src.Type,
			ConfigRef:  src.ConfigRef,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	return ctx.NoContent(http.StatusOK)
}

// ListSources godoc
// @Summary     Returns a list of sources
// @Description Returning a list of sources including both AWS and Azure unless filtered by Type.
// @Tags        onboard
// @Produce     json
// @Param       type query    string false "Type" Enums(aws,azure)
// @Success     200  {object} api.GetSourcesResponse
// @Router      /onboard/api/v1/sources [get]
func (h HttpHandler) ListSources(ctx echo.Context) error {
	sType := ctx.QueryParam("type")
	var sources []Source
	if sType != "" {
		st, err := source.ParseType(sType)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid source type: %s", sType))
		}

		sources, err = h.db.GetSourcesOfType(st)
		if err != nil {
			return err
		}
	} else {
		var err error
		sources, err = h.db.ListSources()
		if err != nil {
			return err
		}
	}

	resp := api.GetSourcesResponse{}
	for _, s := range sources {
		source := api.Source{
			ID:                   s.ID,
			ConnectionID:         s.SourceId,
			ConnectionName:       s.Name,
			Email:                s.Email,
			Type:                 s.Type,
			Description:          s.Description,
			OnboardDate:          s.CreatedAt,
			Enabled:              s.Enabled,
			AssetDiscoveryMethod: s.AssetDiscoveryMethod,
			HealthState:          s.HealthState,
			LastHealthCheckTime:  s.LastHeathCheckTime,
			HealthReason:         s.HealthReason,
		}
		resp = append(resp, source)
	}

	return ctx.JSON(http.StatusOK, resp)
}

// GetSources godoc
// @Summary     Returns a list of sources
// @Description Returning a list of sources including both AWS and Azure unless filtered by Type.
// @Tags        onboard
// @Produce     json
// @Param       type    query    string                false "Type" Enums(aws,azure)
// @Param       request body     api.GetSourcesRequest false "Request Body"
// @Success     200     {object} api.GetSourcesResponse
// @Router      /onboard/api/v1/sources [post]
func (h HttpHandler) GetSources(ctx echo.Context) error {
	var req api.GetSourcesRequest
	if err := bindValidate(ctx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	var reqUUIDs []uuid.UUID
	for _, item := range req.SourceIDs {
		u, err := uuid.Parse(item)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid uuid:"+item)
		}
		reqUUIDs = append(reqUUIDs, u)
	}
	srcs, err := h.db.GetSources(reqUUIDs)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return echo.NewHTTPError(http.StatusBadRequest, "source not found")
		}
		return err
	}

	var res []api.Source
	for _, src := range srcs {
		res = append(res, api.Source{
			ID:                   src.ID,
			ConnectionID:         src.SourceId,
			ConnectionName:       src.Name,
			Email:                src.Email,
			Type:                 src.Type,
			Description:          src.Description,
			OnboardDate:          src.CreatedAt,
			Enabled:              src.Enabled,
			AssetDiscoveryMethod: src.AssetDiscoveryMethod,
			HealthState:          src.HealthState,
			LastHealthCheckTime:  src.LastHeathCheckTime,
			HealthReason:         src.HealthReason,
		})
	}
	return ctx.JSON(http.StatusOK, res)
}

// CountSources godoc
// @Summary     Returns a count of sources
// @Description Returning a count of sources including both AWS and Azure unless filtered by Type.
// @Tags        onboard
// @Produce     json
// @Param       type query    string false "Type" Enums(aws,azure)
// @Success     200  {object} int64
// @Router      /onboard/api/v1/sources/count [get]
func (h HttpHandler) CountSources(ctx echo.Context) error {
	sType := ctx.QueryParam("type")
	var count int64
	if sType != "" {
		st, err := source.ParseType(sType)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid source type: %s", sType))
		}

		count, err = h.db.CountSourcesOfType(st)
		if err != nil {
			return err
		}
	} else {
		var err error
		count, err = h.db.CountSources()
		if err != nil {
			return err
		}
	}

	return ctx.JSON(http.StatusOK, count)
}

func (h HttpHandler) PutSource(ctx echo.Context) error {
	panic("not implemented yet")
}

// DiscoverAwsAccounts godoc
// @Summary     Returns the list of available AWS accounts given the credentials.
// @Description If the account is part of an organization and the account has premission to list the accounts, it will return all the accounts in that organization. Otherwise, it will return the single account these credentials belong to.
// @Tags        onboard
// @Produce     json
// @Success     200       {object} []api.DiscoverAWSAccountsResponse
// @Param       accessKey body     string true "AccessKey"
// @Param       secretKey body     string true "SecretKey"
// @Router      /onboard/api/v1/discover/aws/accounts [post]
func (h HttpHandler) DiscoverAwsAccounts(ctx echo.Context) error {
	// DiscoverAwsAccounts returns the list of available AWS accounts given the credentials.
	// If the account is part of an organization and the account has premission to
	// list the accounts, it will return all the accounts in that organization.
	// Otherwise, it will return the single account these credentials belong to.
	var req api.DiscoverAWSAccountsRequest
	if err := bindValidate(ctx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	accounts, err := discoverAwsAccounts(ctx.Request().Context(), req)
	if err != nil {
		if err == PermissionError {
			return ctx.JSON(http.StatusForbidden, "Key doesn't have enough permission")
		}
		return err
	}

	for idx, account := range accounts {
		_, err := h.db.GetSourceBySourceID(account.AccountID)
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				continue
			}
			return err
		}
		accounts[idx].Status = "DUPLICATE"
	}
	return ctx.JSON(http.StatusOK, accounts)
}

// DiscoverAzureSubscriptions godoc
// @Summary     Returns the list of available Azure subscriptions.
// @Description Returning the list of available Azure subscriptions.
// @Tags        onboard
// @Produce     json
// @Success     200          {object} []api.DiscoverAzureSubscriptionsResponse
// @Param       tenantId     body     string true "TenantId"
// @Param       clientId     body     string true "ClientId"
// @Param       clientSecret body     string true "ClientSecret"
// @Router      /onboard/api/v1/discover/azure/subscriptions [post]
func (h *HttpHandler) DiscoverAzureSubscriptions(ctx echo.Context) error {
	var req api.DiscoverAzureSubscriptionsRequest
	if err := bindValidate(ctx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	subs, err := discoverAzureSubscriptions(ctx.Request().Context(), req)
	if err != nil {
		return err
	}

	for _, sub := range subs {
		_, err := h.db.GetSourceBySourceID(sub.SubscriptionID)
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				continue
			}
			return err
		}
		sub.Status = "DUPLICATE"
	}
	return ctx.JSON(http.StatusOK, subs)
}

// DiscoverAzureSubscriptionsWithSPN godoc
// @Summary     Returns the list of available Azure subscriptions.
// @Description Returning the list of available Azure subscriptions.
// @Tags        onboard
// @Produce     json
// @Success     200     {object} []api.DiscoverAzureSubscriptionsResponse
// @Param       request body     api.DiscoverAzureSubscriptionsSPNRequest true "Request Body"
// @Router      /onboard/api/v1/discover/azure/subscriptions/spn [post]
func (h *HttpHandler) DiscoverAzureSubscriptionsWithSPN(ctx echo.Context) error {
	var req api.DiscoverAzureSubscriptionsSPNRequest
	if err := bindValidate(ctx, &req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	spn, err := h.db.GetSPN(req.SPNId)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "SPN not found")
	}

	cnf, err := h.vault.Read(spn.ConfigRef)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "SPN ref not found")
	}

	azureCnf, err := describe.AzureSubscriptionConfigFromMap(cnf)
	if err != nil {
		return err
	}

	var discoveryReq api.DiscoverAzureSubscriptionsRequest
	discoveryReq.TenantId = azureCnf.TenantID
	discoveryReq.ClientId = azureCnf.ClientID
	discoveryReq.ClientSecret = azureCnf.ClientSecret
	subs, err := discoverAzureSubscriptions(ctx.Request().Context(), discoveryReq)
	if err != nil {
		return err
	}

	for _, sub := range subs {
		_, err := h.db.GetSourceBySourceID(sub.SubscriptionID)
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				continue
			}
			return err
		}
		sub.Status = "DUPLICATE"
	}
	return ctx.JSON(http.StatusOK, subs)
}
