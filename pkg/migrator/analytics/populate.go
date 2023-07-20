package analytics

import (
	"encoding/json"
	analyticsDB "github.com/kaytu-io/kaytu-engine/pkg/analytics/db"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func PopulateDatabase(logger *zap.Logger, dbc *gorm.DB, analyticsPath string) error {
	err := filepath.Walk(analyticsPath, func(path string, info fs.FileInfo, err error) error {
		if strings.HasSuffix(path, ".json") {
			id := strings.TrimSuffix(info.Name(), ".json")
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var metric Metric
			err = json.Unmarshal(content, &metric)
			if err != nil {
				return err
			}

			dbMetric := analyticsDB.Metric{
				ID:         id,
				Connectors: metric.Connectors,
				Name:       metric.Name,
				Query:      metric.Query,
			}
			err = dbc.Model(&analyticsDB.Metric{}).Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "id"}},                                    // key column
				DoUpdates: clause.AssignmentColumns([]string{"connector", "name", "query"}), // column needed to be updated
			}).Create(dbMetric).Error
			if err != nil {
				logger.Error("failure in insert", zap.Error(err))
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
