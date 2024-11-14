package inventory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/opengovern/og-util/pkg/model"
	"github.com/opengovern/og-util/pkg/postgres"
	integration_type "github.com/opengovern/opengovernance/services/integration/integration-type"
	"github.com/opengovern/opengovernance/services/inventory"
	"github.com/opengovern/opengovernance/services/migrator/config"
	"github.com/opengovern/opengovernance/services/migrator/db"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ResourceType struct {
	ResourceName         string
	Category             []string
	ResourceLabel        string
	ServiceName          string
	ListDescriber        string
	GetDescriber         string
	TerraformName        []string
	TerraformNameString  string `json:"-"`
	TerraformServiceName string
	Discovery            string
	IgnoreSummarize      bool
	SteampipeTable       string
	Model                string
}

type Migration struct {
}

func (m Migration) IsGitBased() bool {
	return false
}
func (m Migration) AttachmentFolderPath() string {
	return "/inventory-data-config"
}

func (m Migration) Run(ctx context.Context, conf config.MigratorConfig, logger *zap.Logger) error {
	orm, err := postgres.NewClient(&postgres.Config{
		Host:    conf.PostgreSQL.Host,
		Port:    conf.PostgreSQL.Port,
		User:    conf.PostgreSQL.Username,
		Passwd:  conf.PostgreSQL.Password,
		DB:      "inventory",
		SSLMode: conf.PostgreSQL.SSLMode,
	}, logger)
	if err != nil {
		return fmt.Errorf("new postgres client: %w", err)
	}
	dbm := db.Database{ORM: orm}

	awsResourceTypesContent, err := os.ReadFile(path.Join(m.AttachmentFolderPath(), "aws-resource-types.json"))
	if err != nil {
		return err
	}
	azureResourceTypesContent, err := os.ReadFile(path.Join(m.AttachmentFolderPath(), "azure-resource-types.json"))
	if err != nil {
		return err
	}
	var awsResourceTypes []ResourceType
	var azureResourceTypes []ResourceType
	if err := json.Unmarshal(awsResourceTypesContent, &awsResourceTypes); err != nil {
		return err
	}
	if err := json.Unmarshal(azureResourceTypesContent, &azureResourceTypes); err != nil {
		return err
	}

	err = dbm.ORM.Transaction(func(tx *gorm.DB) error {
		err := tx.Model(&inventory.ResourceType{}).Where("integration_type = ?", integration_type.IntegrationTypeAWSAccount).Unscoped().Delete(&inventory.ResourceType{}).Error
		if err != nil {
			logger.Error("failed to delete aws resource types", zap.Error(err))
			return err
		}

		for _, resourceType := range awsResourceTypes {
			err = tx.Clauses(clause.OnConflict{
				DoNothing: true,
			}).Create(&inventory.ResourceType{
				IntegrationType: integration_type.IntegrationTypeAWSAccount,
				ResourceType:    resourceType.ResourceName,
				ResourceLabel:   resourceType.ResourceLabel,
				ServiceName:     strings.ToLower(resourceType.ServiceName),
				DoSummarize:     !resourceType.IgnoreSummarize,
			}).Error
			if err != nil {
				logger.Error("failed to create aws resource type", zap.Error(err))
				return err
			}

			err = tx.Clauses(clause.OnConflict{
				DoNothing: true,
			}).Create(&inventory.ResourceTypeTag{
				Tag: model.Tag{
					Key:   "category",
					Value: resourceType.Category,
				},
				ResourceType: resourceType.ResourceName,
			}).Error
			if err != nil {
				logger.Error("failed to create aws resource type tag", zap.Error(err))
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failure in aws transaction: %v", err)
	}

	err = dbm.ORM.Transaction(func(tx *gorm.DB) error {
		err := tx.Model(&inventory.ResourceType{}).Where("integration_type = ?", integration_type.IntegrationTypeAzureSubscription).Unscoped().Delete(&inventory.ResourceType{}).Error
		if err != nil {
			logger.Error("failed to delete azure resource types", zap.Error(err))
			return err
		}
		for _, resourceType := range azureResourceTypes {
			err = tx.Clauses(clause.OnConflict{
				DoNothing: true,
			}).Create(&inventory.ResourceType{
				IntegrationType: integration_type.IntegrationTypeAzureSubscription,
				ResourceType:    resourceType.ResourceName,
				ResourceLabel:   resourceType.ResourceLabel,
				ServiceName:     strings.ToLower(resourceType.ServiceName),
				DoSummarize:     !resourceType.IgnoreSummarize,
			}).Error
			if err != nil {
				logger.Error("failed to create azure resource type", zap.Error(err))
				return err
			}

			err = tx.Clauses(clause.OnConflict{
				DoNothing: true,
			}).Create(&inventory.ResourceTypeTag{
				Tag: model.Tag{
					Key:   "category",
					Value: resourceType.Category,
				},
				ResourceType: resourceType.ResourceName,
			}).Error
			if err != nil {
				logger.Error("failed to create azure resource type tag", zap.Error(err))
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failure in azure transaction: %v", err)
	}

	return nil
}
