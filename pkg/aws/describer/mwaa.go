package describer

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/mwaa"
	"gitlab.com/keibiengine/keibi-engine/pkg/aws/model"
)

func MWAAEnvironment(ctx context.Context, cfg aws.Config) ([]Resource, error) {
	client := mwaa.NewFromConfig(cfg)
	paginator := mwaa.NewListEnvironmentsPaginator(client, &mwaa.ListEnvironmentsInput{})

	var values []Resource
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, v := range page.Environments {
			environment, err := client.GetEnvironment(ctx, &mwaa.GetEnvironmentInput{
				Name: &v,
			})
			if err != nil {
				return nil, err
			}

			values = append(values, Resource{
				ARN:  *environment.Environment.Arn,
				Name: *environment.Environment.Name,
				Description: model.MWAAEnvironmentDescription{
					Environment: *environment.Environment,
				},
			})
		}
	}

	return values, nil
}
