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
	"strconv"
	"strings"
	"talk2robots/m/v2/app/ai"
	"talk2robots/m/v2/app/ai/openai"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/converters"
	"talk2robots/m/v2/app/db/mongo"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/models"
	"talk2robots/m/v2/app/payments"
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
	*ai.API
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
	bh.HandleInlineQuery(handleInlineQuery)
	bh.HandleChosenInlineResult(handleChosenInlineResult)
	go bh.Start()

	BOT = &Bot{
		API:        ai.NewAPI(cfg),
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
				"inline_query",
				"chosen_inline_result",
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
	topicID := util.GetTopicID(&message)
	isPrivate := message.Chat.Type == "private"
	_, ctx, cancelContext, err := lib.SetupUserAndContext(chatIDString, "telegram", chatIDString, topicID)
	if err != nil {
		if err == lib.ErrUserBanned {
			log.Infof("User %s is banned", chatIDString)
			return
		}

		log.Errorf("Error setting up user and context: %v", err)
		return
	}

	// process commands
	if message.Voice == nil && message.Audio == nil && message.Video == nil && message.VideoNote == nil && message.Document == nil && message.Photo == nil && (message.Text == string(EmptyCommand) || strings.HasPrefix(message.Text, "/")) {
		if !isPrivate && !strings.Contains(message.Text, "@"+BOT.Name) {
			log.Infof("Ignoring public command w/o @mention in channel: %s", chatIDString)
			return
		}
		chatMember, err := bot.GetChatMember(&telego.GetChatMemberParams{
			ChatID: chatID,
			UserID: message.From.ID,
		})
		if err != nil {
			log.Errorf("Error getting chat member: %v", err)
			return
		}
		if err == nil && !isPrivate && chatMember.MemberStatus() != telego.MemberStatusCreator && chatMember.MemberStatus() != telego.MemberStatusAdministrator {
			log.Infof("Ignoring public command from non-admin in channel: %s", chatIDString)
			return
		}
		AllCommandHandlers.handleCommand(ctx, BOT, &message)
		return
	}

	mode, params := lib.GetMode(chatIDString, topicID)
	log.Infof("chat %s, mode: %s, params: %s", chatIDString, mode, params)
	ctx = context.WithValue(ctx, models.ParamsContext{}, params)
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
		notification := "Your monthly usage limit has been exceeded. Check /status and /upgrade your subscription to continue using the bot. The limits are reset on the 1st of every month."
		notification = lib.AddBotSuffixToGroupCommands(ctx, notification)
		bot.SendMessage(tu.Message(chatID, notification).WithMessageThreadID(message.MessageThreadID))
		config.CONFIG.DataDogClient.Incr("telegram.usage_exceeded", []string{"client:telegram", "channel_type:" + message.Chat.Type}, 1)
		return
	}

	voiceTranscriptionText := ""
	// if the message is voice/audio/video message, process it to upload to WhisperAI API and get the transcription
	if message.Voice != nil || message.Audio != nil || message.Video != nil || message.VideoNote != nil {
		voice_type := "voice"
		switch {
		case message.Audio != nil:
			voice_type = "audio"
		case message.Video != nil:
			voice_type = "video"
		case message.VideoNote != nil:
			voice_type = "note"
		}
		config.CONFIG.DataDogClient.Incr("telegram.voice_message_received", []string{"type:" + voice_type, "channel_type:" + message.Chat.Type}, 1)

		// send typing action to show that bot is working
		if mode != lib.VoiceGPT {
			sendTypingAction(bot, &message)
		} else {
			sendAudioAction(bot, &message)
		}
		voiceTranscriptionText = getVoiceTranscript(ctx, bot, message)

		if mode != lib.Transcribe {
			// combine message text with transcription
			if voiceTranscriptionText != "" {
				message.Text = message.Text + "\n" + voiceTranscriptionText
			}

			// process commands again if it was a voice command
			if message.Text == string(EmptyCommand) || strings.HasPrefix(message.Text, "/") {
				AllCommandHandlers.handleCommand(ctx, BOT, &message)
				return
			}
		}
	}

	if mode == lib.Transcribe {
		ChunkSendMessage(bot, &message, voiceTranscriptionText)
		if isPrivate && message.Text != "" {
			bot.SendMessage(tu.Message(chatID, "The bot is in /transcribe mode. Please send a voice/audio/video message to transcribe or change to another mode (/status)."))
		}
		return
	}

	if mode != lib.VoiceGPT && !(mode == lib.Grammar && !isPrivate) && voiceTranscriptionText != "" {
		ChunkSendMessage(bot, &message, "🗣:\n"+voiceTranscriptionText)
	}

	if message.Text != "" {
		config.CONFIG.DataDogClient.Incr("telegram.text_message_received", []string{"channel_type:" + message.Chat.Type}, 1)
	} else {
		log.Infof("Ignoring empty message in chat: %s", chatIDString)
		return
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
		sendTypingAction(bot, &message)
	} else {
		sendAudioAction(bot, &message)
	}
	if mode == lib.ChatGPT || mode == lib.VoiceGPT {
		go ProcessThreadedStreamingMessage(ctx, bot, &message, mode, engineModel, cancelContext)
	} else if mode == lib.Summarize || (mode == lib.Grammar && isPrivate) {
		go ProcessChatCompleteStreamingMessage(ctx, bot, &message, seedData, userMessagePrimer, mode, engineModel, cancelContext)
	} else {
		go ProcessChatCompleteNonStreamingMessage(ctx, bot, &message, seedData, userMessagePrimer, mode, engineModel)
	}
}

func handleCallbackQuery(bot *telego.Bot, callbackQuery telego.CallbackQuery) {
	userId := callbackQuery.From.ID
	chat := callbackQuery.Message.GetChat()
	messageId := callbackQuery.Message.GetMessageID()

	// log.Infof("Received callback query: %s, for user: %d in chat %d, messageId %d, fetching message..", callbackQuery.Data, userId, chat.ID, messageId)
	chatId := chat.ID
	chatString := fmt.Sprintf("%d", chatId)
	chatType := chat.Type

	// we pass the topic id / message thread id in the callback query data
	callbackParams := strings.Split(callbackQuery.Data, ":")
	topicString := ""
	topicId := 0
	if len(callbackParams) > 1 {
		topicString = callbackParams[1]
		topicId, _ = strconv.Atoi(topicString)
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, models.TopicContext{}, topicString)

	callbackQuery.Data = callbackParams[0]

	log.Infof("Callback query %s for user: %d in chat %d, topic %s, messageId %d", callbackQuery.Data, userId, chatId, topicString, messageId)
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
		handleCommandsInCallbackQuery(callbackQuery, topicString)
	case "models":
		bot.EditMessageReplyMarkup(&telego.EditMessageReplyMarkupParams{
			ChatID:      chat.ChatID(),
			MessageID:   messageId,
			ReplyMarkup: GetModelsKeyboard(ctx),
		})
	case "status":
		bot.EditMessageReplyMarkup(&telego.EditMessageReplyMarkupParams{
			ChatID:      chat.ChatID(),
			MessageID:   messageId,
			ReplyMarkup: GetStatusKeyboard(ctx),
		})
	case string(models.ChatGpt35Turbo), string(models.ChatGpt4), string(models.LlamaV3_8b), string(models.LlamaV3_70b):
		handleEngineSwitchCallbackQuery(callbackQuery, topicString)
	case "downgradefromfreeplus":
		_, ctx, _, _ := lib.SetupUserAndContext(chatString, "telegram", chatString, topicString)
		if !lib.IsUserFreePlus(ctx) {
			return
		}
		err := mongo.MongoDBClient.UpdateUserSubscription(ctx, models.Subscriptions[models.FreeSubscriptionName])
		if err != nil {
			log.Errorf("Failed to downgrade user %s subscription: to free %v", chatString, err)
			bot.SendMessage(tu.Message(tu.ID(chatId), "Failed to downgrade your account to free plan. Please try again later.").WithMessageThreadID(topicId))
			return
		}
		bot.SendMessage(tu.Message(tu.ID(chatId), "You are now a free user!").WithMessageThreadID(topicId))
	case "downgradefrombasic":
		user, ctx, _, err := lib.SetupUserAndContext(chatString, "telegram", chatString, topicString)
		if err != nil {
			log.Errorf("Failed to get user %s: %v", chatString, err)
			return
		}
		if !lib.IsUserBasic(ctx) {
			return
		}

		stripeCustomerId := user.StripeCustomerId
		if stripeCustomerId == "" {
			err := mongo.MongoDBClient.UpdateUserSubscription(ctx, models.Subscriptions[models.FreePlusSubscriptionName])
			if err != nil {
				log.Errorf("Failed to downgrade user %s subscription: to free+ %v", chatString, err)
				bot.SendMessage(tu.Message(tu.ID(chatId), "Failed to downgrade your account to free+ plan. Please try again later.").WithMessageThreadID(topicId))
			}
			bot.SendMessage(tu.Message(tu.ID(chatId), "You are now a free+ user!").WithMessageThreadID(topicId))
			return
		}

		// cancel subscriptions
		payments.StripeCancelSubscription(ctx, stripeCustomerId)
	case "cancel":
		// delete the message
		bot.DeleteMessage(&telego.DeleteMessageParams{
			ChatID:    tu.ID(chatId),
			MessageID: callbackQuery.Message.GetMessageID(),
		})
	default:
		log.Errorf("Unknown callback query: %s, chat id: %s", callbackQuery.Data, chatString)
	}
}

func handleCommandsInCallbackQuery(callbackQuery telego.CallbackQuery, topicString string) {
	chat := callbackQuery.Message.GetChat()
	chatIDString := fmt.Sprint(chat.ID)
	topicID, _ := strconv.Atoi(topicString)
	_, ctx, _, _ := lib.SetupUserAndContext(chatIDString, "telegram", chatIDString, topicString)
	message := telego.Message{
		Chat:            chat,
		Text:            "/" + callbackQuery.Data,
		MessageThreadID: topicID,
	}
	AllCommandHandlers.handleCommand(ctx, BOT, &message)
}

func handleEngineSwitchCallbackQuery(callbackQuery telego.CallbackQuery, topicString string) {
	chat := callbackQuery.Message.GetChat()
	chatID := callbackQuery.From.ID
	if callbackQuery.Message != nil && chat.ID != chatID {
		chatID = chat.ID
	}
	log.Infof("Callback query message in chat ID: %d, user ID: %d, topic: %s", chat.ID, chatID, util.GetTopicIDFromChat(chat))
	chatIDString := fmt.Sprint(chatID)
	topicID, _ := strconv.Atoi(topicString)
	_, ctx, _, _ := lib.SetupUserAndContext(chatIDString, "telegram", chatIDString, topicString)
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
	if callbackQuery.Data == string(models.LlamaV3_8b) {
		redis.SaveEngine(chatIDString, models.LlamaV3_8b)
		_, err := BOT.SendMessage(tu.Message(tu.ID(chatID), "Switched to small Llama3 model, fast and cheap! Note that /chatgpt and /voicegpt modes don't have context awareness (memory) when using Llama models at the moment.").WithMessageThreadID(topicID))
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to send Llama3 small message: %v", err)
		}
		err = BOT.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "Switched to small Llama3 model!",
		})
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to answer callback query: %v", err)
		}
		return
	}
	if callbackQuery.Data == string(models.ChatGpt35Turbo) {
		redis.SaveEngine(chatIDString, models.ChatGpt35Turbo)
		_, err := BOT.SendMessage(tu.Message(tu.ID(chatID), "Switched to GPT-3.5 Turbo model, fast and cheap!").WithMessageThreadID(topicID))
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
			BOT.SendMessage(tu.Message(tu.ID(chatID), "Failed to switch to GPT-4 model, please try again later").WithMessageThreadID(topicID))
			return
		}
		if user.SubscriptionType.Name == models.FreeSubscriptionName || user.SubscriptionType.Name == models.FreePlusSubscriptionName {
			notification := "You need to /upgrade your subscription to use GPT-4 model! Meanwhile, you can still use GPT-3.5 Turbo, it's fast, cheap and quite smart."
			notification = lib.AddBotSuffixToGroupCommands(ctx, notification)
			BOT.SendMessage(tu.Message(tu.ID(chatID), notification).WithMessageThreadID(topicID))
			return
		}
		redis.SaveEngine(chatIDString, models.ChatGpt4)
		notification := "Switched to GPT-4 model, very intelligent, but slower and expensive! Don't forget to check /status regularly to avoid hitting the usage cap."
		notification = lib.AddBotSuffixToGroupCommands(ctx, notification)
		_, err = BOT.SendMessage(tu.Message(tu.ID(chatID), notification).WithMessageThreadID(topicID))
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
	if callbackQuery.Data == string(models.LlamaV3_70b) {
		// fetch user subscription
		user, err := mongo.MongoDBClient.GetUser(ctx)
		if err != nil {
			log.Errorf("Failed to get user: %v", err)
			BOT.SendMessage(tu.Message(tu.ID(chatID), "Failed to switch to Llama3 big model, please try again later").WithMessageThreadID(topicID))
			return
		}
		if user.SubscriptionType.Name == models.FreeSubscriptionName || user.SubscriptionType.Name == models.FreePlusSubscriptionName {
			notification := "You need to /upgrade your subscription to use big Llama3 model! Meanwhile, you can still use small Llama3 model, it's fast, cheap and quite smart."
			notification = lib.AddBotSuffixToGroupCommands(ctx, notification)
			BOT.SendMessage(tu.Message(tu.ID(chatID), notification).WithMessageThreadID(topicID))
			return
		}
		redis.SaveEngine(chatIDString, models.LlamaV3_70b)
		notification := "Switched to big Llama3 model, very intelligent, but slower and expensive! Don't forget to check /status regularly to avoid hitting the usage cap. Note that /chatgpt and /voicegpt modes don't have context awareness (memory) when using Llama models at the moment."
		notification = lib.AddBotSuffixToGroupCommands(ctx, notification)
		_, err = BOT.SendMessage(tu.Message(tu.ID(chatID), notification).WithMessageThreadID(topicID))
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to send big Llama3 message: %v", err)
		}
		err = BOT.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "Switched to big Llama3 engine!",
		})
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to answer callback query: %v", err)
		}
		return
	}

	log.Errorf("Unknown engine switch callback query: %s, chat id: %s", callbackQuery.Data, chatIDString)
}

func handleInlineQuery(bot *telego.Bot, inlineQuery telego.InlineQuery) {
	chatID := inlineQuery.From.ID
	chatIDString := fmt.Sprint(chatID)
	_, ctx, _, _ := lib.SetupUserAndContext(chatIDString, "telegram", chatIDString, "")
	if inlineQuery.Query == "" {
		inlineQuery.Query = "What can you do?"
	}
	log.Infof("Inline query from ID: %d, query size: %d", inlineQuery.From.ID, len(inlineQuery.Query))

	ok := lib.ValidateUserUsage(ctx)
	if !ok {
		config.CONFIG.DataDogClient.Incr("telegram.usage_exceeded", []string{"client:telegram", "channel_type:inline"}, 1)
	}

	config.CONFIG.DataDogClient.Incr("telegram.inline_message_received", []string{"channel_type:" + inlineQuery.ChatType}, 1)

	// get the response
	response, err := BOT.API.ChatComplete(ctx, models.ChatCompletion{
		Model: string(models.LlamaV3_8b),
		Messages: []models.Message{
			{
				Role:    "system",
				Content: openai.AssistantInstructions,
			},
			{
				Role:    "user",
				Content: inlineQuery.Query,
			},
		},
		MaxTokens: 1024,
	})
	if err != nil {
		log.Errorf("Failed to get completion for inline query: %v", err)
		return
	}
	params := &telego.AnswerInlineQueryParams{
		InlineQueryID: inlineQuery.ID,
		CacheTime:     60 * 60 * 24, // 24 hours
	}
	params.WithResults(&telego.InlineQueryResultArticle{
		Type:         "article",
		ID:           "0",
		Title:        "FastGPT",
		URL:          "https://t.me/gienjibot",
		ThumbnailURL: "https://gienji.me/assets/images/image01.jpg",
		Description:  response,
		InputMessageContent: &telego.InputTextMessageContent{
			MessageText: response,
			ParseMode:   "HTML",
		},
	})
	err = bot.AnswerInlineQuery(params)

	// retry w/o parse mode if failed
	if err != nil && strings.Contains(err.Error(), "can't parse entities") {
		params.Results[0].(*telego.InlineQueryResultArticle).InputMessageContent.(*telego.InputTextMessageContent).ParseMode = ""
		err = bot.AnswerInlineQuery(params)
	}

	if err != nil {
		log.Errorf("Failed to answer %d inline query: %v", chatID, err)
	}
}

func handleChosenInlineResult(bot *telego.Bot, chosenInlineResult telego.ChosenInlineResult) {
	userID := chosenInlineResult.From.ID
	log.Infof("Chosen inline result from ID: %d, result ID: %s", userID, chosenInlineResult.ResultID)
}

func getVoiceTranscript(ctx context.Context, bot *telego.Bot, message telego.Message) string {
	startTime := time.Now()
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
			_, _ = bot.SendMessage(tu.Message(chatID, "Telegram API doesn't support downloading files bigger than 20Mb, try sending a shorter voice/audio/video message.").WithMessageThreadID(message.MessageThreadID))
			return ""
		}
		_, err = bot.SendMessage(tu.Message(chatID, "Something went wrong while getting voice/audio/video file, please try again.").WithMessageThreadID(message.MessageThreadID))
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
	whisperFileExtension := ".ogg"
	whisperFile := "/data/" + temporaryFileName + whisperFileExtension

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

	// convert .oga audio format into one of ['m4a', 'mp3', 'webm', 'mp4', 'mpga', 'wav', 'mpeg', 'ogg']
	duration, err := converters.ConvertWithFFMPEG(sourceFile, whisperFile)
	defer safeOsDelete(whisperFile)
	if err != nil {
		log.Errorf("Error converting voice message in chat %s: %v", chatIDString, err)
		return ""
	}
	log.Infof("Parsed voice message in chat %s, duration: %s", chatIDString, duration)
	config.CONFIG.DataDogClient.Timing("transcribe.ffmpeg", time.Since(startTime), []string{"format:" + temporaryFileExtension}, 1)
	config.CONFIG.DataDogClient.Timing("transcribe.ffmpeg.per_duration", time.Since(startTime), []string{"format:" + temporaryFileExtension}, duration.Seconds())

	// read the converted file
	whisperBuffer, err := os.ReadFile(whisperFile)
	if err != nil {
		log.Errorf("Error reading voice message in chat %s: %v", chatIDString, err)
		return ""
	}

	whisper := openai.NewWhisper()
	whisper.Whisper(
		context.WithValue(ctx, models.WhisperDurationContext{}, duration),
		BOT.WhisperConfig,
		io.NopCloser(bytes.NewReader(whisperBuffer)),
		temporaryFileName+whisperFileExtension)

	config.CONFIG.DataDogClient.Timing("transcribe.total", time.Since(startTime), []string{"format:" + temporaryFileExtension}, 1)
	config.CONFIG.DataDogClient.Timing("transcribe.total.per_duration", time.Since(startTime), []string{"format:" + temporaryFileExtension}, duration.Seconds())

	if whisper.Transcript().Text == "" {
		log.Warnf("Failed to transcribe voice message in chat %s from %s, size %d", chatIDString, fileData.FilePath, fileData.FileSize)
		bot.SendMessage(tu.Message(chatID, "Couldn't transcribe the voice/audio/video message, maybe next time?").WithMessageThreadID(message.MessageThreadID))
		return ""
	}

	return whisper.Transcript().Text
}

func sendTypingAction(bot *telego.Bot, message *telego.Message) {
	chatID := message.Chat.ChatID()
	err := bot.SendChatAction(&telego.SendChatActionParams{ChatID: chatID, Action: telego.ChatActionTyping, MessageThreadID: message.MessageThreadID})
	if err != nil {
		log.Errorf("Failed to send chat action: %v", err)
	}
}

func sendAudioAction(bot *telego.Bot, message *telego.Message) {
	chatID := message.Chat.ChatID()
	err := bot.SendChatAction(&telego.SendChatActionParams{ChatID: chatID, Action: telego.ChatActionRecordVoice, MessageThreadID: message.MessageThreadID})
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
