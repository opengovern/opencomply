package describer

import (
	"context"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/storagecache/mgmt/2021-05-01/storagecache"
	"github.com/Azure/go-autorest/autorest"
	"gitlab.com/keibiengine/keibi-engine/pkg/azure/model"
)

func HpcCache(ctx context.Context, authorizer autorest.Authorizer, subscription string) ([]Resource, error) {
	client := storagecache.NewCachesClient(subscription)
	client.Authorizer = authorizer

	result, err := client.List(ctx)
	if err != nil {
		return nil, err
	}

	var values []Resource
	for {
		for _, v := range result.Values() {
			resourceGroup := strings.Split(*v.ID, "/")[4]

			values = append(values, Resource{
				ID:       *v.ID,
				Name:     *v.Name,
				Location: *v.Location,
				Description: model.HpcCacheDescription{
					Cache:         v,
					ResourceGroup: resourceGroup,
				},
			})
		}

		if !result.NotDone() {
			break
		}

		err = result.NextWithContext(ctx)
		if err != nil {
			return nil, err
		}
	}

	return values, nil
}
