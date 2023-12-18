package inventory

import (
	"context"
	"fmt"
	"github.com/kaytu-io/kaytu-engine/pkg/httpserver"
	config3 "github.com/kaytu-io/kaytu-engine/pkg/inventory/config"
	"github.com/kaytu-io/kaytu-util/pkg/config"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"os"
)

var (
	RedisAddress = os.Getenv("REDIS_ADDRESS")

	PostgreSQLHost     = os.Getenv("POSTGRESQL_HOST")
	PostgreSQLPort     = os.Getenv("POSTGRESQL_PORT")
	PostgreSQLDb       = os.Getenv("POSTGRESQL_DB")
	PostgreSQLUser     = os.Getenv("POSTGRESQL_USERNAME")
	PostgreSQLPassword = os.Getenv("POSTGRESQL_PASSWORD")
	PostgreSQLSSLMode  = os.Getenv("POSTGRESQL_SSLMODE")

	SteampipeHost     = os.Getenv("STEAMPIPE_HOST")
	SteampipePort     = os.Getenv("STEAMPIPE_PORT")
	SteampipeDb       = os.Getenv("STEAMPIPE_DB")
	SteampipeUser     = os.Getenv("STEAMPIPE_USERNAME")
	SteampipePassword = os.Getenv("STEAMPIPE_PASSWORD")

	KafkaService = os.Getenv("KAFKA_SERVICE")

	SchedulerBaseUrl  = os.Getenv("SCHEDULER_BASE_URL")
	OnboardBaseUrl    = os.Getenv("ONBOARD_BASE_URL")
	ComplianceBaseUrl = os.Getenv("COMPLIANCE_BASE_URL")

	HttpAddress = os.Getenv("HTTP_ADDRESS")
)

func Command() *cobra.Command {
	return &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				cnf config3.InventoryConfig
			)
			config.ReadFromEnv(&cnf, nil)

			return start(cmd.Context(), cnf)
		},
	}
}

func start(ctx context.Context, cnf config3.InventoryConfig) error {
	logger, err := zap.NewProduction()
	if err != nil {
		return fmt.Errorf("new logger: %w", err)
	}

	handler, err := InitializeHttpHandler(
		cnf.ElasticSearch,
		PostgreSQLHost, PostgreSQLPort, PostgreSQLDb, PostgreSQLUser, PostgreSQLPassword, PostgreSQLSSLMode,
		SteampipeHost, SteampipePort, SteampipeDb, SteampipeUser, SteampipePassword,
		KafkaService,
		SchedulerBaseUrl, OnboardBaseUrl, ComplianceBaseUrl,
		logger,
		RedisAddress,
	)
	if err != nil {
		return fmt.Errorf("init http handler: %w", err)
	}

	return httpserver.RegisterAndStart(logger, HttpAddress, handler)
}
