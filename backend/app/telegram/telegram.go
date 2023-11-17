// main package to control telegram bot
package telegram

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/converters"
	"talk2robots/m/v2/app/db/mongo"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/models"
	"talk2robots/m/v2/app/openai"
	"talk2robots/m/v2/app/util"
	"time"

	"github.com/fasthttp/router"
	"github.com/google/uuid"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
	log "github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
)

type Bot struct {
	*openai.API
	*telego.Bot
	*th.BotHandler
	Name  string
	Dummy bool
	telego.ChatID
	WhisperConfig openai.WhisperConfig
}

var AllCommandHandlers CommandHandlers = CommandHandlers{}
var BOT *Bot

func NewBot(rtr *router.Router, cfg *config.Config) (*Bot, error) {
	bot, err := telego.NewBot(cfg.TelegramBotToken, telego.WithHealthCheck(), util.GetBotLoggerOption(cfg))
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}
	setupCommandHandlers()
	updates, err := signBotForUpdates(bot, rtr)
	if err != nil {
		return nil, fmt.Errorf("failed to sign bot for updates: %w", err)
	}
	bh, err := th.NewBotHandler(bot, updates, th.WithStopTimeout(time.Second*10))
	if err != nil {
		return nil, fmt.Errorf("failed to setup bot handler: %w", err)
	}
	bh.HandleMessage(handleMessage)
	bh.HandleCallbackQuery(handleCallbackQuery)
	go bh.Start()

	BOT = &Bot{
		API:        openai.NewAPI(cfg),
		Bot:        bot,
		BotHandler: bh,
		Name:       cfg.BotName,
		WhisperConfig: openai.WhisperConfig{
			APIKey:             cfg.OpenAIAPIKey,
			WhisperAPIEndpoint: cfg.WhisperAPIEndpoint,
			Mode:               "transcriptions",
			StopTimeout:        5 * time.Second,
			OnTranscribe:       nil,
		},
	}

	return BOT, nil
}

func signBotForUpdates(bot *telego.Bot, rtr *router.Router) (<-chan telego.Update, error) {
	updates, err := bot.UpdatesViaWebhook(
		"/bot"+bot.Token(),
		telego.WithWebhookSet(&telego.SetWebhookParams{
			URL: util.Env("BACKEND_BASE_URL") + "/bot" + bot.Token(),
		}),
		telego.WithWebhookServer(telego.FastHTTPWebhookServer{
			Logger: log.StandardLogger(),
			Server: &fasthttp.Server{},
			Router: rtr,
		}),
	)
	return updates, err
}

func handleMessage(bot *telego.Bot, message telego.Message) {
	chatID := util.GetChatID(&message)
	chatIDString := util.GetChatIDString(&message)
	_, ctx, cancelContext, err := lib.SetupUserAndContext(chatIDString, "telegram", "")
	if err != nil {
		if err == lib.ErrUserBanned {
			log.Infof("User %s is banned", chatIDString)
			return
		}

		log.Errorf("Error setting up user and context: %v", err)
		return
	}
	defer cancelContext()

	// process commands
	if message.Voice == nil && message.Photo == nil && (message.Text == string(EmptyCommand) || strings.HasPrefix(message.Text, "/")) {
		if message.Video != nil && strings.HasPrefix(message.Caption, string(SYSTEMSetOnboardingVideoCommand)) {
			log.Infof("System command received: %+v", message) // audit
			message.Text = string(SYSTEMSetOnboardingVideoCommand)
		}
		AllCommandHandlers.handleCommand(ctx, BOT, &message)
		return
	}

	// user usage exceeded monthly limit, send message and return
	ok := lib.ValidateUserUsage(ctx)
	if !ok {
		bot.SendMessage(tu.Message(chatID, "Your monthly usage limit has been exceeded. Check /status and /upgrade your subscription to continue using the bot."))
		config.CONFIG.DataDogClient.Incr("telegram.usage_exceeded", []string{"client:telegram"}, 1)
		return
	}

	userText := ""
	// if the message is voice message, process it to upload to WhisperAI API and get the text
	if message.Voice != nil {
		config.CONFIG.DataDogClient.Incr("telegram.voice_message_received", nil, 1)
		userText = getVoiceTransript(ctx, bot, message)
		if userText != "" {
			message.Text = userText

			// another typing action to show that bot is still working
			sendTypingAction(bot, chatID)
		}

		// process commands again if it was a voice command
		if message.Text == string(EmptyCommand) || strings.HasPrefix(message.Text, "/") {
			AllCommandHandlers.handleCommand(ctx, BOT, &message)
			return
		}
	}

	if message.Photo != nil {
		config.CONFIG.DataDogClient.Incr("telegram.photo_message_received", nil, 1)
	} else {
		config.CONFIG.DataDogClient.Incr("telegram.text_message_received", nil, 1)
	}

	var seedData []models.Message
	var userMessagePrimer string
	mode := lib.GetMode(chatIDString)
	seedData, userMessagePrimer = lib.GetSeedDataAndPrimer(mode)

	log.Debugf("Received message: %d, in chat: %d, initiating request to OpenAI", message.MessageID, chatID.ID)
	engineModel := redis.GetChatEngine(chatIDString)

	if mode == lib.ChatGPT {
		ProcessStreamingMessage(ctx, bot, &message, seedData, userMessagePrimer, mode, engineModel, cancelContext)
	} else {
		ProcessNonStreamingMessage(ctx, bot, &message, seedData, userMessagePrimer, mode, engineModel)
	}
}

func handleCallbackQuery(bot *telego.Bot, callbackQuery telego.CallbackQuery) {
	log.Infof("Received callback query: %s, for user: %d", callbackQuery.Data, callbackQuery.From.ID)
	config.CONFIG.DataDogClient.Incr("telegram.callback_query", []string{"data:" + callbackQuery.Data}, 1)
	switch callbackQuery.Data {
	case "like":
		log.Infof("User liked a message.")
		config.CONFIG.DataDogClient.Incr("telegram.like", nil, 1)
		bot.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "Thanks for your feedback! 👍",
		})
	case "dislike":
		log.Infof("User disliked a message.")
		config.CONFIG.DataDogClient.Incr("telegram.dislike", nil, 1)
		bot.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "Thanks for your feedback!",
		})
	case string(lib.ChatGPT):
		handleCommandsInCallbackQuery(callbackQuery)
	case string(lib.Grammar):
		handleCommandsInCallbackQuery(callbackQuery)
	case string(lib.Teacher):
		handleCommandsInCallbackQuery(callbackQuery)
	case string(models.ChatGpt35Turbo):
		handleEngineSwitchCallbackQuery(callbackQuery)
	case string(models.ChatGpt4):
		handleEngineSwitchCallbackQuery(callbackQuery)
	default:
		log.Errorf("Unknown callback query: %s", callbackQuery.Data)
	}
}

func handleCommandsInCallbackQuery(callbackQuery telego.CallbackQuery) {
	chatIDString := fmt.Sprint(callbackQuery.From.ID)
	ctx := context.WithValue(context.Background(), models.UserContext{}, chatIDString)
	ctx = context.WithValue(ctx, models.ClientContext{}, "telegram")
	message := telego.Message{
		Chat: telego.Chat{ID: callbackQuery.From.ID},
		Text: "/" + callbackQuery.Data,
	}
	AllCommandHandlers.handleCommand(ctx, BOT, &message)
}

func handleEngineSwitchCallbackQuery(callbackQuery telego.CallbackQuery) {
	chatIDString := fmt.Sprint(callbackQuery.From.ID)
	ctx := context.WithValue(context.Background(), models.UserContext{}, chatIDString)
	ctx = context.WithValue(ctx, models.ClientContext{}, "telegram")
	currentEngine := redis.GetChatEngine(chatIDString)
	if callbackQuery.Data == string(currentEngine) {
		err := BOT.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "You are already using " + callbackQuery.Data + " engine!",
		})
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to answer callback query, already using: %v", err)
		}
		return
	}
	if callbackQuery.Data == string(models.ChatGpt35Turbo) {
		redis.SaveEngine(chatIDString, models.ChatGpt35Turbo)
		_, err := BOT.SendMessage(tu.Message(tu.ID(callbackQuery.From.ID), "Switched to GPT-3.5 Turbo model, fast and cheap!"))
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to send GPT-3.5 message: %v", err)
		}
		err = BOT.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "Switched to " + callbackQuery.Data + " engine!",
		})
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to answer callback query: %v", err)
		}
		return
	}
	if callbackQuery.Data == string(models.ChatGpt4) {
		// fetch user subscription
		user, err := mongo.MongoDBClient.GetUser(ctx)
		if err != nil {
			log.Errorf("Failed to get user: %v", err)
			BOT.SendMessage(tu.Message(tu.ID(callbackQuery.From.ID), "Failed to switch to GPT model, please try again later"))
			return
		}
		if user.SubscriptionType.Name == lib.FreeSubscriptionName || user.SubscriptionType.Name == lib.FreePlusSubscriptionName {
			BOT.SendMessage(tu.Message(tu.ID(callbackQuery.From.ID), "You need to /upgrade your subscription to use GPT-4 engine! Meanwhile, you can still use GPT-3.5 Turbo model, it's fast, cheap and quite smart."))
			return
		}
		redis.SaveEngine(chatIDString, models.ChatGpt4)
		_, err = BOT.SendMessage(tu.Message(tu.ID(callbackQuery.From.ID), "Switched to GPT-4 model, very intelligent, but slow and expensive! Don't forget to check /status regularly to avoid hitting the usage cap."))
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to send GPT-4 message: %v", err)
		}
		err = BOT.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "Switched to " + callbackQuery.Data + " engine!",
		})
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to answer callback query: %v", err)
		}
		return
	}
	log.Errorf("Unknown engine switch callback query: %s, user id: %s", callbackQuery.Data, chatIDString)
}

func getVoiceTransript(ctx context.Context, bot *telego.Bot, message telego.Message) string {
	chatID := util.GetChatID(&message)
	chatIDString := util.GetChatIDString(&message)
	voice := message.Voice
	voiceMessageFileData, err := bot.GetFile(&telego.GetFileParams{FileID: voice.FileID})
	if err != nil {
		log.Errorf("Failed to get voice message in chat %s: %v", chatIDString, err)
		_, err = bot.SendMessage(tu.Message(chatID, "Failed to get the voice message, please try again"))
		if err != nil {
			log.Errorf("Failed to send message in chat %s: %v", chatIDString, err)
		}
		return ""
	}
	log.Debugf("Voice message file data: %+v", voiceMessageFileData)

	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", bot.Token(), voiceMessageFileData.FilePath)
	response, err := http.Get(fileURL)
	if err != nil {
		log.Errorf("Error downloading file in chat %s: %v", chatIDString, err)
		return ""
	}
	defer response.Body.Close()

	// create uuid for the file
	temporaryFileName := uuid.New().String()
	ogaFile := "/data/" + temporaryFileName + ".oga"
	webmFile := "/data/" + temporaryFileName + ".webm"

	// save response.Body to a temporary file
	f, err := os.Create(ogaFile)
	if err != nil {
		log.Errorf("Error creating file %s in chat %s: %v", ogaFile, chatIDString, err)
		return ""
	}
	defer f.Close()
	defer safeOsDelete(ogaFile)
	_, err = io.Copy(f, response.Body)
	if err != nil {
		log.Errorf("Error saving voice message in chat %s: %v", chatIDString, err)
		return ""
	}

	// convert .oga audio format into one of ['m4a', 'mp3', 'webm', 'mp4', 'mpga', 'wav', 'mpeg']
	duration, err := converters.ConvertWithFFMPEG(ogaFile, webmFile)
	defer safeOsDelete(webmFile)
	if err != nil {
		log.Errorf("Error converting voice message in chat %s: %v", chatIDString, err)
		return ""
	}
	log.Infof("Parsed voice message in chat %s, duration: %s", chatIDString, duration)

	// read the converted file
	webmBuffer, err := os.ReadFile(webmFile)
	if err != nil {
		log.Errorf("Error reading voice message in chat %s: %v", chatIDString, err)
		return ""
	}

	whisper := openai.NewWhisper()
	whisper.Whisper(
		context.WithValue(ctx, models.WhisperDurationContext{}, duration),
		BOT.WhisperConfig,
		io.NopCloser(bytes.NewReader(webmBuffer)),
		temporaryFileName+".webm")

	if whisper.Transcript().Text == "" {
		bot.SendMessage(tu.Message(chatID, "Couldn't transcribe the voice message, maybe next time?"))
		return ""
	}
	bot.SendMessage(tu.Message(chatID, "🗣: "+whisper.Transcript().Text))

	return whisper.Transcript().Text
}

func sendTypingAction(bot *telego.Bot, chatID telego.ChatID) {
	err := bot.SendChatAction(&telego.SendChatActionParams{ChatID: chatID, Action: telego.ChatActionTyping})
	if err != nil {
		log.Errorf("Failed to send chat action: %v", err)
	}
}

func safeOsDelete(filename string) {
	err := os.Remove(filename)
	if err != nil {
		log.Errorf("Error deleting file %s: %v", filename, err)
	}
}
