package claude

import (
	"strings"
	"talk2robots/m/v2/app/models"
)

// extract system prompts from completion messages
func Convert(completion models.ChatCompletion) (models.ChatCompletion, string) {
	messagesWithoutSystem := []models.Message{}
	systemPrompt := ""

	for _, message := range completion.Messages {
		if message.Role != "system" {
			messagesWithoutSystem = append(messagesWithoutSystem, message)
		} else {
			systemPrompt += message.Content + "\n\n"
		}
	}

	return models.ChatCompletion{
		Model:     completion.Model,
		Messages:  messagesWithoutSystem,
		MaxTokens: completion.MaxTokens,
	}, systemPrompt
}

func ConvertMultimodal(completion models.ChatMultimodalCompletion) (models.ChatMultimodalCompletion, string) {
	// 1. Note that if you want to include a system prompt, you can use the top-level system parameter — there is no "system" role for input messages in the Messages API.

	// filter messages where type is not system
	messagesWithoutSystem := []models.MultimodalMessage{}
	systemPrompt := ""
	for _, message := range completion.Messages {
		if message.Role != "system" {
			messageWithoutEmptyText := []models.MultimodalContent{}
			for _, content := range message.Content {
				if content.Type == "text" && strings.TrimSpace(content.Text) != "" {
					// Claude AI is very sensitive to whitespace only text
					messageWithoutEmptyText = append(messageWithoutEmptyText, content)
				} else if content.Type == "image_url" {
					// Convert image_url fmt.Sprintf("data:image/jpeg;base64,%s", photoBase64),
					// to ClaudeImageSource struct
					messageWithoutEmptyText = append(messageWithoutEmptyText, models.MultimodalContent{
						Type: "image",
						Source: &models.ClaudeImageSource{
							Type:      "base64",
							MediaType: "image/jpeg",
							Data:      strings.TrimPrefix(content.ImageURL.URL, "data:image/jpeg;base64,"),
						},
					})
				} else if content.Type != "text" {
					messageWithoutEmptyText = append(messageWithoutEmptyText, content)
				}
			}
			if len(messageWithoutEmptyText) > 0 {
				messagesWithoutSystem = append(messagesWithoutSystem, models.MultimodalMessage{
					Role:    message.Role,
					Content: messageWithoutEmptyText,
				})
			}
		} else {
			for _, content := range message.Content {
				systemPrompt += content.Text + "\n\n"
			}
		}
	}

	// 2. messages: roles must alternate between \\\"user\\\" and \\\"assistant\\\", but found multiple \\\"user\\\" roles in a row
	messagesWithAlternateRoles := []models.MultimodalMessage{}
	for i := 0; i < len(messagesWithoutSystem)-1; i++ {
		messagesWithAlternateRoles = append(messagesWithAlternateRoles, messagesWithoutSystem[i])
		if messagesWithoutSystem[i].Role == messagesWithoutSystem[i+1].Role {
			dummyRole := "assistant"
			dummyMessage := "Please continue."
			if messagesWithoutSystem[i].Role == "assistant" {
				dummyRole = "user"
				dummyMessage = "Let me try again."
			}
			messagesWithAlternateRoles = append(messagesWithAlternateRoles, models.MultimodalMessage{
				Role:    dummyRole,
				Content: []models.MultimodalContent{{Type: "text", Text: dummyMessage}},
			})
		}
	}
	messagesWithAlternateRoles = append(messagesWithAlternateRoles, messagesWithoutSystem[len(messagesWithoutSystem)-1])

	return models.ChatMultimodalCompletion{
		Model:     completion.Model,
		Messages:  messagesWithAlternateRoles,
		MaxTokens: completion.MaxTokens,
	}, systemPrompt
}
