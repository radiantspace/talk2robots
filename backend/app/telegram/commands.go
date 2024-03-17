package telegram

import (
	"context"
	"fmt"
	"strings"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/db/mongo"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/models"
	"talk2robots/m/v2/app/payments"
	"talk2robots/m/v2/app/util"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	log "github.com/sirupsen/logrus"
)

type Command string

var EMILY_BIRTHDAY = time.Date(2023, 5, 25, 0, 18, 0, 0, time.FixedZone("UTC+2", 3*60*60))
var VASILISA_BIRTHDAY = time.Date(2007, 12, 13, 23, 45, 0, 0, time.FixedZone("UTC+3", 3*60*60))
var ONBOARDING_TEXT = `Hi, I'm a bot powered by OpenAI! I can:
- Default 🧠: chat with or answer any questions (/chatgpt)
- ✨ New feature 🎙️: talk to AI using voice messages (/voicegpt)
- Correct grammar: (/grammar)
- Explain grammar and mistakes: (/teacher)
- ✨ New feature: remember context in /chatgpt and /voicegpt modes (use /clear to clear current thread)
- ✨ New feature: transcribe voice/audio/video messages (/transcribe)
- ✨ New feature: summarize text/voice/audio/video messages (/summarize)

Enjoy and let me know if any /support is needed!`

const (
	CancelSubscriptionCommand Command = "/downgrade"
	EmiliCommand              Command = "/emily"
	EmptyCommand              Command = ""
	ChatGPTCommand            Command = "/chatgpt"
	VoiceGPTCommand           Command = "/voicegpt"
	GrammarCommand            Command = "/grammar"
	StartCommand              Command = "/start"
	StatusCommand             Command = "/status"
	SupportCommand            Command = "/support"
	TermsCommand              Command = "/terms"
	TeacherCommand            Command = "/teacher"
	UpgradeCommand            Command = "/upgrade"
	VasilisaCommand           Command = "/vasilisa"
	TranscribeCommand         Command = "/transcribe"
	SummarizeCommand          Command = "/summarize"
	ClearThreadCommand        Command = "/clear"

	// commands setting for BotFather
	Commands string = `
start - 🚀 onboarding instructions
chatgpt - 🧠 ask AI anything (with memory)
voicegpt - 🎙 talk to AI using voice messages (with memory)
clear - 🧹 clear current conversation memory
grammar - 👀 grammar checking mode only, no explanations
teacher - 🧑‍🏫 grammar correction and explanations
transcribe - 🎙 transcribe voice/audio/video
summarize - 📝 summarize text/voice/audio/video
status - 📊 subscription status
support - 🤔 contact developer for support
terms - 📜 usage terms
`

	// has to use system command here since it's not possible to trasfer fileId from one bot to another
	// be super careful with this command refactoring and make sure that it's not possible to send this command from any other chat
	SYSTEMSetOnboardingVideoCommand Command = "/setonboardingvideo"

	USAGE_TERMS_URL = "https://radiant.space/gienjibot-terms"
)

type CommandHandler struct {
	Command Command
	Handler func(context.Context, *Bot, *telego.Message)
}

type CommandHandlers []*CommandHandler

func setupCommandHandlers() {
	AllCommandHandlers = []*CommandHandler{
		newCommandHandler(EmptyCommand, emptyCommandHandler),
		newCommandHandler(StartCommand, func(ctx context.Context, bot *Bot, message *telego.Message) {
			notification := lib.AddBotSuffixToGroupCommands(ctx, ONBOARDING_TEXT)
			_, err := bot.SendMessage(tu.Message(util.GetChatID(message), notification).WithMessageThreadID(message.MessageThreadID).WithReplyMarkup(GetStatusKeyboard(ctx)))
			if err != nil {
				log.Errorf("Failed to send StartCommand message: %v", err)
			}
			// try getting onboarding video from redis and sending it to the user
			videoFileId := redis.RedisClient.Get(ctx, "onboarding-video").Val()
			if videoFileId == "" || videoFileId == "get onboarding-video: redis: nil" {
				log.Errorf("Failed to get onboarding video from redis: %v", err)
				return
			}
			log.Infof("Sending onboarding video %s to userID: %s", videoFileId, util.GetChatIDString(message))
			_, err = bot.SendVideo(&telego.SendVideoParams{
				ChatID: util.GetChatID(message),
				Video:  telego.InputFile{FileID: videoFileId},
			})
			if err != nil {
				log.Errorf("Failed to send onboarding video: %v", err)
			}
		}),
		newCommandHandler(EmiliCommand, getModeHandlerFunction(lib.Emili, "היי, אעזור עם הטקסטים והודעות בעברית."+"\n\n"+fmt.Sprintf("אגב, אני בת %.f שעות, כלומר %.f ימים, %.f שבועות, %.1f חודשים או %.1f שנים", time.Since(EMILY_BIRTHDAY).Hours(), time.Since(EMILY_BIRTHDAY).Hours()/24, time.Since(EMILY_BIRTHDAY).Hours()/24/7, 12*(time.Since(EMILY_BIRTHDAY).Hours()/24/365), time.Since(EMILY_BIRTHDAY).Hours()/24/365))),
		newCommandHandler(VasilisaCommand, getModeHandlerFunction(lib.Vasilisa, "Привет, я помогу тебе с текстами и сообщениями на русском языке 😊\n\n"+fmt.Sprintf("Кстати, мне %.f часов, то есть %.f дней или %.1f лет", time.Since(VASILISA_BIRTHDAY).Hours(), time.Since(VASILISA_BIRTHDAY).Hours()/24, time.Since(VASILISA_BIRTHDAY).Hours()/24/365))),
		newCommandHandler(ChatGPTCommand, getModeHandlerFunction(lib.ChatGPT, "🚀 ChatGPT is now fully unleashed! Just tell me or ask me anything you want. I can now remember the context of our conversation. You can use /clear command anytime to wipe my memory and start a new thread.")),
		newCommandHandler(GrammarCommand, getModeHandlerFunction(lib.Grammar, "Will only correct your grammar without any explainations. If you want to get explainations, use /teacher command.")),
		newCommandHandler(TeacherCommand, getModeHandlerFunction(lib.Teacher, "Will correct your grammar and explain any mistakes found.")),
		newCommandHandler(TranscribeCommand, getModeHandlerFunction(lib.Transcribe, "Will transcribe your voice/audio/video messages only.")),
		newCommandHandler(SummarizeCommand, getModeHandlerFunction(lib.Summarize, "Will summarize your text/voice/audio/video messages.")),
		newCommandHandler(VoiceGPTCommand, getModeHandlerFunction(lib.VoiceGPT, "🚀 now I'm like ChatGPT with memory and all, but will respond with voice messages. What do you want to talk about? Use /clear command anytime to wipe my memory and start a new thread.\n\nNote, that this mode is more expensive than regular /chatgpt mode.")),
		newCommandHandler(StatusCommand, statusCommandHandler),
		newCommandHandler(UpgradeCommand, upgradeCommandHandler),
		newCommandHandler(CancelSubscriptionCommand, cancelSubscriptionCommandHandler),
		newCommandHandler(SupportCommand, supportCommandHandler),
		newCommandHandler(TermsCommand, func(ctx context.Context, bot *Bot, message *telego.Message) {
			log.Infof("Terms command received from userID: %s", util.GetChatIDString(message))
			bot.SendMessage(tu.Message(util.GetChatID(message), USAGE_TERMS_URL).WithMessageThreadID(message.MessageThreadID))
		}),
		// we have to use system command here since it's not possible to transfer fileId from one bot to another
		// be super careful with this command refactoring and make sure that it's not possible to send this command from any other chat
		newCommandHandler(SYSTEMSetOnboardingVideoCommand, func(ctx context.Context, bot *Bot, message *telego.Message) {
			if SystemBOT.ChatID != tu.ID(message.Chat.ID) {
				log.Errorf("System command received message from chat %d, but expected from %d", message.Chat.ID, SystemBOT.ChatID.ID)
				emptyCommandHandler(ctx, bot, message)
				return
			}

			// get video from message
			if message.Video == nil {
				bot.SendMessage(tu.Message(SystemBOT.ChatID, "Please provide video").WithMessageThreadID(message.MessageThreadID))
				return
			}
			err := redis.RedisClient.Set(ctx, "onboarding-video", message.Video.FileID, 0).Err()
			if err != nil {
				bot.SendMessage(tu.Message(SystemBOT.ChatID, fmt.Sprintf("Failed to save video: %v", err)).WithMessageThreadID(message.MessageThreadID))
				return
			}
			bot.SendMessage(tu.Message(SystemBOT.ChatID, "Onboarding video saved").WithMessageThreadID(message.MessageThreadID))
		}),
		newCommandHandler(ClearThreadCommand, clearThreadCommandHandler),
	}
}

func newCommandHandler(command Command, handler func(context.Context, *Bot, *telego.Message)) *CommandHandler {
	return &CommandHandler{
		Command: command,
		Handler: handler,
	}
}

func (c CommandHandlers) handleCommand(ctx context.Context, bot *Bot, message *telego.Message) {
	commandArray := strings.Split(message.Text, " ")
	commandString := strings.ReplaceAll(commandArray[0], "@"+bot.Name+"bot", "")
	commandString = strings.ReplaceAll(commandString, "@"+bot.Name, "")
	commandString = strings.ReplaceAll(commandString, "@"+config.CONFIG.BotName, "")
	command := Command(commandString)

	commandHandler := c.getCommandHandler(command)
	if commandHandler != nil {
		config.CONFIG.DataDogClient.Incr("command", []string{"command:" + string(command), "bot_name:" + bot.Name}, 1)
		commandHandler.Handler(ctx, bot, message)
	} else {
		config.CONFIG.DataDogClient.Incr("unknown_command", nil, 1)
		bot.SendMessage(tu.Message(util.GetChatID(message), "Unknown command \U0001f937").WithMessageThreadID(message.MessageThreadID))
	}
}

func (c CommandHandlers) getCommandHandler(command Command) *CommandHandler {
	for _, ch := range c {
		if ch.Command == command {
			return ch
		}
	}
	return nil
}

func getModeHandlerFunction(mode lib.ModeName, response string) func(context.Context, *Bot, *telego.Message) {
	return func(ctx context.Context, bot *Bot, message *telego.Message) {
		messageArray := strings.Split(message.Text, " ")
		params := ""
		if len(messageArray) > 1 {
			params = validateParams(mode, messageArray[1])
		}
		response = lib.AddBotSuffixToGroupCommands(ctx, response)
		bot.SendMessage(tu.Message(util.GetChatID(message), response).WithMessageThreadID(message.MessageThreadID))
		lib.SaveMode(util.GetChatIDString(message), util.GetTopicID(message), mode, params)
	}
}

func upgradeCommandHandler(ctx context.Context, bot *Bot, message *telego.Message) {
	chatString := util.GetChatIDString(message)
	chatID := util.GetChatID(message)
	if lib.IsUserFree(ctx) {
		_, err := bot.SendMessage(tu.Message(chatID, "Upgrading to free+ gives you 5x monthly usage limits, effective immediately 🎉").WithMessageThreadID(message.MessageThreadID))
		if err != nil {
			log.Errorf("Failed to send upgrade message to %s: %v", chatString, err)
		}
		err = mongo.MongoDBClient.UpdateUserSubscription(ctx, models.Subscriptions[models.FreePlusSubscriptionName])
		if err != nil {
			log.Errorf("Failed to update user %s subscription: %v", chatString, err)
			bot.SendMessage(tu.Message(chatID, "Failed to upgrade your account to free+ plan. Please try again later.").WithMessageThreadID(message.MessageThreadID))
			return
		}
		bot.SendMessage(tu.Message(chatID, "You are now a free+ user 🥳! Thanks for trying the bot and the wish to support it's development! 🙏").WithMessageThreadID(message.MessageThreadID))
		return
	}
	if lib.IsUserFreePlus(ctx) {
		_, err := bot.SendMessage(tu.Message(chatID, "Upgrading account to basic paid plan..").WithMessageThreadID(message.MessageThreadID))
		if err != nil {
			log.Errorf("Failed to send paid plan upgrade message to %s: %v", chatString, err)
		}
		// fetch userStripeID from DB
		user, err := mongo.MongoDBClient.GetUser(ctx)
		if err != nil {
			log.Errorf("Failed to get user %s: %v", chatString, err)
			return
		}
		stripeCustomerId := user.StripeCustomerId
		if stripeCustomerId == "" {
			customer, err := payments.StripeCreateCustomer(ctx, bot.Bot, message)
			if err != nil {
				log.Errorf("Failed to create customer for user %s: %v", chatString, err)
				return
			}
			stripeCustomerId = customer.ID
			err = mongo.MongoDBClient.UpdateUserStripeCustomerId(ctx, stripeCustomerId)
			if err != nil {
				log.Errorf("Failed to update user stripe customer id for user %s: %v", chatString, err)
				return
			}
		}

		// create new checkout session
		session, err := payments.StripeCreateCheckoutSession(ctx, bot.Bot, message, stripeCustomerId, payments.BasicPlanPriceId)
		if err != nil {
			log.Errorf("Failed to create checkout session: %v", err)
			return
		}

		notification := "Press the subscribe button below and navigate to our partner, Stripe, to proceed with the payment. You will upgrade to the basic paid plan, which includes:\n💪 200x usage limits compared to the Free+ plan of OpenAI tokens and voice/audio recognition\n🧠 Access to more intelligent GPT-4 model (and more models that OpenAI will release)\n💁🏽 Priority support\n\nBy proceeding with the payments, you agree to /terms of usage.\n\nCancel your subscription at any time with the /downgrade command."
		notification = lib.AddBotSuffixToGroupCommands(ctx, notification)

		// send link to customer as a button in telegram
		bot.SendMessage(
			tu.Message(chatID, notification).WithMessageThreadID(message.MessageThreadID).WithReplyMarkup(
				&telego.InlineKeyboardMarkup{
					InlineKeyboard: [][]telego.InlineKeyboardButton{
						{
							telego.InlineKeyboardButton{
								Text: "Subscribe for $9.99/month",
								URL:  session.URL,
							},
						},
					},
				}))

		config.CONFIG.DataDogClient.Incr("upgrade_command_session", []string{"bot_name:" + bot.Name}, 1)

		return
	}
	if lib.IsUserBasic(ctx) {
		bot.SendMessage(tu.Message(chatID, "You are already a basic paid plan user! Premium upgrade plans are not available yet. Stay tuned for updates!").WithMessageThreadID(message.MessageThreadID))
		return
	}

	log.Errorf("upgradeCommandHandler: unknown user %s subscription: %v", chatString, ctx.Value(models.SubscriptionContext{}))
}

func cancelSubscriptionCommandHandler(ctx context.Context, bot *Bot, message *telego.Message) {
	chatString := util.GetChatIDString(message)
	chatID := util.GetChatID(message)
	if lib.IsUserFree(ctx) {
		bot.SendMessage(tu.Message(chatID, "You are already a free user!").WithMessageThreadID(message.MessageThreadID))
		return
	}

	confirmationMessage := ""
	callbackData := ""
	if lib.IsUserFreePlus(ctx) {
		// send confirmation message with yes/no buttons
		confirmationMessage = "Are you sure you want to cancel your free+ plan?\n\nYou will be downgraded to the free plan immediately."
		callbackData = "downgradefromfreeplus"
	}

	if lib.IsUserBasic(ctx) {
		confirmationMessage = "Are you sure you want to cancel your subscription to the basic plan?\n\nYou will be downgraded to the free+ plan immediately, loosing access to GPT-4 model and increased usage limits. Unused limits will not be refunded."
		callbackData = "downgradefrombasic"
	}

	if callbackData == "" {
		log.Errorf("cancelSubscriptionCommandHandler: unknown user %s subscription: %v", chatString, ctx.Value(models.SubscriptionContext{}))
		return
	}

	log.Infof("Sending downgrade confirmation message to user %s: %s", chatString, callbackData)
	bot.SendMessage(tu.Message(chatID, confirmationMessage).WithMessageThreadID(message.MessageThreadID).WithReplyMarkup(
		&telego.InlineKeyboardMarkup{
			InlineKeyboard: [][]telego.InlineKeyboardButton{
				{
					telego.InlineKeyboardButton{
						// whitecheckmark
						Text:         "Yes",
						CallbackData: callbackData,
					},
					telego.InlineKeyboardButton{
						Text:         "No",
						CallbackData: "cancel",
					},
				},
			},
		}))
}

func emptyCommandHandler(ctx context.Context, bot *Bot, message *telego.Message) {
	_, err := bot.SendMessage(tu.Message(util.GetChatID(message), "There is no message provided to correct or comment on. If you have a message you would like me to review, please provide it.").WithMessageThreadID(message.MessageThreadID))
	if err != nil {
		log.Errorf("Failed to send EmptyCommand message: %v", err)
	}
}

func supportCommandHandler(ctx context.Context, bot *Bot, message *telego.Message) {
	log.Infof("Support command received from userID: %s", util.GetChatIDString(message))
	supportMessage := tu.MessageWithEntities(
		util.GetChatID(message),
		tu.Entity("If you have any questions, please contact us at "),
		tu.Entity("free+support@radiant.space").Email(),
		tu.Entityf(", explaining the problem and mentioning userID: %s.", util.GetChatIDString(message)),
	).WithMessageThreadID(message.MessageThreadID)
	_, err := bot.SendMessage(supportMessage)
	if err != nil {
		log.Errorf("Failed to send SupportCommand message: %v", err)
	}
}

func statusCommandHandler(ctx context.Context, bot *Bot, message *telego.Message) {
	_, err := bot.SendMessage(tu.Message(util.GetChatID(message), GetUserStatus(ctx)).WithMessageThreadID(message.MessageThreadID).WithReplyMarkup(GetStatusKeyboard(ctx)))
	if err != nil {
		log.Errorf("Failed to send StatusCommand message: %v", err)
	}
}

func clearThreadCommandHandler(ctx context.Context, bot *Bot, message *telego.Message) {
	chatID := util.GetChatID(message)
	chatIDString := util.GetChatIDString(message)
	topicIDString := util.GetTopicID(message)
	threadId, _ := redis.RedisClient.Get(ctx, lib.UserCurrentThreadKey(chatIDString, topicIDString)).Result()
	if threadId == "" {
		_, err := bot.SendMessage(tu.Message(chatID, "There is no thread to clear.").WithMessageThreadID(message.MessageThreadID))
		if err != nil {
			log.Errorf("Failed to send ClearThreadCommand message: %v", err)
		}
		return
	}

	redis.RedisClient.Del(ctx, lib.UserCurrentThreadKey(chatIDString, topicIDString))
	redis.RedisClient.Del(ctx, lib.UserCurrentThreadPromptKey(chatIDString, topicIDString))
	_, err := BOT.API.DeleteThread(ctx, threadId)
	if err != nil {
		log.Errorf("Failed to clear thread: %v", err)
	}
	_, err = bot.SendMessage(tu.Message(chatID, "Thread cleared.").WithMessageThreadID(message.MessageThreadID))
	if err != nil {
		log.Errorf("Failed to send ClearThreadCommand message: %v", err)
	}
}

func validateParams(mode lib.ModeName, params string) string {
	if params == "" {
		return ""
	}

	if mode == lib.VoiceGPT || mode == lib.Transcribe || mode == lib.Grammar || mode == lib.ChatGPT || mode == lib.Summarize {
		// params expected to be language code for now
		params = strings.ToLower(params)

		// validate language code
		switch params {
		case "af", "am", "ar", "as", "az", "ba", "be", "bg", "bn", "bo", "br", "bs", "ca", "cs", "cy", "da", "de", "el", "en", "es", "et", "eu", "fa", "fi", "fo", "fr", "gl", "gu", "ha", "he", "hi", "hr", "ht", "hu", "hy", "id", "is", "it", "ja", "jw", "ka", "kk", "km", "kn", "ko", "la", "lb", "ln", "lo", "lt", "lv", "mg", "mi", "mk", "ml", "mn", "mr", "ms", "mt", "my", "ne", "nl", "nn", "no", "oc", "pa", "pl", "ps", "pt", "ro", "ru", "sa", "sd", "si", "sk", "sl", "sn", "so", "sq", "sr", "su", "sv", "sw", "ta", "te", "tg", "th", "tk", "tl", "tr", "tt", "uk", "ur", "uz", "vi", "yi", "yo", "zh":
			return params
		case "afrikaans", "amharic", "arabic", "assamese", "azerbaijani", "bashkir", "belarusian", "bengali", "tibetan", "breton", "bosnian", "catalan", "czech", "welsh", "danish", "german", "greek", "english", "spanish", "estonian", "basque", "persian", "finnish", "faroese", "french", "galician", "gujarati", "hausa", "hebrew", "hindi", "croatian", "haitian", "hungarian", "armenian", "indonesian", "icelandic", "italian", "japanese", "javanese", "georgian", "kazakh", "khmer", "kannada", "korean", "latin", "luxembourgish", "lingala", "lao", "lithuanian", "latvian", "malagasy", "maori", "macedonian", "malayalam", "mongolian", "marathi", "malay", "maltese", "burmese", "nepali", "dutch", "norwegian", "occitan", "punjabi", "polish", "pashto", "portuguese", "romanian", "russian", "sanskrit", "sindhi", "sinhala", "slovak", "slovenian", "shona", "somali", "albanian", "serbian", "sundanese", "swedish", "swahili", "tamil", "telugu", "tajik", "thai", "turkmen", "filipino", "turkish", "tatar", "ukrainian", "urdu", "uzbek", "vietnamese", "yiddish", "yoruba", "chinese":
			return languageToCode(params)
		default:
			log.Warnf("Invalid params %s used for mode %s", params, mode)
			return ""
		}
	}

	return ""
}

func languageToCode(language string) string {
	switch language {
	case "afrikaans":
		return "af"
	case "amharic":
		return "am"
	case "arabic":
		return "ar"
	case "assamese":
		return "as"
	case "azerbaijani":
		return "az"
	case "bashkir":
		return "ba"
	case "belarusian":
		return "be"
	case "bengali":
		return "bn"
	case "tibetan":
		return "bo"
	case "breton":
		return "br"
	case "bosnian":
		return "bs"
	case "catalan":
		return "ca"
	case "czech":
		return "cs"
	case "welsh":
		return "cy"
	case "danish":
		return "da"
	case "german":
		return "de"
	case "greek":
		return "el"
	case "english":
		return "en"
	case "spanish":
		return "es"
	case "estonian":
		return "et"
	case "basque":
		return "eu"
	case "persian":
		return "fa"
	case "finnish":
		return "fi"
	case "faroese":
		return "fo"
	case "french":
		return "fr"
	case "galician":
		return "gl"
	case "gujarati":
		return "gu"
	case "hausa":
		return "ha"
	case "hebrew":
		return "he"
	case "hindi":
		return "hi"
	case "croatian":
		return "hr"
	case "haitian":
		return "ht"
	case "hungarian":
		return "hu"
	case "armenian":
		return "hy"
	case "indonesian":
		return "id"
	case "icelandic":
		return "is"
	case "italian":
		return "it"
	case "japanese":
		return "ja"
	case "javanese":
		return "jw"
	case "georgian":
		return "ka"
	case "kazakh":
		return "kk"
	case "khmer":
		return "km"
	case "kannada":
		return "kn"
	case "korean":
		return "ko"
	case "latin":
		return "la"
	case "luxembourgish":
		return "lb"
	case "lingala":
		return "ln"
	case "lao":
		return "lo"
	case "lithuanian":
		return "lt"
	case "latvian":
		return "lv"
	case "malagasy":
		return "mg"
	case "maori":
		return "mi"
	case "macedonian":
		return "mk"
	case "malayalam":
		return "ml"
	case "mongolian":
		return "mn"
	case "marathi":
		return "mr"
	case "malay":
		return "ms"
	case "maltese":
		return "mt"
	case "burmese":
		return "my"
	case "nepali":
		return "ne"
	case "dutch":
		return "nl"
	case "norwegian":
		return "nn"
	case "occitan":
		return "oc"
	case "punjabi":
		return "pa"
	case "polish":
		return "pl"
	case "pashto":
		return "ps"
	case "portuguese":
		return "pt"
	case "romanian":
		return "ro"
	case "russian":
		return "ru"
	case "sanskrit":
		return "sa"
	case "sindhi":
		return "sd"
	case "sinhala":
		return "si"
	case "slovak":
		return "sk"
	case "slovenian":
		return "sl"
	case "shona":
		return "sn"
	case "somali":
		return "so"
	case "albanian":
		return "sq"
	case "serbian":
		return "sr"
	case "sundanese":
		return "su"
	case "swedish":
		return "sv"
	case "swahili":
		return "sw"
	case "tamil":
		return "ta"
	case "telugu":
		return "te"
	case "tajik":
		return "tg"
	case "thai":
		return "th"
	case "turkmen":
		return "tk"
	case "filipino":
		return "tl"
	case "turkish":
		return "tr"
	case "tatar":
		return "tt"
	case "ukrainian":
		return "uk"
	case "urdu":
		return "ur"
	case "uzbek":
		return "uz"
	case "vietnamese":
		return "vi"
	case "yiddish":
		return "yi"
	case "yoruba":
		return "yo"
	case "chinese":
		return "zh"
	default:
		log.Warnf("Invalid language %s", language)
		return ""
	}
}
