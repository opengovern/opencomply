package pg

import (
	"context"
	"errors"
	integration "github.com/opengovern/opengovernance/services/integration-v2/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (c Client) ListIntegrations(ctx context.Context) ([]integration.Integration, error) {
	var result []integration.Integration
	err := c.db.Preload(clause.Associations).Find(&result).Error
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c Client) GetIntegrationByID(ctx context.Context, opengovernanceId string, id string) (*integration.Integration, error) {
	var result integration.Integration
	var err error
	tx := c.db.Preload(clause.Associations).Model(&integration.Integration{})
	switch {
	case opengovernanceId != "" && id != "":
		err = tx.Where("integration_id = ? AND provider_id = ?", opengovernanceId, id).First(&result).Error
	case opengovernanceId != "" && id == "":
		err = tx.Where("integration_id = ?", opengovernanceId).First(&result).Error
	case opengovernanceId == "" && id != "":
		err = tx.Where("provider_id = ?", id).First(&result).Error
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}
