package payments

import (
	"context"
	"log"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/db/mongo"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/models"
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func init() {
	testClient, err := statsd.New("127.0.0.1:8125", statsd.WithNamespace("tests."))
	if err != nil {
		log.Fatalf("error creating test DataDog client: %v", err)
	}
	config.CONFIG = &config.Config{
		DataDogClient: testClient,
	}

	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})
	logrus.SetLevel(logrus.DebugLevel)
}

func TestBill(t *testing.T) {
	redis.RedisClient = redis.NewMockRedisClient()

	mongo.MongoDBClient = mongo.NewMockMongoDBClient(
		models.MongoUser{
			ID:    "123",
			Usage: 0.1,
		},
	)
	ctx := context.WithValue(context.Background(), models.UserContext{}, "123")
	ctx = context.WithValue(ctx, models.ClientContext{}, "telegram")
	ctx = context.WithValue(ctx, models.ChannelContext{}, "123")
	ctx = context.WithValue(ctx, models.SubscriptionContext{}, models.FreeSubscriptionName)

	usage := models.CostAndUsage{
		Usage: models.Usage{
			PromptTokens:     550,
			CompletionTokens: 450,
			TotalTokens:      1000,
			AudioDuration:    10,
		},
		PricePerInputUnit:  0.001,
		PricePerOutputUnit: 0.002,
	}

	result := Bill(ctx, usage)
	expectedCost := float64(usage.Usage.PromptTokens)*usage.PricePerInputUnit + float64(usage.Usage.CompletionTokens)*usage.PricePerOutputUnit + usage.Usage.AudioDuration*usage.PricePerInputUnit
	assert.Equal(t, expectedCost, result.Cost, "Incorrect cost calculation")
}

func TestCheckThresholdsAndNotify(t *testing.T) {
	// Set up the mock Redis client
	redis.RedisClient = redis.NewMockRedisClient()

	// Set up the mock MongoDB client
	mongo.MongoDBClient = mongo.NewMockMongoDBClient(
		models.MongoUser{
			ID:    "123",
			Usage: 0.1,
			SubscriptionType: models.MongoSubscription{
				Name:         models.FreeSubscriptionName,
				MaximumUsage: 0.1,
			},
		},
	)

	// Set up a custom SendNotification function to track notifications
	var notifications []string
	sendNotificationOriginal := SendNotification
	SendNotification = func(ctx context.Context, message string) {
		notifications = append(notifications, message)
	}
	defer func() { SendNotification = sendNotificationOriginal }() // Restore the original function after the test

	// Set up the test context
	ctx := context.WithValue(context.Background(), models.UserContext{}, "123")
	ctx = context.WithValue(ctx, models.ClientContext{}, "telegram")
	redis.RedisClient.IncrByFloat(ctx, lib.UserTotalCostKey("123"), 0.01)

	// Test CheckThresholdsAndNotify
	CheckThresholdsAndNotify(ctx, 0.05)

	// Verify that the expected notification was sent
	var expectedNotifications []string
	assert.Equal(t, expectedNotifications, notifications, "Unexpected notifications sent")
}

func TestCheckThresholdsAndNotifyMaximum(t *testing.T) {
	// Set up the mock Redis client
	redis.RedisClient = redis.NewMockRedisClient()

	// Set up the mock MongoDB client
	mongo.MongoDBClient = mongo.NewMockMongoDBClient(
		models.MongoUser{
			ID:    "123",
			Usage: 0.1,
			SubscriptionType: models.MongoSubscription{
				Name:         models.FreeSubscriptionName,
				MaximumUsage: 0.1,
			},
		},
	)

	// Set up a custom SendNotification function to track notifications
	var notifications []string
	sendNotificationOriginal := SendNotification
	SendNotification = func(ctx context.Context, message string) {
		notifications = append(notifications, message)
	}
	defer func() { SendNotification = sendNotificationOriginal }() // Restore the original function after the test

	// Set up the test context
	ctx := context.WithValue(context.Background(), models.UserContext{}, "123")
	ctx = context.WithValue(ctx, models.ClientContext{}, "telegram")
	redis.RedisClient.IncrByFloat(ctx, lib.UserTotalCostKey("123"), 0.01)

	// Test CheckThresholdsAndNotify
	CheckThresholdsAndNotify(ctx, 0.1)

	// Verify that the expected notification was sent
	expectedNotifications := []string{"Check available options to /upgrade and continue using me."}
	assert.Equal(t, expectedNotifications, notifications, "Unexpected notifications sent")
}

func TestCheckThresholdsAndNotifyGroup(t *testing.T) {
	originalBotName := config.CONFIG.BotName
	config.CONFIG.BotName = "testbot"
	defer func() { config.CONFIG.BotName = originalBotName }()

	// Set up the mock Redis client
	redis.RedisClient = redis.NewMockRedisClient()

	// Set up the mock MongoDB client
	mongo.MongoDBClient = mongo.NewMockMongoDBClient(
		models.MongoUser{
			ID:    "-123",
			Usage: 0.1,
			SubscriptionType: models.MongoSubscription{
				Name:         models.FreeSubscriptionName,
				MaximumUsage: 0.1,
			},
		},
	)

	// Set up a custom SendNotification function to track notifications
	var notifications []string
	sendNotificationOriginal := SendNotification
	SendNotification = func(ctx context.Context, message string) {
		notifications = append(notifications, message)
	}
	defer func() { SendNotification = sendNotificationOriginal }() // Restore the original function after the test

	// Set up the test context
	ctx := context.WithValue(context.Background(), models.UserContext{}, "-123")
	ctx = context.WithValue(ctx, models.ClientContext{}, "telegram")
	redis.RedisClient.IncrByFloat(ctx, lib.UserTotalCostKey("-123"), 0.01)

	// Test CheckThresholdsAndNotify
	CheckThresholdsAndNotify(ctx, 0.10)

	// Verify that the expected notification was sent
	expectedNotifications := []string{"Check available options to /upgrade@testbot and continue using me."}
	assert.Equal(t, expectedNotifications, notifications, "Unexpected notifications sent")
}
