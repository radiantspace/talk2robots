package models

type UsageThresholds struct {
	Thresholds []UsageThreshold
}

type UsageThreshold struct {
	Percentage float64
	Message    string
}

const (
	FreeSubscriptionName     MongoSubscriptionName = "free"
	FreePlusSubscriptionName MongoSubscriptionName = "free+"
	BasicSubscriptionName    MongoSubscriptionName = "basic"
)

var Subscriptions = map[MongoSubscriptionName]MongoSubscription{
	FreeSubscriptionName: {
		Name:         FreeSubscriptionName,
		MaximumUsage: 0.10,
	},
	FreePlusSubscriptionName: {
		Name:         FreePlusSubscriptionName,
		MaximumUsage: 0.25,
	},
	BasicSubscriptionName: {
		Name:         BasicSubscriptionName,
		MaximumUsage: 9.99,
	},
}
