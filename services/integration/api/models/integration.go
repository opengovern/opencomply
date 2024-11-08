package models

import (
	"github.com/opengovern/og-util/pkg/integration"
	"time"
)

type IntegrationState string

const (
	IntegrationStateActive   IntegrationState = "ACTIVE"
	IntegrationStateInactive IntegrationState = "INACTIVE"
	IntegrationStateArchived IntegrationState = "ARCHIVED"
)

type DiscoverIntegrationRequest struct {
	IntegrationType integration.Type `json:"integration_type"`
	Credentials     map[string]any   `json:"credentials"`
}

type DiscoverIntegrationResponse struct {
	CredentialID string        `json:"credential_id"`
	Integrations []Integration `json:"integrations"`
}

type AddIntegrationsRequest struct {
	IntegrationType integration.Type `json:"integration_type"`
	ProviderIDs     []string         `json:"provider_ids"`
	CredentialID    string           `json:"credential_id"`
}

type UpdateRequest struct {
	Credentials map[string]any `json:"credentials"`
}

type Integration struct {
	IntegrationID   string            `json:"integration_id"`
	ProviderID      string            `json:"provider_id"`
	Name            string            `json:"name"`
	IntegrationType integration.Type  `json:"integration_type"`
	Annotations     map[string]string `json:"annotations"`
	Labels          map[string]string `json:"labels"`

	CredentialID string `json:"credential_id"`

	State     IntegrationState `json:"state"`
	LastCheck *time.Time       `json:"last_check,omitempty"`
}

type ListIntegrationsResponse struct {
	Integrations []Integration `json:"integrations"`
	TotalCount   int           `json:"total_count"`
}

type ListIntegrationsRequest struct {
	IntegrationID   []string `json:"integration_id"`
	IntegrationType []string `json:"integration_type"`
	ProviderIDRegex *string  `json:"provider_id_regex"`
	NameRegex       *string  `json:"integration_name_regex"`
}
