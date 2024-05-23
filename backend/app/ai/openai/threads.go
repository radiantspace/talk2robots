package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"talk2robots/m/v2/app/ai"
	"talk2robots/m/v2/app/ai/sse"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/models"
	"talk2robots/m/v2/app/payments"
	"time"

	log "github.com/sirupsen/logrus"
)

// creates a thread and runs it in one request.
func CreateThreadAndRun(ctx context.Context, assistantId string, thread *models.Thread) (*models.ThreadRunResponse, error) {
	if assistantId == "" {
		return nil, fmt.Errorf("assistantId is required")
	}

	if thread == nil {
		return nil, fmt.Errorf("thread is required")
	}

	reqBody, err := json.Marshal(&models.ThreadRunRequest{
		AssistantID: assistantId,
		Thread:      thread,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/threads/runs", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.CONFIG.OpenAIAPIKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:create_thread_and_run"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := HTTP_CLIENT.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	status = fmt.Sprintf("status:%d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, body)
	}

	var threadRunResponse models.ThreadRunResponse
	err = json.Unmarshal(body, &threadRunResponse)
	if err != nil {
		return nil, err
	}

	return &threadRunResponse, nil
}

// create a run
func CreateRun(ctx context.Context, assistantId string, threadId string) (*models.ThreadRunResponse, error) {
	if assistantId == "" {
		return nil, fmt.Errorf("assistantId is required")
	}

	if threadId == "" {
		return nil, fmt.Errorf("threadId is required")
	}

	requestBody := struct {
		AssistantId string `json:"assistant_id"`
		// model, instructions, tools and metadata can be overriden here if needed
	}{
		AssistantId: assistantId,
	}

	requestBodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/threads/"+threadId+"/runs", bytes.NewBuffer(requestBodyJSON))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.CONFIG.OpenAIAPIKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:create_run"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := HTTP_CLIENT.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	status = fmt.Sprintf("status:%d", resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, body)
		return nil, err
	}

	var threadRunResponse models.ThreadRunResponse
	err = json.Unmarshal(body, &threadRunResponse)
	if err != nil {
		return nil, err
	}

	return &threadRunResponse, nil
}

// get a thread.
func GetThread(ctx context.Context, threadId string) (*models.ThreadResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/threads/"+threadId, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.CONFIG.OpenAIAPIKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:get_thread"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := HTTP_CLIENT.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	status = fmt.Sprintf("status:%d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var threadResponse models.ThreadResponse
	err = json.Unmarshal(body, &threadResponse)
	if err != nil {
		return nil, err
	}

	return &threadResponse, nil
}

// cancels a thread run.
func CancelRun(ctx context.Context, threadId, runId string) (*models.ThreadRunResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/threads/"+threadId+"/runs/"+runId+"/cancel", nil)
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

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var threadRunResponse models.ThreadRunResponse
	err = json.Unmarshal(body, &threadRunResponse)
	if err != nil {
		return nil, err
	}

	return &threadRunResponse, nil
}

// retrieve a run by id and threadId.
func GetThreadRun(ctx context.Context, threadId, runId string) (*models.ThreadRunResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/threads/"+threadId+"/runs/"+runId, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.CONFIG.OpenAIAPIKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:get_thread_run"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := HTTP_CLIENT.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	status = fmt.Sprintf("status:%d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var threadRunResponse models.ThreadRunResponse
	err = json.Unmarshal(body, &threadRunResponse)
	if err != nil {
		return nil, err
	}

	return &threadRunResponse, nil
}

// get last thread run by threadId.
func GetLastThreadRun(ctx context.Context, threadId string) (*models.ThreadRunResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/threads/"+threadId+"/runs", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.CONFIG.OpenAIAPIKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	q := req.URL.Query()
	q.Add("limit", fmt.Sprintf("%d", 1))
	req.URL.RawQuery = q.Encode()

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:get_last_thread_run"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := HTTP_CLIENT.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	status = fmt.Sprintf("status:%d", resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, body)
		return nil, err
	}

	var threadRunResponse models.ThreadRunsResponse
	err = json.Unmarshal(body, &threadRunResponse)
	if err != nil {
		return nil, err
	}

	if len(threadRunResponse.Data) == 0 {
		return nil, fmt.Errorf("no runs found")
	}

	return &threadRunResponse.Data[0], nil
}

// List run steps by threadId and runId.
func ListThreadRunSteps(ctx context.Context, threadId, runId string) (*models.ThreadRunStepsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/threads/"+threadId+"/runs/"+runId+"/steps", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.CONFIG.OpenAIAPIKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:list_thread_run_steps"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := HTTP_CLIENT.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	status = fmt.Sprintf("status:%d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var threadRunStepsResponse models.ThreadRunStepsResponse
	err = json.Unmarshal(body, &threadRunStepsResponse)
	if err != nil {
		return nil, err
	}

	return &threadRunStepsResponse, nil
}

// create a message for threadId.
func CreateThreadMessage(ctx context.Context, threadId string, message *models.Message) (*models.ThreadMessageResponse, error) {
	body, err := json.Marshal(message)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/threads/"+threadId+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.CONFIG.OpenAIAPIKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:create_thread_message"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := HTTP_CLIENT.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	status = fmt.Sprintf("status:%d", resp.StatusCode)

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
		return nil, err
	}

	var threadMessageResponse models.ThreadMessageResponse
	err = json.Unmarshal(body, &threadMessageResponse)
	if err != nil {
		return nil, err
	}

	return &threadMessageResponse, nil
}

// retrieve a message by threadId and messageId.
func GetThreadMessage(ctx context.Context, threadId, messageId string) (*models.ThreadMessageResponse, error) {
	if threadId == "" {
		return nil, fmt.Errorf("threadId is required")
	}
	if messageId == "" {
		return nil, fmt.Errorf("messageId is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/threads/"+threadId+"/messages/"+messageId, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.CONFIG.OpenAIAPIKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:get_thread_message"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := HTTP_CLIENT.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	status = fmt.Sprintf("status:%d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var threadMessageResponse models.ThreadMessageResponse
	err = json.Unmarshal(body, &threadMessageResponse)
	if err != nil {
		return nil, err
	}

	return &threadMessageResponse, nil
}

func DeleteThread(ctx context.Context, threadId string) (*models.DeletedResponse, error) {
	if threadId == "" {
		return nil, fmt.Errorf("threadId is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, "https://api.openai.com/v1/threads/"+threadId, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.CONFIG.OpenAIAPIKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:delete_thread"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := HTTP_CLIENT.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	status = fmt.Sprintf("status:%d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var threadDeletedResponse models.DeletedResponse
	err = json.Unmarshal(body, &threadDeletedResponse)
	if err != nil {
		return nil, err
	}

	return &threadDeletedResponse, nil
}

func ListThreadMessagesForARun(ctx context.Context, threadId string, runId string) ([]models.ThreadMessageResponse, error) {
	if threadId == "" {
		return nil, fmt.Errorf("threadId is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/threads/"+threadId+"/messages", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.CONFIG.OpenAIAPIKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	q := req.URL.Query()
	q.Add("limit", fmt.Sprintf("%d", 10))
	req.URL.RawQuery = q.Encode()

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:list_last_thread_messages"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := HTTP_CLIENT.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	status = fmt.Sprintf("status:%d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var threadMessagesResponse models.ThreadMessagesResponse
	err = json.Unmarshal(body, &threadMessagesResponse)
	if err != nil {
		return nil, err
	}

	if len(threadMessagesResponse.Data) == 0 {
		return nil, fmt.Errorf("no messages found")
	}

	messages := []models.ThreadMessageResponse{}
	for _, message := range threadMessagesResponse.Data {
		if message.RunID == runId {
			messages = append(messages, message)
		}
	}

	return messages, nil
}

func CreateThreadAndRunStreaming(ctx context.Context, assistantId string, model models.Engine, thread *models.Thread, cancelContext context.CancelFunc) (chan string, error) {
	if assistantId == "" {
		return nil, fmt.Errorf("assistantId is required")
	}

	if thread == nil {
		return nil, fmt.Errorf("thread is required")
	}

	body, err := json.Marshal(&models.ThreadRunRequest{
		AssistantID: assistantId,
		Thread:      thread,
		Stream:      true,
		Model:       string(model),
	})
	if err != nil {
		return nil, err
	}

	timeNow := time.Now()
	usage := models.CostAndUsage{
		Engine:             model,
		PricePerInputUnit:  ai.PricePerInputToken(model),
		PricePerOutputUnit: ai.PricePerOutputToken(model),
		Cost:               0,
		Usage:              models.Usage{},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/threads/runs", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.CONFIG.OpenAIAPIKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	client := sse.NewClientFromReq(req)
	messages := make(chan string)
	go subscribeAndProcess(messages, client, ctx, cancelContext, timeNow, "create_thread_and_run_streaming", usage)
	return messages, nil
}

func CreateRunStreaming(ctx context.Context, assistantId string, model models.Engine, threadId string, cancelContext context.CancelFunc) (chan string, error) {
	if assistantId == "" {
		return nil, fmt.Errorf("assistantId is required")
	}

	if threadId == "" {
		return nil, fmt.Errorf("threadId is required")
	}

	requestBody := struct {
		AssistantId string `json:"assistant_id"`
		Stream      bool   `json:"stream"`
		Model       string `json:"model,omitempty"`
	}{
		AssistantId: assistantId,
		Stream:      true,
		Model:       string(model),
	}

	requestBodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	timeNow := time.Now()
	usage := models.CostAndUsage{
		Engine:             model,
		PricePerInputUnit:  ai.PricePerInputToken(model),
		PricePerOutputUnit: ai.PricePerOutputToken(model),
		Cost:               0,
		Usage:              models.Usage{},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/threads/"+threadId+"/runs", bytes.NewBuffer(requestBodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.CONFIG.OpenAIAPIKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	client := sse.NewClientFromReq(req)
	messages := make(chan string)

	go subscribeAndProcess(messages, client, ctx, cancelContext, timeNow, "create_run_streaming", usage)
	return messages, nil
}

func subscribeAndProcess(
	messages chan string,
	client *sse.Client,
	ctx context.Context,
	cancelContext context.CancelFunc,
	timeNow time.Time,
	apiName string,
	usage models.CostAndUsage,
) {
	status := fmt.Sprintf("status:%d", 0)
	userId := ctx.Value(models.UserContext{}).(string)
	topicId := ctx.Value(models.TopicContext{}).(string)
	defer func() {
		close(messages)
		cancelContext()

		go payments.HugePromptAlarm(ctx, usage)
		go payments.Bill(ctx, usage)
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, "api:" + apiName}, 1)
	}()

	err := client.SubscribeWithContext(ctx, "", func(msg *sse.Event) {
		var response models.StreamDataResponse
		if msg.Event == nil || msg.Data == nil {
			return
		}
		if string(msg.Event) == "done" && string(msg.Data) == "[DONE]" {
			log.Infof("[%s] got [DONE] event for user id %s", apiName, userId)
			return
		}

		if err := json.Unmarshal(msg.Data, &response); err != nil {
			log.Errorf("[%s] couldn't parse event %s, response: %s, err: %v, user id: %s", apiName, string(msg.Event), string(msg.Data), err, userId)
			return // or handle error
		}
		status = fmt.Sprintf("status:%s", response.Status)
		if string(msg.Event) == "thread.created" {
			log.Infof("[%s] got thread.created event for user id %s, saving threadId..", apiName, userId)
			redis.RedisClient.Set(context.Background(), lib.UserCurrentThreadKey(userId, topicId), response.Id, 0)
			return
		}

		if string(msg.Event) == "thread.run.completed" {
			log.Infof("[%s] got thread.run.completed event for user id %s", apiName, userId)
			usage.Usage.PromptTokens = response.Usage.PromptTokens
			usage.Usage.CompletionTokens = response.Usage.CompletionTokens
			usage.Usage.TotalTokens = response.Usage.TotalTokens
			return
		}

		if string(msg.Event) == "thread.message.delta" {
			// this log is spammy
			// log.Infof("[%s] got thread.message.delta event for user id %s", apiName, userId)
			for _, content := range response.Delta.Content {
				if content.Text.Value != "" {
					messages <- content.Text.Value
				}
			}
			return
		}

		if string(msg.Event) == "thread.run.created" {
			log.Infof("[%s] got thread.run.created event for user id %s", apiName, userId)
			return
		}

		log.Debugf("[%s] got event %s for user id %s, skipping processing..", apiName, string(msg.Event), userId)
	})
	if err != nil {
		log.Errorf("[%s] couldn't subscribe: %v, user id: %s", apiName, err, userId)
	}
}
