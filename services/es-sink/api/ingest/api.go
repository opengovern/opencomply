package ingest

import (
	"github.com/kaytu-io/kaytu-engine/services/es-sink/service"
	"github.com/kaytu-io/kaytu-util/pkg/es/ingest/entity"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"net/http"
)

type API struct {
	logger        *zap.Logger
	esSinkService *service.EsSinkService
}

func New(logger *zap.Logger, esSinkService *service.EsSinkService) *API {
	return &API{
		logger:        logger,
		esSinkService: esSinkService,
	}
}

func (s API) Ingest(c echo.Context) error {
	var req entity.IngestRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	failedDocs, err := s.esSinkService.Ingest(c.Request().Context(), req.Docs)
	if err != nil {
		s.logger.Error("failed to ingest data", zap.Error(err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to ingest data")
	}

	apiFailedDocs := make([]entity.FailedDoc, 0, len(failedDocs))
	for _, failedDoc := range failedDocs {
		apiFailedDocs = append(apiFailedDocs, entity.FailedDoc{
			Doc: failedDoc.Doc,
			Err: failedDoc.Err,
		})
	}

	return c.JSON(http.StatusOK, entity.IngestResponse{FailedDocs: apiFailedDocs})
}
