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
	"time"
)

// creates a thread and runs it in one request.
func (a *API) CreateThreadAndRun(ctx context.Context, assistantId string, thread *models.Thread) (*models.ThreadRunResponse, error) {
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
	req.Header.Set("Authorization", "Bearer "+a.authToken)
	req.Header.Set("OpenAI-Beta", "assistants=v1")

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:create_thread_and_run"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := a.client.Do(req)
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
func (a *API) CreateRun(ctx context.Context, assistantId string, threadId string) (*models.ThreadRunResponse, error) {
	if assistantId == "" {
		return nil, fmt.Errorf("assistantId is required")
	}

	if threadId == "" {
		return nil, fmt.Errorf("threadId is required")
	}

	requestBody := struct {
		AssistantId string `json:"assistant_id"`
		// model
		// string or null
		// Optional
		// The ID of the Model to be used to execute this run. If a value is provided here, it will override the model associated with the assistant. If not, the model associated with the assistant will be used.

		// instructions
		// string or null
		// Optional
		// Overrides the instructions of the assistant. This is useful for modifying the behavior on a per-run basis.

		// additional_instructions
		// string or null
		// Optional
		// Appends additional instructions at the end of the instructions for the run. This is useful for modifying the behavior on a per-run basis without overriding other instructions.

		// tools
		// array or null
		// Optional
		// Override the tools the assistant can use for this run. This is useful for modifying the behavior on a per-run basis.

		// Show possible types
		// metadata
		// map
		// Optional
		// Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.
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
	req.Header.Set("Authorization", "Bearer "+a.authToken)
	req.Header.Set("OpenAI-Beta", "assistants=v1")

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:create_run"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := a.client.Do(req)
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
func (a *API) GetThread(ctx context.Context, threadId string) (*models.ThreadResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/threads/"+threadId, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.authToken)
	req.Header.Set("OpenAI-Beta", "assistants=v1")

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:get_thread"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := a.client.Do(req)
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
func (a *API) CancelRun(ctx context.Context, threadId, runId string) (*models.ThreadRunResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/threads/"+threadId+"/runs/"+runId+"/cancel", nil)
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
func (a *API) GetThreadRun(ctx context.Context, threadId, runId string) (*models.ThreadRunResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/threads/"+threadId+"/runs/"+runId, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.authToken)
	req.Header.Set("OpenAI-Beta", "assistants=v1")

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:get_thread_run"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := a.client.Do(req)
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
func (a *API) GetLastThreadRun(ctx context.Context, threadId string) (*models.ThreadRunResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/threads/"+threadId+"/runs", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.authToken)
	req.Header.Set("OpenAI-Beta", "assistants=v1")

	q := req.URL.Query()
	q.Add("limit", fmt.Sprintf("%d", 1))
	req.URL.RawQuery = q.Encode()

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:get_last_thread_run"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := a.client.Do(req)
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
func (a *API) ListThreadRunSteps(ctx context.Context, threadId, runId string) (*models.ThreadRunStepsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/threads/"+threadId+"/runs/"+runId+"/steps", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.authToken)
	req.Header.Set("OpenAI-Beta", "assistants=v1")

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:list_thread_run_steps"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := a.client.Do(req)
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
func (a *API) CreateThreadMessage(ctx context.Context, threadId string, message *models.Message) (*models.ThreadMessageResponse, error) {
	body, err := json.Marshal(message)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/threads/"+threadId+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.authToken)
	req.Header.Set("OpenAI-Beta", "assistants=v1")

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:create_thread_message"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := a.client.Do(req)
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
func (a *API) GetThreadMessage(ctx context.Context, threadId, messageId string) (*models.ThreadMessageResponse, error) {
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
	req.Header.Set("Authorization", "Bearer "+a.authToken)
	req.Header.Set("OpenAI-Beta", "assistants=v1")

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:get_thread_message"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := a.client.Do(req)
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

func (a *API) DeleteThread(ctx context.Context, threadId string) (*models.DeletedResponse, error) {
	if threadId == "" {
		return nil, fmt.Errorf("threadId is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, "https://api.openai.com/v1/threads/"+threadId, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.authToken)
	req.Header.Set("OpenAI-Beta", "assistants=v1")

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:delete_thread"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := a.client.Do(req)
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

func (a *API) ListLastThreadMessages(ctx context.Context, threadId string) (*models.ThreadMessageResponse, error) {
	if threadId == "" {
		return nil, fmt.Errorf("threadId is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/threads/"+threadId+"/messages", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.authToken)
	req.Header.Set("OpenAI-Beta", "assistants=v1")

	q := req.URL.Query()
	q.Add("limit", fmt.Sprintf("%d", 1))
	req.URL.RawQuery = q.Encode()

	timeNow := time.Now()
	status := fmt.Sprintf("status:%d", 0)
	api_name := "api:list_last_thread_messages"
	defer func() {
		config.CONFIG.DataDogClient.Timing("openai.threads.latency", time.Since(timeNow), []string{status, api_name}, 1)
	}()

	resp, err := a.client.Do(req)
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

	if threadMessagesResponse.Data[0].Role != "assistant" {
		return &models.ThreadMessageResponse{}, nil
	}

	return &threadMessagesResponse.Data[0], nil
}
