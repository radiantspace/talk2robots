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
	userIdString := ctx.Value(models.UserContext{}).(string)
	topicIdString := ctx.Value(models.TopicContext{}).(string)
	mode, _ := lib.GetMode(userIdString, topicIdString)
	chatGptActive := ""
	voiceGptActive := ""
	grammarActive := ""
	teacherActive := ""
	transcribeActive := ""
	summarizeActive := ""
	translateActive := ""
	switch mode {
	case lib.ChatGPT:
		chatGptActive = " ✅"
	case lib.VoiceGPT:
		voiceGptActive = " ✅"
	case lib.Grammar:
		grammarActive = " ✅"
	case lib.Teacher:
		teacherActive = " ✅"
	case lib.Transcribe:
		transcribeActive = " ✅"
	case lib.Summarize:
		summarizeActive = " ✅"
	case lib.Translate:
		translateActive = " ✅"
	}

	topicString := ctx.Value(models.TopicContext{}).(string)
	return &telego.InlineKeyboardMarkup{
		InlineKeyboard: [][]telego.InlineKeyboardButton{
			{
				{
					Text:         "ChatGPT" + chatGptActive,
					CallbackData: string(lib.ChatGPT) + ":" + topicString,
				},
				{
					Text:         "VoiceGPT" + voiceGptActive,
					CallbackData: string(lib.VoiceGPT) + ":" + topicString,
				},
			},
			{
				{
					Text:         "Grammar" + grammarActive,
					CallbackData: string(lib.Grammar) + ":" + topicString,
				},
				{
					Text:         "Teacher" + teacherActive,
					CallbackData: string(lib.Teacher) + ":" + topicString,
				},
			},
			{
				{
					Text:         "Transcribe" + transcribeActive,
					CallbackData: string(lib.Transcribe) + ":" + topicString,
				},
				{
					Text:         "Summarize" + summarizeActive,
					CallbackData: string(lib.Summarize) + ":" + topicString,
				},
			},
			{
				{
					Text:         "Translate" + translateActive,
					CallbackData: string(lib.Translate) + ":" + topicString,
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
	userIdString := ctx.Value(models.UserContext{}).(string)
	topicString := ctx.Value(models.TopicContext{}).(string)
	model := redis.GetModel(userIdString)
	gpt4oActive := ""
	gpt4oMiniActive := ""
	sonet35Active := ""
	haiku3Active := ""
	bigLlama3Active := ""
	smallLlama3Active := ""
	grokActive := ""
	switch model {
	case models.ChatGpt4o:
		gpt4oActive = "✅ "
	case models.ChatGpt4oMini:
		gpt4oMiniActive = "✅ "
	case models.Sonet35_241022:
		sonet35Active = "✅ "
	case models.Haiku3:
		haiku3Active = "✅ "
	case models.LlamaV3_70b:
		bigLlama3Active = "✅ "
	case models.LlamaV3_8b:
		smallLlama3Active = "✅ "
	case models.Grok:
		grokActive = "✅ "
	}

	return &telego.InlineKeyboardMarkup{
		InlineKeyboard: [][]telego.InlineKeyboardButton{
			{
				{
					Text:         grokActive + "Grok 💰💰🏃🏃🧠🧠🧠",
					CallbackData: string(models.Grok) + ":" + topicString,
				},
			},
			{
				{
					Text:         gpt4oActive + "GPT 4o 💰💰💰🏃🏃🧠🧠🧠🧠",
					CallbackData: string(models.ChatGpt4o) + ":" + topicString,
				},
			},
			{
				{
					Text:         gpt4oMiniActive + "GPT 4o mini 💰🏃🏃🏃🏃🧠🧠",
					CallbackData: string(models.ChatGpt4oMini) + ":" + topicString,
				},
			},
			{
				{
					Text:         sonet35Active + "Claude Sonet 3.5 💰💰💰🏃🏃🧠🧠🧠🧠",
					CallbackData: string(models.Sonet35_241022) + ":" + topicString,
				},
			},
			{
				{
					Text:         haiku3Active + "Claude Haiku 3 💰💰🏃🏃🏃🏃🧠🧠",
					CallbackData: string(models.Haiku3) + ":" + topicString,
				},
			},
			{
				{
					Text:         bigLlama3Active + "Big Llama3 💰💰💰🏃🏃🧠🧠🧠",
					CallbackData: string(models.LlamaV3_70b) + ":" + topicString,
				},
			},
			{
				{
					Text:         smallLlama3Active + "Small Llama3 💰🏃🏃🏃🏃🧠",
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
