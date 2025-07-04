// Run regularly to check status of the system and persist it to the redis
package status

import (
	"context"
	"encoding/json"

	log "github.com/sirupsen/logrus"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/db/mongo"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/status"
	"talk2robots/m/v2/app/workers"
)

var WORKER *workers.Worker

func Run() {
	systemStatus, err := redis.WrapInCache(redis.RedisClient, "system-status", WORKER.Interval*10, FetchStatus)()
	if err != nil {
		log.Errorf("failed to fetch system status: %s", err)
		return
	}
	log.Debugf("system status: %s", systemStatus)
}

func FetchStatus() (string, error) {
	w := WORKER
	systemStatus := status.New(mongo.MongoDBClient, redis.RedisClient, w.AI).GetSystemStatus()
	config.CONFIG.DataDogClient.Gauge("status_worker.claude_ai_available", boolToFloat64(systemStatus.ClaudeAI.Available), nil, 1)
	config.CONFIG.DataDogClient.Gauge("status_worker.fireworks_ai_available", boolToFloat64(systemStatus.FireworksAI.Available), nil, 1)
	config.CONFIG.DataDogClient.Gauge("status_worker.mongo_db_available", boolToFloat64(systemStatus.MongoDB.Available), nil, 1)
	config.CONFIG.DataDogClient.Gauge("status_worker.open_ai_available", boolToFloat64(systemStatus.OpenAI.Available), nil, 1)
	config.CONFIG.DataDogClient.Gauge("status_worker.redis_available", boolToFloat64(systemStatus.Redis.Available), nil, 1)
	config.CONFIG.DataDogClient.Gauge("status_worker.audio_duration_minutes", systemStatus.Usage.AudioDurationMinutes, nil, 1)
	config.CONFIG.DataDogClient.Gauge("status_worker.total_cost", systemStatus.Usage.TotalCost, nil, 1)
	config.CONFIG.DataDogClient.Gauge("status_worker.total_tokens", float64(systemStatus.Usage.TotalTokens), nil, 1)
	config.CONFIG.DataDogClient.Gauge("status_worker.total_users", float64(systemStatus.Usage.TotalUsers), nil, 1)
	config.CONFIG.DataDogClient.Gauge("status_worker.total_free_plus_users", float64(systemStatus.Usage.TotalFreePlusUsers), nil, 1)
	config.CONFIG.DataDogClient.Gauge("status_worker.total_basic_users", float64(systemStatus.Usage.TotalBasicUsers), nil, 1)
	config.CONFIG.DataDogClient.Gauge("status_worker.total_images", float64(systemStatus.Usage.TotalImages), nil, 1)
	config.CONFIG.DataDogClient.Gauge("status_worker.day_active_users", float64(systemStatus.Usage.DayActiveUsers), nil, 1)
	config.CONFIG.DataDogClient.Gauge("status_worker.week_active_users", float64(systemStatus.Usage.WeekActiveUsers), nil, 1)
	config.CONFIG.DataDogClient.Gauge("status_worker.month_active_users", float64(systemStatus.Usage.MonthActiveUsers), nil, 1)
	if !systemStatus.MongoDB.Available {
		reportUnavailableStatus(w.TelegramSystemBot, w.SystemTelegramChatID, w.MainBotName, "MongoDB")
	}
	if !systemStatus.Redis.Available {
		reportUnavailableStatus(w.TelegramSystemBot, w.SystemTelegramChatID, w.MainBotName, "Redis")
	}
	if !systemStatus.OpenAI.Available {
		reportUnavailableStatus(w.TelegramSystemBot, w.SystemTelegramChatID, w.MainBotName, "OpenAI")
	}
	if !systemStatus.FireworksAI.Available {
		reportUnavailableStatus(w.TelegramSystemBot, w.SystemTelegramChatID, w.MainBotName, "FireworksAI")
	}
	if !systemStatus.ClaudeAI.Available {
		reportUnavailableStatus(w.TelegramSystemBot, w.SystemTelegramChatID, w.MainBotName, "ClaudeAI")
	}
	statusBytes, _ := json.Marshal(systemStatus)
	return string(statusBytes), nil
}

func reportUnavailableStatus(bot *telego.Bot, chatID telego.ChatID, mainBotName string, systemName string) {
	if bot == nil {
		log.Error("Telegram System bot is not initialized")
		return
	}
	message := "🔥 " + mainBotName + ": " + systemName + " is down 🔥"
	log.Error(message)
	_, err := bot.SendMessage(context.Background(), tu.Message(chatID, message))
	if err != nil {
		log.Errorf("Failed to send message to telegram: %s", err)
	}
}

func boolToFloat64(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
