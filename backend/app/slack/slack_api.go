package slack

import (
	"fmt"
	"net/http"
	"talk2robots/m/v2/app/lib"

	log "github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	"github.com/valyala/fasthttp"
)

func validateSignature(ctx *fasthttp.RequestCtx) bool {
	// verify slack signing secret
	// https://api.slack.com/authentication/verifying-requests-from-slack
	body := ctx.PostBody()
	headers := lib.ConvertFasthttpHeader(&ctx.Request.Header)

	sv, err := slack.NewSecretsVerifier(headers, BOT.SigningSecret)
	if err != nil {
		ctx.Error("", http.StatusInternalServerError)
		log.Warnf("validateSignature: Error creating secrets verifier: %v", err)
		return false
	}

	if _, err := sv.Write(body); err != nil {
		ctx.Error("", http.StatusInternalServerError)
		log.Warnf("validateSignature: Error writing body to secrets verifier: %v", err)
		return false
	}

	if err := sv.Ensure(); err != nil {
		ctx.Error("", http.StatusUnauthorized)
		log.Warnf("validateSignature: Error ensuring secrets verifier: %v", err)
		return false
	}

	return true
}

func fetchMessage(channelId string, messageTS string) slack.Message {
	messages, err := BOT.GetConversationHistory(
		&slack.GetConversationHistoryParameters{
			ChannelID:          channelId,
			Latest:             messageTS,
			Limit:              1,
			Inclusive:          true,
			IncludeAllMetadata: true,
		},
	)
	if err != nil {
		log.Errorf("Error fetching message in channel %s, ts: %s, error: %v", channelId, messageTS, err)
		return slack.Message{}
	}

	if len(messages.Messages) == 0 {
		log.Errorf("No messages found")
		return slack.Message{}
	}

	log.Infof("Found %d messages in channel %s, first one with TS: %s, thread TS %s", len(messages.Messages), channelId, messages.Messages[0].Timestamp, messages.Messages[0].ThreadTimestamp)

	if messages.Messages[0].Timestamp != messageTS {
		log.Infof("Message %s is part of a thread, fetching thread", messageTS)
		messages.Messages, _, _, err = BOT.GetConversationReplies(
			&slack.GetConversationRepliesParameters{
				ChannelID:          channelId,
				Timestamp:          messageTS,
				Limit:              1,
				Inclusive:          true,
				IncludeAllMetadata: true,
			},
		)

		if err != nil {
			log.Errorf("Error fetching message in a thread in channel %s, ts: %s, error: %v", channelId, messageTS, err)
			return slack.Message{}
		}

		if len(messages.Messages) == 0 {
			log.Errorf("No messages found")
			return slack.Message{}
		}

		log.Infof("Found %d thread messages in channel %s, first one with TS: %s, thread TS %s", len(messages.Messages), channelId, messages.Messages[0].Timestamp, messages.Messages[0].ThreadTimestamp)
	}

	return messages.Messages[0]
}

func fetchMessageThread(channelId string, messageTS string) string {
	messages, _, _, err := BOT.GetConversationReplies(
		&slack.GetConversationRepliesParameters{
			ChannelID: channelId,
			Timestamp: messageTS,
			Limit:     THREAD_MESSAGES_LIMIT_FOR_SUMMARIZE,
			Inclusive: true,
		},
	)
	if err != nil {
		log.Errorf("Error fetching message thread in channel %s, ts: %s, error: %v", channelId, messageTS, err)
		return ""
	}

	if len(messages) == 0 {
		log.Errorf("No messages found")
		return ""
	}

	log.Infof("Found %d messages", len(messages))

	messageText := ""
	for _, message := range messages {
		messageText += fmt.Sprintf("<@%s>: %s\n\n", message.User, message.Text)
	}

	return messageText
}
