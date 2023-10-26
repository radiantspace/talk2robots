// package to connect to OpenAI API
package openai

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

// API is a type for OpenAI API
type API struct {
	authToken string
	client    *http.Client
}

// NewAPI creates new OpenAI API
func NewAPI(cfg *config.Config) *API {
	return &API{
		authToken: cfg.OpenAIAPIKey,
		client: &http.Client{
			Timeout: TIMEOUT,
		},
	}
}

// IsAvailable checks whether OpenAI API is available
func (a *API) IsAvailable(ctx context.Context) bool {
	if a.authToken == "" {
		log.Errorf("PING: OpenAI API key is not set")
		return false
	}

	response, err := a.ChatComplete(ctx, models.ChatCompletion{
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
		log.Errorf("PING: OpenAI API error: %+v", err)
		return false
	}

	log.Debugf("PING: OpenAI API response: %+v", response)
	return true
}
