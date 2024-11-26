package opengovernance

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	authApi "github.com/opengovern/og-util/pkg/api"
	"github.com/opengovern/og-util/pkg/httpclient"
	steampipesdk "github.com/opengovern/og-util/pkg/steampipe"
	"github.com/opengovern/opencomply/pkg/cloudql/sdk/config"
	"github.com/opengovern/opencomply/pkg/cloudql/sdk/pg"
	"github.com/opengovern/opencomply/pkg/cloudql/utils/dag"
	"github.com/opengovern/opencomply/pkg/utils"
	"github.com/opengovern/opencomply/services/metadata/client"
	"github.com/opengovern/opencomply/services/metadata/models"
	"go.uber.org/zap"
)

func newZapLogger() (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.OutputPaths = []string{
		"/home/steampipe/.steampipe/logs/opengovernance.log",
	}
	return cfg.Build()
}

type ViewSync struct {
	logger *zap.Logger

	updateLock sync.Mutex

	metadataClient               client.MetadataServiceClient
	metadataPostgresClientConfig config.ClientConfig

	viewCheckpoint time.Time
}

func newViewSync() *ViewSync {
	logger, _ := newZapLogger()
	v := ViewSync{
		logger:         logger,
		updateLock:     sync.Mutex{},
		metadataClient: client.NewMetadataServiceClient(os.Getenv("METADATA_BASEURL")),
		metadataPostgresClientConfig: config.ClientConfig{
			PgHost:     utils.GetPointer(os.Getenv("METADATA_DB_HOST")),
			PgPort:     utils.GetPointer(os.Getenv("METADATA_DB_PORT")),
			PgPassword: utils.GetPointer(os.Getenv("PG_PASSWORD")),
			PgSslMode:  utils.GetPointer(os.Getenv("METADATA_DB_SSL_MODE")),
			PgUser:     utils.GetPointer("steampipe_user"),
			PgDatabase: utils.GetPointer("metadata"),
		},
	}

	return &v
}

func (v *ViewSync) timeBasedViewSync(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Hour)
	for range ticker.C {
		v.updateViews(ctx)
	}
}

func (v *ViewSync) pullBasedViewSync(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		res, err := v.metadataClient.GetViewsCheckpoint(&httpclient.Context{Ctx: ctx, UserRole: authApi.AdminRole})
		if err != nil {
			v.logger.Error("Error fetching views checkpoint", zap.Error(err))
			v.logger.Sync()
			continue
		}
		if res.Checkpoint.After(v.viewCheckpoint) {
			v.updateViews(ctx)
		}
	}
}

func (v *ViewSync) updateViews(ctx context.Context) {
	v.updateLock.Lock()
	defer v.updateLock.Unlock()
	selfClient, err := steampipesdk.NewSelfClient(ctx)
	if err != nil {
		v.logger.Error("Error creating self client for refreshing materialized views", zap.Error(err))
		v.logger.Sync()
		return
	}
	metadataPostgresClient, err := pg.NewMetadataClient(v.metadataPostgresClientConfig, ctx)
	if err != nil {
		v.logger.Error("Error creating metadata client for refreshing materialized views", zap.Error(err))
		v.logger.Sync()
		return
	}
	v.updateViewsInDatabase(ctx, selfClient, metadataPostgresClient)
	query := `CREATE OR REPLACE FUNCTION RefreshAllMaterializedViews(schema_arg TEXT DEFAULT 'public')
RETURNS INT AS $$
DECLARE
    r RECORD;

BEGIN
    RAISE NOTICE 'Refreshing materialized view in schema %', schema_arg;
    if pg_is_in_recovery()  then 
    return 1;
    else
    FOR r IN SELECT matviewname FROM pg_matviews WHERE schemaname = schema_arg 
    LOOP
        RAISE NOTICE 'Refreshing %.%', schema_arg, r.matviewname;
        EXECUTE 'REFRESH MATERIALIZED VIEW ' || schema_arg || '.' || r.matviewname; 
    END LOOP;
    end if;
    RETURN 1;
END 
$$ LANGUAGE plpgsql;`

	_, err = selfClient.GetConnection().Exec(ctx, query)
	if err != nil {
		v.logger.Error("Error creating RefreshAllMaterializedViews function", zap.Error(err))
		v.logger.Sync()
		return
	}
	_, err = selfClient.GetConnection().Exec(ctx, "SELECT RefreshAllMaterializedViews('public')")
	if err != nil {
		v.logger.Error("Error refreshing materialized views", zap.Error(err))
		v.logger.Sync()
		return
	}

	selfClient.GetConnection().Close()
	db, _ := metadataPostgresClient.DB().DB()
	db.Close()
	v.viewCheckpoint = time.Now()
}

func (v *ViewSync) updateViewsInDatabase(ctx context.Context, selfClient *steampipesdk.SelfClient, metadataClient pg.Client) {
	var queryViews []models.QueryView

	err := metadataClient.DB().Find(&queryViews).Error
	if err != nil {
		v.logger.Error("Error fetching query views from metadata", zap.Error(err))
		v.logger.Sync()
		return
	}

	qvMap := make(map[string]models.QueryView)
	qvDag := dag.NewDirectedAcyclicGraph()
	for _, qv := range queryViews {
		qvMap[qv.ID] = qv
		qvDag.AddNodeIdempotent(qv.ID)
		for _, dep := range qv.Dependencies {
			qvDag.AddEdge(qv.ID, dep)
		}
	}

	sortedViewIds, err := qvDag.TopologicalSort()
	if err != nil {
		v.logger.Error("Error sorting views topologically", zap.Error(err))
		v.logger.Sync()
		return
	}

initLoop:
	for i := 0; i < 60; i++ {
		time.Sleep(10 * time.Second)

		for _, viewId := range sortedViewIds {
			view, ok := qvMap[viewId]
			if !ok {
				v.logger.Error("Error fetching view from map", zap.String("view", viewId))
				v.logger.Sync()
				continue
			}
			dropQuery := "DROP MATERIALIZED VIEW IF EXISTS " + view.ID + " CASCADE"
			_, err := selfClient.GetConnection().Exec(ctx, dropQuery)
			if err != nil {
				v.logger.Error("Error dropping materialized view", zap.Error(err), zap.String("view", view.ID))
				v.logger.Sync()
				continue
			}

			query := "CREATE MATERIALIZED VIEW IF NOT EXISTS " + view.ID + " AS " + view.Query
			_, err = selfClient.GetConnection().Exec(ctx, query)
			if strings.Contains(err.Error(), "SQLSTATE 42P01") {
				continue initLoop
			}
			if err != nil {
				v.logger.Error("Error creating materialized view", zap.Error(err), zap.String("view", view.ID))
				v.logger.Sync()
				continue
			}
		}
	}
}

func (v *ViewSync) start(ctx context.Context) {
	v.logger.Info("Initializing materialized views")
	v.logger.Info("Creating self client")
	v.logger.Sync()
	selfClient, err := steampipesdk.NewSelfClient(ctx)
	if err != nil {
		v.logger.Error("Error creating self client for init materialized views", zap.Error(err))
		v.logger.Sync()
		return
	}
	v.logger.Info("Creating metadata client")
	v.logger.Sync()
	metadataClient, err := pg.NewMetadataClient(v.metadataPostgresClientConfig, ctx)
	if err != nil {
		v.logger.Error("Error creating metadata client for init materialized views", zap.Error(err))
		v.logger.Sync()
		return
	}

	v.updateLock.Lock()
	v.updateViewsInDatabase(ctx, selfClient, metadataClient)
	v.updateLock.Unlock()

	selfClient.GetConnection().Close()
	db, _ := metadataClient.DB().DB()
	db.Close()

	v.viewCheckpoint = time.Now()

	go v.timeBasedViewSync(ctx)
	go v.pullBasedViewSync(ctx)
}
