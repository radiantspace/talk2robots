package util

import (
	"encoding/base64"
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
	return fmt.Sprintf("%d", c.ID)
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

func IsAudioMessage(message *telego.Message) (bool, string) {
	if message.Voice != nil || message.Audio != nil || message.Video != nil || message.VideoNote != nil || message.Document != nil {
		voice_type := "voice"
		switch {
		case message.Audio != nil:
			voice_type = "audio"
		case message.Video != nil:
			voice_type = "video"
		case message.VideoNote != nil:
			voice_type = "note"
		case message.Document != nil:
			voice_type = "document"

			if !strings.HasPrefix(message.Document.MimeType, "audio/") && !strings.HasPrefix(message.Document.MimeType, "video/") {
				chatIDString := GetChatIDString(message)
				log.Warnf("Ignoring non-audio document message in chat %s, mimetype: %s", chatIDString, message.Document.MimeType)
				return false, ""
			}
		}
		return true, voice_type
	}
	return false, ""
}

func SafeOsDelete(filename string) {
	// test file does not exist
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return
	}

	err := os.Remove(filename)
	if err != nil {
		log.Errorf("Error deleting file %s: %v", filename, err)
	}
}

func Base64Decode(s string) string {
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		log.Errorf("Error decoding base64 string: %v", err)
		return ""
	}
	return string(decoded)
}
