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
	return models.Engine(engine)
}

func IsUserBanned(chatID string) bool {
	banned, err := RedisClient.Get(context.Background(), chatID+":banned").Result()
	if err != nil {
		return false
	}
	return banned == "true"
}
