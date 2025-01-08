package configs

import "github.com/opengovern/og-util/pkg/integration"
import _ "embed"

//go:embed ui-spec.json
var UISpec []byte

const (
	IntegrationTypeDopplerAccount = integration.Type("doppler_account") // example: AWS_ACCOUNT, AZURE_SUBSCRIPTION
)
