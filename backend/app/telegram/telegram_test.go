package telegram

import (
	"reflect"
	"regexp"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/db/mongo"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/models"
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/mymmrac/telego"
	log "github.com/sirupsen/logrus"
	"github.com/undefinedlabs/go-mpatch"
)

func init() {
	setupTestDatadog()

	redis.RedisClient = redis.NewMockRedisClient()

	mongo.MongoDBClient = mongo.NewMockMongoDBClient(
		models.MongoUser{
			ID:    "123",
			Usage: 0.1,
		},
	)

	setupTestBot()
	setupCommandHandlers()

	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})
	log.SetLevel(log.DebugLevel)
}

func getTestBot() *telego.Bot {
	return &telego.Bot{}
}

func setupTestBot() {
	BOT = &Bot{
		Name: "testbot",
		Bot:  getTestBot(),
	}
}

func setupTestDatadog() {
	testClient, err := statsd.New("127.0.0.1:8125", statsd.WithNamespace("tests."))
	if err != nil {
		log.Fatalf("error creating test DataDog client: %v", err)
	}
	config.CONFIG = &config.Config{
		DataDogClient: testClient,
	}
}

func getChatMemberFuncAssertion(t *testing.T, expectedChatID int64, expectedUserID int64) func(bot *telego.Bot, params *telego.GetChatMemberParams) (telego.ChatMember, error) {
	return func(bot *telego.Bot, params *telego.GetChatMemberParams) (telego.ChatMember, error) {
		if params.ChatID.ID != expectedChatID {
			t.Errorf("Expected chat ID %d, got %d", expectedChatID, params.ChatID.ID)
		}
		if params.UserID != expectedUserID {
			t.Errorf("Expected user ID %d, got %d", expectedUserID, params.UserID)
		}

		return &telego.ChatMemberAdministrator{}, nil
	}
}

func getSendMessageFuncAssertion(t *testing.T, expectedRegex string, expectedChatID int64) func(bot *telego.Bot, params *telego.SendMessageParams) (*telego.Message, error) {
	return func(bot *telego.Bot, params *telego.SendMessageParams) (*telego.Message, error) {
		if params.ChatID.ID != expectedChatID {
			t.Errorf("Expected chat ID %d, got %d", expectedChatID, params.ChatID.ID)
		}

		matched, err := regexp.MatchString(expectedRegex, params.Text)
		if err != nil {
			t.Errorf("Error matching regex: %v", err)
		}
		if !matched {
			t.Errorf("Expected message to match regex %s, got %s", expectedRegex, params.Text)
		}

		return &telego.Message{
			MessageID: 12345,
			Text:      params.Text,
			Chat: telego.Chat{
				ID: params.ChatID.ID,
			},
		}, nil
	}
}

func getEditMessageFuncAssertion(t *testing.T, expectedRegex string, expectedChatID int64) func(bot *telego.Bot, params *telego.EditMessageTextParams) (*telego.Message, error) {
	return func(bot *telego.Bot, params *telego.EditMessageTextParams) (*telego.Message, error) {
		if params.ChatID.ID != expectedChatID {
			t.Errorf("Expected chat ID %d, got %d", expectedChatID, params.ChatID.ID)
		}

		matched, err := regexp.MatchString(expectedRegex, params.Text)
		if err != nil {
			t.Errorf("Error matching regex: %v", err)
		}
		if !matched {
			t.Errorf("Expected message to match regex %s, got %s", expectedRegex, params.Text)
		}

		return &telego.Message{
			MessageID: params.MessageID,
			Text:      params.Text,
			Chat: telego.Chat{
				ID: params.ChatID.ID,
			},
		}, nil
	}
}

func TestHandleEmptyPublicMessage(t *testing.T) {
	message := telego.Message{
		Chat: telego.Chat{
			ID: 123,
		},
	}

	// act
	handleMessage(BOT.Bot, message)
}

func TestHandleEmptyPrivateMessage(t *testing.T) {
	message := telego.Message{
		Chat: telego.Chat{
			ID:   123,
			Type: "private",
		},
	}

	sendMessagePatch, err := mpatch.PatchInstanceMethodByName(
		reflect.TypeOf(BOT.Bot),
		"SendMessage",
		getSendMessageFuncAssertion(t, "There is no message provided to correct or comment on", 123),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sendMessagePatch.Unpatch()

	// act
	handleMessage(BOT.Bot, message)
}

func TestHandlePrivateStartCommandMessage(t *testing.T) {
	// arrange
	message := telego.Message{
		Chat: telego.Chat{
			ID:   123,
			Type: "private",
		},
		Text: "/start",
	}

	sendMessagePatch, err := mpatch.PatchInstanceMethodByName(
		reflect.TypeOf(BOT.Bot),
		"SendMessage",
		getSendMessageFuncAssertion(t, "Hi, I'm a bot powered by AI!", 123),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sendMessagePatch.Unpatch()

	// act
	handleMessage(BOT.Bot, message)
}

func TestHandlePublicStartCommandMessage(t *testing.T) {
	// arrange
	message := telego.Message{
		From: &telego.User{
			ID: 234,
		},
		Chat: telego.Chat{
			ID:   -123,
			Type: "supergroup",
		},
		Text: "/start@testbot",
	}

	sendMessagePatch, err := mpatch.PatchInstanceMethodByName(
		reflect.TypeOf(BOT.Bot),
		"SendMessage",
		getSendMessageFuncAssertion(t, "Hi, I'm a bot powered by AI!", -123),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sendMessagePatch.Unpatch()

	getChatMemberPatch, err := mpatch.PatchInstanceMethodByName(
		reflect.TypeOf(BOT.Bot),
		"GetChatMember",
		getChatMemberFuncAssertion(t, -123, 234),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer getChatMemberPatch.Unpatch()

	// act
	handleMessage(BOT.Bot, message)
}

func TestHandlePublicStartCommandNoMentionMessage(t *testing.T) {
	// arrange
	message := telego.Message{
		Chat: telego.Chat{
			ID:   123,
			Type: "supergroup",
		},
		Text: "/start",
	}

	// act
	handleMessage(BOT.Bot, message)
}

func TestHandlePublicUnknownCommandMessage(t *testing.T) {
	// arrange
	message := telego.Message{
		From: &telego.User{
			ID: 234,
		},
		Chat: telego.Chat{
			ID:   -123,
			Type: "supergroup",
		},
		Text: "/destroy@testbot",
	}

	sendMessagePatch, err := mpatch.PatchInstanceMethodByName(
		reflect.TypeOf(BOT.Bot),
		"SendMessage",
		getSendMessageFuncAssertion(t, "Unknown command", -123),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sendMessagePatch.Unpatch()

	getChatMemberPatch, err := mpatch.PatchInstanceMethodByName(
		reflect.TypeOf(BOT.Bot),
		"GetChatMember",
		getChatMemberFuncAssertion(t, -123, 234),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer getChatMemberPatch.Unpatch()

	// act
	handleMessage(BOT.Bot, message)
}
