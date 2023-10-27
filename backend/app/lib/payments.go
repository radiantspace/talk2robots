package lib

import "talk2robots/m/v2/app/models"

const (
	FreeSubscriptionName     models.MongoSubscriptionName = "free"
	FreePlusSubscriptionName models.MongoSubscriptionName = "free+"
	BasicSubscriptionName    models.MongoSubscriptionName = "basic"
)

var Subscriptions = map[models.MongoSubscriptionName]models.MongoSubscription{
	FreeSubscriptionName: {
		Name:         FreeSubscriptionName,
		MaximumUsage: 0.01,
	},
	FreePlusSubscriptionName: {
		Name:         FreePlusSubscriptionName,
		MaximumUsage: 0.05,
	},
	BasicSubscriptionName: {
		Name:         BasicSubscriptionName,
		MaximumUsage: 9.99,
	},
}

func UserTotalCostKey(user string) string {
	return user + ":total_cost"
}

func UserTotalTokensKey(user string) string {
	return user + ":total_tokens"
}

func UserTotalAudioMinutesKey(user string) string {
	return user + ":total_audio_minutes"
}
