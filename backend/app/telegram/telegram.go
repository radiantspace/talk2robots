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
	Handler func(ctx *fasthttp.RequestCtx)
	Server  *fasthttp.Server
	Name    string
	Dummy   bool
	telego.ChatID
	WhisperConfig openai.WhisperConfig
}

var AllCommandHandlers CommandHandlers = CommandHandlers{}
var BOT *Bot

func NewBot(cfg *config.Config) (*Bot, error) {
	bot, err := telego.NewBot(cfg.TelegramBotToken, telego.WithHealthCheck(context.Background()), util.GetBotLoggerOption(cfg))
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	botInfo, err := bot.GetMe(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get bot info: %w", err)
	} else {
		log.Infof("Bot info: %+v", botInfo)
		cfg.BotName = botInfo.Username
	}

	setupCommandHandlers()
	updates, server, err := signBotForUpdates(bot, false) // false means this is a regular bot, not a system bot
	if err != nil {
		return nil, fmt.Errorf("failed to sign bot for updates: %w", err)
	}
	bh, err := th.NewBotHandler(bot, updates) // th.WithStopTimeout(time.Second*10))
	if err != nil {
		return nil, fmt.Errorf("failed to setup bot handler: %w", err)
	}
	bh.HandleMessage(handleMessage)
	bh.HandleCallbackQuery(handleCallbackQuery)
	bh.HandleInlineQuery(handleInlineQuery)
	bh.HandleChosenInlineResult(handleChosenInlineResult)
	bh.Handle(handleGeneralUpdate, th.Any())
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
		Server:  server,
		Handler: server.Handler,
	}

	return BOT, nil
}

func signBotForUpdates(bot *telego.Bot, system bool) (<-chan telego.Update, *fasthttp.Server, error) {
	path := "/bot"
	if system {
		path = "/sbot"
	}
	ctx := context.Background()
	err := bot.SetWebhook(ctx, &telego.SetWebhookParams{
		URL:         util.Env("BACKEND_BASE_URL") + path,
		SecretToken: bot.SecretToken(),
		AllowedUpdates: []string{
			telego.MessageUpdates,
			telego.CallbackQueryUpdates,
			telego.InlineQueryUpdates,
			telego.ChosenInlineResultUpdates,
			telego.MessageReactionUpdates,
			telego.MessageReactionCountUpdates,
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to set webhook: %w", err)
	}

	server := &fasthttp.Server{}
	updates, err := bot.UpdatesViaWebhook(
		context.Background(),
		telego.WebhookFastHTTP(server, "/bot", bot.SecretToken()),
	)
	return updates, server, err
}

func handleMessage(bhctx *th.Context, message telego.Message) error {
	bot := bhctx.Bot()

	return handleMessageWithBot(bot, message)
}

func handleMessageWithBot(bot *telego.Bot, message telego.Message) error {
	chatID := util.GetChatID(&message)
	chatIDString := util.GetChatIDString(&message)
	topicID := util.GetTopicID(&message)
	isPrivate := message.Chat.Type == "private"
	_, ctx, cancelContext, err := lib.SetupUserAndContext(chatIDString, "telegram", chatIDString, topicID)
	if err != nil {
		if err == lib.ErrUserBanned {
			log.Infof("User %s is banned", chatIDString)
			return err
		}

		log.Errorf("Error setting up user and context: %v", err)
		return err
	}

	// process commands
	if message.Voice == nil && message.Audio == nil && message.Video == nil && message.VideoNote == nil && message.Document == nil && message.Photo == nil && (message.Text == string(EmptyCommand) || strings.HasPrefix(message.Text, "/")) {
		if !isPrivate {
			if !strings.Contains(message.Text, "@"+BOT.Name) {
				log.Infof("Ignoring public command w/o @mention in channel: %s", chatIDString)
				return err
			}

			// only allow admins to use commands in channels
			chatMember, err := bot.GetChatMember(context.Background(), &telego.GetChatMemberParams{
				ChatID: chatID,
				UserID: message.From.ID,
			})
			if err != nil {
				log.Errorf("Error getting chat member: %v", err)
				return err
			}
			chatMemberStatus := chatMember.MemberStatus()
			if chatMemberStatus != telego.MemberStatusCreator && chatMemberStatus != telego.MemberStatusAdministrator && chatMemberStatus != telego.MemberStatusLeft {
				log.Infof("Ignoring public command from user %d with status %s in channel: %s", message.From.ID, chatMember.MemberStatus(), chatIDString)
				return nil
			}
		}

		AllCommandHandlers.handleCommand(ctx, BOT, &message)
		return nil
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
		return nil
	}

	if message.Video != nil && strings.HasPrefix(message.Caption, string(SYSTEMSetOnboardingVideoCommand)) {
		log.Infof("System command received: %+v", message) // audit
		message.Text = string(SYSTEMSetOnboardingVideoCommand)
		AllCommandHandlers.handleCommand(ctx, BOT, &message)
		return nil
	}

	// user usage exceeded monthly limit, send message and return
	ok, subscription := lib.ValidateUserUsage(ctx)
	if !ok {
		// notification := "Your monthly usage limit has been exceeded. Check available /upgrade options to continue using the bot. The limits are reset on the 1st of every month."
		// notification = lib.AddBotSuffixToGroupCommands(ctx, notification)
		// bot.SendMessage(tu.Message(chatID, notification).WithMessageThreadID(message.MessageThreadID))

		upgradeCommand := lib.AddBotSuffixToGroupCommands(ctx, string(UpgradeCommand))
		message.Text = upgradeCommand
		config.CONFIG.DataDogClient.Incr("telegram.usage_exceeded", []string{"client:telegram", "channel_type:" + message.Chat.Type, "subscription:" + string(subscription)}, 1)
		AllCommandHandlers.handleCommand(ctx, BOT, &message)
		return nil
	}

	voiceTranscriptionText := ""
	// if the message is voice/audio/video message, process it to upload to WhisperAI API and get the transcription
	if ok, voice_type := util.IsAudioMessage(&message); ok {
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
				return nil
			}
		}
	} else if message.Text != "" {
		config.CONFIG.DataDogClient.Incr("telegram.text_message_received", []string{"channel_type:" + message.Chat.Type}, 1)
	}

	if mode == lib.Transcribe {
		ChunkSendMessage(bot, &message, voiceTranscriptionText)
		if isPrivate && message.Text != "" {
			bot.SendMessage(context.Background(), tu.Message(chatID, "The bot is in /transcribe mode. Please send a voice/audio/video message to transcribe or change to another mode (/status)."))
		}
		return nil
	}

	if mode != lib.VoiceGPT && !(mode == lib.Grammar && !isPrivate) && voiceTranscriptionText != "" {
		ChunkSendMessage(bot, &message, "🗣:\n"+voiceTranscriptionText)
	}

	if IsCreateImageCommand(message.Text) {
		config.CONFIG.DataDogClient.Incr("telegram.create_image_received", []string{"channel_type:" + message.Chat.Type}, 1)
		log.Infof("Generating image in a chat %s..", chatIDString)
		sendImageAction(bot, &message)
		url, revisedPrompt, err := openai.CreateImage(ctx, message.Text)
		if err != nil {
			if strings.Contains(err.Error(), "content_policy_violation") {
				log.Warnf("Content policy violation in chat %s", chatIDString)
				config.CONFIG.DataDogClient.Incr("telegram.image.content_policy_violation", []string{"client:telegram", "channel_type:" + message.Chat.Type}, 1)
				bot.SendMessage(context.Background(), tu.Message(chatID, "Sorry, I can't create an image with that content. Please try again with a different prompt.").WithMessageThreadID(message.MessageThreadID))
				return err
			}
			log.Errorf("Error creating image in chat %s: %v", chatIDString, err)
			return err
		}
		log.Infof("Sending image to chat %s", chatIDString)
		if len(revisedPrompt) > 1000 {
			revisedPrompt = revisedPrompt[:997] + "..."
		}
		if url != "" {
			_, err := bot.SendPhoto(context.Background(), &telego.SendPhotoParams{
				ChatID:          chatID,
				Photo:           telego.InputFile{URL: url},
				Caption:         revisedPrompt,
				MessageThreadID: message.MessageThreadID,
			})

			if err != nil {
				log.Errorf("Error sending image to chat %s: %v", chatIDString, err)
			}
		}
		return nil
	}

	if message.Photo != nil {
		config.CONFIG.DataDogClient.Incr("telegram.photo_message_received", []string{"channel_type:" + message.Chat.Type}, 1)
	}

	var seedData []models.Message
	var userMessagePrimer string
	seedData, userMessagePrimer = lib.GetSeedDataAndPrimer(mode)

	log.Debugf("Received message: %d, in chat: %d, initiating request to AI", message.MessageID, chatID.ID)
	engineModel := redis.GetModel(chatIDString)

	// send action to show that bot is working
	if mode != lib.VoiceGPT {
		sendTypingAction(bot, &message)
	} else {
		sendAudioAction(bot, &message)
	}
	if mode == lib.ChatGPT || mode == lib.VoiceGPT {
		go ProcessThreadedStreamingMessage(ctx, bot, &message, mode, engineModel, cancelContext)
	} else if mode == lib.Summarize || mode == lib.Translate || (mode == lib.Grammar && isPrivate) {
		go ProcessChatCompleteStreamingMessage(ctx, bot, &message, seedData, userMessagePrimer, mode, engineModel, cancelContext)
	} else {
		go ProcessChatCompleteNonStreamingMessage(ctx, bot, &message, seedData, userMessagePrimer, mode, engineModel)
	}

	return nil
}

func handleCallbackQuery(bhctx *th.Context, callbackQuery telego.CallbackQuery) error {
	bot := bhctx.Bot()
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
	ctx = context.WithValue(ctx, models.UserContext{}, chatString)

	callbackQuery.Data = callbackParams[0]

	log.Infof("Callback query %s for user: %d in chat %d, topic %s, messageId %d", callbackQuery.Data, userId, chatId, topicString, messageId)
	config.CONFIG.DataDogClient.Incr("telegram.callback_query", []string{"data:" + callbackQuery.Data, "channel_type:" + chatType}, 1)
	switch callbackQuery.Data {
	case "like":
		log.Infof("User %d liked a message in chat %d.", userId, chatId)
		_, err := bot.EditMessageReplyMarkup(context.Background(), &telego.EditMessageReplyMarkupParams{
			ChatID:      chat.ChatID(),
			MessageID:   messageId,
			ReplyMarkup: nil,
		})
		if err != nil {
			log.Errorf("[like] Failed to edit message reply markup in chat %d: %v", chatId, err)
		}
		config.CONFIG.DataDogClient.Incr("telegram.like", []string{"channel_type:" + chatType}, 1)
		bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "Thanks for your feedback! 👍",
		})
	case "dislike":
		log.Infof("User %d disliked a message in chat %d.", userId, chatId)
		_, err := bot.EditMessageReplyMarkup(ctx, &telego.EditMessageReplyMarkupParams{
			ChatID:      chat.ChatID(),
			MessageID:   messageId,
			ReplyMarkup: nil,
		})
		if err != nil {
			log.Errorf("[like] Failed to edit message reply markup in chat %d: %v", chatId, err)
		}
		config.CONFIG.DataDogClient.Incr("telegram.dislike", []string{"channel_type:" + chatType}, 1)
		bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "Thanks for your feedback!",
		})
	case string(lib.ChatGPT), string(lib.VoiceGPT), string(lib.Grammar), string(lib.Teacher), string(lib.Summarize), string(lib.Transcribe), string(lib.Translate):
		handleCommandsInCallbackQuery(callbackQuery, topicString)
	case "models":
		bot.EditMessageReplyMarkup(ctx, &telego.EditMessageReplyMarkupParams{
			ChatID:      chat.ChatID(),
			MessageID:   messageId,
			ReplyMarkup: GetModelsKeyboard(ctx),
		})
	case "images":
		bot.EditMessageReplyMarkup(ctx, &telego.EditMessageReplyMarkupParams{
			ChatID:      chat.ChatID(),
			MessageID:   messageId,
			ReplyMarkup: GetImageModelsKeyboard(ctx),
		})
	case "status":
		bot.EditMessageReplyMarkup(ctx, &telego.EditMessageReplyMarkupParams{
			ChatID:      chat.ChatID(),
			MessageID:   messageId,
			ReplyMarkup: GetStatusKeyboard(ctx),
		})
	case string(models.ChatGpt35Turbo), string(models.ChatGpt4), string(models.ChatGpt4o), string(models.ChatGpt4oMini), string(models.ChatGpt4Turbo), string(models.ChatGpt4TurboVision), string(models.LlamaV3_8b), string(models.LlamaV3_70b), string(models.Sonet35), string(models.Haiku3), string(models.Opus3), string(models.Sonet35_241022), string(models.Grok):
		handleEngineSwitchCallbackQuery(callbackQuery, topicString)
	case string(models.DallE3), string(models.Midjourney6), string(models.StableDiffusion3), string(models.Playground25):
		handleImageModelSwitchCallbackQuery(callbackQuery, topicString)
	case "downgradefromfreeplus":
		_, ctx, _, _ := lib.SetupUserAndContext(chatString, "telegram", chatString, topicString)
		if !lib.IsUserFreePlus(ctx) {
			return nil
		}
		err := mongo.MongoDBClient.UpdateUserSubscription(ctx, models.Subscriptions[models.FreeSubscriptionName])
		if err != nil {
			log.Errorf("Failed to downgrade user %s subscription: to free %v", chatString, err)
			bot.SendMessage(context.Background(), tu.Message(tu.ID(chatId), "Failed to downgrade your account to free plan. Please try again later.").WithMessageThreadID(topicId))
			return err
		}
		bot.SendMessage(context.Background(), tu.Message(tu.ID(chatId), "You are now a free user!").WithMessageThreadID(topicId))
	case "downgradefrombasic":
		user, ctx, _, err := lib.SetupUserAndContext(chatString, "telegram", chatString, topicString)
		if err != nil {
			log.Errorf("Failed to get user %s: %v", chatString, err)
			return err
		}
		if !lib.IsUserBasic(ctx) {
			log.Warnf("User %s is not a basic user", chatString)
			return nil
		}

		stripeCustomerId := user.StripeCustomerId
		if stripeCustomerId == "" {
			err := mongo.MongoDBClient.UpdateUserSubscription(ctx, models.Subscriptions[models.FreePlusSubscriptionName])
			if err != nil {
				log.Errorf("Failed to downgrade user %s subscription: to free+ %v", chatString, err)
				bot.SendMessage(context.Background(), tu.Message(tu.ID(chatId), "Failed to downgrade your account to free+ plan. Please try again later.").WithMessageThreadID(topicId))
			}
			bot.SendMessage(context.Background(), tu.Message(tu.ID(chatId), "You are now a free+ user!").WithMessageThreadID(topicId))
			return err
		}

		// cancel subscriptions
		payments.StripeCancelSubscription(ctx, stripeCustomerId)
	case "cancel":
		// delete the message
		bot.DeleteMessage(ctx, &telego.DeleteMessageParams{
			ChatID:    tu.ID(chatId),
			MessageID: callbackQuery.Message.GetMessageID(),
		})
	case "pending":
		// do nothing
	default:
		log.Errorf("Unknown callback query: %s, chat id: %s", callbackQuery.Data, chatString)
	}

	return nil
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

	BOT.EditMessageReplyMarkup(ctx, &telego.EditMessageReplyMarkupParams{
		ChatID:      chat.ChatID(),
		MessageID:   callbackQuery.Message.GetMessageID(),
		ReplyMarkup: GetStatusKeyboard(ctx),
	})
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
	currentEngine := redis.GetModel(chatIDString)
	if callbackQuery.Data == string(currentEngine) {
		err := BOT.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "You are already using " + callbackQuery.Data + " engine!",
		})
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to answer callback query, already using: %v", err)
		}
		return
	}

	defer func() {
		// update message reply markup
		BOT.EditMessageReplyMarkup(ctx, &telego.EditMessageReplyMarkupParams{
			ChatID:      chat.ChatID(),
			MessageID:   callbackQuery.Message.GetMessageID(),
			ReplyMarkup: GetModelsKeyboard(ctx),
		})
	}()
	if callbackQuery.Data == string(models.LlamaV3_8b) {
		go redis.SaveModel(chatIDString, models.LlamaV3_8b)
		_, err := BOT.SendMessage(ctx, tu.Message(tu.ID(chatID), "Switched to small Llama3 model, fast and cheap!").WithMessageThreadID(topicID))
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to send Llama3 small message: %v", err)
		}
		err = BOT.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "Switched to small Llama3 model!",
		})
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to answer callback query: %v", err)
		}
		return
	}
	if callbackQuery.Data == string(models.ChatGpt35Turbo) {
		go redis.SaveModel(chatIDString, models.ChatGpt35Turbo)
		_, err := BOT.SendMessage(ctx, tu.Message(tu.ID(chatID), "Switched to GPT-3.5 Turbo model, fast and cheap!").WithMessageThreadID(topicID))
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to send GPT-3.5 message: %v", err)
		}
		err = BOT.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "Switched to " + callbackQuery.Data + " engine!",
		})
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to answer callback query: %v", err)
		}
		return
	}
	if callbackQuery.Data == string(models.ChatGpt4oMini) {
		go redis.SaveModel(chatIDString, models.ChatGpt4oMini)
		_, err := BOT.SendMessage(ctx, tu.Message(tu.ID(chatID), "Switched to GPT-4o Mini model, fast and cheap!").WithMessageThreadID(topicID))
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to send GPT-4o Mini message: %v", err)
		}
		err = BOT.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "Switched to " + callbackQuery.Data + " engine!",
		})
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to answer callback query: %v", err)
		}
		return
	}
	if callbackQuery.Data == string(models.ChatGpt4) || callbackQuery.Data == string(models.ChatGpt4Turbo) || callbackQuery.Data == string(models.ChatGpt4TurboVision) || callbackQuery.Data == string(models.ChatGpt4o) {
		// fetch user subscription
		user, err := mongo.MongoDBClient.GetUser(ctx)
		if err != nil {
			log.Errorf("Failed to get user: %v", err)
			BOT.SendMessage(ctx, tu.Message(tu.ID(chatID), fmt.Sprintf("Failed to switch to %s model, please try again later", callbackQuery.Data)).WithMessageThreadID(topicID))
			return
		}
		if user.SubscriptionType.Name == models.FreeSubscriptionName || user.SubscriptionType.Name == models.FreePlusSubscriptionName {
			notification := fmt.Sprintf("To use %s model check available /upgrade options! Meanwhile, you can still use GPT-3.5 Turbo, it's fast, cheap and quite smart.", callbackQuery.Data)
			notification = lib.AddBotSuffixToGroupCommands(ctx, notification)
			BOT.SendMessage(ctx, tu.Message(tu.ID(chatID), notification).WithMessageThreadID(topicID))
			return
		}
		go redis.SaveModel(chatIDString, models.Engine(callbackQuery.Data))
		notification := fmt.Sprintf("Switched to %s model, very intelligent, but slower and expensive! Don't forget to check /status regularly to avoid hitting the usage cap.", callbackQuery.Data)
		notification = lib.AddBotSuffixToGroupCommands(ctx, notification)
		_, err = BOT.SendMessage(ctx, tu.Message(tu.ID(chatID), notification).WithMessageThreadID(topicID))
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to send GPT-4 message: %v", err)
		}
		err = BOT.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "Switched to " + callbackQuery.Data + " engine!",
		})
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to answer callback query: %v", err)
		}
		return
	}
	if callbackQuery.Data == string(models.Sonet35) || callbackQuery.Data == string(models.Haiku3) || callbackQuery.Data == string(models.Opus3) || callbackQuery.Data == string(models.Sonet35_241022) {
		// fetch user subscription
		user, err := mongo.MongoDBClient.GetUser(ctx)
		if err != nil {
			log.Errorf("Failed to get user: %v", err)
			BOT.SendMessage(ctx, tu.Message(tu.ID(chatID), fmt.Sprintf("Failed to switch to %s model, please try again later", callbackQuery.Data)).WithMessageThreadID(topicID))
			return
		}
		if user.SubscriptionType.Name == models.FreeSubscriptionName || user.SubscriptionType.Name == models.FreePlusSubscriptionName {
			notification := fmt.Sprintf("To use %s models check available /upgrade options! Meanwhile, you can still use GPT-3.5 Turbo, it's fast, cheap and quite smart.", "Claude AI")
			notification = lib.AddBotSuffixToGroupCommands(ctx, notification)
			BOT.SendMessage(ctx, tu.Message(tu.ID(chatID), notification).WithMessageThreadID(topicID))
			return
		}
		go redis.SaveModel(chatIDString, models.Engine(callbackQuery.Data))
		notification := fmt.Sprintf("Switched to %s model! Don't forget to check /status regularly to avoid hitting the usage cap.", callbackQuery.Data)
		notification = lib.AddBotSuffixToGroupCommands(ctx, notification)
		_, err = BOT.SendMessage(ctx, tu.Message(tu.ID(chatID), notification).WithMessageThreadID(topicID))
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to send Claude AI message: %v", err)
		}
		err = BOT.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "Switched to " + callbackQuery.Data + " engine!",
		})
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to answer callback query: %v", err)
		}
		return
	}
	if callbackQuery.Data == string(models.LlamaV3_70b) {
		go redis.SaveModel(chatIDString, models.LlamaV3_70b)
		notification := "Switched to big Llama3 model, intelligent, but slower and expensive! Don't forget to check /status regularly to avoid hitting the usage cap."
		notification = lib.AddBotSuffixToGroupCommands(ctx, notification)
		_, err := BOT.SendMessage(ctx, tu.Message(tu.ID(chatID), notification).WithMessageThreadID(topicID))
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to send big Llama3 message: %v", err)
		}
		err = BOT.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "Switched to big Llama3 engine!",
		})
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to answer callback query: %v", err)
		}
		return
	}
	if callbackQuery.Data == string(models.Grok) {
		go redis.SaveModel(chatIDString, models.Grok)
		notification := "Switched to Grok model, intelligent and fun!"
		notification = lib.AddBotSuffixToGroupCommands(ctx, notification)
		_, err := BOT.SendMessage(ctx, tu.Message(tu.ID(chatID), notification).WithMessageThreadID(topicID))
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to send Grok message: %v", err)
		}
		err = BOT.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "Switched to Grok engine!",
		})
		if err != nil {
			log.Errorf("handleEngineSwitchCallbackQuery failed to answer callback query: %v", err)
		}
		return
	}

	log.Errorf("Unknown engine switch callback query: %s, chat id: %s", callbackQuery.Data, chatIDString)
}

func handleImageModelSwitchCallbackQuery(callbackQuery telego.CallbackQuery, topicString string) {
	chat := callbackQuery.Message.GetChat()
	chatID := callbackQuery.From.ID
	if callbackQuery.Message != nil && chat.ID != chatID {
		chatID = chat.ID
	}
	log.Infof("Callback query message in chat ID: %d, user ID: %d, topic: %s", chat.ID, chatID, util.GetTopicIDFromChat(chat))
	chatIDString := fmt.Sprint(chatID)
	topicID, _ := strconv.Atoi(topicString)
	_, ctx, _, _ := lib.SetupUserAndContext(chatIDString, "telegram", chatIDString, topicString)
	currentModel := redis.GetImageModel(chatIDString)
	if callbackQuery.Data == string(currentModel) {
		err := BOT.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQuery.ID,
			Text:            "You are already using " + callbackQuery.Data + " model!",
		})
		if err != nil {
			log.Errorf("handleImageModelSwitchCallbackQuery failed to answer callback query, already using: %v", err)
		}
		return
	}
	go redis.SaveImageModel(chatIDString, models.Engine(callbackQuery.Data))
	notification := fmt.Sprintf("Switched to %s image model, enjoy!", callbackQuery.Data)
	notification = lib.AddBotSuffixToGroupCommands(ctx, notification)
	_, err := BOT.SendMessage(ctx, tu.Message(tu.ID(chatID), notification).WithMessageThreadID(topicID))
	if err != nil {
		log.Errorf("handleImageModelSwitchCallbackQuery failed to send image model message: %v", err)
	}
	err = BOT.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
		CallbackQueryID: callbackQuery.ID,
		Text:            "Switched to " + callbackQuery.Data + " image model!",
	})
	if err != nil {
		log.Errorf("handleImageModelSwitchCallbackQuery failed to answer callback query: %v", err)
	}
}

func handleInlineQuery(bhctx *th.Context, inlineQuery telego.InlineQuery) error {
	bot := bhctx.Bot()
	chatID := inlineQuery.From.ID
	chatIDString := fmt.Sprint(chatID)
	_, ctx, _, _ := lib.SetupUserAndContext(chatIDString, "telegram", chatIDString, "")
	if inlineQuery.Query == "" {
		inlineQuery.Query = "What can you do?"
	}
	log.Infof("Inline query from ID: %d, query size: %d", inlineQuery.From.ID, len(inlineQuery.Query))

	ok, subscription := lib.ValidateUserUsage(ctx)
	if !ok {
		config.CONFIG.DataDogClient.Incr("telegram.usage_exceeded", []string{"client:telegram", "channel_type:inline", "subscription:" + string(subscription)}, 1)
	}

	config.CONFIG.DataDogClient.Incr("telegram.inline_message_received", []string{"channel_type:" + inlineQuery.ChatType}, 1)

	// get the response
	response, err := BOT.API.ChatComplete(ctx, models.ChatCompletion{
		Model: string(models.LlamaV3_8b),
		Messages: []models.Message{
			{
				Role:    "system",
				Content: config.AI_INSTRUCTIONS,
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
		return err
	}
	params := &telego.AnswerInlineQueryParams{
		InlineQueryID: inlineQuery.ID,
		CacheTime:     60 * 60 * 24, // 24 hours
	}
	params.WithResults(&telego.InlineQueryResultArticle{
		Type:         "article",
		ID:           "0",
		Title:        "FastGPT",
		URL:          "https://t.me/gienjibot?start=s=home",
		ThumbnailURL: "https://gienji.me/assets/images/image01.jpg",
		Description:  response,
		InputMessageContent: &telego.InputTextMessageContent{
			MessageText: response,
			ParseMode:   "HTML",
		},
	})
	err = bot.AnswerInlineQuery(context.Background(), params)

	// retry w/o parse mode if failed
	if err != nil && strings.Contains(err.Error(), "can't parse entities") {
		params.Results[0].(*telego.InlineQueryResultArticle).InputMessageContent.(*telego.InputTextMessageContent).ParseMode = ""
		err = bot.AnswerInlineQuery(context.Background(), params)
	}

	if err != nil {
		log.Errorf("Failed to answer %d inline query: %v", chatID, err)
	}

	return err
}

func handleChosenInlineResult(bhctx *th.Context, chosenInlineResult telego.ChosenInlineResult) error {
	userID := chosenInlineResult.From.ID
	log.Infof("Chosen inline result from ID: %d, result ID: %s", userID, chosenInlineResult.ResultID)
	return nil
}

func handleGeneralUpdate(bhctx *th.Context, update telego.Update) error {
	log.Debugf("handleGeneralUpdate: %v", update)

	if update.MessageReaction != nil && update.MessageReaction.NewReaction != nil {
		for _, reaction := range update.MessageReaction.NewReaction {
			reactionType := reaction.ReactionType()
			reactionString := "none"
			mood := "neutral"
			if reactionType == "emoji" {
				// cast to telego.ReactionTypeEmoji
				reactionEmoji := reaction.(*telego.ReactionTypeEmoji).Emoji

				// convert emoji to string representation
				if name, exists := positiveEmojiMap[reactionEmoji]; exists {
					reactionString = name
					mood = "positive"
				} else if name, exists := negativeEmojiMap[reactionEmoji]; exists {
					reactionString = name
					mood = "negative"
				} else {
					log.Warnf("Unknown emoji reaction: %s", reactionEmoji)
				}
			}

			log.Infof("Message reaction in chat %s: %s", fmt.Sprintf("%d", update.MessageReaction.Chat.ID), reactionString)
			config.CONFIG.DataDogClient.Incr("telegram.message_reaction", []string{"channel_type:" + update.MessageReaction.Chat.Type, "reaction:" + reactionString, "mood:" + mood}, 1)
		}
	}

	if update.MessageReactionCount != nil {
		log.Infof("Message reaction count in chat %s: %+v", fmt.Sprintf("%d", update.MessageReactionCount.Chat.ID), update.MessageReactionCount.Reactions)
		for _, reaction := range update.MessageReactionCount.Reactions {
			reactionType := reaction.Type.ReactionType()
			reactionString := "none"
			mood := "neutral"
			if reactionType == "emoji" {
				// cast to telego.ReactionTypeEmoji
				reactionEmoji := reaction.Type.(*telego.ReactionTypeEmoji).Emoji

				// convert emoji to string representation
				if name, exists := positiveEmojiMap[reactionEmoji]; exists {
					reactionString = name
					mood = "positive"
				} else if name, exists := negativeEmojiMap[reactionEmoji]; exists {
					reactionString = name
					mood = "negative"
				} else {
					log.Warnf("Unknown emoji reaction: %s", reactionEmoji)
				}
			}
			reactionCount := reaction.TotalCount
			config.CONFIG.DataDogClient.Count("telegram.message_reaction_count", int64(reactionCount), []string{"channel_type:" + update.MessageReactionCount.Chat.Type, "reaction:" + reactionString, "mood:" + mood}, 1)
		}
	}

	return nil
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
	fileData, err := bot.GetFile(context.Background(), &telego.GetFileParams{FileID: fileId})
	if err != nil {
		log.Errorf("Failed to get voice/audio/video file data in chat %s: %v", chatIDString, err)
		if strings.Contains(err.Error(), "file is too big") {
			_, _ = bot.SendMessage(context.Background(), tu.Message(chatID, "Telegram API doesn't support downloading files bigger than 20Mb, try sending a shorter voice/audio/video message.").WithMessageThreadID(message.MessageThreadID))
			return ""
		}
		_, err = bot.SendMessage(context.Background(), tu.Message(chatID, "Something went wrong while getting voice/audio/video file, please try again.").WithMessageThreadID(message.MessageThreadID))
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
	defer util.SafeOsDelete(sourceFile)
	_, err = io.Copy(f, response.Body)
	if err != nil {
		log.Errorf("Error saving voice message in chat %s: %v", chatIDString, err)
		return ""
	}

	// convert .oga audio format into one of ['m4a', 'mp3', 'webm', 'mp4', 'mpga', 'wav', 'mpeg', 'ogg']
	duration, err := converters.ConvertWithFFMPEG(sourceFile, whisperFile)
	defer util.SafeOsDelete(whisperFile)
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
		bot.SendMessage(context.Background(), tu.Message(chatID, "Couldn't transcribe the voice/audio/video message, maybe next time?").WithMessageThreadID(message.MessageThreadID))
		return ""
	}

	return whisper.Transcript().Text
}

func sendTypingAction(bot *telego.Bot, message *telego.Message) {
	ctx := context.Background()
	chatID := message.Chat.ChatID()
	err := bot.SendChatAction(ctx, &telego.SendChatActionParams{ChatID: chatID, Action: telego.ChatActionTyping, MessageThreadID: message.MessageThreadID})
	if err != nil {
		log.Errorf("Failed to send chat action: %v", err)
	}
}

func sendImageAction(bot *telego.Bot, message *telego.Message) {
	ctx := context.Background()
	chatID := message.Chat.ChatID()
	err := bot.SendChatAction(ctx, &telego.SendChatActionParams{ChatID: chatID, Action: telego.ChatActionUploadPhoto, MessageThreadID: message.MessageThreadID})
	if err != nil {
		log.Errorf("Failed to send chat action: %v", err)
	}
}

func sendAudioAction(bot *telego.Bot, message *telego.Message) {
	ctx := context.Background()
	chatID := message.Chat.ChatID()
	err := bot.SendChatAction(ctx, &telego.SendChatActionParams{ChatID: chatID, Action: telego.ChatActionRecordVoice, MessageThreadID: message.MessageThreadID})
	if err != nil {
		log.Errorf("Failed to send chat action: %v", err)
	}
}
