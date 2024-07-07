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
)

func CreateAssistant(ctx context.Context, assistant *models.AssistantRequest) (*models.AssistantResponse, error) {
	if assistant.Model == "" {
		assistant.Model = string(models.ChatGpt4Turbo)
	}
	if assistant.Name == "" {
		assistant.Name = "General Assistant On " + string(models.ChatGpt4TurboVision)
	}
	if assistant.Tools == nil {
		// assistant.Tools = []models.AssistantTool{
		// {
		// 	Type: "code_interpreter",
		// },
		// {
		// 	Type: "retrieval",
		// },
		// }
	}
	if assistant.Instructions == "" {
		assistant.Instructions = config.AI_INSTRUCTIONS
	}

	body, err := json.Marshal(assistant)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/assistants", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.CONFIG.OpenAIAPIKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	resp, err := HTTP_CLIENT.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
		return nil, err
	}

	var assistantResponse models.AssistantResponse
	err = json.Unmarshal(body, &assistantResponse)
	if err != nil {
		return nil, err
	}

	return &assistantResponse, nil
}

// Get assistant
func GetAssistant(ctx context.Context, assistantID string) (*models.AssistantResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/assistants/"+assistantID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.CONFIG.OpenAIAPIKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	resp, err := HTTP_CLIENT.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
		return nil, err
	}

	var assistantResponse models.AssistantResponse
	err = json.Unmarshal(body, &assistantResponse)
	if err != nil {
		return nil, err
	}

	return &assistantResponse, nil
}

// List assistants
func ListAssistants(ctx context.Context, limit int, order string, after string, before string) (*models.AssistantListResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/assistants", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.CONFIG.OpenAIAPIKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	q := req.URL.Query()
	if limit > 0 {
		q.Add("limit", fmt.Sprintf("%d", limit))
	}
	if order != "" {
		q.Add("order", order)
	}
	if after != "" {
		q.Add("after", after)
	}
	if before != "" {
		q.Add("before", before)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := HTTP_CLIENT.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return &models.AssistantListResponse{Data: []models.AssistantResponse{}}, nil
		}
		err = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var assistantResponse models.AssistantListResponse
	err = json.Unmarshal(body, &assistantResponse)
	if err != nil {
		return nil, err
	}

	return &assistantResponse, nil
}
