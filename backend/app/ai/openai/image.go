package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/models"
	"talk2robots/m/v2/app/payments"
	"time"
)

// https://openai.com/pricing
const (
	DALLE3_S  = 0.04
	DALLE3_HD = 0.08
)

func CreateImage(ctx context.Context, prompt string) (string, error) {
	timeNow := time.Now()

	// Set the request body
	requestBody := struct {
		Model   string `json:"model"`
		Prompt  string `json:"prompt"`
		N       int    `json:"n"`
		Size    string `json:"size"`
		Quality string `json:"quality"`
	}{
		Model:   string(models.DallE3),
		Prompt:  prompt,
		N:       1,
		Size:    "1024x1024",
		Quality: "hd",
	}

	// Convert the request body to JSON
	requestBodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	// Create the HTTP request
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/images/generations", bytes.NewBuffer(requestBodyJSON))
	if err != nil {
		return "", err
	}

	// Set the request headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.CONFIG.OpenAIAPIKey)

	usage := models.CostAndUsage{
		Engine:            models.DallE3,
		PricePerInputUnit: DALLE3_HD,
		Cost:              0,
		Usage: models.Usage{
			PromptTokens: 1,
		},
	}
	status := fmt.Sprintf("status:%d", 0)
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.image.latency", time.Since(timeNow), []string{status, "model:" + string(models.DallE3)}, 1)
	}()

	// Send the HTTP request
	client := http.DefaultClient
	resp, err := client.Do(req)
	if resp != nil {
		status = fmt.Sprintf("status:%d", resp.StatusCode)
	}
	if err != nil {
		return "", err
	}

	// Read the response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Check the response status code
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d, response: %s", resp.StatusCode, responseBody)
	}

	go payments.Bill(ctx, usage)

	return string(responseBody), nil
}
