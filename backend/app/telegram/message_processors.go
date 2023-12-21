package telegram

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/models"
	"talk2robots/m/v2/app/util"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	log "github.com/sirupsen/logrus"
)

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
	messages := util.MessagesToMultimodalMessages(seedData)

	// check if message had an image attachments and pass it on in base64 format to the model
	if message.Photo == nil || len(message.Photo) == 0 {
		messages = append(messages, models.MultimodalMessage{
			Role:    "user",
			Content: []models.MultimodalContent{{Type: "text", Text: userMessagePrimer + message.Text}},
		},
		)
	} else {
		// check user's subscription status
		if lib.IsUserFree(ctx) || lib.IsUserFreePlus(ctx) {
			bot.SendMessage(&telego.SendMessageParams{
				ChatID: chatID,
				Text:   "Image vision is not currently available on free plans, since it's kinda expensive. Please /upgrade to use this feature.",
			})
			return
		}

		engineModel = models.ChatGpt4TurboVision

		// get the last image for now
		photo := message.Photo
		photoSize := photo[len(photo)-1]
		photoFile, err := bot.GetFile(&telego.GetFileParams{FileID: photoSize.FileID})
		if err != nil {
			log.Errorf("Failed to get image file params from telegram: %s", err)
			bot.SendMessage(&telego.SendMessageParams{
				ChatID: chatID,
				Text:   "😔 can't accept image messages at the moment",
			})
			return
		}
		fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", bot.Token(), photoFile.FilePath)
		photoResponse, err := http.Get(fileURL)
		if err != nil {
			log.Errorf("Error downloading image file in chat %s: %v", chatIDString, err)
			bot.SendMessage(&telego.SendMessageParams{
				ChatID: chatID,
				Text:   "😔 can't accept image messages at the moment",
			})
			return
		}
		defer photoResponse.Body.Close()

		photoBytes := make([]byte, photoResponse.ContentLength)
		_, err = io.ReadFull(photoResponse.Body, photoBytes)
		if err != nil {
			log.Errorf("Error reading image file in chat %s: %v", chatIDString, err)
			bot.SendMessage(&telego.SendMessageParams{
				ChatID: chatID,
				Text:   "😔 can't accept image messages at the moment",
			})
			return
		}
		photoBase64 := base64.StdEncoding.EncodeToString(photoBytes)

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
		_, err = bot.SendMessage(tu.Message(chatID, "Oopsie, it looks like my AI brain isn't working 🧠🔥. Please try again later."))
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
			_, err = bot.EditMessageText(&telego.EditMessageTextParams{
				ChatID:      chatID,
				MessageID:   responseMessage.MessageID,
				Text:        responseText,
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
			log.Debugf("Sending part %d: %s", i, part)
			message := tu.Message(chatID, strings.Trim(part, "\n")).WithReplyMarkup(getLikeDislikeReplyMarkup())
			_, err = bot.SendMessage(message)
		}
		if err != nil {
			log.Errorf("Failed to send message in chat %s: %s", chatIDString, err)
		}
	} else {
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
