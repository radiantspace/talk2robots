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
	testClient, err := statsd.New("127.0.0.1:8125", statsd.WithNamespace("tests."))
	if err != nil {
		log.Fatalf("error creating test DataDog client: %v", err)
	}
	config.CONFIG = &config.Config{
		DataDogClient: testClient,
	}

	redis.RedisClient = redis.NewMockRedisClient()

	mongo.MongoDBClient = mongo.NewMockMongoDBClient(
		models.MongoUser{
			ID:    "123",
			Usage: 0.1,
		},
	)

	setupBot()
	setupCommandHandlers()
}

func getTestBot() *telego.Bot {
	return &telego.Bot{}
}

func setupBot() {
	BOT = &Bot{
		Name: "testbot",
		Bot:  getTestBot(),
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

		return &telego.Message{}, nil
	}
}

func TestHandleEmptyPublicMessage(t *testing.T) {
	message := telego.Message{
		Chat: telego.Chat{
			ID: 123,
		},
	}

	// act
	HandleMessage(BOT.Bot, message)
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
	HandleMessage(BOT.Bot, message)
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
		getSendMessageFuncAssertion(t, "Hi, I'm a bot powered by OpenAI!", 123),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sendMessagePatch.Unpatch()

	// act
	HandleMessage(BOT.Bot, message)
}

func TestHandlePublicStartCommandMessage(t *testing.T) {
	// arrange
	message := telego.Message{
		Chat: telego.Chat{
			ID:   123,
			Type: "supergroup",
		},
		Text: "/start@testbot",
	}

	sendMessagePatch, err := mpatch.PatchInstanceMethodByName(
		reflect.TypeOf(BOT.Bot),
		"SendMessage",
		getSendMessageFuncAssertion(t, "Hi, I'm a bot powered by OpenAI!", 123),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sendMessagePatch.Unpatch()

	// act
	HandleMessage(BOT.Bot, message)
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
	HandleMessage(BOT.Bot, message)
}

func TestHandlePublicUnknownCommandMessage(t *testing.T) {
	// arrange
	message := telego.Message{
		Chat: telego.Chat{
			ID:   123,
			Type: "supergroup",
		},
		Text: "/destroy@testbot",
	}

	sendMessagePatch, err := mpatch.PatchInstanceMethodByName(
		reflect.TypeOf(BOT.Bot),
		"SendMessage",
		getSendMessageFuncAssertion(t, "Unknown command", 123),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sendMessagePatch.Unpatch()

	// act
	HandleMessage(BOT.Bot, message)
}
