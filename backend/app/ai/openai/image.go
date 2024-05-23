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
	DALLE3_S  float64 = 0.04
	DALLE3_HD float64 = 0.08
)

func CreateImage(ctx context.Context, prompt string) (string, string, error) {
	timeNow := time.Now()

	// Set the request body
	requestBody := struct {
		Model   string `json:"model"`
		Prompt  string `json:"prompt"`
		N       int    `json:"n"`
		Size    string `json:"size"`
		Quality string `json:"quality,omitempty"`
	}{
		Model:  string(models.DallE3),
		Prompt: prompt,
		N:      1,
		Size:   "1024x1024",
	}

	// if lib.IsUserBasic(ctx) {
	// 	requestBody.Quality = "hd"
	// }

	// Convert the request body to JSON
	requestBodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return "", "", err
	}

	// Create the HTTP request
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/images/generations", bytes.NewBuffer(requestBodyJSON))
	if err != nil {
		return "", "", err
	}

	// Set the request headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.CONFIG.OpenAIAPIKey)

	usage := models.CostAndUsage{
		Engine:     models.DallE3,
		ImagePrice: DALLE3_S,
		Usage: models.Usage{
			ImagesCount: 1,
		},
	}

	// if lib.IsUserBasic(ctx) {
	// 	usage.ImagePrice = DALLE3_HD
	// }

	status := fmt.Sprintf("status:%d", 0)
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.image.latency", time.Since(timeNow), []string{status, "model:" + string(models.DallE3)}, 1)
	}()

	// Send the HTTP request
	resp, err := HTTP_CLIENT.Do(req)
	if resp != nil {
		status = fmt.Sprintf("status:%d", resp.StatusCode)
	}
	if err != nil {
		return "", "", err
	}

	// Read the response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	// "{\n  \"created\": 1714859364,\n  \"data\": [\n    {\n      \"revised_prompt\": \"Create an image of a fluffy Siberian cat. It has a multi-colored coat in hues of cream, brown, and black. The cat is perched on a vintage bicycle with a shiny silver finish. The bicycle is stationary and placed against a backdrop of a vivid sunset, casting a warm orange glow that reflects off of the spokes of the bicycle wheels.\",\n      \"url\": \"https://oaidalleapiprodscus.blob.core.windows.net/private/org-mXLEStBZvlLX16KcOWmBQxRi/user-FolUm62ffE794657USBZWAuZ/img-fu7BaN8wBqO93SpduqbPrjzL.png?st=2024-05-04T20%3A49%3A24Z\u0026se=2024-05-04T22%3A49%3A24Z\u0026sp=r\u0026sv=2021-08-06\u0026sr=b\u0026rscd=inline\u0026rsct=image/png\u0026skoid=6aaadede-4fb3-4698-a8f6-684d7786b067\u0026sktid=a48cca56-e6da-484e-a814-9c849652bcb3\u0026skt=2024-05-04T21%3A45%3A40Z\u0026ske=2024-05-05T21%3A45%3A40Z\u0026sks=b\u0026skv=2021-08-06\u0026sig=XTWxaLMaxX%2Bt8sg8QIbax0jy2cZ0TDcr7W9/i83Yca4%3D\"\n    }\n  ]\n}\n"
	responseJson := struct {
		Data []struct {
			RevisedPrompt string `json:"revised_prompt"`
			URL           string `json:"url"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(responseBody, &responseJson); err != nil {
		return "", "", err
	}

	// Check the response status code
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("unexpected status code: %d, response: %s", resp.StatusCode, responseBody)
	}

	go payments.Bill(ctx, usage)

	return responseJson.Data[0].URL, responseJson.Data[0].RevisedPrompt, nil
}
