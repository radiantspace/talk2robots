package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/models"
	"time"

	log "github.com/sirupsen/logrus"
)

func (a *API) CreateSpeech(ctx context.Context, tts models.TTSRequest) ([]byte, error) {
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
		Speed:          1.25,
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
	req.Header.Set("Authorization", "Bearer "+a.authToken)

	// Send the HTTP request
	client := http.DefaultClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// TODO: billing

	// Check the response status code
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("failed to generate speech")
	}

	// Read the response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	config.CONFIG.DataDogClient.Timing("openai.tts.latency", time.Since(timeNow), []string{"model:" + string(tts.Model)}, 1)

	return responseBody, nil
}
