package config

import "github.com/opengovern/og-util/pkg/config"

type WorkerConfig struct {
	NATS             config.NATS
	PostgreSQL       config.Postgres
	ElasticSearch    config.ElasticSearch
	Steampipe        config.Postgres
	Integration      config.OpenGovernanceService
	Scheduler        config.OpenGovernanceService
	Inventory        config.OpenGovernanceService
	EsSink           config.OpenGovernanceService
	PennywiseBaseURL string `yaml:"pennywise_base_url"`

	DoTelemetry          bool   `yaml:"do_telemetry"`
	TelemetryWorkspaceID string `yaml:"telemetry_workspace_id"`
	TelemetryHostname    string `yaml:"telemetry_hostname"`
	TelemetryBaseURL     string `yaml:"telemetry_base_url"`
}
