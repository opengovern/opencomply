package describer

import (
	"context"
	"github.com/Azure/azure-sdk-for-go/services/storagesync/mgmt/2020-03-01/storagesync"
	"github.com/Azure/go-autorest/autorest"
	"gitlab.com/keibiengine/keibi-engine/pkg/azure/model"
)

func StorageSync(ctx context.Context, authorizer autorest.Authorizer, subscription string) ([]Resource, error) {
	client := storagesync.NewServicesClient(subscription)
	client.Authorizer = authorizer

	result, err := client.ListBySubscription(ctx)
	if err != nil {
		return nil, err
	}

	var values []Resource
	for _, storage := range *result.Value {
		values = append(values, Resource{
			ID: *storage.ID,
			Description: model.StorageSyncDescription{storage}})
	}
	return values, nil
}
