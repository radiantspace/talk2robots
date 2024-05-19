package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/models"
	"talk2robots/m/v2/app/payments"
	"time"

	log "github.com/sirupsen/logrus"
)

func CreateSpeech(ctx context.Context, tts *models.TTSRequest) (io.ReadCloser, error) {
	if tts.Input == "" {
		log.Warnf("input is required for tts")
		return nil, errors.New("input is required for tts")
	}

	if tts.Voice == "" {
		tts.Voice = "shimmer"
	}

	if tts.Model == "" {
		tts.Model = "tts-1"
	}

	if len(tts.Input) > 4096 {
		log.Warnf("trimming input for tts")
		tts.Input = tts.Input[:4096]
	}

	timeNow := time.Now()

	// Set the request body
	requestBody := struct {
		Model          string  `json:"model"`
		Input          string  `json:"input"`
		Voice          string  `json:"voice"`
		ResponseFormat string  `json:"response_format,omitempty"`
		Speed          float64 `json:"speed,omitempty"`
	}{
		Model:          string(tts.Model),
		Input:          tts.Input,
		Voice:          tts.Voice,
		ResponseFormat: "opus",
		Speed:          1.00,
	}

	// Convert the request body to JSON
	requestBodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	// Create the HTTP request
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/audio/speech", bytes.NewBuffer(requestBodyJSON))
	if err != nil {
		return nil, err
	}

	// Set the request headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.CONFIG.OpenAIAPIKey)

	usage := models.CostAndUsage{
		Engine:            tts.Model,
		PricePerInputUnit: 0.015 / 1000,
		Cost:              0,
		Usage: models.Usage{
			PromptTokens: len(tts.Input),
		},
	}
	status := fmt.Sprintf("status:%d", 0)
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.tts.latency", time.Since(timeNow), []string{status, "model:" + string(tts.Model)}, 1)
	}()

	// Send the HTTP request
	resp, err := HTTP_CLIENT.Do(req)
	if err != nil {
		return nil, err
	}
	status = fmt.Sprintf("status:%d", resp.StatusCode)

	// Check the response status code
	if resp.StatusCode != http.StatusOK {
		// Read the response body
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		return nil, fmt.Errorf("unexpected status code: %d, response: %s", resp.StatusCode, responseBody)
	}

	go payments.Bill(ctx, usage)

	return resp.Body, nil
}
