package status

import (
	"context"
	"talk2robots/m/v2/app/ai"
	"talk2robots/m/v2/app/db/mongo"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/models"
	"time"

	"github.com/sirupsen/logrus"
)

type SystemStatus struct {
	MongoDB     *Status     `json:"mongodb"`
	Redis       *Status     `json:"redis"`
	OpenAI      *Status     `json:"openai"`
	FireworksAI *Status     `json:"fireworksai"`
	Time        time.Time   `json:"time"`
	Usage       SystemUsage `json:"usage"`
}

type SystemUsage struct {
	TotalUsers           int64   `json:"total_users"`
	TotalFreePlusUsers   int64   `json:"total_free_plus_users"`
	TotalBasicUsers      int64   `json:"total_basic_users"`
	TotalTokens          int64   `json:"total_tokens"`
	TotalCost            float64 `json:"total_cost"`
	TotalImages          int64   `json:"total_images"`
	AudioDurationMinutes float64 `json:"audio_duration_minutes"`
	DayActiveUsers       int64   `json:"day_active_users"`
	WeekActiveUsers      int64   `json:"week_active_users"`
	MonthActiveUsers     int64   `json:"month_active_users"`
}

// Status
type Status struct {
	Available bool `json:"available"`
}

// RedisStatus is a status of Redis
type RedisStatus struct {
	Connected bool      `json:"connected"`
	Time      time.Time `json:"time"`
}

// SystemStatusHandler is a handler for system status
type SystemStatusHandler struct {
	MongoDB mongo.MongoClient
	Redis   redis.Client
	AI      *ai.API
}

// New creates a new instance of SystemStatusHandler
func New(mongoDB mongo.MongoClient, redis redis.Client, ai *ai.API) *SystemStatusHandler {
	return &SystemStatusHandler{
		MongoDB: mongoDB,
		Redis:   redis,
		AI:      ai,
	}
}

// GetSystemStatus gets a status of the system
func (h *SystemStatusHandler) GetSystemStatus() SystemStatus {
	mongoAvailable := false
	ctxPing, cancelPing := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelPing()
	err := h.MongoDB.Ping(ctxPing, nil)
	if err != nil {
		logrus.WithError(err).Warn("GetSystemStatus: failed to ping MongoDB")
	} else {
		mongoAvailable = true
	}
	aiContext := context.WithValue(context.Background(), models.UserContext{}, "SYSTEM:STATUS")
	aiContext = context.WithValue(aiContext, models.ClientContext{}, "none")
	aiContext = context.WithValue(aiContext, models.SubscriptionContext{}, models.FreeSubscriptionName)
	aiContext = context.WithValue(aiContext, models.ChannelContext{}, "none")
	status := SystemStatus{
		MongoDB: &Status{
			Available: mongoAvailable,
		},
		Redis: &Status{
			Available: h.Redis != nil && h.Redis.Ping(context.Background()).Err() == nil,
		},
		OpenAI: &Status{
			Available: h.AI != nil && h.AI.IsAvailable(aiContext, models.ChatGpt35Turbo),
		},
		FireworksAI: &Status{
			Available: h.AI != nil && h.AI.IsAvailable(aiContext, models.LlamaV3_8b),
		},
		Usage: SystemUsage{},
		Time:  time.Now(),
	}
	if status.Redis.Available {
		tokens := h.Redis.Get(context.Background(), "system_totals:tokens")
		if tokens.Err() == nil {
			status.Usage.TotalTokens, _ = tokens.Int64()
		}
		cost := h.Redis.Get(context.Background(), "system_totals:cost")
		if cost.Err() == nil {
			status.Usage.TotalCost, _ = cost.Float64()
		}
		audioDurationMinutes := h.Redis.Get(context.Background(), "system_totals:audio_minutes")
		if audioDurationMinutes.Err() == nil {
			status.Usage.AudioDurationMinutes, _ = audioDurationMinutes.Float64()
		}
		images := h.Redis.Get(context.Background(), "system_totals:images")
		if images.Err() == nil {
			status.Usage.TotalImages, _ = images.Int64()
		}
	}
	if status.MongoDB.Available {
		users, _ := h.MongoDB.GetUsersCount(context.Background())
		status.Usage.TotalUsers = users
		freePlusUsers, _ := h.MongoDB.GetUsersCountForSubscription(context.Background(), "free+")
		status.Usage.TotalFreePlusUsers = freePlusUsers
		basicUsers, _ := h.MongoDB.GetUsersCountForSubscription(context.Background(), "basic")
		status.Usage.TotalBasicUsers = basicUsers

		dayActiveUsers, _ := h.MongoDB.GetUserIdsUsedSince(context.Background(), time.Now().UTC().AddDate(0, 0, -1), 0, 1000000)
		status.Usage.DayActiveUsers = int64(len(dayActiveUsers))

		weekActiveUsers, _ := h.MongoDB.GetUserIdsUsedSince(context.Background(), time.Now().UTC().AddDate(0, 0, -7), 0, 1000000)
		status.Usage.WeekActiveUsers = int64(len(weekActiveUsers))

		monthActiveUsers, _ := h.MongoDB.GetUserIdsUsedSince(context.Background(), time.Now().UTC().AddDate(0, -1, 0), 0, 1000000)
		status.Usage.MonthActiveUsers = int64(len(monthActiveUsers))
	}
	return status
}
