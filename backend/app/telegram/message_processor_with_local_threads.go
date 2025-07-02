package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/db/mongo"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/models"
	"talk2robots/m/v2/app/util"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	log "github.com/sirupsen/logrus"
)

func ProcessStreamingMessageWithLocalThreads(
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
	messages, isNewThread, err := prepareMessagesForLocalThread(ctx, bot, message)
	if err != nil {
		log.Errorf("[ProcessThreadedStreamingMessage] Failed to prepare messages in chat: %s, %v", chatIDString, err)
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
		log.Errorf("[ProcessThreadedStreamingMessage] Failed get streaming response from AI in chat: %s, %v", chatIDString, err)
		_, err = bot.SendMessage(ctx, tu.Message(chatID, OOPSIE).WithMessageThreadID(message.MessageThreadID))
		if err != nil {
			log.Errorf("[ProcessThreadedStreamingMessage] Failed to send error message in chat: %s, %v", chatIDString, err)
		}
		return
	}

	processMessageChannelWithLocalThread(ctx, bot, message, messageChannel, messages, isNewThread)
}

func processMessageChannelWithLocalThread(
	ctx context.Context,
	bot *telego.Bot,
	message *telego.Message,
	messageChannel chan string,
	messages []models.MultimodalMessage,
	isNewThread bool,
) {
	chatID := util.GetChatID(message)
	chatIDString := util.GetChatIDString(message)
	responseText := "..."
	responseMessage, err := bot.SendMessage(ctx, tu.Message(chatID, responseText).WithMessageThreadID(message.MessageThreadID).WithReplyMarkup(
		GetPendingReplyMarkup(),
	))
	if err != nil {
		log.Errorf("[processMessageChannel] Failed to send primer message in chat: %s, %v", chatIDString, err)
		bot.SendMessage(ctx, tu.Message(chatID, OOPSIE).WithMessageThreadID(message.MessageThreadID))
		return
	}
	isVoice, _ := util.IsAudioMessage(message)

	// only update message every 3 seconds to prevent rate limiting from telegram
	ticker := time.NewTicker(3 * time.Second)
	previousMessageLength := len(responseText)
	defer func() {
		log.Infof("[processMessageChannel] Finalizing message for streaming connection for chat: %s", chatIDString)
		ticker.Stop()
		finalMessageString := strings.TrimPrefix(responseText, "...")

		_, err = ChunkEditSendMessage(ctx, bot, responseMessage, finalMessageString, isVoice, true)
		if err != nil {
			log.Errorf("[processMessageChannel] Failed to ChunkEditSendMessage message in chat: %s, %v", chatIDString, err)
		}

		go func() {
			if len(messages) == 0 || len(messages[len(messages)-1].Content) == 0 {
				return
			}
			createdAt := ""
			if isNewThread {
				createdAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			}
			// if a last message has photo base64 contents, remove it and replace with text
			for i, content := range messages[len(messages)-1].Content {
				if content.Type == "image_url" {
					messages[len(messages)-1].Content[i] = models.MultimodalContent{
						Type: "text",
						Text: "User sent a photo..", // TODO: get photo caption
					}
				}
			}

			messages = append(messages, models.MultimodalMessage{
				Role:    "assistant",
				Content: []models.MultimodalContent{{Type: "text", Text: finalMessageString}},
			})
			threadJsonBytes, err := json.Marshal(messages)
			if err != nil {
				log.Errorf("[processMessageChannel] Failed to marshal thread in chat %s: %v", chatIDString, err)
				return
			}
			threadJson := string(threadJsonBytes)
			newCtx := context.WithValue(context.Background(), models.UserContext{}, ctx.Value(models.UserContext{}).(string))
			err = mongo.MongoDBClient.UpdateUserThread(newCtx, &models.MongoUserThread{
				ThreadJson: threadJson,
				CreatedAt:  createdAt,
			})
			if err != nil {
				log.Errorf("[processMessageChannel] Failed to update thread in chat %s: %v", chatIDString, err)
			}
		}()
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
			trimmedResponseText := strings.TrimPrefix(responseText, "...")

			var nextMessageObject *telego.Message
			nextMessageObject, err = ChunkEditSendMessage(ctx, bot, responseMessage, trimmedResponseText, isVoice, false)
			if err != nil {
				log.Errorf("[processMessageChannel] Failed to ChunkEditSendMessage message in chat: %s, %v", chatIDString, err)
			}
			if nextMessageObject != nil {
				responseMessage = nextMessageObject
				responseText = nextMessageObject.Text
				nextMessageObject = nil
			}
			if err != nil {
				log.Errorf("[processMessageChannel] Failed to edit message in chat: %s, %v", chatIDString, err)
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

func prepareMessagesForLocalThread(
	ctx context.Context,
	bot *telego.Bot,
	message *telego.Message,
) (messages []models.MultimodalMessage, isNewThread bool, err error) {
	messages = make([]models.MultimodalMessage, 0)
	chatIDString := util.GetChatIDString(message)
	thread, err := mongo.MongoDBClient.GetUserThread(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "failed to find user thread") {
			log.Infof("No thread found for chat: %s, creating new one..", chatIDString)
			messages = append(messages, models.MultimodalMessage{
				Role: "system",
				Content: []models.MultimodalContent{
					{Type: "text", Text: config.AI_INSTRUCTIONS},
					{Type: "text", Text: getUserInfo(message)},
				},
			})
			isNewThread = true
		} else {
			log.Errorf("Failed to get thread from mongo in chat %s: %s", chatIDString, err)
		}
	} else {
		threadJson := thread.ThreadJson
		threadJsonBytes := []byte(threadJson)
		var threadMessages []models.MultimodalMessage
		err = json.Unmarshal(threadJsonBytes, &threadMessages)
		if err != nil {
			log.Errorf("Failed to unmarshal thread in chat %s: %s", chatIDString, err)
		} else {
			messages = append(messages, threadMessages...)
		}
	}

	messages = append(messages, models.MultimodalMessage{
		Role:    "system",
		Content: []models.MultimodalContent{{Type: "text", Text: getWorldInfo()}},
	})

	// check if message had an image attachments and pass it on in base64 format to the model
	if message.Photo == nil || len(message.Photo) == 0 {
		messages = append(messages, models.MultimodalMessage{
			Role:    "user",
			Content: []models.MultimodalContent{{Type: "text", Text: message.Text}},
		})
		return messages, isNewThread, nil
	}

	photoMultiModelContent, err := getPhotoBase64(message, ctx, bot)
	if err != nil {
		log.Errorf("Failed to get photo(s) base64 in chat %s: %s", chatIDString, err)
		return messages, isNewThread, nil
	}
	photoMultiModelContent = append(photoMultiModelContent, models.MultimodalContent{
		Type: "text",
		Text: message.Text + "\n" + message.Caption,
	})
	messages = append(messages, models.MultimodalMessage{
		Role:    "user",
		Content: photoMultiModelContent,
	})
	return messages, isNewThread, nil
}

func GetPendingReplyMarkup() *telego.InlineKeyboardMarkup {
	// set up inline keyboard for like/dislike buttons
	btnPending := telego.InlineKeyboardButton{Text: "🧠", CallbackData: "pending"}
	return &telego.InlineKeyboardMarkup{InlineKeyboard: [][]telego.InlineKeyboardButton{{btnPending}}}
}

func GetLikeDislikeReplyMarkup() *telego.InlineKeyboardMarkup {
	// set up inline keyboard for like/dislike buttons
	btnLike := telego.InlineKeyboardButton{Text: "👍", CallbackData: "like"}
	btnDislike := telego.InlineKeyboardButton{Text: "👎", CallbackData: "dislike"}
	return &telego.InlineKeyboardMarkup{InlineKeyboard: [][]telego.InlineKeyboardButton{{btnLike, btnDislike}}}
}

func getUserInfo(message *telego.Message) string {
	userInfo := ""
	if message.From != nil {
		userInfo = fmt.Sprintf("Telegram user info, maybe relevant as a name or any other judgements: %s %s, language: %s, username: %s",
			message.From.FirstName, message.From.LastName, message.From.LanguageCode, message.From.Username)
	}
	return userInfo
}

func getWorldInfo() string {
	worldInfo := fmt.Sprintf("Datetime %s", time.Now().UTC().Format("2006-01-02 15:04:05 MST"))
	return worldInfo
}
