package credentials

import (
	"github.com/labstack/echo/v4"
	"github.com/opengovern/og-util/pkg/api"
	"github.com/opengovern/og-util/pkg/httpserver"
	"github.com/opengovern/og-util/pkg/vault"
	"github.com/opengovern/opengovernance/services/integration/api/models"
	"github.com/opengovern/opengovernance/services/integration/db"
	"go.uber.org/zap"
	"net/http"
)

type API struct {
	vault    vault.VaultSourceConfig
	logger   *zap.Logger
	database db.Database
}

func New(
	vault vault.VaultSourceConfig,
	database db.Database,
	logger *zap.Logger,
) API {
	return API{
		vault:    vault,
		database: database,
		logger:   logger.Named("credentials"),
	}
}

func (h API) Register(g *echo.Group) {
	g.GET("", httpserver.AuthorizeHandler(h.List, api.ViewerRole))
	g.POST("/list", httpserver.AuthorizeHandler(h.CredentialsFilteredList, api.ViewerRole))
	g.DELETE("/:credentialId", httpserver.AuthorizeHandler(h.Delete, api.EditorRole))
	g.GET("/:credentialId", httpserver.AuthorizeHandler(h.Get, api.ViewerRole))
	g.PUT("/:credentialId", httpserver.AuthorizeHandler(h.UpdateCredential, api.ViewerRole))
}

// Delete godoc
//
//	@Summary		Delete credential
//	@Description	Delete credential
//	@Security		BearerToken
//	@Tags			credentials
//	@Produce		json
//	@Success		200
//	@Param			credentialId	path	string	true	"credentialId"
//	@Router			/integration/api/v1/credentials/{credentialId} [delete]
func (h API) Delete(c echo.Context) error {
	credentialId := c.Param("credentialId")

	err := h.database.DeleteCredential(credentialId)
	if err != nil {
		h.logger.Error("failed to delete credential", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete credential")
	}

	return c.NoContent(http.StatusOK)
}

// List godoc
//
//	@Summary		List credentials
//	@Description	List credentials
//	@Security		BearerToken
//	@Tags			credentials
//	@Produce		json
//	@Success		200	{object}	models.ListCredentialsResponse
//	@Router			/integration/api/v1/credentials [get]
func (h API) List(c echo.Context) error {
	credentials, err := h.database.ListCredentials()
	if err != nil {
		h.logger.Error("failed to list credentials", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list credential")
	}

	var items []models.Credential
	for _, credential := range credentials {
		item, err := credential.ToApi(true)
		if err != nil {
			h.logger.Error("failed to convert credentials to API model", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to convert credentials to API model")
		}
		items = append(items, *item)
	}

	return c.JSON(http.StatusOK, models.ListCredentialsResponse{
		Credentials: items,
		TotalCount:  len(items),
	})
}

// UpdateCredential godoc
//
//	@Summary		List credentials
//	@Description	List credentials
//	@Security		BearerToken
//	@Tags			credentials
//	@Produce		json
//	@Param			credentialId	path		string							true	"credentialId"
//	@Param			request			body		models.UpdateCredentialRequest	true	"Request"
//	@Success		200				{object}	models.ListCredentialsResponse
//	@Router			/integration/api/v1/credentials/{credentialId} [put]
func (h API) UpdateCredential(c echo.Context) error {
	credentialId := c.Param("credentialId")

	var req models.UpdateCredentialRequest

	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	credential, err := h.database.GetCredential(credentialId)
	if err != nil {
		h.logger.Error("failed to get credential", zap.Error(err))
		return echo.NewHTTPError(http.StatusNotFound, "credential not found")
	}

	mapData, err := h.vault.Decrypt(c.Request().Context(), credential.Secret)
	if err != nil {
		h.logger.Error("failed to decrypt secret", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to decrypt config")
	}
	for k, v := range req.Credentials {
		mapData[k] = v
	}
	secret, err := h.vault.Encrypt(c.Request().Context(), mapData)
	if err != nil {
		h.logger.Error("failed to encrypt secret", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to encrypt config")
	}
	err = h.database.UpdateCredential(credentialId, secret)
	if err != nil {
		h.logger.Error("failed to update credential", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update credential")
	}

	return c.NoContent(http.StatusOK)
}

// CredentialsFilteredList godoc
//
//	@Summary		List credentials
//	@Description	List credentials
//	@Security		BearerToken
//	@Tags			credentials
//	@Produce		json
//	@Param			request	body		models.ListCredentialsRequest	true	"Request"
//	@Success		200		{object}	models.ListCredentialsResponse
//	@Router			/integration/api/v1/credentials/list [post]
func (h API) CredentialsFilteredList(c echo.Context) error {
	var req models.ListCredentialsRequest

	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	credentials, err := h.database.ListCredentialsFiltered(req.CredentialID, req.IntegrationType)
	if err != nil {
		h.logger.Error("failed to list credentials", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list credential")
	}

	var items []models.Credential
	for _, credential := range credentials {
		item, err := credential.ToApi(false)
		if err != nil {
			h.logger.Error("failed to convert credentials to API model", zap.Error(err))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to convert credentials to API model")
		}
		items = append(items, *item)
	}

	return c.JSON(http.StatusOK, models.ListCredentialsResponse{
		Credentials: items,
		TotalCount:  len(items),
	})
}

// Get godoc
//
//	@Summary		Get credential
//	@Description	Get credential
//	@Security		BearerToken
//	@Tags			credentials
//	@Produce		json
//	@Success		200
//	@Param			credentialId	path	string	true	"credentialId"
//	@Router			/integration/api/v1/credentials/{credentialId} [get]
func (h API) Get(c echo.Context) error {
	credentialId := c.Param("credentialId")

	credential, err := h.database.GetCredential(credentialId)
	if err != nil {
		h.logger.Error("failed to get credential", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get credential")
	}

	item, err := credential.ToApi(true)
	if err != nil {
		h.logger.Error("failed to convert credentials to API model", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to convert integration to API model")
	}
	return c.JSON(http.StatusOK, item)
}
