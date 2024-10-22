package describe

import (
	"errors"
	apiAuth "github.com/opengovern/og-util/pkg/api"
	"github.com/opengovern/og-util/pkg/httpclient"
	"github.com/opengovern/opengovernance/pkg/utils"
	"strings"

	"github.com/opengovern/og-aws-describer/aws"
	"github.com/opengovern/og-azure-describer/azure"
	analyticsDb "github.com/opengovern/opengovernance/pkg/analytics/db"
	"github.com/opengovern/opengovernance/pkg/describe/api"
	"github.com/opengovern/opengovernance/pkg/metadata/models"
)

func (s *Scheduler) ListDiscoveryResourceTypes() (api.ListDiscoveryResourceTypes, error) {
	var result api.ListDiscoveryResourceTypes

	assetDiscoveryEnabledMetadata, err := s.metadataClient.GetConfigMetadata(&httpclient.Context{UserRole: apiAuth.AdminRole}, models.MetadataKeyAssetDiscoveryEnabled)
	if err != nil {
		return result, err
	}
	assetDiscoveryEnabled := assetDiscoveryEnabledMetadata.GetValue().(bool)

	spendDiscoveryEnabledMetadata, err := s.metadataClient.GetConfigMetadata(&httpclient.Context{UserRole: apiAuth.AdminRole}, models.MetadataKeySpendDiscoveryEnabled)
	if err != nil {
		return result, err
	}
	spendDiscoveryEnabled := spendDiscoveryEnabledMetadata.GetValue().(bool)

	azureDiscoveryType, err := s.metadataClient.GetConfigMetadata(&httpclient.Context{UserRole: apiAuth.AdminRole}, models.MetadataKeyAzureDiscoveryRequiredOnly)
	if err != nil {
		return result, err
	}

	awsDiscoveryType, err := s.metadataClient.GetConfigMetadata(&httpclient.Context{UserRole: apiAuth.AdminRole}, models.MetadataKeyAWSDiscoveryRequiredOnly)
	if err != nil {
		return result, err
	}

	azureRequiredOnly := azureDiscoveryType.GetValue().(bool)
	awsRequiredOnly := awsDiscoveryType.GetValue().(bool)

	awsResourceTypes, azureResourceTypes := aws.ListResourceTypes(), azure.ListResourceTypes()
	if !assetDiscoveryEnabled {
		var rts []string

		for _, rt := range awsResourceTypes {
			if !strings.Contains(rt, "Cost") {
				continue
			}
			rts = append(rts, rt)
		}
		awsResourceTypes = rts

		rts = nil
		for _, rt := range azureResourceTypes {
			if !strings.Contains(rt, "Cost") {
				continue
			}
			rts = append(rts, rt)
		}
		azureResourceTypes = rts
	}

	if !spendDiscoveryEnabled {
		var rts []string

		for _, rt := range awsResourceTypes {
			if strings.Contains(rt, "Cost") {
				continue
			}
			rts = append(rts, rt)
		}
		awsResourceTypes = rts

		rts = nil
		for _, rt := range azureResourceTypes {
			if strings.Contains(rt, "Cost") {
				continue
			}
			rts = append(rts, rt)
		}
		azureResourceTypes = rts
	}

	if !azureRequiredOnly && !awsRequiredOnly {
		result.AzureResourceTypes = azureResourceTypes
		result.AWSResourceTypes = awsResourceTypes
		return result, nil
	}

	var resourceTypes []string
	assetMetrics, err := s.inventoryClient.ListAnalyticsMetrics(&httpclient.Context{UserRole: apiAuth.AdminRole}, utils.GetPointer(analyticsDb.MetricTypeAssets))
	if err != nil {
		return result, err
	}
	spendMetrics, err := s.inventoryClient.ListAnalyticsMetrics(&httpclient.Context{UserRole: apiAuth.AdminRole}, utils.GetPointer(analyticsDb.MetricTypeSpend))
	if err != nil {
		return result, err
	}
	for _, metric := range append(assetMetrics, spendMetrics...) {
		rts := extractResourceTypes(metric.Query, metric.Connectors)
		resourceTypes = append(resourceTypes, rts...)
	}
	result.AzureResourceTypes = append(result.AzureResourceTypes, "Microsoft.CostManagement/CostByResourceType")
	result.AWSResourceTypes = append(result.AWSResourceTypes, "AWS::CostExplorer::ByServiceDaily")

	queries, err := s.complianceClient.ListQueries(&httpclient.Context{UserRole: apiAuth.AdminRole})
	if err != nil {
		return result, err
	}
	controls, err := s.complianceClient.ListControl(&httpclient.Context{UserRole: apiAuth.AdminRole}, nil, nil)
	if err != nil {
		return result, err
	}
	for _, control := range controls {
		if !control.ManualVerification && control.Query != nil {
			for _, query := range queries {
				if control.Query.ID == query.ID {
					rts := extractResourceTypes(query.QueryToExecute, query.Connector)
					resourceTypes = append(resourceTypes, rts...)
					break
				}
			}
		}
	}

	for _, resourceType := range resourceTypes {
		resourceType = strings.ToLower(resourceType)
		if strings.HasPrefix(resourceType, "aws") {
			for _, awsResourceType := range awsResourceTypes {
				if strings.ToLower(awsResourceType) == resourceType {
					resourceType = awsResourceType
					break
				}
			}
			result.AWSResourceTypes = append(result.AWSResourceTypes, resourceType)
		} else if strings.HasPrefix(resourceType, "microsoft") {
			for _, azureResourceType := range azureResourceTypes {
				if strings.ToLower(azureResourceType) == resourceType {
					resourceType = azureResourceType
					break
				}
			}
			result.AzureResourceTypes = append(result.AzureResourceTypes, resourceType)
		} else if strings.HasPrefix(resourceType, "azure") {
			result.AzureResourceTypes = append(result.AzureResourceTypes, resourceType)
		} else {
			return result, errors.New("invalid resource type:" + resourceType)
		}
	}

	result.AWSResourceTypes = UniqueArray(result.AWSResourceTypes)
	result.AzureResourceTypes = UniqueArray(result.AzureResourceTypes)

	if !azureRequiredOnly {
		result.AzureResourceTypes = azureResourceTypes
	}
	if !awsRequiredOnly {
		result.AWSResourceTypes = awsResourceTypes
	}

	return result, nil
}
