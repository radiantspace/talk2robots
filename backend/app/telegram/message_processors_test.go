package telegram

import (
	"context"
	"reflect"
	"talk2robots/m/v2/app/ai/openai"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/models"
	"testing"
	"time"

	"github.com/mymmrac/telego"
	"github.com/undefinedlabs/go-mpatch"
)

func TestProcessThreadedStreamingMessage(t *testing.T) {
	message := telego.Message{
		Chat: telego.Chat{
			ID:   123,
			Type: "private",
		},
		Text: "Tell me about 25 most famous Jedi and write two paragraphs about each of them",
	}
	ctx, cancelContext := context.WithCancel(context.Background())
	ctx = context.WithValue(ctx, models.UserContext{}, "123")
	ctx = context.WithValue(ctx, models.ClientContext{}, "telegram")
	ctx = context.WithValue(ctx, models.ChannelContext{}, "123")

	// OpenAI API patch
	openAIPatch, err := mpatch.PatchMethod(
		openai.CreateThreadAndRunStreaming,
		func(ctx context.Context, assistantId string, model models.Engine, thread *models.Thread, cancelContext context.CancelFunc) (chan string, error) {
			messages := make(chan string)
			go func() {
				defer close(messages)
				defer cancelContext()
				message := "This is one message of 256 characters. This is one message of ___ characters. This is one message of ___ characters. This is one message of ___ characters. This is one message of ___ characters. This is one message of ___ characters. This is one longgggg.\n"
				for i := 0; i < 60; i++ {
					// sleep for 1 second
					<-time.After(60 * time.Millisecond)
					messages <- message
				}
			}()
			return messages, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer openAIPatch.Unpatch()

	// SendMessage patch
	sendMessagePatch, err := mpatch.PatchInstanceMethodByName(
		reflect.TypeOf(BOT.Bot),
		"SendMessage",
		getSendMessageFuncAssertion(t, "...", 123),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sendMessagePatch.Unpatch()

	// EditMessage patch
	editMessagePatch, _ := mpatch.PatchInstanceMethodByName(
		reflect.TypeOf(BOT.Bot),
		"EditMessageText",
		getEditMessageFuncAssertion(t, "This is one message of 256 characters", 123),
	)
	defer editMessagePatch.Unpatch()

	ProcessThreadedStreamingMessage(ctx, BOT.Bot, &message, lib.ChatGPT, models.ChatGpt4oMini, cancelContext)
}
