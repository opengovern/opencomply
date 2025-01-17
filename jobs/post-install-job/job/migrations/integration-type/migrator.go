package integration_type

import (
	"context"
	"fmt"
	"github.com/opengovern/og-util/pkg/postgres"
	"github.com/opengovern/opencomply/jobs/post-install-job/config"
	"github.com/opengovern/opencomply/jobs/post-install-job/db"
	"github.com/opengovern/opencomply/services/integration/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Migration struct {
}

func (m Migration) IsGitBased() bool {
	return true
}
func (m Migration) AttachmentFolderPath() string {
	return config.ConfigzGitPath
}

func (m Migration) Run(ctx context.Context, conf config.MigratorConfig, logger *zap.Logger) error {
	orm, err := postgres.NewClient(&postgres.Config{
		Host:    conf.PostgreSQL.Host,
		Port:    conf.PostgreSQL.Port,
		User:    conf.PostgreSQL.Username,
		Passwd:  conf.PostgreSQL.Password,
		DB:      "integration_types",
		SSLMode: conf.PostgreSQL.SSLMode,
	}, logger)
	if err != nil {
		return fmt.Errorf("new postgres client: %w", err)
	}
	dbm := db.Database{ORM: orm}

	err = dbm.ORM.AutoMigrate(&models.IntegrationPlugin{})
	if err != nil {
		logger.Error("failed to auto migrate integration binaries", zap.Error(err))
		return err
	}

	parser := GitParser{}
	err = parser.ExtractIntegrations(logger)
	if err != nil {
		return err
	}

	err = dbm.ORM.Transaction(func(tx *gorm.DB) error {
		err := tx.Model(&models.IntegrationPlugin{}).Where("1 = 1").Unscoped().Delete(&models.IntegrationPlugin{}).Error
		if err != nil {
			logger.Error("failed to delete integration binaries", zap.Error(err))
			return err
		}

		for _, iPlugin := range parser.Integrations.Plugins {
			integrationBinary, err := parser.ExtractIntegrationBinaries(logger, iPlugin)
			if err != nil {
				return err
			}
			if integrationBinary == nil {
				continue
			}
			err = tx.Clauses(clause.OnConflict{
				DoNothing: true,
			}).Create(integrationBinary).Error
			if err != nil {
				logger.Error("failed to create integration binary", zap.Error(err))
				return err
			}
		}

		return nil
	})

	return nil
}