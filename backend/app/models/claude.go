package models

const (
	Haiku3         Engine = "claude-3-haiku-20240307"
	Opus3          Engine = "claude-3-opus-20240229"
	Sonet35        Engine = "claude-3-5-sonnet-20240620"
	Sonet35_241022 Engine = "claude-3-5-sonnet-20241022"
)

type ClaudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type ClaudeMessageInput struct {
	// Temperature defines the amount of randomness injected into the response.
	// Note that even with a temperature of 0.0, results will not be fully
	// deterministic.
	Temperature *float64 `json:"temperature,omitempty"`
	// TopK is used to remove long tail low probability responses by only
	// sampling from the top K options for each subsequent token.
	// Recommended for advanced use cases only. You usually only need to use
	// Temperature.
	TopK *int `json:"top_k,omitempty"`
	// TopP is the nucleus-sampling parameter. Temperature or TopP should be
	// used, but not both.
	// Recommended for advanced use cases only. You usually only need to use
	// Temperature.
	TopP *float64 `json:"top_p,omitempty"`
	// Model defines the language model that will be used to complete the
	// prompt. See model.go for a list of available models.
	Model Engine `json:"model"`
	// System provides a means of specifying context and instructions to the
	// model, such as specifying a particular goal or role.
	System string `json:"system,omitempty"`
	// Messages are the input messages, models are trained to operate on
	// alternating user and assistant conversational turns. When creating a new
	// message, prior conversational turns can be specified with this field,
	// and the model generates the next Message in the conversation.
	Messages []Message `json:"messages"`
	// StopSequences defines custom text sequences that will cause the model to
	// stop generating. If the model encounters any of the sequences, the
	// StopReason field will be set to "stop_sequence" and the response
	// StopSequence field will be set to the sequence that caused the model to
	// stop.
	StopSequences []string `json:"stop_sequences,omitempty"`
	// MaxTokens defines the maximum number of tokens to generate before
	// stopping. Token generation may stop before reaching this limit, this only
	// specifies the absolute maximum number of tokens to generate. Different
	// models have different maximum token limits.
	MaxTokens int `json:"max_tokens"`
}

type ClaudeChatResponse struct {
	Content      []*ClaudeContent `json:"content"`
	ID           *string          `json:"id"`
	Model        *string          `json:"model"`
	Role         *string          `json:"role"`
	StopReason   *string          `json:"stop_reason"`
	StopSequence *string          `json:"stop_sequence"`
	Type         *string          `json:"type"`
	Usage        *ClaudeUsage     `json:"usage"`
}

type ClaudeContent struct {
	Type *string `json:"type"`
	Text *string `json:"text"`
}

type ClaudeStreamEvent struct {
	ContentBlock   *ClaudeContent `json:"content_block"`
	ContentBlockID *int           `json:"content_block_id"`
	Delta          *ClaudeContent `json:"delta"`
	Index          *int           `json:"index"`
	Message        *ClaudeMessage `json:"message"`
	StopReason     *string        `json:"stop_reason"`
	StopSequence   *string        `json:"stop_sequence"`
	Type           *string        `json:"type"`
	Usage          *ClaudeUsage   `json:"usage"`
}

type ClaudeMessage struct {
	ID           *string          `json:"id"`
	Type         *string          `json:"type"`
	Role         *string          `json:"role"`
	Content      []*ClaudeContent `json:"content"`
	Model        *string          `json:"model"`
	StopReason   *string          `json:"stop_reason"`
	StopSequence *string          `json:"stop_sequence"`
	Usage        *ClaudeUsage     `json:"usage"`
}

type ClaudeImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}
