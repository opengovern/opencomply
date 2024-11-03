package job

import (
	"archive/zip"
	"strings"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	// "strings"

	"github.com/opengovern/og-util/pkg/api"
	"github.com/opengovern/og-util/pkg/httpclient"
	"github.com/opengovern/opengovernance/pkg/metadata/client"
	"github.com/opengovern/opengovernance/pkg/metadata/models"
	"github.com/opengovern/opengovernance/services/migrator/config"
	"go.uber.org/zap"
)
func Unzip(src, dest,url string) error {
    r, err := zip.OpenReader(src)
    if err != nil {
        return err
    }
    defer func() {
        if err := r.Close(); err != nil {
            panic(err)
        }
    }()

    os.MkdirAll(dest, 0755)

    // Closure to address file descriptors issue with all the deferred .Close() methods
    extractAndWriteFile := func(f *zip.File) error {
        rc, err := f.Open()
        if err != nil {
            return err
        }
        defer func() {
            if err := rc.Close(); err != nil {
                panic(err)
            }
        }()

        path := filepath.Join(dest, f.Name)

        // Check for ZipSlip (Directory traversal)
        // if !strings.HasPrefix(path, filepath.Clean(dest) + string(os.PathSeparator)) {
        //     return fmt.Errorf("illegal file path: %s", path)
        // }

        if f.FileInfo().IsDir() {
			if(!strings.Contains(url,strings.Split(f.Name, "-")[0])){
  os.MkdirAll(path, f.Mode())
			}
          
        } else {
            os.MkdirAll(filepath.Dir(path), f.Mode())
            f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
            if err != nil {
                return err
            }
            defer func() {
                if err := f.Close(); err != nil {
                    panic(err)
                }
            }()

            _, err = io.Copy(f, rc)
            if err != nil {
                return err
            }
        }
        return nil
    }

    for _, f := range r.File {
        err := extractAndWriteFile(f)
        if err != nil {
            return err
        }
    }

    return nil
}

func GitClone(conf config.MigratorConfig, logger *zap.Logger) (string, error) {
	gitConfig := GitConfig{
		AnalyticsGitURL:         conf.AnalyticsGitURL,
		ControlEnrichmentGitURL: conf.ControlEnrichmentGitURL,
		githubToken:             conf.GithubToken,
	}

	metadataClient := client.NewMetadataServiceClient(conf.Metadata.BaseURL)
	value, err := metadataClient.GetConfigMetadata(&httpclient.Context{
		UserRole: api.AdminRole,
	}, models.MetadataKeyAnalyticsGitURL)

	if err == nil && len(value.GetValue().(string)) > 0 {
		gitConfig.AnalyticsGitURL = value.GetValue().(string)
	} else if err != nil {
		logger.Error("failed to get analytics git url from metadata", zap.Error(err))
	}

	logger.Info("using git repo", zap.String("url", gitConfig.AnalyticsGitURL))

	// refs := make([]string, 0, 2)
	URL := gitConfig.AnalyticsGitURL

    resp, err := http.Get(URL)
    if err != nil {
        logger.Error("err: %s", zap.Error(err))
    }


    defer resp.Body.Close()
    

    // Create the file
    out, err := os.Create("test.zip")
    if err != nil {
        logger.Error("err: %s", zap.Error(err))
    }
    defer out.Close()

    // Write the body to file
    _, err = io.Copy(out, resp.Body)
	logger.Error("err: %s", zap.Error(err))
	os.RemoveAll(config.ConfigzGitPath)
	Unzip("test.zip", config.ConfigzGitPath,URL)
	os.Remove("test.zip")

	
	logger.Info("finished fetching configz data")

	
	// logger.Info("using git repo for enrichmentor", zap.String("url", gitConfig.ControlEnrichmentGitURL))

	// os.RemoveAll(config.ControlEnrichmentGitPath)
	// URL = gitConfig.ControlEnrichmentGitURL
	// resp, err = http.Get(URL)
	//  if err != nil {
    //     logger.Error("err: %s", zap.Error(err))
    // }


    // defer resp.Body.Close()
    // // Create the file
    // out, err = os.Create("test1.zip")
    // if err != nil {
    //     logger.Error("err: %s", zap.Error(err))
    // }
    // defer out.Close()

    // // Write the body to file
    // _, err = io.Copy(out, resp.Body)
	// logger.Error("err: %s", zap.Error(err))
	// Unzip("test1.zip", config.ControlEnrichmentGitPath)
	// os.Remove("test1.zip")
	// logger.Info("finished fetching control enrichment data")
	return string("both completed need releases"), nil
}
