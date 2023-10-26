// https://beta.openai.com/docs/api-reference/create-completion
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"talk2robots/m/v2/app/models"
)

// Complete completes text
func (a *API) Complete(ctx context.Context, completion models.Completion) (string, error) {
	if completion.Engine == "" {
		completion.Engine = models.Davinci
	}
	if completion.MaxTokens == 0 {
		completion.MaxTokens = 20
	}
	if completion.Temperature == 0 {
		completion.Temperature = 0.9
	}
	if completion.TopP == 0 {
		completion.TopP = 1
	}
	if completion.FrequencyPenalty == 0 {
		completion.FrequencyPenalty = 0
	}
	if completion.PresencePenalty == 0 {
		completion.PresencePenalty = 0
	}

	data := map[string]interface{}{
		"prompt":            completion.Prompt,
		"max_tokens":        completion.MaxTokens,
		"temperature":       completion.Temperature,
		"top_p":             completion.TopP,
		"frequency_penalty": completion.FrequencyPenalty,
		"presence_penalty":  completion.PresencePenalty,
		"stop":              completion.Stop,
	}

	body, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/engines/"+string(completion.Engine)+"/completions", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.authToken)

	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New("OpenAI API returned status code " + strconv.Itoa(resp.StatusCode))
	}

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var response models.CompletionResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return "", err
	}

	if len(response.Choices) == 0 {
		return "", errors.New("OpenAI API returned empty choices")
	}

	return strings.TrimSpace(response.Choices[0].Text), nil
}
