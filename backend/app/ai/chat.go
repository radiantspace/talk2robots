// https://platform.openai.com/docs/api-reference/chat/create
// https://readme.fireworks.ai/reference/createchatcompletion
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"talk2robots/m/v2/app/ai/claude"
	"talk2robots/m/v2/app/ai/sse"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/models"
	"talk2robots/m/v2/app/payments"
	"time"
	"unicode/utf8"

	log "github.com/sirupsen/logrus"
)

// https://openai.com/pricing
// https://fireworks.ai/pricing
const (
	// gpt-3.5-turbo-0125
	CHAT_INPUT_PRICE  = 0.5 / 1000000
	CHAT_OUTPUT_PRICE = 1.5 / 1000000

	// gpt-4o-mini
	CHAT_GPT4O_MINI_INPUT_PRICE  = 0.15 / 1000000
	CHAT_GPT4O_MINI_OUTPUT_PRICE = 0.6 / 1000000

	// gpt-4
	CHAT_GPT4_INPUT_PRICE  = 30.0 / 1000000
	CHAT_GPT4_OUTPUT_PRICE = 60.0 / 1000000

	// gpt-4-turbo (-0125, -1106, -vision)
	CHAT_GPT4_TURBO_INPUT_PRICE  = 10.0 / 1000000
	CHAT_GPT4_TURBO_OUTPUT_PRICE = 30.0 / 1000000

	// gpt-4o
	CHAT_GPT4O_INPUT_PRICE  = 5.0 / 1000000
	CHAT_GPT4O_OUTPUT_PRICE = 15.0 / 1000000

	// fireworks
	FIREWORKS_16B_80B_PRICE = 0.9 / 1000000
	FIREWORKS_0B_16B_PRICE  = 0.2 / 1000000

	// claude haiku 3.0
	HAIKU_INPUT_PRICE  = 0.25 / 1000000
	HAIKU_OUTPUT_PRICE = 1.25 / 1000000

	// claude sonet 3.5
	SONET_INPUT_PRICE  = 3.0 / 1000000
	SONET_OUTPUT_PRICE = 15.0 / 1000000

	// claude opus 3.0
	OPUS_INPUT_PRICE  = 15.0 / 1000000
	OPUS_OUTPUT_PRICE = 75.0 / 1000000

	CHARS_PER_TOKEN = 2.0 // average number of characters per token, must be tuned or moved to tiktoken
)

// Complete completes text
func (a *API) ChatComplete(ctx context.Context, completion models.ChatCompletion) (string, error) {
	timeNow := time.Now()
	if completion.Model == "" {
		completion.Model = string(models.ChatGpt4oMini)
	}

	if IsClaudeAI(models.Engine(completion.Model)) {
		return a.chatCompleteClaude(ctx, completion)
	}

	promptTokens := 0.0
	for _, message := range completion.Messages {
		promptTokens += 4 + ApproximateTokensCount(message.Content)
	}
	if completion.MaxTokens == 0 {
		// calculate max tokens based on prompt words count
		completion.MaxTokens = int(maxTokensForModel(models.Engine(completion.Model), promptTokens))
	}

	usage := models.CostAndUsage{
		Engine:             models.Engine(completion.Model),
		PricePerInputUnit:  PricePerInputToken(models.Engine(completion.Model)),
		PricePerOutputUnit: PricePerOutputToken(models.Engine(completion.Model)),
		Cost:               0,
		Usage:              models.Usage{},
	}

	data := map[string]interface{}{
		"max_tokens": completion.MaxTokens,
		"messages":   completion.Messages,
		"model":      completion.Model,
		"user":       ctx.Value(models.UserContext{}).(string),
	}

	body, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	url := urlFromModel(models.Engine(completion.Model))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authTokenFromModel(models.Engine(completion.Model)))

	status := fmt.Sprintf("status:%d", 0)
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.chat_complete.latency", time.Since(timeNow), []string{status, "model:" + completion.Model}, 1)
		config.CONFIG.DataDogClient.Timing("openai.chat_complete.latency_per_token", time.Since(timeNow), []string{status, "model:" + completion.Model}, float64(usage.Usage.CompletionTokens))
	}()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New("ChatComplete: " + resp.Status)
	}

	var response models.ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		if err == io.EOF {
			return "", errors.New("ChatComplete: empty response")
		}
		return "", err
	}
	usage.Usage = response.Usage
	go payments.Bill(ctx, usage)
	return response.Choices[0].Message.Content, nil
}

func (a *API) ChatCompleteStreaming(ctx context.Context, completion models.ChatMultimodalCompletion, cancelContext context.CancelFunc) (chan string, error) {
	timeNow := time.Now()
	if completion.Model == "" {
		completion.Model = string(models.ChatGpt35Turbo)
	}

	if IsClaudeAI(models.Engine(completion.Model)) {
		return chatCompleteStreamingClaude(ctx, completion, cancelContext)
	}

	promptTokens := 0.0
	for _, message := range completion.Messages {
		for _, content := range message.Content {
			promptTokens += 4 + ApproximateTokensCount(content.Text)
		}
	}
	if completion.MaxTokens == 0 {
		// calculate max tokens based on prompt words count
		completion.MaxTokens = int(maxTokensForModel(models.Engine(completion.Model), promptTokens))
	}

	usage := models.CostAndUsage{
		Engine:             models.Engine(completion.Model),
		PricePerInputUnit:  PricePerInputToken(models.Engine(completion.Model)),
		PricePerOutputUnit: PricePerOutputToken(models.Engine(completion.Model)),
		Cost:               0,
		Usage: models.Usage{
			PromptTokens: int(promptTokens),
		},
	}

	data := map[string]interface{}{
		"max_tokens": completion.MaxTokens,
		"messages":   completion.Messages,
		"model":      completion.Model,
		"stream":     true,
		"user":       ctx.Value(models.UserContext{}).(string),
	}

	body, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	url := urlFromModel(models.Engine(completion.Model))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authTokenFromModel(models.Engine(completion.Model)))

	messages := make(chan string)

	go func() {
		defer func() {
			close(messages)
			cancelContext()

			usage.Usage.TotalTokens = usage.Usage.PromptTokens + usage.Usage.CompletionTokens
			go payments.Bill(ctx, usage)
			config.CONFIG.DataDogClient.Timing("openai.chat_complete_streaming.latency", time.Since(timeNow), []string{"model:" + completion.Model}, 1)
			config.CONFIG.DataDogClient.Timing("openai.chat_complete_streaming.latency_per_token", time.Since(timeNow), []string{"model:" + completion.Model}, float64(usage.Usage.CompletionTokens))
		}()
		client := sse.NewClientFromReq(req)
		err := client.SubscribeWithContext(ctx, "", func(msg *sse.Event) {
			var response models.ChatResponse
			if msg.Data != nil && len(msg.Data) > 2 && string(msg.Data[:1]) == "[" && string(msg.Data) == "[DONE]" {
				log.Infof("ChatCompleteStreaming got [DONE] message for user id %s", ctx.Value(models.UserContext{}).(string))
				return
			}
			if err := json.Unmarshal(msg.Data, &response); err != nil {
				log.Errorf("ChatCompleteStreaming couldn't parse response: %s, err: %v", string(msg.Data), err)
				return // or handle error
			}

			for _, choice := range response.Choices {
				if choice.Delta.Content != "" {
					usage.Usage.CompletionTokens += int(ApproximateTokensCount(choice.Delta.Content))
					messages <- choice.Delta.Content
				}
			}
		})
		if err != nil {
			log.Errorf("ChatCompleteStreaming couldn't subscribe: %v", err)
		}
	}()
	return messages, nil
}

func urlFromModel(model models.Engine) string {
	if IsFireworksAI(model) {
		return "https://api.fireworks.ai/inference/v1/chat/completions"
	}
	if IsClaudeAI(model) {
		return "https://api.anthropic.com/v1/messages"
	}
	return "https://api.openai.com/v1/chat/completions"
}

func authTokenFromModel(model models.Engine) string {
	if IsFireworksAI(model) {
		return config.CONFIG.FireworksAPIKey
	}
	if IsClaudeAI(model) {
		return config.CONFIG.ClaudeAPIKey
	}

	return config.CONFIG.OpenAIAPIKey
}

// if this snippet will make too much mistakes, we can use this
// https://github.com/pkoukk/tiktoken-go
func ApproximateTokensCount(message string) float64 {
	return math.Max(float64(utf8.RuneCountInString(message))/CHARS_PER_TOKEN, 1)
}

func PricePerInputToken(model models.Engine) float64 {
	switch model {
	case models.ChatGpt4:
		return CHAT_GPT4_INPUT_PRICE
	case models.ChatGpt4TurboVision, models.ChatGpt4Turbo:
		return CHAT_GPT4_TURBO_INPUT_PRICE
	case models.LlamaV3_8b:
		return FIREWORKS_0B_16B_PRICE
	case models.LlamaV3_70b:
		return FIREWORKS_16B_80B_PRICE
	case models.ChatGpt4o:
		return CHAT_GPT4O_INPUT_PRICE
	case models.ChatGpt4oMini:
		return CHAT_GPT4O_MINI_INPUT_PRICE
	case models.Sonet35:
		return SONET_INPUT_PRICE
	case models.Haiku3:
		return HAIKU_INPUT_PRICE
	default:
		return CHAT_INPUT_PRICE
	}
}

func PricePerOutputToken(model models.Engine) float64 {
	switch model {
	case models.ChatGpt4:
		return CHAT_GPT4_OUTPUT_PRICE
	case models.ChatGpt4TurboVision, models.ChatGpt4Turbo:
		return CHAT_GPT4_TURBO_OUTPUT_PRICE
	case models.LlamaV3_8b:
		return FIREWORKS_0B_16B_PRICE
	case models.LlamaV3_70b:
		return FIREWORKS_16B_80B_PRICE
	case models.ChatGpt4o:
		return CHAT_GPT4O_OUTPUT_PRICE
	case models.ChatGpt4oMini:
		return CHAT_GPT4O_MINI_OUTPUT_PRICE
	case models.Sonet35:
		return SONET_OUTPUT_PRICE
	case models.Haiku3:
		return HAIKU_OUTPUT_PRICE
	default:
		return CHAT_OUTPUT_PRICE
	}
}

func LimitPromptTokensForModel(model models.Engine, promptTokensCount float64) int {
	// limit context to max - 1024 tokens
	switch model {
	case models.ChatGpt4Turbo, models.ChatGpt4TurboVision, models.ChatGpt4o, models.ChatGpt4oMini:
		return int(math.Min(127*1024, promptTokensCount))
	case models.ChatGpt4:
		return int(math.Min(7*1024, promptTokensCount))
	case models.ChatGpt35Turbo:
		return int(math.Min(15*1024, promptTokensCount))
	case models.LlamaV3_70b, models.LlamaV3_8b:
		return int(math.Min(7*1024, promptTokensCount))
	case models.Sonet35, models.Haiku3, models.Opus3:
		return int(math.Min(199*1024, promptTokensCount))
	default:
		return int(math.Min(3*1024, promptTokensCount))
	}
}

// need to tune this for speed and accuracy
func maxTokensForModel(model models.Engine, promptTokensCount float64) float64 {
	switch model {
	case models.ChatGpt4:
		return 2000
	default:
		return 2000
	}
}

func (a *API) chatCompleteClaude(ctx context.Context, completion models.ChatCompletion) (string, error) {
	timeNow := time.Now()
	promptTokens := 0.0
	completion, systemPrompt := claude.Convert(completion)
	for _, message := range completion.Messages {
		promptTokens += 4 + ApproximateTokensCount(message.Content)
	}
	if completion.MaxTokens == 0 {
		// calculate max tokens based on prompt words count
		completion.MaxTokens = int(maxTokensForModel(models.Engine(completion.Model), promptTokens))
	}

	usage := models.CostAndUsage{
		Engine:             models.Engine(completion.Model),
		PricePerInputUnit:  PricePerInputToken(models.Engine(completion.Model)),
		PricePerOutputUnit: PricePerOutputToken(models.Engine(completion.Model)),
		Cost:               0,
		Usage:              models.Usage{},
	}

	data := map[string]interface{}{
		"max_tokens": completion.MaxTokens,
		"messages":   completion.Messages,
		"model":      completion.Model,
		"system":     systemPrompt,
	}

	body, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	url := urlFromModel(models.Engine(completion.Model))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", authTokenFromModel(models.Engine(completion.Model)))
	req.Header.Set("anthropic-version", "2023-06-01")

	status := fmt.Sprintf("status:%d", 0)
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	defer func() {
		config.CONFIG.DataDogClient.Timing("ai.chat_complete.latency", time.Since(timeNow), []string{status, "model:" + completion.Model}, 1)
		config.CONFIG.DataDogClient.Timing("ai.chat_complete.latency_per_token", time.Since(timeNow), []string{status, "model:" + completion.Model}, float64(usage.Usage.CompletionTokens))
	}()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New("ChatComplete: " + resp.Status)
	}

	var response models.ClaudeChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		if err == io.EOF {
			return "", errors.New("ChatComplete: empty response")
		}
		return "", err
	}
	usage.Usage.PromptTokens = response.Usage.InputTokens
	usage.Usage.CompletionTokens = response.Usage.OutputTokens

	go payments.Bill(ctx, usage)
	return *response.Content[0].Text, nil
}

func chatCompleteStreamingClaude(ctx context.Context, completion models.ChatMultimodalCompletion, cancelContext context.CancelFunc) (chan string, error) {
	timeNow := time.Now()
	promptTokens := 0.0
	completion, systemPrompt := claude.ConvertMultimodal(completion)
	if completion.MaxTokens == 0 {
		// calculate max tokens based on prompt words count
		completion.MaxTokens = int(maxTokensForModel(models.Engine(completion.Model), promptTokens))
	}

	usage := models.CostAndUsage{
		Engine:             models.Engine(completion.Model),
		PricePerInputUnit:  PricePerInputToken(models.Engine(completion.Model)),
		PricePerOutputUnit: PricePerOutputToken(models.Engine(completion.Model)),
		Cost:               0,
		Usage:              models.Usage{},
	}

	data := map[string]interface{}{
		"max_tokens": completion.MaxTokens,
		"messages":   completion.Messages,
		"model":      completion.Model,
		"system":     systemPrompt,
		"stream":     true,
	}

	body, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	url := urlFromModel(models.Engine(completion.Model))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", authTokenFromModel(models.Engine(completion.Model)))
	req.Header.Set("anthropic-version", "2023-06-01")

	messages := make(chan string)

	go func() {
		defer func() {
			close(messages)
			cancelContext()

			usage.Usage.TotalTokens = usage.Usage.PromptTokens + usage.Usage.CompletionTokens
			go payments.Bill(ctx, usage)
			config.CONFIG.DataDogClient.Timing("ai.chat_complete_streaming.latency", time.Since(timeNow), []string{"model:" + completion.Model}, 1)
			config.CONFIG.DataDogClient.Timing("ai.chat_complete_streaming.latency_per_token", time.Since(timeNow), []string{"model:" + completion.Model}, float64(usage.Usage.CompletionTokens))
		}()

		// event: message_start
		// data: {"type": "message_start", "message": {"id": "msg_1nZdL29xx5MUA1yADyHTEsnR8uuvGzszyY", "type": "message", "role": "assistant", "content": [], "model": "claude-3-5-sonnet-20240620", "stop_reason": null, "stop_sequence": null, "usage": {"input_tokens": 25, "output_tokens": 1}}}

		// event: content_block_start
		// data: {"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}}

		// event: ping
		// data: {"type": "ping"}

		// event: content_block_delta
		// data: {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hello"}}

		// event: content_block_delta
		// data: {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "!"}}

		// event: content_block_stop
		// data: {"type": "content_block_stop", "index": 0}

		// event: message_delta
		// data: {"type": "message_delta", "delta": {"stop_reason": "end_turn", "stop_sequence":null}, "usage": {"output_tokens": 15}}

		// event: message_stop
		// data: {"type": "message_stop"}
		client := sse.NewClientFromReq(req)
		err := client.SubscribeWithContext(ctx, "", func(msg *sse.Event) {
			var response models.ClaudeStreamEvent
			if err := json.Unmarshal(msg.Data, &response); err != nil {
				log.Errorf("ChatCompleteStreamingClaude couldn't parse response: %s, err: %v", string(msg.Data), err)
				return
			}
			log.Debugf("ChatCompleteStreamingClaude got event: %s", string(msg.Data))

			if *response.Type == "message_start" && response.Message.Usage != nil {
				currentUsage := response.Message.Usage
				log.Debugf("ChatCompleteStreamingClaude got message_start, input_tokens: %d, output_tokens: %d", currentUsage.InputTokens, currentUsage.OutputTokens)
				usage.Usage.PromptTokens += currentUsage.InputTokens
				usage.Usage.CompletionTokens += currentUsage.OutputTokens
				log.Debugf("ChatCompleteStreamingClaude usage: %+v", usage.Usage)
			}

			if *response.Type == "message_delta" && response.Usage != nil {
				currentUsage := response.Usage
				log.Debugf("ChatCompleteStreamingClaude got message_delta, output_tokens: %d", currentUsage.OutputTokens)
				usage.Usage.CompletionTokens += currentUsage.OutputTokens
				log.Debugf("ChatCompleteStreamingClaude usage: %+v", usage.Usage)
			}

			if *response.Type == "content_block_delta" && response.Delta != nil && response.Delta.Text != nil {
				messages <- *(*response.Delta).Text
			}
		})
		if err != nil {
			log.Errorf("ChatCompleteStreamingClaude couldn't subscribe: %v", err)
		}
	}()
	return messages, nil
}
