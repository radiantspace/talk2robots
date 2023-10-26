package models

// Engine is a type for OpenAI API engine
type Engine string

// Engine types
const (
	Ada            Engine = "ada"
	Babbage        Engine = "babbage"
	Curie          Engine = "curie"
	Davinci        Engine = "davinci"
	ChatGpt35Turbo Engine = "gpt-3.5-turbo"
	ChatGpt4       Engine = "gpt-4"
	Whisper        Engine = "whisper-1"
)

type CostAndUsage struct {
	Engine       Engine  `json:"engine"`
	PricePerUnit float64 `json:"price_per_unit"`
	Cost         float64 `json:"cost"`
	Usage        Usage   `json:"usage"`
	User         string  `json:"user"`
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

// Message is a type for OpenAI API message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
