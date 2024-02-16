package redis

import (
	"context"
	"talk2robots/m/v2/app/models"
	"time"

	log "github.com/sirupsen/logrus"
)

func SaveEngine(chatID string, engine models.Engine) {
	log.Info("Setting engine to ", string(engine), " for chat ", chatID)
	RedisClient.Set(context.Background(), chatID+":engine", string(engine), time.Hour*24*30)
}

func GetChatEngine(chatID string) models.Engine {
	engine, err := RedisClient.Get(context.Background(), chatID+":engine").Result()
	if err != nil {
		log.Info("No engine set for chat ", chatID, ", setting to default")
		SaveEngine(chatID, models.ChatGpt35Turbo)
		return models.ChatGpt35Turbo
	}

	// use newer, cheeper and faster model, instead of old Gpt4
	if models.Engine(engine) == models.ChatGpt4 {
		SaveEngine(chatID, models.ChatGpt4TurboVision)
		return models.ChatGpt4TurboVision
	}

	// use proper gpt3.5 turbo model instead of gpt3.5 1106
	if models.Engine(engine) == models.ChatGpt35Turbo1106 {
		SaveEngine(chatID, models.ChatGpt35Turbo)
		return models.ChatGpt35Turbo
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
