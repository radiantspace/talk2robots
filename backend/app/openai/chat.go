// https://platform.openai.com/docs/api-reference/chat/create
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/models"
	"talk2robots/m/v2/app/openai/sse"
	"talk2robots/m/v2/app/payments"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// https://openai.com/pricing
// Multiple models, each with different capabilities and price points.
// Prices are per 1,000 tokens. You can think of tokens as pieces of words, where 1,000 tokens is about 750 words.
// This paragraph is 35 tokens.

// Example for GPT-4:
// $3 can buy you 50K tokens, which is ~37.5K words
// $9 can buy you 150K tokens, which is ~112.5K words

const (
	// gpt-3.5-turbo-1106
	CHAT_INPUT_PRICE  = 0.001 / 1000
	CHAT_OUTPUT_PRICE = 0.002 / 1000

	// gpt-4
	CHAT_GPT4_INPUT_PRICE  = 0.03 / 1000
	CHAT_GPT4_OUTPUT_PRICE = 0.06 / 1000

	// gpt-4-1106-vision-preview
	CHAT_GPT4_TURBO_VISION_INPUT_PRICE  = 0.01 / 1000
	CHAT_GPT4_TURBO_VISION_OUTPUT_PRICE = 0.02 / 1000

	WORDS_PER_TOKEN = 750.0 / 1000.0
)

// Complete completes text
func (a *API) ChatComplete(ctx context.Context, completion models.ChatCompletion) (string, error) {
	timeNow := time.Now()
	if completion.Model == "" {
		completion.Model = string(models.ChatGpt35Turbo)
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.authToken)

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
	promptTokens := 0.0
	for _, message := range completion.Messages {
		promptTokens += 4 + ApproximateTokensCount(message.Content[0].Text)
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.authToken)

	client := sse.NewClientFromReq(req)
	messages := make(chan string)

	go func() {
		defer func() {
			close(messages)
			cancelContext()

			backgroundContext := context.WithValue(context.Background(), models.UserContext{}, ctx.Value(models.UserContext{}).(string))
			backgroundContext = context.WithValue(backgroundContext, models.SubscriptionContext{}, ctx.Value(models.SubscriptionContext{}).(models.MongoSubscriptionName))
			backgroundContext = context.WithValue(backgroundContext, models.ClientContext{}, ctx.Value(models.ClientContext{}).(string))
			backgroundContext = context.WithValue(backgroundContext, models.ChannelContext{}, ctx.Value(models.ChannelContext{}).(string))
			usage.Usage.TotalTokens = usage.Usage.PromptTokens + usage.Usage.CompletionTokens
			go payments.Bill(backgroundContext, usage)
			config.CONFIG.DataDogClient.Timing("openai.chat_complete_streaming.latency", time.Since(timeNow), []string{"model:" + completion.Model}, 1)
			config.CONFIG.DataDogClient.Timing("openai.chat_complete_streaming.latency_per_token", time.Since(timeNow), []string{"model:" + completion.Model}, float64(usage.Usage.CompletionTokens))
		}()
		err := client.SubscribeWithContext(ctx, uuid.New().String(), func(msg *sse.Event) {
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

// if this snippet will make too much mistakes, we can use this
// https://github.com/pkoukk/tiktoken-go
func ApproximateTokensCount(message string) float64 {
	return math.Max(float64(len(strings.Fields(message)))/WORDS_PER_TOKEN, 1)
}

func PricePerInputToken(model models.Engine) float64 {
	switch model {
	case models.ChatGpt4:
		return CHAT_GPT4_INPUT_PRICE
	case models.ChatGpt4TurboVision, models.ChatGpt4Turbo:
		return CHAT_GPT4_TURBO_VISION_INPUT_PRICE
	default:
		return CHAT_INPUT_PRICE
	}
}

func PricePerOutputToken(model models.Engine) float64 {
	switch model {
	case models.ChatGpt4:
		return CHAT_GPT4_OUTPUT_PRICE
	case models.ChatGpt4TurboVision, models.ChatGpt4Turbo:
		return CHAT_GPT4_TURBO_VISION_OUTPUT_PRICE
	default:
		return CHAT_OUTPUT_PRICE
	}
}

func LimitPromptTokensForModel(model models.Engine, promptTokensCount float64) int {
	// limit context to max - 1024 tokens
	switch model {
	case models.ChatGpt4Turbo, models.ChatGpt4TurboVision:
		return int(math.Min(127*1024, promptTokensCount))
	case models.ChatGpt4:
		return int(math.Min(7*1024, promptTokensCount))
	case models.ChatGpt35Turbo:
		return int(math.Min(15*1024, promptTokensCount))
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
