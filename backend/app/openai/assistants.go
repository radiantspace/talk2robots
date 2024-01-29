package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"talk2robots/m/v2/app/models"
)

func (a *API) CreateAssistant(ctx context.Context, assistant *models.AssistantRequest) (*models.AssistantResponse, error) {
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
		assistant.Instructions = "Don't advice unless asked explicitly. You're Telegram chat bot that can:\n" +
			`- Default 🧠: chat with or answer any questions /chatgpt
- talk to AI using voice messages /voicegpt
- Correct grammar: /grammar
- Explain grammar and mistakes: /teacher
- /transcribe voice/audio/video messages
- /summarize text/voice/audio/video messages
- remember context in /chatgpt and /voicegpt modes, use /clear to cleanup context memory (to avoid increased cost)`

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
	req.Header.Set("Authorization", "Bearer "+a.authToken)
	req.Header.Set("OpenAI-Beta", "assistants=v1")

	resp, err := a.client.Do(req)
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
func (a *API) GetAssistant(ctx context.Context, assistantID string) (*models.AssistantResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/assistants/"+assistantID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.authToken)
	req.Header.Set("OpenAI-Beta", "assistants=v1")

	resp, err := a.client.Do(req)
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
func (a *API) ListAssistants(ctx context.Context, limit int, order string, after string, before string) (*models.AssistantListResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/assistants", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.authToken)
	req.Header.Set("OpenAI-Beta", "assistants=v1")

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

	resp, err := a.client.Do(req)
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
