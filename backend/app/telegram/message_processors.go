package telegram

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"talk2robots/m/v2/app/ai"
	"talk2robots/m/v2/app/ai/openai"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/models"
	"talk2robots/m/v2/app/payments"
	"talk2robots/m/v2/app/util"
	"time"

	"github.com/google/uuid"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	log "github.com/sirupsen/logrus"
)

const OOPSIE = "Oopsie, it looks like my AI brain isn't working 🧠🔥. Please try again later."

func ProcessChatCompleteStreamingMessage(
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
		log.Errorf("Failed get streaming response from AI: %s", err)
		_, err = bot.SendMessage(tu.Message(chatID, OOPSIE).WithMessageThreadID(message.MessageThreadID))
		if err != nil {
			log.Errorf("Failed to send error message in chat: %s, %v", chatIDString, err)
		}
		return
	}

	processMessageChannel(ctx, bot, message, messageChannel, userMessagePrimer, mode)
}

func ProcessThreadedStreamingMessage(
	ctx context.Context,
	bot *telego.Bot,
	message *telego.Message,
	mode lib.ModeName,
	engineModel models.Engine,
	cancelContext context.CancelFunc,
) {
	chatID := util.GetChatID(message)
	chatIDString := util.GetChatIDString(message)
	topicID := util.GetTopicID(message)

	if ai.IsFireworksAI(engineModel) || ai.IsClaudeAI(engineModel) {
		ProcessStreamingMessageWithLocalThreads(ctx, bot, message, []models.Message{}, "", mode, engineModel, cancelContext)
		return
	}

	var messages chan string

	threadRunId := ""
	threadId, err := redis.RedisClient.Get(ctx, lib.UserCurrentThreadKey(chatIDString, topicID)).Result()
	if err != nil {
		log.Debugf("Failed to get current thread for chat %s: %s", chatIDString, err)
	}
	if threadId == "" {
		log.Infof("No thread found for chat %s, creating new thread..", chatIDString)

		messages, err = openai.CreateThreadAndRunStreaming(ctx, models.AssistantIdForModel(engineModel), engineModel, &models.Thread{
			Messages: []models.Message{
				{
					Content: message.Text,
					Role:    "user",
				},
			},
		}, cancelContext)

		if err != nil {
			log.Errorf("Failed to create and run thread streaming for user id: %s, error: %v", chatIDString, err)
			bot.SendMessage(tu.Message(chatID, OOPSIE).WithMessageThreadID(message.MessageThreadID))
			return
		}
	} else {
		log.Infof("Found thread %s for chat %s, adding a message..", threadId, chatIDString)

		err = createThreadMessageWithRetries(ctx, threadId, threadRunId, message.Text, chatIDString)
		if err != nil {
			log.Errorf("Failed to add message to thread in chat %s: %s", chatID, err)
			bot.SendMessage(tu.Message(chatID, OOPSIE).WithMessageThreadID(message.MessageThreadID))
			return
		}

		messages, err = openai.CreateRunStreaming(ctx, models.AssistantIdForModel(engineModel), engineModel, threadId, cancelContext)
		if err != nil {
			log.Errorf("Failed to create and run streaming for user id: %s, error: %v", chatIDString, err)
			bot.SendMessage(tu.Message(chatID, OOPSIE).WithMessageThreadID(message.MessageThreadID))
			return
		}
	}

	processMessageChannel(ctx, bot, message, messages, "", mode)
}

func ProcessThreadedNonStreamingMessage(
	ctx context.Context,
	bot *telego.Bot,
	message *telego.Message,
	mode lib.ModeName,
	engineModel models.Engine,
) {
	chatID := util.GetChatID(message)
	chatIDString := util.GetChatIDString(message)
	topicID := util.GetTopicID(message)

	if ai.IsFireworksAI(engineModel) {
		ProcessChatCompleteNonStreamingMessage(ctx, bot, message, []models.Message{}, "", mode, engineModel)
		return
	}

	usage := models.CostAndUsage{
		Engine:             engineModel,
		PricePerInputUnit:  ai.PricePerInputToken(engineModel),
		PricePerOutputUnit: ai.PricePerOutputToken(engineModel),
		Cost:               0,
		Usage:              models.Usage{},
	}
	currentThreadPromptTokens, _ := redis.RedisClient.IncrBy(ctx, lib.UserCurrentThreadPromptKey(chatIDString, topicID), int64(ai.ApproximateTokensCount(message.Text))).Result()
	usage.Usage.PromptTokens = ai.LimitPromptTokensForModel(engineModel, float64(currentThreadPromptTokens))

	payments.HugePromptAlarm(ctx, usage)

	var threadRun *models.ThreadRunResponse
	threadRunId := ""
	threadId, _ := redis.RedisClient.Get(ctx, lib.UserCurrentThreadKey(chatIDString, topicID)).Result()
	if threadId == "" {
		log.Infof("No thread found for chat %s, creating new thread", chatIDString)

		threadRun, err := openai.CreateThreadAndRun(ctx, models.AssistantIdForModel(engineModel), &models.Thread{
			Messages: []models.Message{
				{
					Content: message.Text,
					Role:    "user",
				},
			},
		})
		if err != nil {
			log.Errorf("Failed to create thread: %s", err)
			bot.SendMessage(tu.Message(chatID, OOPSIE).WithMessageThreadID(message.MessageThreadID))
			return
		}
		threadId = threadRun.ThreadID
		threadRunId = threadRun.ID
		redis.RedisClient.Set(ctx, lib.UserCurrentThreadKey(chatIDString, topicID), threadId, 0)
	} else {
		log.Infof("Found thread %s for chat %s, adding a message..", threadId, chatIDString)

		err := createThreadMessageWithRetries(ctx, threadId, threadRunId, message.Text, chatIDString)
		if err != nil {
			log.Errorf("Failed to add message to thread in chat %s: %s", chatID, err)
			bot.SendMessage(tu.Message(chatID, OOPSIE).WithMessageThreadID(message.MessageThreadID))
			return
		}

		threadRun, err = openai.CreateRun(ctx, models.AssistantIdForModel(engineModel), threadId)
		if err != nil {
			log.Errorf("Failed to create run in chat %s: %s", chatIDString, err)
			bot.SendMessage(tu.Message(chatID, OOPSIE).WithMessageThreadID(message.MessageThreadID))
			return
		}
		threadRunId = threadRun.ID
	}

	_, err := pollThreadRun(ctx, threadId, chatIDString, threadRunId)
	if err != nil {
		log.Errorf("Failed to final poll thread run in chat %s: %s", chatIDString, err)
		bot.SendMessage(tu.Message(chatID, OOPSIE).WithMessageThreadID(message.MessageThreadID).WithMessageThreadID(message.MessageThreadID))
		return
	}

	// get messages from thread
	threadMessage, err := openai.ListThreadMessagesForARun(ctx, threadId, threadRunId)
	if err != nil {
		log.Errorf("Failed to get messages from thread in chat %s: %s", chatIDString, err)
		bot.SendMessage(tu.Message(chatID, OOPSIE).WithMessageThreadID(message.MessageThreadID))
		return
	}

	totalContent := ""
	for _, message := range threadMessage {
		for _, content := range message.Content {
			if content.Type == "text" {
				usage.Usage.CompletionTokens += int(ai.ApproximateTokensCount(content.Text.Value))
				totalContent += content.Text.Value

				// increase also current-thread-prompt-tokens, cause it will be used in the next iteration
				_, err := redis.RedisClient.IncrBy(ctx, lib.UserCurrentThreadPromptKey(chatIDString, topicID), int64(usage.Usage.CompletionTokens)).Result()
				if err != nil {
					log.Errorf("Failed to increment current-thread-prompt-tokens in chat %s: %s", chatIDString, err)
				}
			}
		}
		totalContent += "\n"
	}

	usage.Usage.TotalTokens = usage.Usage.PromptTokens + usage.Usage.CompletionTokens
	go payments.Bill(ctx, usage)

	if mode != lib.VoiceGPT {
		ChunkSendMessage(bot, message, totalContent)
	} else {
		ChunkSendVoice(ctx, bot, message, totalContent, true)
	}
}

func ProcessChatCompleteNonStreamingMessage(ctx context.Context, bot *telego.Bot, message *telego.Message, seedData []models.Message, userMessagePrimer string, mode lib.ModeName, engineModel models.Engine) {
	chatID := util.GetChatID(message)
	isPrivate := message.Chat.Type == "private"
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
		log.Errorf("Failed get response from Open AI in chat %s: %s", chatID, err)

		if isPrivate {
			bot.SendMessage(tu.Message(chatID, OOPSIE).WithMessageThreadID(message.MessageThreadID))
		}
		return
	}

	if mode == lib.Teacher || mode == lib.Grammar {
		if strings.Contains(response, "[correct]") {
			log.Infof("Correct message in chat %s 👍", chatID)
			err = bot.SetMessageReaction(&telego.SetMessageReactionParams{
				ChatID:    chatID,
				MessageID: message.MessageID,
				Reaction:  []telego.ReactionType{&telego.ReactionTypeEmoji{Type: "emoji", Emoji: "👍"}},
			})
			if err != nil {
				log.Errorf("Failed to set reaction for message in chat %s: %s", chatID, err)
			}
			return
		}
		response = postprocessMessage(response, mode, userMessagePrimer)

		// split response into two parts: corrected message and explanation, using Explanation: as a separator
		separator := "Explanation:"
		parts := strings.Split(response, separator)
		for _, part := range parts {
			ChunkSendMessage(bot, message, part)
		}
	} else {
		ChunkSendMessage(bot, message, response)
	}
}

func getLikeDislikeReplyMarkup(messageThreadId int) *telego.InlineKeyboardMarkup {
	topicIdString := fmt.Sprintf("%d", messageThreadId)
	// set up inline keyboard for like/dislike buttons
	btnLike := telego.InlineKeyboardButton{Text: "👍", CallbackData: "like:" + topicIdString}
	btnDislike := telego.InlineKeyboardButton{Text: "👎", CallbackData: "dislike:" + topicIdString}
	return &telego.InlineKeyboardMarkup{InlineKeyboard: [][]telego.InlineKeyboardButton{{btnLike, btnDislike}}}
}

func getPendingReplyMarkup() *telego.InlineKeyboardMarkup {
	// set up inline keyboard for like/dislike buttons
	btnPending := telego.InlineKeyboardButton{Text: "🧠", CallbackData: "pending"}
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

	photoMultiModelContent, err := getPhotoBase64(message, ctx, bot)
	if err != nil {
		if strings.Contains(err.Error(), "free plan") {
			bot.SendMessage(&telego.SendMessageParams{
				ChatID:          chatID,
				Text:            "Image vision is not currently available on free plans. Check /upgrade options to use this feature.",
				MessageThreadID: message.MessageThreadID,
			})
		} else {
			bot.SendMessage(&telego.SendMessageParams{
				ChatID:          chatID,
				Text:            "😔 can't accept image messages at the moment",
				MessageThreadID: message.MessageThreadID,
			})
		}
		return nil, engineModel, err
	}

	photoMultiModelContent = append(photoMultiModelContent, models.MultimodalContent{
		Type: "text",
		Text: message.Text + "\n" + message.Caption,
	})

	messages = append(messages, models.MultimodalMessage{
		Role:    "user",
		Content: photoMultiModelContent,
	})

	return messages, engineModel, nil
}

func getPhotoBase64(message *telego.Message, ctx context.Context, bot *telego.Bot) (photoMultiModelContent []models.MultimodalContent, err error) {
	chatIDString := util.GetChatIDString(message)
	photoMultiModelContent = make([]models.MultimodalContent, 0)
	for i, photoSize := range message.Photo {
		// skip all photos but last for now
		if i != len(message.Photo)-1 {
			continue
		}

		photoWidth := photoSize.Width
		log.Infof("Got photo from chat %s, width: %d, height: %d, kbytes: %.1fk", chatIDString, photoWidth, photoSize.Height, float64(photoSize.FileSize/1024))
		var photoFile *telego.File
		photoFile, err = bot.GetFile(&telego.GetFileParams{FileID: photoSize.FileID})
		if err != nil {
			err = fmt.Errorf("failed to get image file params in chat %s: %s", chatIDString, err)
			continue
		}
		fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", bot.Token(), photoFile.FilePath)

		var photoResponse *http.Response
		photoResponse, err = http.Get(fileURL)
		if err != nil {
			err = fmt.Errorf("error downloading image file in chat %s: %v", chatIDString, err)
			continue
		}
		defer photoResponse.Body.Close()

		photoBytes := make([]byte, photoResponse.ContentLength)
		_, err = io.ReadFull(photoResponse.Body, photoBytes)
		if err != nil {
			err = fmt.Errorf("error reading image file in chat %s: %v", chatIDString, err)
			continue
		}
		// resizedPhotoBytes, err := util.ResizeImage(photoBytes, math.Min(float64(photoWidth), 1024))
		// if err != nil {
		// 	return "", fmt.Errorf("error resizing image file in chat %s: %v", chatIDString, err)
		// }

		// if len(resizedPhotoBytes) < len(photoBytes) {
		// 	return base64.StdEncoding.EncodeToString(resizedPhotoBytes), nil
		// }
		// log.Infof("Resized photo is larger than original (%.1fk), sending original photo in chat %s", float64(len(resizedPhotoBytes)/1024), chatIDString)

		photoBase64 := base64.StdEncoding.EncodeToString(photoBytes)
		photoMultiModelContent = append(photoMultiModelContent, models.MultimodalContent{
			Type: "image_url",
			ImageURL: &struct {
				URL string `json:"url,omitempty"`
			}{
				URL: fmt.Sprintf("data:image/jpeg;base64,%s", photoBase64),
			},
		})
	}

	return photoMultiModelContent, nil
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
	_, err := openai.CreateThreadMessage(ctx, threadId, messageBody)
	if err != nil {
		if strings.Contains(err.Error(), "while a run") && strings.Contains(err.Error(), "is active") {
			pollThreadRun(ctx, threadId, chatIDString, runId)
			_, err := openai.CreateThreadMessage(ctx, threadId, messageBody)
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
		threadRun, err := openai.GetLastThreadRun(ctx, threadId)
		if err != nil {
			return nil, err
		}
		runId = threadRun.ID
	}
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Infof("[pollThreadRun] Context cancelled, closing streaming connection in chat: %s", chatIDString)
			return nil, fmt.Errorf("context cancelled in chat %s", chatIDString)
		case <-ticker.C:
			threadRun, err := openai.GetThreadRun(ctx, threadId, runId)
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

// sends a message in up to 4000 chars chunks
func ChunkSendMessage(bot *telego.Bot, message *telego.Message, text string) {
	if text == "" {
		return
	}
	chatID := message.Chat.ChatID()
	for _, chunk := range util.ChunkString(text, 4000) {
		_, err := bot.SendMessage(tu.Message(chatID, chunk).WithMessageThreadID(message.MessageThreadID).WithReplyMarkup(getLikeDislikeReplyMarkup(message.MessageThreadID)))
		if err != nil {
			if strings.Contains(err.Error(), "message thread not found") {
				// retry without message thread id
				_, err = bot.SendMessage(tu.Message(chatID, chunk).WithReplyMarkup(getLikeDislikeReplyMarkup(message.MessageThreadID)))
			}

			if err != nil {
				log.Errorf("Failed to send message to telegram: %v, chatID: %s, threadID: %d", err, chatID, message.MessageThreadID)
			}
		}
		time.Sleep(1 * time.Second)
	}
}

// update current message and sends a new message in up to 4000 chars chunks
func ChunkEditSendMessage(
	ctx context.Context,
	bot *telego.Bot,
	message *telego.Message,
	text string,
	voice bool,
	finalize bool,
) (lastMessage *telego.Message, err error) {
	if text == "" {
		return nil, nil
	}
	chatID := message.Chat.ChatID()
	messageID := message.MessageID
	chunks := util.ChunkString(text, 4000)
	for i, chunk := range chunks {
		last := false
		markup := getLikeDislikeReplyMarkup(message.MessageThreadID)
		if i == len(chunks)-1 && !finalize {
			markup = getPendingReplyMarkup()
			last = true
		}
		if i == 0 {
			log.Debugf("[ChunkEditSendMessage] chunk %d (size %d) - editing message %d in chat %s", i, len(chunk), messageID, chatID)
			params := &telego.EditMessageTextParams{
				ChatID:      chatID,
				MessageID:   messageID,
				Text:        chunk,
				ReplyMarkup: markup,
				ParseMode:   "MarkdownV2",
			}
			_, err = bot.EditMessageText(params)

			if err != nil && strings.Contains(err.Error(), "can't parse entities") {
				params.ParseMode = "HTML"
				_, err = bot.EditMessageText(params)
			}

			if err != nil && strings.Contains(err.Error(), "can't parse entities") {
				params.ParseMode = ""
				_, err = bot.EditMessageText(params)
			}

			time.Sleep(1 * time.Second) // sleep to prevent rate limiting
		} else {
			log.Debugf("[ChunkEditSendMessage] chunk %d (size %d) - sending new message in chat %s", i, len(chunk), chatID)
			lastMessage, err = bot.SendMessage(tu.Message(chatID, chunk).WithParseMode("MarkdownV2").WithMessageThreadID(message.MessageThreadID).WithReplyMarkup(markup))

			if err != nil && strings.Contains(err.Error(), "can't parse entities") {
				lastMessage, err = bot.SendMessage(tu.Message(chatID, chunk).WithParseMode("HTML").WithMessageThreadID(message.MessageThreadID).WithReplyMarkup(markup))
			}

			if err != nil && strings.Contains(err.Error(), "can't parse entities") {
				lastMessage, err = bot.SendMessage(tu.Message(chatID, chunk).WithMessageThreadID(message.MessageThreadID).WithReplyMarkup(markup))
			}

			time.Sleep(1 * time.Second) // sleep to prevent rate limiting
		}
		if !last && voice {
			ChunkSendVoice(ctx, bot, message, chunk, false)
		}
	}
	return lastMessage, err
}

type NamedReader struct {
	io.Reader
	name string
}

func (nr NamedReader) Name() string {
	return nr.name
}

func ChunkSendVoice(ctx context.Context, bot *telego.Bot, message *telego.Message, text string, caption bool) {
	chatID := message.Chat.ChatID()
	for _, chunk := range util.ChunkString(text, 1000) {
		sendAudioAction(bot, message)
		voiceReader, err := openai.CreateSpeech(ctx, &models.TTSRequest{
			Model: models.TTS,
			Input: chunk,
		})
		if err != nil {
			log.Errorf("Failed to get voice message: %v for chatID: %d", err, chatID.ID)
			continue
		}
		defer voiceReader.Close()

		temporaryFileName := uuid.New().String()
		voiceFile := telego.InputFile{
			File: NamedReader{
				Reader: voiceReader,
				name:   temporaryFileName + ".ogg",
			},
		}

		trimmedChunk := chunk
		if len(chunk) > 1000 {
			trimmedChunk = chunk[:1000] + "..."
		}
		voiceParams := &telego.SendVoiceParams{
			ChatID:          chatID,
			Voice:           voiceFile,
			MessageThreadID: message.MessageThreadID,
			ParseMode:       "HTML",
		}
		if caption {
			voiceParams.Caption = trimmedChunk
		}
		_, err = bot.SendVoice(voiceParams.WithReplyMarkup(getLikeDislikeReplyMarkup(message.MessageThreadID)))
		if err != nil && strings.Contains(err.Error(), "can't parse entities") {
			voiceParams.ParseMode = ""
			_, err = bot.SendVoice(voiceParams.WithReplyMarkup(getLikeDislikeReplyMarkup(message.MessageThreadID)))
		}
		time.Sleep(1 * time.Second) // sleep to prevent rate limiting
		if err != nil {
			log.Errorf("Failed to send voice message: %v in chatID: %d", err, chatID.ID)
			continue
		}
	}
}

func postprocessMessage(message string, mode lib.ModeName, userMessagePrimer string) string {
	trimmedResponseText := strings.TrimPrefix(message, "...")
	if mode == lib.Teacher || mode == lib.Grammar {
		// drop primer from response if it was used
		trimmedResponseText = strings.TrimPrefix(trimmedResponseText, userMessagePrimer)

		// change [correct] to ✅
		trimmedResponseText = strings.ReplaceAll(trimmedResponseText, "[correct]", "✅")
	}
	return trimmedResponseText
}

func processMessageChannel(
	ctx context.Context,
	bot *telego.Bot,
	message *telego.Message,
	messageChannel chan string,
	userMessagePrimer string,
	mode lib.ModeName,
) {
	chatID := util.GetChatID(message)
	chatIDString := util.GetChatIDString(message)
	responseText := "..."
	responseMessage, err := bot.SendMessage(tu.Message(chatID, responseText).WithMessageThreadID(message.MessageThreadID).WithReplyMarkup(
		getPendingReplyMarkup(),
	))
	if err != nil {
		log.Errorf("Failed to send primer message in chat: %s, %v", chatIDString, err)
		bot.SendMessage(tu.Message(chatID, OOPSIE).WithMessageThreadID(message.MessageThreadID))
		return
	}
	// only update message every 3 seconds to prevent rate limiting from telegram
	ticker := time.NewTicker(3 * time.Second)
	previousMessageLength := len(responseText)
	defer func() {
		log.Infof("Finalizing message for streaming connection for chat: %s", chatIDString)
		ticker.Stop()
		finalMessageString := postprocessMessage(responseText, mode, userMessagePrimer)

		if finalMessageString == "✅" {
			// if the final message is just a checkmark, delete response and add thumbs up reaction to original message
			err = bot.DeleteMessage(&telego.DeleteMessageParams{
				ChatID:    chatID,
				MessageID: responseMessage.MessageID,
			})
			if err != nil {
				log.Errorf("Failed to delete message in chat: %s, %v", chatIDString, err)
			}
			err = bot.SetMessageReaction(&telego.SetMessageReactionParams{
				ChatID:    chatID,
				MessageID: message.MessageID,
				Reaction:  []telego.ReactionType{&telego.ReactionTypeEmoji{Type: "emoji", Emoji: "👍"}},
			})
			if err != nil {
				log.Errorf("Failed to add reaction to message in chat: %s, %v", chatIDString, err)
			}
		} else {
			_, err = ChunkEditSendMessage(ctx, bot, responseMessage, finalMessageString, mode == lib.VoiceGPT, true)
			if err != nil {
				log.Errorf("Failed to ChunkEditSendMessage message in chat: %s, %v", chatIDString, err)
			}
		}
		if err != nil {
			log.Errorf("Failed to add reply markup to message in chat: %s, %v", chatIDString, err)
		}
	}()
	for {
		select {
		case <-ctx.Done():
			log.Infof("[processMessageChannel] Context cancelled, closing streaming connection in chat: %s", chatIDString)
			return
		case <-ticker.C:
			if previousMessageLength == len(responseText) {
				continue
			}
			previousMessageLength = len(responseText)
			trimmedResponseText := postprocessMessage(responseText, mode, userMessagePrimer)

			var nextMessageObject *telego.Message
			nextMessageObject, err = ChunkEditSendMessage(ctx, bot, responseMessage, trimmedResponseText, mode == lib.VoiceGPT, false)
			if err != nil {
				log.Errorf("Failed to ChunkEditSendMessage message in chat: %s, %v", chatIDString, err)
			}
			if nextMessageObject != nil {
				responseMessage = nextMessageObject
				responseText = nextMessageObject.Text
				nextMessageObject = nil
			}
			if err != nil {
				log.Errorf("Failed to edit message in chat: %s, %v", chatIDString, err)
			}
		case message := <-messageChannel:
			if len(message) == 0 {
				continue
			}
			responseText = strings.TrimPrefix(responseText, "...")
			responseText += message
			log.Debugf("Received message (new size %d, total size %d) in chat: %s", len(message), len(responseText), chatIDString)
		}
	}
}
