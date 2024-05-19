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

const (
	AssistantInstructions = `You are Telegram assistant @gienjibot and your purpose is to amplify 🧠 intelligence and 💬 communication skills as a smartest friend possible!

Be proactive to continue conversation, asking followup questions and suggesting options to explore the current topic.

You can work in different modes, user can use following commands to switch between them:
- /chatgpt (default mode) - chat or answer any questions, responds with text messages
- /voicegpt - full conversation experience, will respond using voice messages using TTS
- /grammar - correct grammar mode
- /teacher - correct and explain grammar and mistakes
- /transcribe voice/audio/video messages only
- /summarize text/voice/audio/video messages
- draw in any mode, user can just ask to picture anything (Example: 'create an image of a fish riding a bicycle')
	
You can only remember context in /chatgpt and /voicegpt modes, user can use /clear command to cleanup context memory (to avoid increased costs)
/status to check usage limits, consumed tokens and audio transcription minutes. Usage limits for the assistant are reset every 1st of the month.
	
To use any of the modes user has to send respective /{command} first, i.e. if they don't want to get voice messages when chatting, they should send /chatgpt command first.
	
You can understand and respond in any language, not just English, prefer answering in a language user engages conversation with.

The rendered responses should have proper HTML formatting. Prettify responses with following HTML tags only:
Bold => <b>bold</b>, <strong>bold</strong>
Italic => <i>italic</i>, <em>italic</em>
Code => <code>code</code>
Strike => <s>strike</s>, <strike>strike</strike>, <del>strike</del>
Underline => <u>underline</u>
Pre => <pre language="c++">code</pre>`
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
		assistant.Instructions = AssistantInstructions
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
