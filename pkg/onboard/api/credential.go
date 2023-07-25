package api

import (
	"time"

	"github.com/kaytu-io/kaytu-util/pkg/source"
)

type CreateCredentialRequest struct {
	SourceType source.Type `json:"source_type" example:"Azure"`
	Config     any         `json:"config"`
}

type CreateCredentialResponse struct {
	ID string `json:"id"`
}

type UpdateCredentialRequest struct {
	Connector source.Type `json:"connector" example:"Azure"`
	Name      *string     `json:"name"`
	Config    any         `json:"config"`
}

type ListCredentialResponse struct {
	TotalCredentialCount int          `json:"totalCredentialCount"`
	Credentials          []Credential `json:"credentials"`
}

type CredentialType string

const (
	CredentialTypeAutoAzure             CredentialType = "auto-azure"
	CredentialTypeAutoAws               CredentialType = "auto-aws"
	CredentialTypeManualAwsOrganization CredentialType = "manual-aws-org"
	CredentialTypeManualAzureSpn        CredentialType = "manual-azure-spn"
)

type Credential struct {
	ID             string         `json:"id"`
	Name           *string        `json:"name,omitempty"`
	ConnectorType  source.Type    `json:"connectorType"`
	CredentialType CredentialType `json:"credentialType"`
	Enabled        bool           `json:"enabled"`
	OnboardDate    time.Time      `json:"onboardDate"`

	Config any `json:"config"`

	LastHealthCheckTime time.Time           `json:"lastHealthCheckTime"`
	HealthStatus        source.HealthStatus `json:"healthStatus"`
	HealthReason        *string             `json:"healthReason,omitempty"`

	Metadata map[string]any `json:"metadata,omitempty"`

	Connections []Connection `json:"connections,omitempty"`

	TotalConnections     *int `json:"total_connections"`
	EnabledConnections   *int `json:"enabled_connections"`
	UnhealthyConnections *int `json:"unhealthy_connections"`
}
