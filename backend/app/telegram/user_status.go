package telegram

import (
	"context"
	"fmt"
	"strings"
	"talk2robots/m/v2/app/ai/fireworks"
	"talk2robots/m/v2/app/ai/midjourney"
	"talk2robots/m/v2/app/ai/openai"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/models"

	"github.com/mymmrac/telego"
	log "github.com/sirupsen/logrus"
)

func GetUserStatus(ctx context.Context) string {
	userIdString := ctx.Value(models.UserContext{}).(string)
	topicIdString := ctx.Value(models.TopicContext{}).(string)
	mode, params := lib.GetMode(userIdString, topicIdString)
	paramsString := ""
	if params != "" {
		paramsString = fmt.Sprintf(" (%s)", params)
	}
	model := redis.GetModel(userIdString)
	subscriptionName := ctx.Value(models.SubscriptionContext{}).(models.MongoSubscriptionName)
	subscription := models.Subscriptions[subscriptionName]
	usage := GetUserUsage(userIdString)
	tokens := GetUserTokens(userIdString)
	audioMinutes := GetUserAudioMinutes(userIdString)
	imagesCount := GetUserImages(userIdString)
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
		Mode: %s%s
		AI model: %s

		Subscription: %s
		AI credits: $%.2f/mo

		Monthly usage (will reset on the 1st of the next month)
		consumption: %.3f$ (%.1f%%)
		tokens processed: %d
		audio transcribed, minutes: %.2f
		images created: %d`, entity, mode, paramsString, model, subscriptionToDisplay, subscription.MaximumUsage, usage, usagePercent, tokens, audioMinutes, imagesCount)
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
			// {
			// 	{
			// 		Text:         "Choose Image AI 🎨",
			// 		CallbackData: "images:" + topicString,
			// 	},
			// },
		},
	}
}

func GetModelsKeyboard(ctx context.Context) *telego.InlineKeyboardMarkup {
	topicString := ctx.Value(models.TopicContext{}).(string)
	return &telego.InlineKeyboardMarkup{
		InlineKeyboard: [][]telego.InlineKeyboardButton{
			{
				{
					Text:         "GPT 4o (best) 💰💰💰🏃🏃🧠🧠🧠🧠",
					CallbackData: string(models.ChatGpt4o) + ":" + topicString,
				},
			},
			{
				{
					Text:         "GPT 4o mini 💰🏃🏃🏃🏃🧠🧠",
					CallbackData: string(models.ChatGpt4oMini) + ":" + topicString,
				},
			},
			{
				{
					Text:         "Claude Sonet 3.5 💰💰💰🏃🏃🧠🧠🧠🧠",
					CallbackData: string(models.Sonet35_241022) + ":" + topicString,
				},
			},
			{
				{
					Text:         "Claude Haiku 3 💰💰🏃🏃🏃🏃🧠🧠",
					CallbackData: string(models.Haiku3) + ":" + topicString,
				},
			},
			{
				{
					Text:         "Big Llama3 💰💰💰🏃🏃🧠🧠🧠",
					CallbackData: string(models.LlamaV3_70b) + ":" + topicString,
				},
			},
			{
				{
					Text:         "Small Llama3 💰🏃🏃🏃🏃🧠",
					CallbackData: string(models.LlamaV3_8b) + ":" + topicString,
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

func GetImageModelsKeyboard(ctx context.Context) *telego.InlineKeyboardMarkup {
	topicString := ctx.Value(models.TopicContext{}).(string)
	return &telego.InlineKeyboardMarkup{
		InlineKeyboard: [][]telego.InlineKeyboardButton{
			{
				{
					Text:         fmt.Sprintf("Dalle-3 (best) %.2f$/image\n🚀🚀🧠🧠🧠🧠🎨🎨🎨", openai.DALLE3_S),
					CallbackData: string(models.DallE3) + ":" + topicString,
				},
			},
			{
				{
					Text:         fmt.Sprintf("Midjourney 6 %.2f$/image\n🚀🧠🧠🎨🎨🎨🎨", midjourney.MIDJOURNEY6),
					CallbackData: string(models.Midjourney6) + ":" + topicString,
				},
			},
			{
				{
					Text:         fmt.Sprintf("Stable Diffusion 3 %.3f$/image\n🚀🚀🚀🧠🧠🎨🎨🎨", fireworks.STABLEDIFFUSION3),
					CallbackData: string(models.StableDiffusion3) + ":" + topicString,
				},
			},
			{
				{
					Text:         fmt.Sprintf("Playground 2.5 %.2f$/image\n🚀🚀🚀🚀🧠🎨", fireworks.PLAYGROUND25),
					CallbackData: string(models.Playground25) + ":" + topicString,
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

func GetUserImages(userId string) int64 {
	imagesCount, err := redis.RedisClient.Get(context.Background(), lib.UserTotalImagesKey(userId)).Int64()
	if err != nil {
		if err.Error() == "redis: nil" {
			return 0
		}
		log.Errorf("GetUserImages: failed to get user %s images count: %v", userId, err)
		return 0
	}
	return imagesCount
}
