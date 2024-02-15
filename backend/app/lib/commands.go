package lib

import (
	"context"
	"talk2robots/m/v2/app/models"
)

func IsUserFree(ctx context.Context) bool {
	subscription := ctx.Value(models.SubscriptionContext{}).(models.MongoSubscriptionName)
	return subscription == models.FreeSubscriptionName
}

func IsUserFreePlus(ctx context.Context) bool {
	subscription := ctx.Value(models.SubscriptionContext{}).(models.MongoSubscriptionName)
	return subscription == models.FreePlusSubscriptionName
}

func IsUserBasic(ctx context.Context) bool {
	subscription := ctx.Value(models.SubscriptionContext{}).(models.MongoSubscriptionName)
	return subscription == models.BasicSubscriptionName
}
