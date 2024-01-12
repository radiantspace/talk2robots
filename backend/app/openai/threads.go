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

// creates a thread and runs it in one request.
func (a *API) CreateThreadRun(ctx context.Context, assistantId string, thread *models.ThreadRequest) (*models.ThreadRunResponse, error) {
	if assistantId == "" {
		return nil, fmt.Errorf("assistantId is required")
	}

	if thread == nil {
		return nil, fmt.Errorf("req is required")
	}

	reqBody, err := json.Marshal(thread)
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

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
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

// get a thread.
func (a *API) GetThread(ctx context.Context, threadId string) (*models.ThreadResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/threads/"+threadId, nil)
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

// Retrieve a run by id.
// GET

// https://api.openai.com/v1/threads/{thread_id}/runs/{run_id}

// Retrieves a run.

// Path parameters
// thread_id
// string
// Required
// The ID of the thread that was run.

// run_id
// string
// Required
// The ID of the run to retrieve.

// Returns
// The run object matching the specified ID.

// Example request
// curl

// curl
// curl https://api.openai.com/v1/threads/thread_abc123/runs/run_abc123 \
//   -H "Authorization: Bearer $OPENAI_API_KEY" \
//   -H "OpenAI-Beta: assistants=v1"
// Response
// {
//   "id": "run_abc123",
//   "object": "thread.run",
//   "created_at": 1699075072,
//   "assistant_id": "asst_abc123",
//   "thread_id": "thread_abc123",
//   "status": "completed",
//   "started_at": 1699075072,
//   "expires_at": null,
//   "cancelled_at": null,
//   "failed_at": null,
//   "completed_at": 1699075073,
//   "last_error": null,
//   "model": "gpt-3.5-turbo",
//   "instructions": null,
//   "tools": [
//     {
//       "type": "code_interpreter"
//     }
//   ],
//   "file_ids": [
//     "file-abc123",
//     "file-abc456"
//   ],
//   "metadata": {}
// }

// retrieve a run by id and threadId.
func (a *API) GetThreadRun(ctx context.Context, threadId, runId string) (*models.ThreadRunResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/threads/"+threadId+"/runs/"+runId, nil)
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

// List run steps by threadId and runId.
func (a *API) ListThreadRunSteps(ctx context.Context, threadId, runId string) (*models.ThreadRunStepsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/threads/"+threadId+"/runs/"+runId+"/steps", nil)
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

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		err = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		return nil, err
	}

	body, err = io.ReadAll(resp.Body)
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

	var threadDeletedResponse models.DeletedResponse
	err = json.Unmarshal(body, &threadDeletedResponse)
	if err != nil {
		return nil, err
	}

	return &threadDeletedResponse, nil
}
