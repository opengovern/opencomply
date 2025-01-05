package db


import (
	"github.com/opengovern/opencomply/services/core/db/models"
	"gorm.io/gorm"
)


type Database struct {
	orm *gorm.DB
}

func NewDatabase(orm *gorm.DB) Database {
	return Database{orm: orm}
}

func (db Database) Initialize() error {
	err := db.orm.AutoMigrate(
		// shared
		&models.Query{},
		&models.QueryParameter{},
		// inventory
		&models.ResourceType{},
		&models.NamedQuery{},
		&models.NamedQueryTag{},
		&models.NamedQueryHistory{},
		&models.ResourceTypeTag{},
		&models.ResourceCollection{},
		&models.ResourceCollectionTag{},
		&models.ResourceTypeV2{},
		// metadata
		&models.ConfigMetadata{},
		&models.PolicyParameterValues{},
		&models.QueryView{},
		&models.QueryViewTag{},
		&models.PlatformConfiguration{},
	)
	if err != nil {
		return err
	}

	return nil
}