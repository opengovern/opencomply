package core

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	dexApi "github.com/dexidp/dex/api/v2"
	helmv2 "github.com/fluxcd/helm-controller/api/v2beta1"
	"github.com/google/uuid"
	api6 "github.com/hashicorp/vault/api"
	config3 "github.com/opengovern/og-util/pkg/config"
	"github.com/opengovern/og-util/pkg/opengovernance-es-sdk"
	"github.com/opengovern/og-util/pkg/postgres"
	"github.com/opengovern/og-util/pkg/steampipe"
	"github.com/opengovern/og-util/pkg/vault"
	db2 "github.com/opengovern/opencomply/jobs/post-install-job/db"
	"github.com/opengovern/opencomply/jobs/post-install-job/db/model"
	complianceClient "github.com/opengovern/opencomply/services/compliance/client"
	"github.com/opengovern/opencomply/services/core/config"
	"github.com/opengovern/opencomply/services/core/db"
	"github.com/opengovern/opencomply/services/core/db/models"
	integrationClient "github.com/opengovern/opencomply/services/integration/client"
	describeClient "github.com/opengovern/opencomply/services/scheduler/client"
	"go.uber.org/zap"
	v1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type HttpHandler struct {
	client            opengovernance.Client
	db                db.Database
	steampipeConn     *steampipe.Database
	schedulerClient   describeClient.SchedulerServiceClient
	integrationClient integrationClient.IntegrationServiceClient
	complianceClient  complianceClient.ComplianceServiceClient
	logger *zap.Logger
	viewCheckpoint time.Time
	cfg                config.Config
	kubeClient         client.Client
	vault              vault.VaultSourceConfig
	vaultSecretHandler vault.VaultSecretHandler
	dexClient          dexApi.DexClient
	migratorDb         *db2.Database

}

func InitializeHttpHandler(
	cfg config.Config,
	steampipeHost string, steampipePort string, steampipeDb string, steampipeUsername string, steampipePassword string,
	schedulerBaseUrl string, integrationBaseUrl string, complianceBaseUrl string,
	logger *zap.Logger,dexClient dexApi.DexClient,esConf config3.ElasticSearch,
) (h *HttpHandler, err error) {
	h = &HttpHandler{}
	ctx := context.Background()

	fmt.Println("Initializing http handler")
	// shared
	// setup postgres connection
	psqlCfg := postgres.Config{
		Host:    cfg.Postgres.Host,
		Port:    cfg.Postgres.Port,
		User:    cfg.Postgres.Username,
		Passwd:  cfg.Postgres.Password,
		DB:      cfg.Postgres.DB,
		SSLMode: cfg.Postgres.SSLMode,
	}
	orm, err := postgres.NewClient(&psqlCfg, logger)
	if err != nil {
		return nil, fmt.Errorf("new postgres client: %w", err)
	}

	h.db = db.NewDatabase(orm)
	fmt.Println("Connected to the postgres database: ", cfg.Postgres.DB)

	err = h.db.Initialize()
	if err != nil {
		return nil, err
	}
	fmt.Println("Initialized postgres database: ", cfg.Postgres.DB)
	// metadata
	apps, err := h.db.ListApp()
	if err != nil {
		return nil, err
	}
	if len(apps) == 0 {
		err = h.db.CreateApp(&models.PlatformConfiguration{
			InstallID: uuid.New(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
		if err != nil {
			return nil, err
		}
	}

	migratorDbCfg := postgres.Config{
		Host:    cfg.Postgres.Host,
		Port:    cfg.Postgres.Port,
		User:    cfg.Postgres.Username,
		Passwd:  cfg.Postgres.Password,
		DB:      "migrator",
		SSLMode: cfg.Postgres.SSLMode,
	}
	migratorOrm, err := postgres.NewClient(&migratorDbCfg, logger)
	if err != nil {
		return nil, fmt.Errorf("new postgres client: %w", err)
	}
	if err := migratorOrm.AutoMigrate(&model.Migration{}); err != nil {
		return nil, fmt.Errorf("gorm migrate: %w", err)
	}
	migratorDb := &db2.Database{ORM: migratorOrm}
	

	kubeClient, err := NewKubeClient()
	if err != nil {
		return nil, err
	}
	err = v1.AddToScheme(kubeClient.Scheme())
	if err != nil {
		return nil, fmt.Errorf("add v1 to scheme: %w", err)
	}
	h.kubeClient = kubeClient
	h.cfg = cfg
	h.migratorDb = migratorDb
	h.dexClient = dexClient
	h.viewCheckpoint =time.Now().Add(-time.Hour * 2)
	switch cfg.Vault.Provider {
	case vault.AwsKMS:
		h.vault, err = vault.NewKMSVaultSourceConfig(ctx, cfg.Vault.Aws, cfg.Vault.KeyId)
		if err != nil {
			logger.Error("new kms vaultClient source config", zap.Error(err))
			return nil, fmt.Errorf("new kms vaultClient source config: %w", err)
		}
	case vault.AzureKeyVault:
		h.vault, err = vault.NewAzureVaultClient(ctx, logger, cfg.Vault.Azure, cfg.Vault.KeyId)
		if err != nil {
			logger.Error("new azure vaultClient source config", zap.Error(err))
			return nil, fmt.Errorf("new azure vaultClient source config: %w", err)
		}
		h.vaultSecretHandler, err = vault.NewAzureVaultSecretHandler(logger, cfg.Vault.Azure)
		if err != nil {
			logger.Error("new azure vaultClient secret handler", zap.Error(err))
			return nil, fmt.Errorf("new azure vaultClient secret handler: %w", err)
		}
	case vault.HashiCorpVault:
		h.vaultSecretHandler, err = vault.NewHashiCorpVaultSecretHandler(ctx, logger, cfg.Vault.HashiCorp)
		if err != nil {
			logger.Error("new hashicorp vaultClient secret handler", zap.Error(err))
			return nil, fmt.Errorf("new hashicorp vaultClient secret handler: %w", err)
		}

		h.vault, err = vault.NewHashiCorpVaultClient(ctx, logger, cfg.Vault.HashiCorp, cfg.Vault.KeyId)
		if err != nil {
			if strings.Contains(err.Error(), api6.ErrSecretNotFound.Error()) {
				b := make([]byte, 32)
				_, err := rand.Read(b)
				if err != nil {
					return nil, err
				}

				_, err = h.vaultSecretHandler.SetSecret(ctx, cfg.Vault.KeyId, b)
				if err != nil {
					return nil, err
				}

				h.vault, err = vault.NewHashiCorpVaultClient(ctx, logger, cfg.Vault.HashiCorp, cfg.Vault.KeyId)
				if err != nil {
					logger.Error("new hashicorp vaultClient source config after setSecret", zap.Error(err))
					return nil, fmt.Errorf("new hashicorp vaultClient source config after setSecret: %w", err)
				}
			} else {
				logger.Error("new hashicorp vaultClient source config", zap.Error(err))
				return nil, fmt.Errorf("new hashicorp vaultClient source config: %w", err)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported vault provider: %s", cfg.Vault.Provider)
	}

	switch cfg.Vault.Provider {
	case vault.AzureKeyVault, vault.HashiCorpVault:
		_, err = h.vaultSecretHandler.GetSecret(ctx, h.cfg.Vault.KeyId)
		if err != nil {
			// create new aes key
			b := make([]byte, 32)
			_, err := rand.Read(b)
			if err != nil {
				h.logger.Error("failed to generate random bytes", zap.Error(err))
			}
			_, err = h.vaultSecretHandler.SetSecret(ctx, h.cfg.Vault.KeyId, b)
			if err != nil {
				h.logger.Error("failed to set secret", zap.Error(err))
			}
		}
	default:
		h.logger.Error("unsupported vault provider", zap.Any("provider", h.cfg.Vault.Provider))
	}
	// setup steampipe connection
	// inventory
	steampipeConn, err := steampipe.NewSteampipeDatabase(steampipe.Option{
		Host: steampipeHost,
		Port: steampipePort,
		User: steampipeUsername,
		Pass: steampipePassword,
		Db:   steampipeDb,
	})
	h.steampipeConn = steampipeConn
	if err != nil {
		return nil, err
	}
	fmt.Println("Initialized steampipe database: ", steampipeConn)

	h.client, err = opengovernance.NewClient(opengovernance.ClientConfig{
		Addresses:     []string{esConf.Address},
		Username:      &esConf.Username,
		Password:      &esConf.Password,
		IsOnAks:       &esConf.IsOnAks,
		IsOpenSearch:  &esConf.IsOpenSearch,
		AwsRegion:     &esConf.AwsRegion,
		AssumeRoleArn: &esConf.AssumeRoleArn,
	})
	if err != nil {
		return nil, err
	}
	h.schedulerClient = describeClient.NewSchedulerServiceClient(schedulerBaseUrl)

	h.integrationClient = integrationClient.NewIntegrationServiceClient(integrationBaseUrl)
	h.complianceClient = complianceClient.NewComplianceClient(complianceBaseUrl)

	h.logger = logger

	return h, nil
}


func NewKubeClient() (client.Client, error) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := helmv2.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := v1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	kubeClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	return kubeClient, nil
}
