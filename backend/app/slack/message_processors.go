package slack

import (
	"context"
	"strings"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/models"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
)

func ProcessStreamingMessage(
	ctx context.Context,
	channelId string,
	messageTs string,
	message string,
	seedData []models.Message,
	userMessagePrimer string,
	mode lib.ModeName,
	engineModel models.Engine,
	cancelContext context.CancelFunc,
	replyInThread bool,
) {
	userId := ctx.Value(models.UserContext{}).(string)
	messageChannel, err := BOT.API.ChatCompleteStreaming(
		ctx,
		models.ChatCompletion{
			Model: string(engineModel),
			Messages: []models.Message(append(
				seedData,
				models.Message{
					Role:    "user",
					Content: userMessagePrimer + message,
				},
			)),
		},
		cancelContext,
	)

	if err != nil {
		log.Errorf("Failed get streaming response from Open AI: %s", err)
		_, _, _, err = BOT.SendMessage(channelId, slack.MsgOptionText("Oopsie, it looks like my AI brain isn't working 🧠🔥. Please try again later.", false), slack.MsgOptionPostEphemeral(userId))
		if err != nil {
			log.Errorf("Failed to send error message in chat: %s, user: %s, %v", channelId, userId, err)
		}
		return
	}

	responseText := "🧠: "
	if mode == lib.Teacher || mode == lib.Emili || mode == lib.Vasilisa {
		responseText = "👩‍🏫: "
	} else if mode == lib.Grammar {
		responseText = "👀: "
	}
	messageOptions := []slack.MsgOption{
		slack.MsgOptionText(responseText, false),
	}
	if replyInThread {
		messageOptions = append(messageOptions, slack.MsgOptionTS(messageTs))
	}
	_, ts, _, err := BOT.SendMessageContext(ctx, channelId, messageOptions...)
	if err != nil {
		log.Errorf("Failed to send primer message in chat: %s, userId: %s, %v", channelId, userId, err)
	}
	// only update message every 5 seconds
	ticker := time.NewTicker(5 * time.Second)
	previousMessageLength := len(responseText)
	defer func() {
		log.Infof("Finalizing message for streaming connection for chat: %s, user: %s", channelId, userId)
		ticker.Stop()
		finalMessageParams := slack.MsgOptionText(responseText, false)
		_, _, _, err = BOT.UpdateMessageContext(context.Background(), channelId, ts, finalMessageParams)
		if err != nil {
			log.Errorf("Failed to finalize a message in chat: %s, user: %s, %v", channelId, userId, err)
		}
	}()
	for {
		select {
		case <-ctx.Done():
			log.Infof("Context cancelled, closing streaming connection in chat: %s, user: %s", channelId, userId)
			return
		case <-ticker.C:
			if previousMessageLength == len(responseText) {
				continue
			}
			previousMessageLength = len(responseText)
			_, ts, _, err = BOT.UpdateMessageContext(ctx, channelId, ts, slack.MsgOptionText(responseText, false))
			if err != nil {
				log.Errorf("Failed to edit message in chat: %s, user: %s, %v", channelId, userId, err)
			}
		case message := <-messageChannel:
			log.Debugf("Sending message: %s, in chat: %s", message, channelId)
			responseText += message

			if mode == lib.Grammar || mode == lib.Teacher || mode == lib.Emili || mode == lib.Vasilisa {
				responseText = strings.ReplaceAll(responseText, userMessagePrimer, "")
			}
		}
	}
}
