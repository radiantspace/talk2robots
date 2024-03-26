package util

import (
	"fmt"
	"os"
	"strings"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/models"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	log "github.com/sirupsen/logrus"
)

func Env(name string, defaultValue ...string) string {
	value, ok := os.LookupEnv(name)
	if !ok && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	Assert(ok, "Environment variable "+name+" not found")
	return value
}

func Assert(ok bool, args ...any) {
	if !ok {
		log.Fatal("Assertion failed, killing app!!!", append([]any{"FATAL:"}, args...))
		os.Exit(1)
	}
}

func GetBotLoggerOption(cfg *config.Config) telego.BotOption {
	if cfg.Environment == "production" {
		return telego.WithDefaultLogger(false, true)
	} else {
		return telego.WithDefaultDebugLogger()
	}
}

func GetChatID(m *telego.Message) telego.ChatID {
	return tu.ID(m.Chat.ID)
}

func GetChatIDString(m *telego.Message) string {
	return fmt.Sprintf("%d", m.Chat.ID)
}

func GetTopicID(m *telego.Message) string {
	if m.Chat.Type != telego.ChatTypeSupergroup {
		return ""
	}
	return fmt.Sprintf("%d", m.MessageThreadID)
}

func GetTopicIDFromChat(c telego.Chat) string {
	if c.Type != telego.ChatTypeSupergroup {
		return ""
	}
	return fmt.Sprintf("%d", c.LinkedChatID)
}

func MessagesToMultimodalMessages(messages []models.Message) []models.MultimodalMessage {
	multimodalMessages := make([]models.MultimodalMessage, len(messages))
	for i, message := range messages {
		multimodalMessages[i] = models.MultimodalMessage{
			Role:    message.Role,
			Content: []models.MultimodalContent{{Type: "text", Text: message.Content}},
		}
	}
	return multimodalMessages
}

func ChunkString(s string, chunkSize int) []string {
	chunks := []string{}
	lines := strings.Split(s, "\n")
	if len(lines) == 0 {
		return chunks
	}

	currentChunk := ""
	for i_line, line := range lines {
		if len(currentChunk)+len(line)+1 > chunkSize && currentChunk != "" {
			chunks = append(chunks, currentChunk)
			currentChunk = ""
		}
		if currentChunk != "" && i_line < len(lines) {
			currentChunk += "\n"
		}

		if len(line) > chunkSize {
			// split current line by words
			words := strings.Fields(line)
			currentChunk = ""
			for _, word := range words {
				if len(currentChunk)+len(word)+1 > chunkSize {
					chunks = append(chunks, currentChunk)
					currentChunk = ""
				}
				if currentChunk != "" {
					currentChunk += " "
				}
				currentChunk += word
			}
			if currentChunk != "" && i_line < len(lines)-1 {
				currentChunk += "\n"
			}
		} else {
			currentChunk += line
		}
	}
	if currentChunk != "" {
		chunks = append(chunks, currentChunk)
	}
	return chunks
}
