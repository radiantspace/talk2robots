package telegram

import (
	"context"
	"fmt"
	"strings"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/models"

	"github.com/mymmrac/telego"
	log "github.com/sirupsen/logrus"
)

func GetUserStatus(ctx context.Context) string {
	userIdString := ctx.Value(models.UserContext{}).(string)
	topicIdString := ctx.Value(models.TopicContext{}).(string)
	mode, _ := lib.GetMode(userIdString, topicIdString)
	subscriptionName := ctx.Value(models.SubscriptionContext{}).(models.MongoSubscriptionName)
	subscription := models.Subscriptions[subscriptionName]
	usage := GetUserUsage(userIdString)
	tokens := GetUserTokens(userIdString)
	audioMinutes := GetUserAudioMinutes(userIdString)
	usagePercent := usage / subscription.MaximumUsage * 100

	subscriptionToDisplay := string(subscription.Name)
	if subscription.Name == models.BasicSubscriptionName {
		subscriptionToDisplay = string(subscription.Name) + " ($9.99/mo)"
	}

	entity := "User"
	if strings.HasPrefix(userIdString, "-") {
		entity = "Group"
	}

	return fmt.Sprintf(`⚙️ %s status:
		Mode: %s
		Subscription: %s
		Maximum AI usage: $%.2f/mo
		Monthly consumption: %.1f%%
		Monthly tokens processed: %d
		Monthly audio transcribed, minutes: %.2f`, entity, mode, subscriptionToDisplay, subscription.MaximumUsage, usagePercent, tokens, audioMinutes)
}

func GetStatusKeyboard(ctx context.Context) *telego.InlineKeyboardMarkup {
	// userIdString := ctx.Value(models.UserContext{}).(string)
	// mode := GetMode(userIdString)
	topicString := ctx.Value(models.TopicContext{}).(string)
	return &telego.InlineKeyboardMarkup{
		InlineKeyboard: [][]telego.InlineKeyboardButton{
			{
				{
					Text:         "ChatGPT",
					CallbackData: string(lib.ChatGPT) + ":" + topicString,
				},
				{
					Text:         "VoiceGPT",
					CallbackData: string(lib.VoiceGPT) + ":" + topicString,
				},
			},
			{
				{
					Text:         "Grammar",
					CallbackData: string(lib.Grammar) + ":" + topicString,
				},
				{
					Text:         "Teacher",
					CallbackData: string(lib.Teacher) + ":" + topicString,
				},
			},
			{
				{
					Text:         "Transcribe",
					CallbackData: string(lib.Transcribe) + ":" + topicString,
				},
				{
					Text:         "Summarize",
					CallbackData: string(lib.Summarize) + ":" + topicString,
				},
			},
			{
				{
					Text:         "Choose AI 🧠",
					CallbackData: "models:" + topicString,
				},
			},
		},
	}
}

func GetModelsKeyboard(ctx context.Context) *telego.InlineKeyboardMarkup {
	topicString := ctx.Value(models.TopicContext{}).(string)
	return &telego.InlineKeyboardMarkup{
		InlineKeyboard: [][]telego.InlineKeyboardButton{
			{
				{
					Text:         "GPT 3.5 💰💰🚀🚀🚀🧠",
					CallbackData: string(models.ChatGpt35Turbo) + ":" + topicString,
				},
			},
			{
				{
					Text:         "GPT 4 💰💰💰💰🚀🧠🧠🧠🧠",
					CallbackData: string(models.ChatGpt4) + ":" + topicString,
				},
			},
			{
				{
					Text:         "Small Llama3 💰🚀🚀🚀🚀🧠",
					CallbackData: string(models.LlamaV3_8b) + ":" + topicString,
				},
			},
			{
				{
					Text:         "Big Llama3 💰💰💰🚀🚀🧠🧠🧠",
					CallbackData: string(models.LlamaV3_70b) + ":" + topicString,
				},
			},
			{
				{
					Text:         "Back ⬅️",
					CallbackData: "status:" + topicString,
				},
			},
		},
	}
}

func GetUserUsage(userId string) float64 {
	tokens, err := redis.RedisClient.Get(context.Background(), lib.UserTotalCostKey(userId)).Float64()
	if err != nil {
		if err.Error() == "redis: nil" {
			return 0
		}
		log.Errorf("GetUserUsage: failed to get user %s usage: %v", userId, err)
		return 0
	}
	return tokens
}

func GetUserTokens(userId string) int64 {
	tokens, err := redis.RedisClient.Get(context.Background(), lib.UserTotalTokensKey(userId)).Int64()
	if err != nil {
		if err.Error() == "redis: nil" {
			return 0
		}
		log.Errorf("GetUserTokens: failed to get user %s tokens: %v", userId, err)
		return 0
	}
	return tokens
}

func GetUserAudioMinutes(userId string) float64 {
	minutes, err := redis.RedisClient.Get(context.Background(), lib.UserTotalAudioMinutesKey(userId)).Float64()
	if err != nil {
		if err.Error() == "redis: nil" {
			return 0
		}
		log.Errorf("GetUserAudioMinutes: failed to get user %s audio minutes: %v", userId, err)
		return 0
	}
	return minutes
}
