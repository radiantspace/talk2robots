package payments

import (
	"context"
	"encoding/json"
	"fmt"
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
	models.FreeSubscriptionName: {
		Thresholds: []models.UsageThreshold{
			{
				Percentage: 0.5,
				Message:    "⚠️ Thanks for using the bot! You are halfway through your free monthly usage. Please consider an /upgrade to a paid plan. Use /status to see your current usage.",
			},
			{
				Percentage: 0.8,
				Message:    "⚠️ You are 80% through your free monthly usage. Please consider an /upgrade to a paid plan. Use /status to see your current usage.",
			},
			{
				Percentage: 1.0,
				Message:    "🚫 You have reached your free monthly usage limit. Further requests may not be served until the next month. If you find this bot useful, please consider an /upgrade to a paid plan.",
			},
		},
	},
	models.FreePlusSubscriptionName: {
		Thresholds: []models.UsageThreshold{
			{
				Percentage: 0.5,
				Message:    "⚠️ Thanks for using the bot! You are halfway through your free monthly usage. Please consider an /upgrade to a paid plan. Use /status to see your current usage.",
			},
			{
				Percentage: 0.8,
				Message:    "⚠️ You are 80% through your free monthly usage. Please consider an /upgrade to a paid plan. Use /status to see your current usage.",
			},
			{
				Percentage: 1.0,
				Message:    "🚫 You have reached your free monthly usage limit. Further requests may not be served until the next month. If you find this bot useful, please consider an /upgrade to a paid plan.",
			},
		},
	},
	models.BasicSubscriptionName: {
		Thresholds: []models.UsageThreshold{
			{
				Percentage: 0.5,
				Message:    "⚠️ Thanks for using the bot! You are halfway through your paid monthly usage. Use /status to track your current usage.",
			},
			{
				Percentage: 0.8,
				Message:    "⚠️ You are 80% through your paid monthly usage. Use /status to track your current usage.",
			},
			{
				Percentage: 1.0,
				Message:    "🚫 You have reached your paid monthly usage limit. Further requests may not be served until the next month. Use /status to track your current usage.",
			},
		},
	},
}

func HugePromptAlarm(ctx context.Context, usage models.CostAndUsage) {
	userId := ctx.Value(models.UserContext{}).(string)
	userIdInt, _ := strconv.ParseInt(userId, 10, 64)
	chatID := telego.ChatID{
		ID: userIdInt,
	}

	MAX_TOKENS_ALARM := 10 * 1024
	if usage.Usage.PromptTokens > MAX_TOKENS_ALARM {
		log.Warnf("Prompt tokens for chat %s exceeded max tokens alarm: %d", userId, usage.Usage.PromptTokens)
		PaymentsBot.SendMessage(tu.Message(chatID, fmt.Sprintf("⚠️ Your prompt (including previous conversation) is very long. This may lead to increased costs and the bot timeouts.\nConsider /clear the memory to start a new thread and/or use shorter messages.\n\nRequest tokens - %d.\nProjected cost of the request - $%.3f", usage.Usage.PromptTokens, usage.PricePerInputUnit*float64(usage.Usage.PromptTokens))))
	}
}

func Bill(originalContext context.Context, usage models.CostAndUsage) models.CostAndUsage {
	ctx := context.WithValue(context.Background(), models.UserContext{}, originalContext.Value(models.UserContext{}).(string))
	ctx = context.WithValue(ctx, models.SubscriptionContext{}, originalContext.Value(models.SubscriptionContext{}).(models.MongoSubscriptionName))
	ctx = context.WithValue(ctx, models.ClientContext{}, originalContext.Value(models.ClientContext{}).(string))
	ctx = context.WithValue(ctx, models.ChannelContext{}, originalContext.Value(models.ChannelContext{}).(string))
	ctx = context.WithValue(ctx, models.ParamsContext{}, originalContext.Value(models.ParamsContext{}))
	defer ctx.Done()
	usage.Cost =
		float64(usage.Usage.PromptTokens)*usage.PricePerInputUnit +
			float64(usage.Usage.CompletionTokens)*usage.PricePerOutputUnit +
			usage.Usage.AudioDuration*usage.PricePerInputUnit
	_, err := redis.RedisClient.IncrByFloat(ctx, "system_totals:cost", usage.Cost).Result()
	if err != nil {
		log.Errorf("[billing] error incrementing system cost: %v", err)
	}

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
	log.Infof("[billing] bill: %+v", billJson)

	if usage.Usage.TotalTokens > 0 {
		_, err = redis.RedisClient.IncrBy(context.Background(), lib.UserTotalTokensKey(usage.User), int64(usage.Usage.TotalTokens)).Result()
		if err != nil {
			log.Errorf("[billing] error incrementing user total tokens: %v", err)
		}
		_, err = redis.RedisClient.IncrBy(context.Background(), "system_totals:tokens", int64(usage.Usage.TotalTokens)).Result()
		if err != nil {
			log.Errorf("[billing] error incrementing system total tokens: %v", err)
		}
		config.CONFIG.DataDogClient.Distribution("billing.tokens", float64(usage.Usage.TotalTokens), []string{"engine:" + string(usage.Engine), "user_type:" + userType}, 1)
	}

	if usage.Usage.AudioDuration > 0 {
		_, err = redis.RedisClient.IncrByFloat(context.Background(), lib.UserTotalAudioMinutesKey(usage.User), usage.Usage.AudioDuration).Result()
		if err != nil {
			log.Errorf("[billing] error incrementing user total audio minutes: %v", err)
		}
		_, err = redis.RedisClient.IncrByFloat(context.Background(), "system_totals:audio_minutes", usage.Usage.AudioDuration).Result()
		if err != nil {
			log.Errorf("[billing] error incrementing system total audio minutes: %v", err)
		}
		config.CONFIG.DataDogClient.Distribution("billing.audio_minutes", usage.Usage.AudioDuration, []string{"engine:" + string(usage.Engine), "user_type:" + userType}, 1)
	}

	userTotalCost, err := redis.RedisClient.IncrByFloat(context.Background(), lib.UserTotalCostKey(usage.User), usage.Cost).Result()
	if err != nil {
		log.Errorf("[billing] error getting user total cost: %s", err)
	} else {
		err = mongo.MongoDBClient.UpdateUserUsage(ctx, userTotalCost)
		if err != nil {
			log.Errorf("[billing] error updating user usage: %s", err)
		}
	}
	return usage
}

func CheckThresholdsAndNotify(ctx context.Context, incomingCost float64) {
	user := ctx.Value(models.UserContext{}).(string)
	client := ctx.Value(models.ClientContext{}).(string)
	currentCost, err := redis.RedisClient.Get(context.Background(), lib.UserTotalCostKey(user)).Float64()
	if err != nil && err.Error() != "redis: nil" {
		log.Errorf("CheckThresholdsAndNotify: error getting user %s total cost: %s", user, err)
	}
	mongoUser, err := mongo.MongoDBClient.GetUser(ctx)
	if err != nil {
		log.Errorf("CheckThresholdsAndNotify: error getting user %s from mongo: %s", user, err)
		return
	}
	// check if subscription name is in map
	_, ok := models.Subscriptions[mongoUser.SubscriptionType.Name]
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
			log.Infof("CheckThresholdsAndNotify: user %s has reached %.1f%% of their maximum usage for subscription %s. Sending notification..", user, threshold.Percentage*100, mongoUser.SubscriptionType.Name)

			notification := lib.AddBotSuffixToGroupCommands(ctx, threshold.Message)
			SendNotification(ctx, notification)
		}
	}
}

var SendNotification = func(ctx context.Context, message string) {
	userString := strings.TrimPrefix(ctx.Value(models.UserContext{}).(string), "slack:")
	userId, _ := strconv.ParseInt(userString, 10, 64)
	client := ctx.Value(models.ClientContext{}).(string)

	if client == string(lib.TelegramClientName) {
		_, err := PaymentsBot.SendMessage(tu.Message(tu.ID(userId), message))
		if err != nil {
			log.Errorf("[billing] error sending telegram message: %v", err)
		}
	}

	if client == string(lib.SlackClientName) {
		channel := ctx.Value(models.ChannelContext{}).(string)
		_, _, _, err := PaymentsSlackClient.SendMessage(channel, slack.MsgOptionText(message, false), slack.MsgOptionPostEphemeral(userString))
		if err != nil {
			log.Errorf("[billing] error sending slack message: %v", err)
		}
	}
}
