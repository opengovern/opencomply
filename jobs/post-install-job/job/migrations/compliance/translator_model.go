package compliance

import (
	"github.com/opengovern/opencomply/jobs/post-install-job/job/migrations/shared"
)

type FrameworkFile struct {
	Framework Framework `json:"framework"`
}

type FrameworkMetadata struct {
	Defaults struct {
		AutoAssign        *bool `json:"auto-assign"`
		Enabled           bool  `json:"enabled"`
		TracksDriftEvents bool  `json:"tracks-drift-events"`
	} `json:"defaults"`
	Tags map[string][]string `json:"tags"`
}

type Framework struct {
	ID           string            `json:"id" yaml:"id"`
	Title        string            `json:"title" yaml:"title"`
	Description  string            `json:"description" yaml:"description"`
	SectionCode  string            `json:"section-code" yaml:"section-code"`
	Metadata     FrameworkMetadata `json:"metadata" yaml:"metadata"`
	ControlGroup []Framework       `json:"control-group" yaml:"control-group"`
	Controls     []string          `json:"controls" yaml:"controls"`
}

type Benchmark struct {
	ID                string              `json:"ID" yaml:"ID"`
	Title             string              `json:"Title" yaml:"Title"`
	SectionCode       string              `json:"SectionCode" yaml:"SectionCode"`
	Description       string              `json:"Description" yaml:"Description"`
	Children          []string            `json:"Children" yaml:"Children"`
	Tags              map[string][]string `json:"Tags" yaml:"Tags"`
	AutoAssign        *bool               `json:"AutoAssign" yaml:"AutoAssign"`
	Enabled           bool                `json:"Enabled" yaml:"Enabled"`
	TracksDriftEvents bool                `json:"TracksDriftEvents" yaml:"TracksDriftEvents"`
	Controls          []string            `json:"Controls" yaml:"Controls"`
}

type Control struct {
	ID                 string              `json:"ID" yaml:"ID"`
	Title              string              `json:"Title" yaml:"Title"`
	IntegrationType    []string            `json:"IntegrationType" yaml:"IntegrationType"`
	Description        string              `json:"Description" yaml:"Description"`
	Query              *shared.Query       `json:"Query" yaml:"Query"`
	ManualVerification bool                `json:"ManualVerification" yaml:"ManualVerification"`
	Severity           string              `json:"Severity" yaml:"Severity"`
	Tags               map[string][]string `json:"Tags" yaml:"Tags"`
	Managed            bool                `json:"Managed" yaml:"Managed"`
}

type QueryView struct {
	ID          string        `json:"id" yaml:"ID"`
	Title       string        `json:"title" yaml:"Title"`
	Description string        `json:"description" yaml:"Description"`
	Query       *shared.Query `json:"query" yaml:"Query"`

	Dependencies []string `json:"dependencies" yaml:"Dependencies"`
}
