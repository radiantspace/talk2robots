package payments

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/db/mongo"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/models"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/slack-go/slack"

	log "github.com/sirupsen/logrus"
)

var (
	PaymentsBot         *telego.Bot
	PaymentsSlackClient *slack.Client
)

// At 50%, 80% and 100% of the maximum usage, the user will receive a notification
// that they are approaching their maximum usage.
var UsageThresholds = map[models.MongoSubscriptionName]models.UsageThresholds{
	lib.FreeSubscriptionName: {
		Thresholds: []models.UsageThreshold{
			{
				Percentage: 0.5,
				Message:    "⚠️ Thanks for using the bot! You are halfway through your free monthly usage. Please consider an /upgrade to a paid plan. Use /status to see your current usage.",
			},
			{
				Percentage: 0.8,
				Message:    "⚠️ You are 80%% through your free monthly usage. Please consider an /upgrade to a paid plan. Use /status to see your current usage.",
			},
			{
				Percentage: 1.0,
				Message:    "🚫 You have reached your free monthly usage limit. Further requests may not be served until the next month. If you find this bot useful, please consider an /upgrade to a paid plan.",
			},
		},
	},
	lib.FreePlusSubscriptionName: {
		Thresholds: []models.UsageThreshold{
			{
				Percentage: 0.5,
				Message:    "⚠️ Thanks for using the bot! You are halfway through your free monthly usage. Please consider an /upgrade to a paid plan. Use /status to see your current usage.",
			},
			{
				Percentage: 0.8,
				Message:    "⚠️ You are 80%% through your free monthly usage. Please consider an /upgrade to a paid plan. Use /status to see your current usage.",
			},
			{
				Percentage: 1.0,
				Message:    "🚫 You have reached your free monthly usage limit. Further requests may not be served until the next month. If you find this bot useful, please consider an /upgrade to a paid plan.",
			},
		},
	},
	lib.BasicSubscriptionName: {
		Thresholds: []models.UsageThreshold{
			{
				Percentage: 0.5,
				Message:    "⚠️ Thanks for using the bot! You are halfway through your paid monthly usage. Use /status to track your current usage.",
			},
			{
				Percentage: 0.8,
				Message:    "⚠️ You are 80%% through your paid monthly usage. Use /status to track your current usage.",
			},
			{
				Percentage: 1.0,
				Message:    "🚫 You have reached your paid monthly usage limit. Further requests may not be served until the next month. Use /status to track your current usage.",
			},
		},
	},
}

func Bill(ctx context.Context, usage models.CostAndUsage) models.CostAndUsage {
	defer ctx.Done()
	usage.Cost = float64(usage.Usage.TotalTokens)*usage.PricePerUnit + usage.Usage.AudioDuration*usage.PricePerUnit
	redis.RedisClient.IncrByFloat(ctx, "system_totals:cost", usage.Cost)

	usage.User = ctx.Value(models.UserContext{}).(string)
	client := ctx.Value(models.ClientContext{}).(string)

	userType := "system"
	if usage.User != "SYSTEM:STATUS" {
		userType = "user"
		CheckThresholdsAndNotify(ctx, usage.Cost)
	}

	config.CONFIG.DataDogClient.Distribution("billing.cost", usage.Cost, []string{"engine:" + string(usage.Engine), "user_type:" + userType, "client:" + client}, 1)
	billBytes, _ := json.Marshal(usage)
	billJson := string(billBytes)
	log.Infof("Billing: %+v", billJson)

	if usage.Usage.TotalTokens > 0 {
		redis.RedisClient.IncrBy(ctx, lib.UserTotalTokensKey(usage.User), int64(usage.Usage.TotalTokens))
		redis.RedisClient.IncrBy(ctx, "system_totals:tokens", int64(usage.Usage.TotalTokens))
		config.CONFIG.DataDogClient.Distribution("billing.tokens", float64(usage.Usage.TotalTokens), []string{"engine:" + string(usage.Engine), "user_type:" + userType}, 1)
	}

	if usage.Usage.AudioDuration > 0 {
		redis.RedisClient.IncrByFloat(ctx, lib.UserTotalAudioMinutesKey(usage.User), usage.Usage.AudioDuration)
		redis.RedisClient.IncrByFloat(ctx, "system_totals:audio_minutes", usage.Usage.AudioDuration)
		config.CONFIG.DataDogClient.Distribution("billing.audio_minutes", usage.Usage.AudioDuration, []string{"engine:" + string(usage.Engine), "user_type:" + userType}, 1)
	}

	redis.RedisClient.IncrByFloat(ctx, lib.UserTotalCostKey(usage.User), usage.Cost)
	userTotalCost, err := redis.RedisClient.Get(ctx, lib.UserTotalCostKey(usage.User)).Float64()
	if err != nil {
		log.Errorf("Error getting user total cost: %s", err)
	} else {
		mongo.MongoDBClient.UpdateUserUsage(ctx, userTotalCost)
	}
	return usage
}

func CheckThresholdsAndNotify(ctx context.Context, incomingCost float64) {
	user := ctx.Value(models.UserContext{}).(string)
	client := ctx.Value(models.ClientContext{}).(string)
	currentCost, err := redis.RedisClient.Get(ctx, lib.UserTotalCostKey(user)).Float64()
	if err != nil {
		log.Errorf("CheckThresholdsAndNotify: error getting user total cost: %s", err)
		return
	}
	mongoUser, err := mongo.MongoDBClient.GetUser(ctx)
	if err != nil {
		log.Errorf("CheckThresholdsAndNotify: error getting user from mongo: %s", err)
		return
	}
	// check if subscription name is in map
	_, ok := lib.Subscriptions[mongoUser.SubscriptionType.Name]
	if !ok {
		log.Errorf("CheckThresholdsAndNotify: subscription name %s not found in map", mongoUser.SubscriptionType.Name)
		return
	}

	thresholds, ok := UsageThresholds[mongoUser.SubscriptionType.Name]
	if !ok {
		log.Errorf("CheckThresholdsAndNotify: usage thresholds for subscription name %s not found in map", mongoUser.SubscriptionType.Name)
		return
	}
	for _, threshold := range thresholds.Thresholds {
		thresholdCost := mongoUser.SubscriptionType.MaximumUsage * threshold.Percentage
		if currentCost < thresholdCost && currentCost+incomingCost >= thresholdCost {
			config.CONFIG.DataDogClient.Incr("billing.threshold_reached", []string{
				"subscription:" + string(mongoUser.SubscriptionType.Name),
				"threshold:" + strconv.FormatFloat(threshold.Percentage*100, 'f', 0, 64),
				"client:" + client,
			}, 1)
			log.Infof("User %s has reached %.1f%% of their maximum usage for subscription %s. Sending notification..", user, threshold.Percentage*100, mongoUser.SubscriptionType.Name)
			SendNotification(ctx, threshold.Message)
		}
	}
}

var SendNotification = func(ctx context.Context, message string) {
	userString := strings.TrimPrefix(ctx.Value(models.UserContext{}).(string), "slack:")
	userId, _ := strconv.ParseInt(userString, 10, 64)
	client := ctx.Value(models.ClientContext{}).(string)

	if client == string(lib.TelegramClientName) {
		PaymentsBot.SendMessage(tu.Message(tu.ID(userId), message))
	}

	if client == string(lib.SlackClientName) {
		channel := ctx.Value(models.ChannelContext{}).(string)
		PaymentsSlackClient.SendMessage(channel, slack.MsgOptionText(message, false), slack.MsgOptionPostEphemeral(userString))
	}
}
