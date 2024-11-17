package elasticsearch

import (
	"context"
	"github.com/opengovern/og-util/pkg/opengovernance-es-sdk"
	"github.com/opengovern/opengovernance/services/migrator/config"
	"go.uber.org/zap"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Migration struct {
}

func (m Migration) IsGitBased() bool {
	return false
}

func (m Migration) AttachmentFolderPath() string {
	return "/elasticsearch-index-config"
}

func (m Migration) Run(ctx context.Context, conf config.MigratorConfig, logger *zap.Logger) error {
	logger.Info("running", zap.String("es_address", conf.ElasticSearch.Address))

	var externalID *string
	if len(conf.ElasticSearch.ExternalID) > 0 {
		externalID = &conf.ElasticSearch.ExternalID
	}
	elastic, err := opengovernance.NewClient(opengovernance.ClientConfig{
		Addresses:     []string{conf.ElasticSearch.Address},
		Username:      &conf.ElasticSearch.Username,
		Password:      &conf.ElasticSearch.Password,
		IsOpenSearch:  &conf.ElasticSearch.IsOpenSearch,
		IsOnAks:       &conf.ElasticSearch.IsOnAks,
		AwsRegion:     &conf.ElasticSearch.AwsRegion,
		AssumeRoleArn: &conf.ElasticSearch.AssumeRoleArn,
		ExternalID:    externalID,
	})
	if err != nil {
		logger.Error("failed to create es client due to", zap.Error(err))
		return err
	}

	for {
		err := elastic.Healthcheck(ctx)
		if err != nil {
			if err.Error() == "unhealthy" {
				logger.Warn("Waiting for status to be GREEN or YELLOW. Sleeping for 10 seconds...")
				time.Sleep(10 * time.Second)
				continue
			}
			logger.Error("failed to check es healthcheck due to", zap.Error(err))
			return err
		}
		break
	}
	logger.Warn("Starting es migration")

	var files []string
	err = filepath.Walk(m.AttachmentFolderPath(), func(path string, info fs.FileInfo, err error) error {
		if strings.HasSuffix(info.Name(), ".json") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		logger.Error("failed to get files", zap.Error(err))
		return err
	}

	var finalErr error
	for _, fp := range files {
		if strings.Contains(fp, "_component_template") {
			err = CreateTemplate(ctx, elastic, logger, fp)
			if err != nil {
				finalErr = err
				logger.Error("failed to create component template", zap.Error(err), zap.String("filepath", fp))
			}
		}
	}

	for _, fp := range files {
		if !strings.Contains(fp, "_component_template") {
			err = CreateTemplate(ctx, elastic, logger, fp)
			if err != nil {
				finalErr = err
				logger.Error("failed to create template", zap.Error(err), zap.String("filepath", fp))
			}
		}
	}

	// Increase number of shards per node to 2000
	// curl -X PUT http://localhost:9200/_cluster/settings --json '{"persistent":{"cluster.max_shards_per_node": 2000}}'
	reqBody := `{"persistent":{"cluster.max_shards_per_node": 2000}}`
	res, err := elastic.ES().Cluster.PutSettings(strings.NewReader(reqBody))
	if err != nil {
		logger.Error("failed to increase number of shards per node", zap.Error(err))
		finalErr = err
	} else if res.StatusCode != 200 {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			logger.Error("failed to read response body", zap.Error(err))
		}
		logger.Error("failed to increase number of shards per node", zap.String("body", string(body)), zap.Int("status_code", res.StatusCode))
	}

	return finalErr
}

func CreateTemplate(ctx context.Context, es opengovernance.Client, logger *zap.Logger, fp string) error {
	fn := filepath.Base(fp)
	idx := strings.LastIndex(fn, ".")
	fne := fn[:idx]

	f, err := os.ReadFile(fp)
	if err != nil {
		return err
	}

	if strings.HasSuffix(fne, "_component_template") {
		err = es.CreateComponentTemplate(ctx, fne, string(f))
		if err != nil {
			logger.Error("failed to create component template", zap.Error(err), zap.String("filepath", fp))
			return err
		}
	} else {
		err = es.CreateIndexTemplate(ctx, fne, string(f))
		if err != nil {
			logger.Error("failed to create index template", zap.Error(err), zap.String("filepath", fp))
			return err
		}
	}

	return nil
}
