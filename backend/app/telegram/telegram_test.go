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
}

func getTestBot() *telego.Bot {
	return &telego.Bot{}
}

func setupBot() {
	BOT = &Bot{
		Name: "test",
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

// func HandleMessage(bot *telego.Bot, message telego.Message) {
// 	chatID := util.GetChatID(&message)
// 	chatIDString := util.GetChatIDString(&message)
// 	isPrivate := message.Chat.Type == "private"
// 	_, ctx, cancelContext, err := lib.SetupUserAndContext(chatIDString, "telegram", chatIDString)
// 	if err != nil {
// 		if err == lib.ErrUserBanned {
// 			log.Infof("User %s is banned", chatIDString)
// 			return
// 		}

// 		log.Errorf("Error setting up user and context: %v", err)
// 		return
// 	}

// 	// process commands
// 	if message.Voice == nil && message.Audio == nil && message.Video == nil && message.VideoNote == nil && message.Document == nil && message.Photo == nil && (message.Text == string(EmptyCommand) || strings.HasPrefix(message.Text, "/")) {
// 		if !isPrivate && !strings.Contains(message.Text, "@"+BOT.Name) {
// 			log.Infof("Ignoring public command w/o @mention in channel: %s", chatIDString)
// 			return
// 		}
// 		AllCommandHandlers.handleCommand(ctx, BOT, &message)
// 		return
// 	}

// 	mode, params := lib.GetMode(chatIDString)
// 	log.Infof("chat %s, mode: %s, params: %s", chatIDString, mode, params)
// 	ctx = context.WithValue(ctx, models.ParamsContext{}, params)
// 	// while in channels, only react to
// 	// 1. @mentions
// 	// 2. audio messages in /transcribe mode
// 	// 3. /grammar fixes
// 	if !isPrivate && mode != lib.Transcribe && mode != lib.Grammar && !strings.Contains(message.Text, "@"+BOT.Name) {
// 		log.Infof("Ignoring public message w/o @mention and not in transcribe or grammar mode in channel: %s", chatIDString)
// 		return
// 	}

// 	if message.Video != nil && strings.HasPrefix(message.Caption, string(SYSTEMSetOnboardingVideoCommand)) {
// 		log.Infof("System command received: %+v", message) // audit
// 		message.Text = string(SYSTEMSetOnboardingVideoCommand)
// 		AllCommandHandlers.handleCommand(ctx, BOT, &message)
// 		return
// 	}

// 	// user usage exceeded monthly limit, send message and return
// 	ok := lib.ValidateUserUsage(ctx)
// 	if !ok {
// 		notification := "Your monthly usage limit has been exceeded. Check /status and /upgrade your subscription to continue using the bot. The limits are reset on the 1st of every month."
// 		notification = lib.AddBotSuffixToGroupCommands(ctx, notification)
// 		bot.SendMessage(tu.Message(chatID, notification))
// 		config.CONFIG.DataDogClient.Incr("telegram.usage_exceeded", []string{"client:telegram", "channel_type:" + message.Chat.Type}, 1)
// 		return
// 	}

// 	voiceTranscriptionText := ""
// 	// if the message is voice/audio/video message, process it to upload to WhisperAI API and get the transcription
// 	if message.Voice != nil || message.Audio != nil || message.Video != nil || message.VideoNote != nil || message.Document != nil {
// 		voice_type := "voice"
// 		switch {
// 		case message.Audio != nil:
// 			voice_type = "audio"
// 		case message.Video != nil:
// 			voice_type = "video"
// 		case message.VideoNote != nil:
// 			voice_type = "note"
// 		case message.Document != nil:
// 			voice_type = "document"
// 		}
// 		config.CONFIG.DataDogClient.Incr("telegram.voice_message_received", []string{"type:" + voice_type, "channel_type:" + message.Chat.Type}, 1)

// 		// send typing action to show that bot is working
// 		if mode != lib.VoiceGPT {
// 			sendTypingAction(bot, chatID)
// 		} else {
// 			sendAudioAction(bot, chatID)
// 		}
// 		voiceTranscriptionText = getVoiceTranscript(ctx, bot, message)

// 		if mode != lib.Transcribe {
// 			// combine message text with transcription
// 			if voiceTranscriptionText != "" {
// 				message.Text = message.Text + "\n" + voiceTranscriptionText
// 			}

// 			// process commands again if it was a voice command
// 			if message.Text == string(EmptyCommand) || strings.HasPrefix(message.Text, "/") {
// 				AllCommandHandlers.handleCommand(ctx, BOT, &message)
// 				return
// 			}
// 		}
// 	}

// 	if mode == lib.Transcribe {
// 		ChunkSendMessage(bot, chatID, voiceTranscriptionText)
// 		if isPrivate && message.Text != "" {
// 			bot.SendMessage(tu.Message(chatID, "The bot is in /transcribe mode. Please send a voice/audio/video message to transcribe or change to another mode (/status)."))
// 		}
// 		return
// 	}

// 	if mode != lib.VoiceGPT && !(mode == lib.Grammar && !isPrivate) && voiceTranscriptionText != "" {
// 		ChunkSendMessage(bot, chatID, "🗣:\n"+voiceTranscriptionText)
// 	}

// 	if message.Text != "" {
// 		config.CONFIG.DataDogClient.Incr("telegram.text_message_received", []string{"channel_type:" + message.Chat.Type}, 1)
// 	} else {
// 		log.Infof("Ignoring empty message in chat: %s", chatIDString)
// 		return
// 	}

// 	if message.Photo != nil {
// 		config.CONFIG.DataDogClient.Incr("telegram.photo_message_received", []string{"channel_type:" + message.Chat.Type}, 1)
// 	}

// 	var seedData []models.Message
// 	var userMessagePrimer string
// 	seedData, userMessagePrimer = lib.GetSeedDataAndPrimer(mode)

// 	log.Debugf("Received message: %d, in chat: %d, initiating request to OpenAI", message.MessageID, chatID.ID)
// 	engineModel := redis.GetChatEngine(chatIDString)

// 	// send action to show that bot is working
// 	if mode != lib.VoiceGPT {
// 		sendTypingAction(bot, chatID)
// 	} else {
// 		sendAudioAction(bot, chatID)
// 	}
// 	if mode == lib.ChatGPT || mode == lib.VoiceGPT {
// 		go ProcessThreadedStreamingMessage(ctx, bot, &message, mode, engineModel, cancelContext)
// 	} else if mode == lib.Summarize || (mode == lib.Grammar && isPrivate) {
// 		go ProcessChatCompleteStreamingMessage(ctx, bot, &message, seedData, userMessagePrimer, mode, engineModel, cancelContext)
// 	} else {
// 		go ProcessChatCompleteNonStreamingMessage(ctx, bot, &message, seedData, userMessagePrimer, mode, engineModel)
// 	}
// }

func TestHandleEmptyPublicMessage(t *testing.T) {
	message := telego.Message{
		Chat: telego.Chat{
			ID: 123,
		},
	}

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
		getSendMessageFuncAssertion(t, "Unknown command", 123),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sendMessagePatch.Unpatch()

	HandleMessage(BOT.Bot, message)
}
