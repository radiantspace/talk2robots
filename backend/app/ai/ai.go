// package to connect to AI API
package ai

import (
	"context"
	"net/http"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/models"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	TIMEOUT = 60 * time.Second
)

type API struct {
	client *http.Client
}

// NewAPI creates new AI API
func NewAPI(cfg *config.Config) *API {
	return &API{
		client: &http.Client{
			Timeout: TIMEOUT,
		},
	}
}

// IsAvailable checks whether AI API is available
func (a *API) IsAvailable(ctx context.Context, model models.Engine) bool {
	response, err := a.ChatComplete(ctx, models.ChatCompletion{
		Model: string(model),
		Messages: []models.Message{
			{
				Role:    "system",
				Content: "Reply only \"OK\" or \"Not OK\"",
			},
			{
				Role:    "user",
				Content: "test",
			},
		},
	})
	if err != nil {
		log.Errorf("PING: API error: %+v", err)
		return false
	}

	log.Debugf("PING: API response: %+v", response)
	return true
}

func IsFireworksAI(model models.Engine) bool {
	if model == models.LlamaV3_70b || model == models.LlamaV3_8b || model == models.Firellava_13b || model == models.Llava_yi_34b {
		return true
	}

	return false
}

func IsClaudeAI(model models.Engine) bool {
	if model == models.Sonet35 || model == models.Haiku3 || model == models.Opus3 || model == models.Sonet35_241022 {
		return true
	}

	return false
}
