package telegram

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/models"
	"talk2robots/m/v2/app/openai"
	"talk2robots/m/v2/app/payments"
	"talk2robots/m/v2/app/util"
	"time"

	"github.com/google/uuid"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	log "github.com/sirupsen/logrus"
)

const OOPSIE = "Oopsie, it looks like my AI brain isn't working 🧠🔥. Please try again later."

func ProcessStreamingMessage(
	ctx context.Context,
	bot *telego.Bot,
	message *telego.Message,
	seedData []models.Message,
	userMessagePrimer string,
	mode lib.ModeName,
	engineModel models.Engine,
	cancelContext context.CancelFunc,
) {
	chatID := util.GetChatID(message)
	chatIDString := util.GetChatIDString(message)
	messages, engineModel, err := prepareMessages(ctx, bot, message, seedData, userMessagePrimer, mode, engineModel)
	if err != nil {
		log.Errorf("Failed to prepare messages: %s", err)
		return
	}

	messageChannel, err := BOT.API.ChatCompleteStreaming(
		ctx,
		models.ChatMultimodalCompletion{
			Model:    string(engineModel),
			Messages: messages,
		},
		cancelContext,
	)

	if err != nil {
		log.Errorf("Failed get streaming response from Open AI: %s", err)
		_, err = bot.SendMessage(tu.Message(chatID, OOPSIE))
		if err != nil {
			log.Errorf("Failed to send error message in chat: %s, %v", chatIDString, err)
		}
		return
	}

	responseText := "🧠: "
	responseMessage, err := bot.SendMessage(tu.Message(chatID, responseText).WithReplyMarkup(
		getPendingReplyMarkup(),
	))
	if err != nil {
		log.Errorf("Failed to send primer message in chat: %s, %v", chatIDString, err)
	}
	// only update message every 5 seconds to prevent rate limiting from telegram
	ticker := time.NewTicker(5 * time.Second)
	previousMessageLength := len(responseText)
	defer func() {
		log.Infof("Finalizing message for streaming connection for chat: %s", chatIDString)
		ticker.Stop()
		finalMessageParams := telego.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   responseMessage.MessageID,
			Text:        responseText,
			ReplyMarkup: getLikeDislikeReplyMarkup(),
		}
		_, err = bot.EditMessageText(&finalMessageParams)
		if err != nil {
			log.Errorf("Failed to add reply markup to message in chat: %s, %v", chatIDString, err)
		}
	}()
	for {
		select {
		case <-ctx.Done():
			log.Infof("Context cancelled, closing streaming connection in chat: %s", chatIDString)
			return
		case <-ticker.C:
			if previousMessageLength == len(responseText) {
				continue
			}
			previousMessageLength = len(responseText)
			trimmedResponseText := responseText
			if mode == lib.Teacher || mode == lib.Grammar {
				// drop primer from response if it was used
				trimmedResponseText = strings.TrimPrefix(responseText, userMessagePrimer)
			}
			// TODO: split into multiple messages if too long
			if len(trimmedResponseText) > 4000 {
				trimmedResponseText = trimmedResponseText[:4000] + "... (truncated, since telegram has a 4096 character limit)"
			}
			_, err = bot.EditMessageText(&telego.EditMessageTextParams{
				ChatID:      chatID,
				MessageID:   responseMessage.MessageID,
				Text:        trimmedResponseText,
				ReplyMarkup: getPendingReplyMarkup(),
			})
			if err != nil {
				log.Errorf("Failed to edit message in chat: %s, %v", chatIDString, err)
			}
		case message := <-messageChannel:
			log.Debugf("Sending message: %s, in chat: %s", message, chatIDString)
			responseText += message
		}
	}
}

func ProcessThreadedMessage(
	ctx context.Context,
	bot *telego.Bot,
	message *telego.Message,
	mode lib.ModeName,
	engineModel models.Engine,
) {
	chatID := util.GetChatID(message)
	chatIDString := util.GetChatIDString(message)

	usage := models.CostAndUsage{
		Engine:             engineModel,
		PricePerInputUnit:  openai.PricePerInputToken(engineModel),
		PricePerOutputUnit: openai.PricePerOutputToken(engineModel),
		Cost:               0,
		Usage:              models.Usage{},
	}
	usage.Usage.PromptTokens = int(openai.ApproximateTokensCount(message.Text))

	var threadRun *models.ThreadRunResponse
	threadRunId := ""
	threadId, err := redis.RedisClient.Get(ctx, chatIDString+":current-thread").Result()
	if threadId == "" {
		log.Infof("No thread found for chat %s, creating new thread", chatIDString)

		threadRun, err = BOT.API.CreateThreadAndRun(ctx, models.AssistantIdForModel(engineModel), &models.Thread{
			Messages: []models.Message{
				{
					Content: message.Text,
					Role:    "user",
				},
			},
		})
		if err != nil {
			log.Errorf("Failed to create thread: %s", err)
			bot.SendMessage(tu.Message(chatID, OOPSIE))
			return
		}
		threadId = threadRun.ThreadID
		threadRunId = threadRun.ID
		redis.RedisClient.Set(ctx, chatIDString+":current-thread", threadId, 0)
	} else {
		log.Infof("Found thread %s for chat %s, adding a message..", threadId, chatIDString)

		err := createThreadMessageWithRetries(ctx, threadId, threadRunId, message.Text, chatIDString)
		if err != nil {
			log.Errorf("Failed to add message to thread in chat %s: %s", chatID, err)
			bot.SendMessage(tu.Message(chatID, OOPSIE))
			return
		}

		threadRun, err = BOT.API.CreateRun(ctx, models.AssistantIdForModel(engineModel), threadId)
		if err != nil {
			log.Errorf("Failed to create run in chat %s: %s", chatIDString, err)
			bot.SendMessage(tu.Message(chatID, OOPSIE))
			return
		}
		threadRunId = threadRun.ID
	}

	_, err = pollThreadRun(ctx, threadRun.ThreadID, chatIDString, threadRunId)
	if err != nil {
		log.Errorf("Failed to final poll thread run in chat %s: %s", chatIDString, err)
		bot.SendMessage(tu.Message(chatID, OOPSIE))
		return
	}

	// get messages from thread
	threadMessage, err := BOT.API.ListThreadMessagesForARun(ctx, threadRun.ThreadID, threadRunId)
	if err != nil {
		log.Errorf("Failed to get messages from thread in chat %s: %s", chatIDString, err)
		bot.SendMessage(tu.Message(chatID, OOPSIE))
		return
	}

	totalContent := ""
	if mode != lib.VoiceGPT {
		totalContent = "🧠: "
	}

	for _, message := range threadMessage {
		for _, content := range message.Content {
			if content.Type == "text" {
				usage.Usage.CompletionTokens += int(openai.ApproximateTokensCount(content.Text.Value))
				totalContent += content.Text.Value
			}
		}
		totalContent += "\n"
	}

	usage.Usage.TotalTokens = usage.Usage.PromptTokens + usage.Usage.CompletionTokens
	go payments.Bill(ctx, usage)

	if mode != lib.VoiceGPT {
		util.TelegramChunkSendMessage(bot, chatID, totalContent)
	} else {
		TelegramChunkSendVoice(ctx, bot, chatID, totalContent)
	}
}

func ProcessNonStreamingMessage(ctx context.Context, bot *telego.Bot, message *telego.Message, seedData []models.Message, userMessagePrimer string, mode lib.ModeName, engineModel models.Engine) {
	chatID := util.GetChatID(message)
	chatIDString := util.GetChatIDString(message)
	response, err := BOT.API.ChatComplete(ctx, models.ChatCompletion{
		Model: string(engineModel),
		Messages: []models.Message(append(
			seedData,
			models.Message{
				Role:    "user",
				Content: userMessagePrimer + message.Text,
			},
		)),
	})
	if err != nil {
		log.Errorf("Failed get response from Open AI: %s", err)
		bot.SendMessage(tu.Message(chatID, "Oopsie, it looks like my AI brain isn't working 🧠🔥. Please try again later 🕜."))
		return
	}

	if mode == lib.Teacher || mode == lib.Grammar {
		// drop primer from response if it was used
		response = strings.TrimPrefix(response, userMessagePrimer)

		// split response into two parts: corrected message and explanation, using Explanation: as a separator
		separator := "Explanation:"
		parts := strings.Split(response, separator)
		for i, part := range parts {
			if len(part) > 4000 {
				part = part[:4000] + "... (truncated, since telegram has a 4096 character limit)"
			}
			log.Debugf("Sending part %d: %s", i, part)
			message := tu.Message(chatID, strings.Trim(part, "\n")).WithReplyMarkup(getLikeDislikeReplyMarkup())
			_, err = bot.SendMessage(message)
		}
		if err != nil {
			log.Errorf("Failed to send message in chat %s: %s", chatIDString, err)
		}
	} else {
		if len(response) > 4000 {
			response = response[:4000] + "... (truncated, since telegram has a 4096 character limit)"
		}
		bot.SendMessage(tu.Message(chatID, "🧠: "+response).WithReplyMarkup(getLikeDislikeReplyMarkup()))
	}
}

func getLikeDislikeReplyMarkup() *telego.InlineKeyboardMarkup {
	// set up inline keyboard for like/dislike buttons
	btnLike := telego.InlineKeyboardButton{Text: "👍", CallbackData: "like"}
	btnDislike := telego.InlineKeyboardButton{Text: "👎", CallbackData: "dislike"}
	return &telego.InlineKeyboardMarkup{InlineKeyboard: [][]telego.InlineKeyboardButton{{btnLike, btnDislike}}}
}

func getPendingReplyMarkup() *telego.InlineKeyboardMarkup {
	// set up inline keyboard for like/dislike buttons
	btnPending := telego.InlineKeyboardButton{Text: "...", CallbackData: "pending"}
	return &telego.InlineKeyboardMarkup{InlineKeyboard: [][]telego.InlineKeyboardButton{{btnPending}}}
}

func prepareMessages(
	ctx context.Context,
	bot *telego.Bot,
	message *telego.Message,
	seedData []models.Message,
	userMessagePrimer string,
	mode lib.ModeName,
	engineModel models.Engine,
) (messages []models.MultimodalMessage, model models.Engine, err error) {
	chatID := util.GetChatID(message)
	messages = util.MessagesToMultimodalMessages(seedData)

	// check if message had an image attachments and pass it on in base64 format to the model
	if message.Photo == nil || len(message.Photo) == 0 {
		messages = append(messages, models.MultimodalMessage{
			Role:    "user",
			Content: []models.MultimodalContent{{Type: "text", Text: userMessagePrimer + message.Text}},
		},
		)
		return messages, engineModel, nil
	}

	photoBase64, err := getPhotoBase64(message, ctx, bot)
	if err != nil {
		if strings.Contains(err.Error(), "free plan") {
			bot.SendMessage(&telego.SendMessageParams{
				ChatID: chatID,
				Text:   "Image vision is not currently available on free plans, since it's kinda expensive. Please /upgrade to use this feature.",
			})
		} else {
			bot.SendMessage(&telego.SendMessageParams{
				ChatID: chatID,
				Text:   "😔 can't accept image messages at the moment",
			})
		}
		return nil, engineModel, err
	}

	messages = append(messages, models.MultimodalMessage{
		Role: "user",
		Content: []models.MultimodalContent{
			{
				Type: "text",
				Text: userMessagePrimer + message.Text + "\n" + message.Caption,
			},
			{
				Type: "image_url",
				ImageURL: struct {
					URL string `json:"url"`
				}{
					URL: fmt.Sprintf("data:image/jpeg;base64,%s", photoBase64),
				},
			},
		},
	})

	return messages, models.ChatGpt4TurboVision, nil
}

func getPhotoBase64(message *telego.Message, ctx context.Context, bot *telego.Bot) (photoBase64 string, err error) {
	chatIDString := util.GetChatIDString(message)
	// photo message detected, check user's subscription status
	if lib.IsUserFree(ctx) || lib.IsUserFreePlus(ctx) {
		return "", fmt.Errorf("user %s tried to use image vision on free plan", chatIDString)
	}

	// get the last image for now
	photo := message.Photo
	photoSize := photo[len(photo)-1]
	var photoFile *telego.File
	photoFile, err = bot.GetFile(&telego.GetFileParams{FileID: photoSize.FileID})
	if err != nil {
		return "", fmt.Errorf("failed to get image file params in chat %s: %s", chatIDString, err)
	}
	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", bot.Token(), photoFile.FilePath)

	var photoResponse *http.Response
	photoResponse, err = http.Get(fileURL)
	if err != nil {
		return "", fmt.Errorf("error downloading image file in chat %s: %v", chatIDString, err)
	}
	defer photoResponse.Body.Close()

	photoBytes := make([]byte, photoResponse.ContentLength)
	_, err = io.ReadFull(photoResponse.Body, photoBytes)
	if err != nil {
		return "", fmt.Errorf("error reading image file in chat %s: %v", chatIDString, err)
	}
	return base64.StdEncoding.EncodeToString(photoBytes), nil
}

func createThreadMessageWithRetries(
	ctx context.Context,
	threadId string,
	runId string,
	message string,
	chatIDString string,
) error {
	messageBody := &models.Message{
		Content: message,
		Role:    "user",
	}
	_, err := BOT.API.CreateThreadMessage(ctx, threadId, messageBody)
	if err != nil {
		if strings.Contains(err.Error(), "while a run") && strings.Contains(err.Error(), "is active") {
			pollThreadRun(ctx, threadId, chatIDString, runId)
			_, err := BOT.API.CreateThreadMessage(ctx, threadId, messageBody)
			return err
		} else {
			return err
		}
	}

	return nil
}

func pollThreadRun(ctx context.Context, threadId string, chatIDString string, runId string) (*models.ThreadRunResponse, error) {
	ticker := time.NewTicker(2 * time.Second)
	if runId == "" {
		threadRun, err := BOT.API.GetLastThreadRun(ctx, threadId)
		if err != nil {
			return nil, err
		}
		runId = threadRun.ID
	}
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Infof("Context cancelled, closing streaming connection in chat: %s", chatIDString)
			return nil, fmt.Errorf("context cancelled in chat %s", chatIDString)
		case <-ticker.C:
			threadRun, err := BOT.API.GetThreadRun(ctx, threadId, runId)
			if err != nil {
				return nil, err
			}
			// The status of the run, which can be either queued, in_progress, requires_action, cancelling, cancelled, failed, completed, or expired.
			switch threadRun.Status {
			case "in_progress":
				continue
			case "completed":
				log.Infof("Thread %s completed for chat %s", threadRun.ThreadID, chatIDString)
				return threadRun, nil
			default:
				return nil, fmt.Errorf("thread %s failed for chat %s with state %s", threadRun.ThreadID, chatIDString, threadRun.Status)
			}
		}
	}
}

type NamedReader struct {
	io.Reader
	name string
}

func (nr NamedReader) Name() string {
	return nr.name
}

func TelegramChunkSendVoice(ctx context.Context, bot *telego.Bot, chatID telego.ChatID, text string) {
	for _, chunk := range util.ChunkString(text, 4000) {
		voiceBytes, err := BOT.API.CreateSpeech(ctx, &models.TTSRequest{
			Model: models.TTS,
			Input: chunk,
		})
		if err != nil {
			log.Errorf("Failed to get voice bytes: %s", err)
			continue
		}

		temporaryFileName := uuid.New().String()
		voiceFile := telego.InputFile{
			File: NamedReader{
				Reader: bytes.NewReader(voiceBytes),
				name:   temporaryFileName + ".ogg",
			},
		}

		_, err = bot.SendVoice(&telego.SendVoiceParams{
			ChatID:  chatID,
			Voice:   voiceFile,
			Caption: chunk,
		})
		if err != nil {
			log.Errorf("Failed to send voice message: %s", err)
			continue
		}
	}
}
