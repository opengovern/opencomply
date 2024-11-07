package onboard

import (
	"errors"
	"fmt"
	"github.com/goccy/go-yaml"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type ConnectionGroup struct {
	Name  string `json:"name" yaml:"name"`
	Query string `json:"query" yaml:"query"`
}

type GitParser struct {
	//connectionGroups []model.ConnectionGroup
}

func (g *GitParser) ExtractConnectionGroups(queryPath string) error {
	//g.connectionGroups = append(g.connectionGroups, defaultConnectionGroups...)
	err := filepath.WalkDir(queryPath, func(path string, d fs.DirEntry, err error) error {
		if strings.HasSuffix(path, ".yaml") {
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failure in reading file: %v", err)
			}

			var cg ConnectionGroup
			err = yaml.Unmarshal(content, &cg)
			if err != nil {
				return err
			}

			fileName := filepath.Base(path)
			if strings.HasSuffix(fileName, ".yaml") {
				fileName = fileName[:len(fileName)-len(".yaml")]
			}

			//g.connectionGroups = append(g.connectionGroups, model.ConnectionGroup{
			//	Name:  fileName,
			//	Query: cg.Query,
			//})
		}

		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failure in walking directory: %v", err)
	}
	return nil
}
