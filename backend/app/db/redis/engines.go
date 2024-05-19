package redis

import (
	"context"
	"talk2robots/m/v2/app/models"

	log "github.com/sirupsen/logrus"
)

func SaveEngine(chatID string, engine models.Engine) {
	log.Info("Setting engine to ", string(engine), " for chat ", chatID)
	RedisClient.Set(context.Background(), chatID+":engine", string(engine), 0)
}

func GetChatEngine(chatID string) models.Engine {
	engine, err := RedisClient.Get(context.Background(), chatID+":engine").Result()
	if err != nil {
		log.Info("No engine set for chat ", chatID, ", setting to default")
		SaveEngine(chatID, models.ChatGpt35Turbo)
		return models.ChatGpt35Turbo
	}

	// use proper gpt3.5 turbo model instead of gpt3.5 1106
	if models.Engine(engine) == models.ChatGpt35Turbo1106 {
		SaveEngine(chatID, models.ChatGpt35Turbo)
		return models.ChatGpt35Turbo
	}

	// use new gpt-4o instead of gpt-4-turbo and gpt-4-turbo-vision
	if models.Engine(engine) == models.ChatGpt4Turbo || models.Engine(engine) == models.ChatGpt4TurboVision {
		SaveEngine(chatID, models.ChatGpt4o)
		return models.ChatGpt4o
	}

	return models.Engine(engine)
}

func IsUserBanned(chatID string) bool {
	banned, err := RedisClient.Get(context.Background(), chatID+":banned").Result()
	if err != nil {
		return false
	}
	return banned == "true"
}
