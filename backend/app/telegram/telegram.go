// main package to control telegram bot
package telegram

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

	botInfo, err := bot.GetMe()
	if err != nil {
		return nil, fmt.Errorf("failed to get bot info: %w", err)
	} else {
		log.Infof("Bot info: %+v", botInfo)
		cfg.BotName = botInfo.Username
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
			AllowedUpdates: []string{
				"message",
				"callback_query",
				// TODO: uncomment these when https://github.com/mymmrac/telego/pull/157/files lands
				// "message_reaction",
				// "message_reaction_count",
			},
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
	isPrivate := message.Chat.Type == "private"
	_, ctx, cancelContext, err := lib.SetupUserAndContext(chatIDString, "telegram", chatIDString)
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
	if message.Voice == nil && message.Audio == nil && message.Video == nil && message.VideoNote == nil && message.Document == nil && message.Photo == nil && (message.Text == string(EmptyCommand) || strings.HasPrefix(message.Text, "/")) {
		if !isPrivate && !strings.Contains(message.Text, "@"+BOT.Name) {
			log.Infof("Ignoring public command w/o @mention in channel: %s", chatIDString)
			return
		}
		AllCommandHandlers.handleCommand(ctx, BOT, &message)
		return
	}

	mode := lib.GetMode(chatIDString)
	// while in channels, only react to
	// 1. @mentions
	// 2. audio messages in /transcribe mode
	// 3. /grammar fixes
	if !isPrivate && mode != lib.Transcribe && mode != lib.Grammar && !strings.Contains(message.Text, "@"+BOT.Name) {
		log.Infof("Ignoring public message w/o @mention and not in transcribe or grammar mode in channel: %s", chatIDString)
		return
	}

	if message.Video != nil && strings.HasPrefix(message.Caption, string(SYSTEMSetOnboardingVideoCommand)) {
		log.Infof("System command received: %+v", message) // audit
		message.Text = string(SYSTEMSetOnboardingVideoCommand)
		AllCommandHandlers.handleCommand(ctx, BOT, &message)
		return
	}

	// user usage exceeded monthly limit, send message and return
	ok := lib.ValidateUserUsage(ctx)
	if !ok {
		bot.SendMessage(tu.Message(chatID, "Your monthly usage limit has been exceeded. Check /status and /upgrade your subscription to continue using the bot."))
		config.CONFIG.DataDogClient.Incr("telegram.usage_exceeded", []string{"client:telegram", "channel_type:" + message.Chat.Type}, 1)
		return
	}

	if message.Text != "" {
		config.CONFIG.DataDogClient.Incr("telegram.text_message_received", []string{"channel_type:" + message.Chat.Type}, 1)
		if mode == lib.Transcribe {
			if isPrivate {
				bot.SendMessage(tu.Message(chatID, "The bot is in /transcribe mode. Please send a voice/audio/video message to transcribe or change to another mode (/status)."))
			}
			return
		}
	}

	voiceTranscriptionText := ""
	// if the message is voice/audio/video message, process it to upload to WhisperAI API and get the transcription
	if message.Voice != nil || message.Audio != nil || message.Video != nil || message.VideoNote != nil || message.Document != nil {
		voice_type := "voice"
		switch {
		case message.Audio != nil:
			voice_type = "audio"
		case message.Video != nil:
			voice_type = "video"
		case message.VideoNote != nil:
			voice_type = "note"
		case message.Document != nil:
			voice_type = "document"
		}
		config.CONFIG.DataDogClient.Incr("telegram.voice_message_received", []string{"type:" + voice_type, "channel_type:" + message.Chat.Type}, 1)

		// send typing action to show that bot is working
		if mode != lib.VoiceGPT {
			sendTypingAction(bot, chatID)
		} else {
			sendAudioAction(bot, chatID)
		}
		voiceTranscriptionText = getVoiceTranscript(ctx, bot, message)
		// combine message text with transcription
		if voiceTranscriptionText != "" {
			message.Text = message.Text + "\n" + voiceTranscriptionText
		}

		// process commands again if it was a voice command
		if message.Text == string(EmptyCommand) || strings.HasPrefix(message.Text, "/") {
			AllCommandHandlers.handleCommand(ctx, BOT, &message)
			return
		}

		if mode == lib.Transcribe {
			ChunkSendMessage(bot, chatID, voiceTranscriptionText)
			return
		}

		if mode != lib.VoiceGPT && !(mode == lib.Grammar && !isPrivate) {
			ChunkSendMessage(bot, chatID, "🗣:\n"+voiceTranscriptionText)
		}
	}

	if message.Photo != nil {
		config.CONFIG.DataDogClient.Incr("telegram.photo_message_received", []string{"channel_type:" + message.Chat.Type}, 1)
	}

	var seedData []models.Message
	var userMessagePrimer string
	seedData, userMessagePrimer = lib.GetSeedDataAndPrimer(mode)

	log.Debugf("Received message: %d, in chat: %d, initiating request to OpenAI", message.MessageID, chatID.ID)
	engineModel := redis.GetChatEngine(chatIDString)

	// send action to show that bot is working
	if mode != lib.VoiceGPT {
		sendTypingAction(bot, chatID)
	} else {
		sendAudioAction(bot, chatID)
	}
	if mode == lib.ChatGPT || mode == lib.VoiceGPT {
		ProcessThreadedMessage(ctx, bot, &message, mode, engineModel)
	} else if mode == lib.Summarize || (mode == lib.Grammar && isPrivate) {
		ProcessStreamingMessage(ctx, bot, &message, seedData, userMessagePrimer, mode, engineModel, cancelContext)
	} else {
		ProcessNonStreamingMessage(ctx, bot, &message, seedData, userMessagePrimer, mode, engineModel)
	}
}

func handleCallbackQuery(bot *telego.Bot, callbackQuery telego.CallbackQuery) {
	userId := callbackQuery.From.ID
	chat := callbackQuery.Message.GetChat()
	chatId := chat.ID
	chatType := chat.Type
	log.Infof("Received callback query: %s, for user: %d in chat %d", callbackQuery.Data, userId, chatId)
	config.CONFIG.DataDogClient.Incr("telegram.callback_query", []string{"data:" + callbackQuery.Data, "channel_type:" + chatType}, 1)
	switch callbackQuery.Data {
	case "like":
		log.Infof("User %d liked a message in chat %d.", userId, chatId)
		config.CONFIG.DataDogClient.Incr("telegram.like", []string{"channel_type:" + chatType}, 1)
		bot.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "Thanks for your feedback! 👍",
		})
	case "dislike":
		log.Infof("User %d disliked a message in chat %d.", userId, chatId)
		config.CONFIG.DataDogClient.Incr("telegram.dislike", []string{"channel_type:" + chatType}, 1)
		bot.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "Thanks for your feedback!",
		})
	case string(lib.ChatGPT), string(lib.VoiceGPT), string(lib.Grammar), string(lib.Teacher), string(lib.Summarize), string(lib.Transcribe):
		handleCommandsInCallbackQuery(callbackQuery)
	case string(models.ChatGpt35Turbo), string(models.ChatGpt4):
		handleEngineSwitchCallbackQuery(callbackQuery)
	default:
		log.Errorf("Unknown callback query: %s", callbackQuery.Data)
	}
}

func handleCommandsInCallbackQuery(callbackQuery telego.CallbackQuery) {
	chat := callbackQuery.Message.GetChat()
	chatIDString := fmt.Sprint(chat.ID)
	ctx := context.WithValue(context.Background(), models.UserContext{}, chatIDString)
	ctx = context.WithValue(ctx, models.ClientContext{}, "telegram")
	message := telego.Message{
		Chat: telego.Chat{ID: chat.ID},
		Text: "/" + callbackQuery.Data,
	}
	AllCommandHandlers.handleCommand(ctx, BOT, &message)
}

func handleEngineSwitchCallbackQuery(callbackQuery telego.CallbackQuery) {
	chat := callbackQuery.Message.GetChat()
	chatID := callbackQuery.From.ID
	if callbackQuery.Message != nil && chat.ID != chatID {
		log.Infof("Callback query message in chat ID: %d, user ID: %d", chat.ID, chatID)
		chatID = chat.ID
	}
	chatIDString := fmt.Sprint(chatID)
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
		_, err := BOT.SendMessage(tu.Message(tu.ID(chatID), "Switched to GPT-3.5 Turbo model, fast and cheap!"))
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
			BOT.SendMessage(tu.Message(tu.ID(chatID), "Failed to switch to GPT model, please try again later"))
			return
		}
		if user.SubscriptionType.Name == models.FreeSubscriptionName || user.SubscriptionType.Name == models.FreePlusSubscriptionName {
			BOT.SendMessage(tu.Message(tu.ID(chatID), "You need to /upgrade your subscription to use GPT-4 engine! Meanwhile, you can still use GPT-3.5 Turbo model, it's fast, cheap and quite smart."))
			return
		}
		redis.SaveEngine(chatIDString, models.ChatGpt4)
		_, err = BOT.SendMessage(tu.Message(tu.ID(chatID), "Switched to GPT-4 model, very intelligent, but slower and expensive! Don't forget to check /status regularly to avoid hitting the usage cap."))
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
	log.Errorf("Unknown engine switch callback query: %s, chat id: %s", callbackQuery.Data, chatIDString)
}

func getVoiceTranscript(ctx context.Context, bot *telego.Bot, message telego.Message) string {
	chatID := util.GetChatID(&message)
	chatIDString := util.GetChatIDString(&message)

	var fileId string
	switch {
	case message.Voice != nil:
		fileId = message.Voice.FileID
	case message.Audio != nil:
		fileId = message.Audio.FileID
	case message.Video != nil:
		fileId = message.Video.FileID
	case message.VideoNote != nil:
		fileId = message.VideoNote.FileID
	case message.Document != nil:
		fileId = message.Document.FileID
	default:
		log.Errorf("No voice/audio/video message in chat %s", chatIDString)
		return ""
	}
	fileData, err := bot.GetFile(&telego.GetFileParams{FileID: fileId})
	if err != nil {
		log.Errorf("Failed to get voice/audio/video file data in chat %s: %v", chatIDString, err)
		if strings.Contains(err.Error(), "file is too big") {
			_, _ = bot.SendMessage(tu.Message(chatID, "Telegram API doesn't support downloading files bigger than 20Mb, try sending a shorter voice/audio/video message."))
			return ""
		}
		_, err = bot.SendMessage(tu.Message(chatID, "Something went wrong while getting voice/audio/video file, please try again."))
		if err != nil {
			log.Errorf("Failed to send message in chat %s: %v", chatIDString, err)
		}
		return ""
	}
	log.Debugf("Voice message file data: %+v", fileData)

	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", bot.Token(), fileData.FilePath)
	response, err := http.Get(fileURL)
	if err != nil {
		log.Errorf("Error downloading file in chat %s: %v", chatIDString, err)
		return ""
	}
	defer response.Body.Close()

	// create uuid for the file
	temporaryFileName := uuid.New().String()
	temporaryFileExtension := filepath.Ext(fileData.FilePath)
	if message.Voice != nil {
		temporaryFileExtension = ".oga"
	}
	sourceFile := "/data/" + temporaryFileName + temporaryFileExtension
	whisperFile := "/data/" + temporaryFileName + ".ogg"

	// save response.Body to a temporary file
	f, err := os.Create(sourceFile)
	if err != nil {
		log.Errorf("Error creating file %s in chat %s: %v", sourceFile, chatIDString, err)
		return ""
	} else {
		log.Infof("Created file %s for conversion in chat %s, size: %d", sourceFile, chatIDString, fileData.FileSize)
	}
	defer f.Close()
	defer safeOsDelete(sourceFile)
	_, err = io.Copy(f, response.Body)
	if err != nil {
		log.Errorf("Error saving voice message in chat %s: %v", chatIDString, err)
		return ""
	}

	// convert .oga audio format into one of ['m4a', 'mp3', 'webm', 'mp4', 'mpga', 'wav', 'mpeg']
	duration, err := converters.ConvertWithFFMPEG(sourceFile, whisperFile)
	defer safeOsDelete(whisperFile)
	if err != nil {
		log.Errorf("Error converting voice message in chat %s: %v", chatIDString, err)
		return ""
	}
	log.Infof("Parsed voice message in chat %s, duration: %s", chatIDString, duration)

	// read the converted file
	webmBuffer, err := os.ReadFile(whisperFile)
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
		log.Warnf("Failed to transcribe voice message in chat %s from %s, size %d", chatIDString, fileData.FilePath, fileData.FileSize)
		bot.SendMessage(tu.Message(chatID, "Couldn't transcribe the voice/audio/video message, maybe next time?"))
		return ""
	}

	return whisper.Transcript().Text
}

func sendTypingAction(bot *telego.Bot, chatID telego.ChatID) {
	err := bot.SendChatAction(&telego.SendChatActionParams{ChatID: chatID, Action: telego.ChatActionTyping})
	if err != nil {
		log.Errorf("Failed to send chat action: %v", err)
	}
}

func sendAudioAction(bot *telego.Bot, chatID telego.ChatID) {
	err := bot.SendChatAction(&telego.SendChatActionParams{ChatID: chatID, Action: telego.ChatActionRecordVoice})
	if err != nil {
		log.Errorf("Failed to send chat action: %v", err)
	}
}

func sendFindAction(bot *telego.Bot, chatID telego.ChatID) {
	err := bot.SendChatAction(&telego.SendChatActionParams{ChatID: chatID, Action: telego.ChatActionFindLocation})
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
