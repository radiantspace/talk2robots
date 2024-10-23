package redis

import (
	"context"
	"talk2robots/m/v2/app/models"

	log "github.com/sirupsen/logrus"
)

func SaveModel(chatID string, engine models.Engine) {
	log.Info("Setting engine to ", string(engine), " for chat ", chatID)
	RedisClient.Set(context.Background(), chatID+":engine", string(engine), 0)
}

func GetModel(chatID string) models.Engine {
	engine, err := RedisClient.Get(context.Background(), chatID+":engine").Result()
	if err != nil {
		log.Info("No engine set for chat ", chatID, ", setting to default")
		go SaveModel(chatID, models.ChatGpt4oMini)
		return models.ChatGpt4oMini
	}

	// use proper gpt4o-mini model instead of gpt3.5
	if models.Engine(engine) == models.ChatGpt35Turbo {
		go SaveModel(chatID, models.ChatGpt4oMini)
		return models.ChatGpt4oMini
	}

	// use proper gpt4o-mini model instead of gpt3.5 1106
	if models.Engine(engine) == models.ChatGpt35Turbo1106 {
		go SaveModel(chatID, models.ChatGpt4oMini)
		return models.ChatGpt4oMini
	}

	// use new gpt-4o instead of gpt-4-turbo, gpt-4-turbo-vision or gpt-4
	if models.Engine(engine) == models.ChatGpt4Turbo || models.Engine(engine) == models.ChatGpt4TurboVision || models.Engine(engine) == models.ChatGpt4 {
		go SaveModel(chatID, models.ChatGpt4o)
		return models.ChatGpt4o
	}

	// use updated sonet35 model
	if models.Engine(engine) == models.Sonet35 {
		go SaveModel(chatID, models.Sonet35_241022)
		return models.Sonet35_241022
	}

	return models.Engine(engine)
}

func SaveImageModel(chatID string, engine models.Engine) {
	log.Info("Setting image engine to ", string(engine), " for chat ", chatID)
	RedisClient.Set(context.Background(), chatID+":image_engine", string(engine), 0)
}

func GetImageModel(chatID string) models.Engine {
	engine, err := RedisClient.Get(context.Background(), chatID+":image_engine").Result()
	if err != nil {
		log.Info("No image engine set for chat ", chatID, ", setting to default")
		go SaveImageModel(chatID, models.DallE3)
		return models.DallE3
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
