package models

// Engine is a type for OpenAI API engine
type Engine string

// Engine types
const (
	Ada                 Engine = "ada"
	Babbage             Engine = "babbage"
	Curie               Engine = "curie"
	Davinci             Engine = "davinci"
	ChatGpt35Turbo      Engine = "gpt-3.5-turbo-1106"
	ChatGpt4            Engine = "gpt-4"
	ChatGpt4TurboVision Engine = "gpt-4-vision-preview"
	Whisper             Engine = "whisper-1"
	TTS                 Engine = "tts-1"
)

type CostAndUsage struct {
	Engine             Engine  `json:"engine"`
	PricePerInputUnit  float64 `json:"price_per_input_unit"`
	PricePerOutputUnit float64 `json:"price_per_output_unit"`
	Cost               float64 `json:"cost"`
	Usage              Usage   `json:"usage"`
	User               string  `json:"user"`
}

// Completion is a type for OpenAI API completion
type Completion struct {
	Engine           Engine
	Prompt           string
	MaxTokens        int
	Temperature      float64
	TopP             float64
	FrequencyPenalty float64
	PresencePenalty  float64
	Stop             []string
}

type TTSRequest struct {
	Model Engine
	Input string
	Voice string
}

// Response is a type for OpenAI API response
type CompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []Choice
}

// Choice is a type for OpenAI API choice
type Choice struct {
	Text         string `json:"text"`
	Index        int    `json:"index"`
	LogProbs     LogProbs
	FinishReason string `json:"finish_reason"`
}

// LogProbs is a type for OpenAI API log probs
type LogProbs struct {
	TokenLogProbs []TokenLogProbs
}

// TokenLogProbs is a type for OpenAI API token log probs
type TokenLogProbs struct {
	TokenID           int     `json:"token_id"`
	Token             string  `json:"token"`
	LogProb           float64 `json:"log_prob"`
	NormalizedLogProb float64 `json:"normalized_log_prob"`
}

// ChatCompletion is a type for OpenAI API chat completion
type ChatCompletion struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`

	// optional
	MaxTokens int `json:"max_tokens,omitempty"`
}

type ChatMultimodalCompletion struct {
	Model    string              `json:"model"`
	Messages []MultimodalMessage `json:"messages"`

	// optional
	MaxTokens int `json:"max_tokens,omitempty"`
}

// Message is a type for OpenAI API message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// MultimodalCompletion is a type for OpenAI API multimodal completion
type MultimodalMessage struct {
	Role    string              `json:"role"`
	Content []MultimodalContent `json:"content"`
}

type MultimodalContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL struct {
		URL string `json:"url"`
	} `json:"image_url,omitempty"`
}

// ChatResponse is a type for OpenAI API chat response
type ChatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Choices []ChatChoice `json:"choices"`
	Usage   Usage        `json:"usage"`
}

type ChatChoice struct {
	Delta        Delta  `json:"delta,omitempty"`
	FinishReason string `json:"finish_reason"`
	Index        int    `json:"index"`
	LogProbs     LogProbs
	Message      Message `json:"message"`
}

type Delta struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Usage is a type for OpenAI API usage
type Usage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	AudioDuration    float64 `json:"audio_duration"` // only for whisper API
}

type ThreadRequest struct {
	Messages []MultimodalMessage `json:"messages"`
	Metadata struct {
	} `json:"metadata"`
}

type ThreadResponse struct {
	ID        string `json:"id"`
	Object    string `json:"object"`
	CreatedAt int64  `json:"created_at"`
	Metadata  struct {
	} `json:"metadata"`
}

type ThreadMessageResponse struct {
	ID          string              `json:"id"`
	Object      string              `json:"object"`
	CreatedAt   int64               `json:"created_at"`
	ThreadID    string              `json:"thread_id"`
	Role        string              `json:"role"`
	Content     []MultimodalContent `json:"content"`
	FileIDs     []string            `json:"file_ids"`
	AssistantID string              `json:"assistant_id"`
	RunID       string              `json:"run_id"`
	Metadata    struct {
	} `json:"metadata"`
}

type ThreadMessage struct {
	ID          string              `json:"id"`
	Object      string              `json:"object"`
	CreatedAt   int64               `json:"created_at"`
	ThreadID    string              `json:"thread_id"`
	Role        string              `json:"role"`
	Content     []MultimodalContent `json:"content"`
	FileIDs     []string            `json:"file_ids"`
	AssistantID string              `json:"assistant_id"`
	RunID       string              `json:"run_id"`
	Metadata    string              `json:"metadata"`
}

type MessageFile struct {
	ID        string `json:"id"`
	Object    string `json:"object"`
	CreatedAt int64  `json:"created_at"`
	MessageID string `json:"message_id"`
	FileID    string `json:"file_id"`
}

type AssistantRequest struct {
	Model        string          `json:"model"`
	Name         string          `json:"name"`
	Tools        []AssistantTool `json:"tools"`
	Instructions string          `json:"instructions"`
	FileIDs      []string        `json:"file_ids"`
	Metadata     struct{}        `json:"metadata"`
	Description  string          `json:"description"`
}

type AssistantTool struct {
	Type string `json:"type"`
}

type AssistantResponse struct {
	ID           string          `json:"id"`
	Object       string          `json:"object"`
	CreatedAt    int64           `json:"created_at"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Model        string          `json:"model"`
	Instructions string          `json:"instructions"`
	Tools        []AssistantTool `json:"tools"`
	FileIDs      []string        `json:"file_ids"`
	Metadata     struct {
	} `json:"metadata"`
}

type ThreadRunResponse struct {
	ID           string          `json:"id"`
	Object       string          `json:"object"`
	CreatedAt    int64           `json:"created_at"`
	AssistantID  string          `json:"assistant_id"`
	ThreadID     string          `json:"thread_id"`
	Status       string          `json:"status"`
	StartedAt    int64           `json:"started_at"`
	ExpiresAt    int64           `json:"expires_at"`
	CancelledAt  int64           `json:"cancelled_at"`
	FailedAt     int64           `json:"failed_at"`
	CompletedAt  int64           `json:"completed_at"`
	LastError    string          `json:"last_error"`
	Model        string          `json:"model"`
	Instructions string          `json:"instructions"`
	Tools        []AssistantTool `json:"tools"`
	FileIDs      []string        `json:"file_ids"`
	Metadata     struct{}        `json:"metadata"`
}

type ThreadRunStepsResponse struct {
	Object  string          `json:"object"`
	Data    []ThreadRunStep `json:"data"`
	FirstID string          `json:"first_id"`
	LastID  string          `json:"last_id"`
	HasMore bool            `json:"has_more"`
}

type ThreadRunStep struct {
	ID          string      `json:"id"`
	Object      string      `json:"object"`
	CreatedAt   int64       `json:"created_at"`
	RunID       string      `json:"run_id"`
	AssistantID string      `json:"assistant_id"`
	ThreadID    string      `json:"thread_id"`
	Type        string      `json:"type"`
	Status      string      `json:"status"`
	CancelledAt int64       `json:"cancelled_at"`
	CompletedAt int64       `json:"completed_at"`
	ExpiredAt   int64       `json:"expired_at"`
	FailedAt    int64       `json:"failed_at"`
	LastError   string      `json:"last_error"`
	StepDetails StepDetails `json:"step_details"`
}

type StepDetails struct {
	Type            string                     `json:"type"`
	MessageCreation MessageCreationStepDetails `json:"message_creation"`
	ToolCalls       []ToolCallStepDetails      `json:"tool_calls"`
}

type MessageCreationStepDetails struct {
	MessageID string `json:"message_id"`
}

type ToolCallStepDetails struct {
	ID              string                             `json:"id"`
	Type            string                             `json:"type"`
	CodeInterpreter CodeInterpreterToolCallStepDetails `json:"code_interpreter"`
	Retrieval       RetrievalToolCallStepDetails       `json:"retrieval"`
}

type CodeInterpreterToolCallStepDetails struct {
	Input   string `json:"input"`
	Outputs []struct {
		Type string `json:"type"`
		Logs string `json:"logs"`
	} `json:"outputs"`
}

type RetrievalToolCallStepDetails struct {
}

type DeletedResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Deleted bool   `json:"deleted"`
}
