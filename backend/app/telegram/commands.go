package telegram

import (
	"context"
	"fmt"
	"strings"
	"talk2robots/m/v2/app/ai/openai"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/db/mongo"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/models"
	"talk2robots/m/v2/app/payments"
	"talk2robots/m/v2/app/util"
	"time"
	"unicode"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	log "github.com/sirupsen/logrus"
)

type Command string

var EMILY_BIRTHDAY = time.Date(2023, 5, 25, 0, 18, 0, 0, time.FixedZone("UTC+2", 3*60*60))
var VASILISA_BIRTHDAY = time.Date(2007, 12, 13, 23, 45, 0, 0, time.FixedZone("UTC+3", 3*60*60))
var ONBOARDING_TEXT = `I'm Gienji, a smart assistant which is available 24/7 to amplify your 🧠 intelligence and 💬 communication skills! I'm here to unlock your full potential!

Here are some of the things I can do:
- 🧠 /chatgpt - chat or answer any questions, respond with text messages
- 🎙️ /voicegpt - full conversation experience, respond using voice messages
- remember context in /chatgpt and /voicegpt modes (use /clear to clear current thread)
- 🖼️ draw, just ask to picture anything (Example: 'create an image of a fish riding a bicycle')
- /translate [language code or name] - translate messages to English or other language (Example: /translate es)
- /grammar - correct grammar mode, will only correct last sent message
- /teacher - correct and explain grammar and mistakes
- /transcribe voice/audio/video messages only
- /summarize text/voice/audio/video messages
- /status - check usage limits, consumed tokens and audio transcription minutes. Usage limits for the assistant are reset every 1st of the month.

Enjoy and let me know if any /support is needed!`

const (
	StartCommand              Command = "/start"
	ChatGPTCommand            Command = "/chatgpt"
	VoiceGPTCommand           Command = "/voicegpt"
	GrammarCommand            Command = "/grammar"
	ClearThreadCommand        Command = "/clear"
	TeacherCommand            Command = "/teacher"
	TranscribeCommand         Command = "/transcribe"
	SummarizeCommand          Command = "/summarize"
	TranslateCommand          Command = "/translate"
	StatusCommand             Command = "/status"
	SupportCommand            Command = "/support"
	TermsCommand              Command = "/terms"
	UpgradeCommand            Command = "/upgrade"
	CancelSubscriptionCommand Command = "/downgrade"
	BillingCommand            Command = "/billing"
	VasilisaCommand           Command = "/vasilisa"
	EmiliCommand              Command = "/emily"
	EmptyCommand              Command = ""

	// commands setting for BotFather
	Commands string = `
start - 🚀 onboarding instructions
chatgpt - 🧠 ask AI anything (with memory)
voicegpt - 🎙 talk to AI using voice messages (with memory)
clear - 🧹 clear current conversation memory
grammar - 👀 grammar checking mode only, no explanations
teacher - 🧑‍🏫 grammar correction and explanations
transcribe - 🎙 transcribe voice/audio/video
translate - 🌍 translate text to English or the specified language (Example: /translate es)
summarize - 📝 summarize text/voice/audio/video
status - 📊 status and settings
billing - 💳 manage or cancel your subscription
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
		newCommandHandler(StartCommand, startCommandHandler),
		newCommandHandler(EmiliCommand, getModeHandlerFunction(lib.Emili, "היי, אעזור עם הטקסטים והודעות בעברית."+"\n\n"+fmt.Sprintf("אגב, אני בת %.f שעות, כלומר %.f ימים, %.f שבועות, %.1f חודשים או %.1f שנים", time.Since(EMILY_BIRTHDAY).Hours(), time.Since(EMILY_BIRTHDAY).Hours()/24, time.Since(EMILY_BIRTHDAY).Hours()/24/7, 12*(time.Since(EMILY_BIRTHDAY).Hours()/24/365), time.Since(EMILY_BIRTHDAY).Hours()/24/365))),
		newCommandHandler(VasilisaCommand, getModeHandlerFunction(lib.Vasilisa, "Привет, я помогу тебе с текстами и сообщениями на русском языке 😊\n\n"+fmt.Sprintf("Кстати, мне %.f часов, то есть %.f дней или %.1f лет", time.Since(VASILISA_BIRTHDAY).Hours(), time.Since(VASILISA_BIRTHDAY).Hours()/24, time.Since(VASILISA_BIRTHDAY).Hours()/24/365))),
		newCommandHandler(ChatGPTCommand, getModeHandlerFunction(lib.ChatGPT, "🚀 ChatGPT is now fully unleashed! Just tell me or ask me anything you want. I can now remember the context of our conversation. You can use /clear command anytime to wipe my memory and start a new thread.")),
		newCommandHandler(GrammarCommand, getModeHandlerFunction(lib.Grammar, "Will only correct your grammar without any explainations. If you want to get explainations, use /teacher command.")),
		newCommandHandler(TeacherCommand, getModeHandlerFunction(lib.Teacher, "Will correct your grammar and explain any mistakes found.")),
		newCommandHandler(TranscribeCommand, getModeHandlerFunction(lib.Transcribe, "Will transcribe your voice/audio/video messages only.")),
		newCommandHandler(SummarizeCommand, getModeHandlerFunction(lib.Summarize, "Will summarize your text/voice/audio/video messages.")),
		newCommandHandler(VoiceGPTCommand, getModeHandlerFunction(lib.VoiceGPT, "🚀 now I'm like ChatGPT with memory and all, but will respond with voice messages. What do you want to talk about? Use /clear command anytime to wipe my memory and start a new thread.\n\nNote, that this mode is more expensive than regular /chatgpt mode.")),
		newCommandHandler(TranslateCommand, getModeHandlerFunction(lib.Translate, "Will translate your messages to English.")),
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
		newCommandHandler(BillingCommand, func(ctx context.Context, bot *Bot, message *telego.Message) {
			// call stripe to get customer info link
			chatID := util.GetChatID(message)
			chatString := util.GetChatIDString(message)
			user, err := mongo.MongoDBClient.GetUser(ctx)
			if err != nil {
				log.Errorf("Failed to get user %s: %v", chatString, err)
				return
			}

			if user.StripeCustomerId == "" {
				bot.SendMessage(tu.Message(chatID, "You don't have billing setup.").WithMessageThreadID(message.MessageThreadID))
				return
			}

			notification := `Tap 'Continue' button to manage or cancel your subscription, use the email you used for registering. If you don't remember which email you used, check your inboxes for Stripe messages or reach out to /support 🚀`
			notification = lib.AddBotSuffixToGroupCommands(ctx, notification)
			stripePortalLink := "https://billing.stripe.com/p/login/bIYbMG468cuR9a06oo"

			// send link to customer as a button in telegram
			bot.SendMessage(
				tu.Message(chatID, notification).WithMessageThreadID(message.MessageThreadID).WithReplyMarkup(
					&telego.InlineKeyboardMarkup{
						InlineKeyboard: [][]telego.InlineKeyboardButton{
							{
								telego.InlineKeyboardButton{
									Text: "Continue",
									URL:  stripePortalLink,
								},
							},
						},
					}))
		}),
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
		if mode == lib.Translate {
			response = strings.ReplaceAll(response, "English", getLanguageName(params))
		}
		response = lib.AddBotSuffixToGroupCommands(ctx, response)
		bot.SendMessage(tu.Message(util.GetChatID(message), response).WithMessageThreadID(message.MessageThreadID))
		lib.SaveMode(util.GetChatIDString(message), util.GetTopicID(message), mode, params)
	}
}

func upgradeCommandHandler(ctx context.Context, bot *Bot, message *telego.Message) {
	chatString := util.GetChatIDString(message)
	chatID := util.GetChatID(message)
	if lib.IsUserFree(ctx) || lib.IsUserFreePlus(ctx) {
		// _, err := bot.SendMessage(tu.Message(chatID, "Upgrading account to basic paid plan..").WithMessageThreadID(message.MessageThreadID))
		// if err != nil {
		// 	log.Errorf("Failed to send paid plan upgrade message to %s: %v", chatString, err)
		// }
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

		notification := "Tap 'Continue' button to keep using me.\n\n" + ONBOARDING_TEXT
		notification = lib.AddBotSuffixToGroupCommands(ctx, notification)

		// send link to customer as a button in telegram
		bot.SendMessage(
			tu.Message(chatID, notification).WithMessageThreadID(message.MessageThreadID).WithReplyMarkup(
				&telego.InlineKeyboardMarkup{
					InlineKeyboard: [][]telego.InlineKeyboardButton{
						{
							telego.InlineKeyboardButton{
								Text: "Continue",
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
	topicString := util.GetTopicID(message)
	if lib.IsUserFree(ctx) {
		bot.SendMessage(tu.Message(chatID, "You are already a free user!").WithMessageThreadID(message.MessageThreadID))
		return
	}

	confirmationMessage := ""
	callbackData := ""
	if lib.IsUserFreePlus(ctx) {
		// send confirmation message with yes/no buttons
		confirmationMessage = "Are you sure you want to cancel your free+ plan?\n\nYou will be downgraded to the free plan immediately."
		callbackData = "downgradefromfreeplus:" + topicString
	}

	if lib.IsUserBasic(ctx) {
		confirmationMessage = "Are you sure you want to cancel your subscription to the basic plan?\n\nYou will be downgraded to the free+ plan immediately, loosing access to GPT-4 model and increased usage limits. Unused limits will NOT be refunded."
		callbackData = "downgradefrombasic:" + topicString
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
						CallbackData: "cancel:" + topicString,
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

	_, err := bot.SendMessage(tu.Message(chatID, "All memory cleared!").WithMessageThreadID(message.MessageThreadID))
	if err != nil {
		log.Errorf("Failed to send ClearThreadCommand message: %v", err)
	}

	// clear local thread
	go func() {
		err := mongo.MongoDBClient.DeleteUserThread(context.WithValue(context.Background(), models.UserContext{}, chatIDString))
		if err != nil {
			log.Errorf("Failed to clear local thread in chat %s: %v", chatIDString, err)
			return
		}
	}()

	// clear OpenAI thread
	go func() {
		threadId, _ := redis.RedisClient.Get(ctx, lib.UserCurrentThreadKey(chatIDString, topicIDString)).Result()
		if threadId == "" {
			return
		}

		redis.RedisClient.Del(ctx, lib.UserCurrentThreadKey(chatIDString, topicIDString))
		redis.RedisClient.Del(ctx, lib.UserCurrentThreadPromptKey(chatIDString, topicIDString))
		_, err = openai.DeleteThread(ctx, threadId)
		if err != nil {
			log.Errorf("Failed to clear OpenAI thread: %v", err)
		}
	}()
}

func validateParams(mode lib.ModeName, params string) string {
	if params == "" {
		return ""
	}

	if mode == lib.VoiceGPT || mode == lib.Transcribe || mode == lib.Grammar || mode == lib.ChatGPT || mode == lib.Summarize || mode == lib.Translate {
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

func getLanguageName(languageCode string) string {
	switch languageCode {
	case "af":
		return "Afrikaans"
	case "am":
		return "Amharic"
	case "ar":
		return "Arabic"
	case "as":
		return "Assamese"
	case "az":
		return "Azerbaijani"
	case "ba":
		return "Bashkir"
	case "be":
		return "Belarusian"
	case "bn":
		return "Bengali"
	case "bo":
		return "Tibetan"
	case "br":
		return "Breton"
	case "bs":
		return "Bosnian"
	case "ca":
		return "Catalan"
	case "cs":
		return "Czech"
	case "cy":
		return "Welsh"
	case "da":
		return "Danish"
	case "de":
		return "German"
	case "el":
		return "Greek"
	case "en":
		return "English"
	case "es":
		return "Spanish"
	case "et":
		return "Estonian"
	case "eu":
		return "Basque"
	case "fa":
		return "Persian"
	case "fi":
		return "Finnish"
	case "fo":
		return "Faroese"
	case "fr":
		return "French"
	case "gl":
		return "Galician"
	case "gu":
		return "Gujarati"
	case "ha":
		return "Hausa"
	case "he":
		return "Hebrew"
	case "hi":
		return "Hindi"
	case "hr":
		return "Croatian"
	case "ht":
		return "Haitian"
	case "hu":
		return "Hungarian"
	case "hy":
		return "Armenian"
	case "id":
		return "Indonesian"
	case "is":
		return "Icelandic"
	case "it":
		return "Italian"
	case "ja":
		return "Japanese"
	case "jw":
		return "Javanese"
	case "ka":
		return "Georgian"
	case "kk":
		return "Kazakh"
	case "km":
		return "Khmer"
	case "kn":
		return "Kannada"
	case "ko":
		return "Korean"
	case "la":
		return "Latin"
	case "lb":
		return "Luxembourgish"
	case "ln":
		return "Lingala"
	case "lo":
		return "Lao"
	case "lt":
		return "Lithuanian"
	case "lv":
		return "Latvian"
	case "mg":
		return "Malagasy"
	case "mi":
		return "Maori"
	case "mk":
		return "Macedonian"
	case "ml":
		return "Malayalam"
	case "mn":
		return "Mongolian"
	case "mr":
		return "Marathi"
	case "ms":
		return "Malay"
	case "mt":
		return "Maltese"
	case "my":
		return "Burmese"
	case "ne":
		return "Nepali"
	case "nl":
		return "Dutch"
	case "nn":
		return "Norwegian"
	case "oc":
		return "Occitan"
	case "pa":
		return "Punjabi"
	case "pl":
		return "Polish"
	case "ps":
		return "Pashto"
	case "pt":
		return "Portuguese"
	case "ro":
		return "Romanian"
	case "ru":
		return "Russian"
	case "sa":
		return "Sanskrit"
	case "sd":
		return "Sindhi"
	case "si":
		return "Sinhala"
	case "sk":
		return "Slovak"
	case "sl":
		return "Slovenian"
	case "sn":
		return "Shona"
	case "so":
		return "Somali"
	case "sq":
		return "Albanian"
	case "sr":
		return "Serbian"
	case "su":
		return "Sundanese"
	case "sv":
		return "Swedish"
	case "sw":
		return "Swahili"
	case "ta":
		return "Tamil"
	case "te":
		return "Telugu"
	case "tg":
		return "Tajik"
	case "th":
		return "Thai"
	case "tk":
		return "Turkmen"
	case "tl":
		return "Filipino"
	case "tr":
		return "Turkish"
	case "tt":
		return "Tatar"
	case "uk":
		return "Ukrainian"
	case "ur":
		return "Urdu"
	case "uz":
		return "Uzbek"
	case "vi":
		return "Vietnamese"
	case "yi":
		return "Yiddish"
	case "yo":
		return "Yoruba"
	case "zh":
		return "Chinese"
	default:
		return "English"
	}
}

func IsCreateImageCommand(prompt string) bool {
	// should return true if the prompt is a create image command, i.e. first couple of words are "create/draw/picture/imagine/image"
	// and false otherwise

	// Normalize the prompt by converting to lowercase and removing punctuation
	cleanPrompt := strings.Map(func(r rune) rune {
		if unicode.IsPunct(r) {
			return -1
		}
		return r
	}, strings.ToLower(prompt))

	log.Debugf("Cleaned prompt: %s", cleanPrompt)

	triggerWords := []string{"draw", "drawing", "picture", "imagine", "image", "paint", "painting", "sketch", "sketching", "illustration", "illustrate", "art", "design"}
	stopWords := []string{"write", "article", "dont", "work", "working", "job", "jobs", "task", "tasks", "assignment", "assignments", "homework", "homeworks", "essay", "essays", "report", "reports", "paper", "papers", "document", "documents", "text", "message", "letter", "email", "conversation", "speak", "speech"}

	// Split the prompt into words
	words := strings.Fields(cleanPrompt)
	foundTrigger := false
	foundStop := false
	for i, word := range words {
		if i > 5 {
			return foundTrigger && !foundStop
		}

		if contains(triggerWords, word) {
			foundTrigger = true
		}

		if contains(stopWords, word) {
			foundStop = true
		}
	}

	return foundTrigger && !foundStop
}

func contains(arr []string, word string) bool {
	for _, a := range arr {
		if a == word {
			return true
		}
	}
	return false
}

func startCommandHandler(ctx context.Context, bot *Bot, message *telego.Message) {
	params := ""
	startCommandParams := strings.Split(message.Text, " ")
	if len(startCommandParams) > 1 {
		params = startCommandParams[1]
		if len(startCommandParams) > 64 {
			params = ""
			// ignore params longer than 64 characters
			// https://core.telegram.org/api/links#bot-links
		} else {
			// base64 decode params
			decoded := util.Base64Decode(params)
			if decoded != "" {
				params = decoded
			}
		}
	}
	ddParams := params
	if ddParams == "" {
		ddParams = "empty"
	}
	log.Infof("Start command params: %s", params)
	// parse format: s=web&m=transcribe&l=es
	// s - source, m - mode, l - language
	source := ""
	mode := ""
	language := ""
	if params != "" {
		paramsArray := strings.Split(params, "&")
		for _, param := range paramsArray {
			paramArray := strings.Split(param, "=")
			if len(paramArray) == 2 {
				switch strings.ToLower(paramArray[0]) {
				case "s":
					// source
					source = paramArray[1]
				case "m":
					// mode
					mode = paramArray[1]
				case "l":
					// language
					language = paramArray[1]
				default:
					log.Warnf("Unknown start command param: %s", param)
				}
			}
		}

		config.CONFIG.DataDogClient.Incr("start_command", []string{"source:" + source, "mode:" + mode, "language:" + language}, 1)
		go mongo.MongoDBClient.UpdateUserSourceModeLanguage(ctx, source, mode, language)
	} else {
		config.CONFIG.DataDogClient.Incr("start_command", []string{"source:empty", "mode:empty", "language:empty"}, 1)
	}

	if mode == "transcribe" {
		// switch to transcribe mode
		message := telego.Message{
			Chat:            message.Chat,
			Text:            "/transcribe " + language,
			MessageThreadID: message.MessageThreadID,
		}
		AllCommandHandlers.handleCommand(ctx, BOT, &message)
	} else {
		sendGeneralOnboardingVideo(ctx, bot, message)
	}

	notification := lib.AddBotSuffixToGroupCommands(ctx, ONBOARDING_TEXT)
	chatId := util.GetChatID(message)
	_, err := bot.SendMessage(tu.Message(chatId, notification).WithMessageThreadID(message.MessageThreadID).WithReplyMarkup(GetStatusKeyboard(ctx)))
	if err != nil {
		log.Errorf("Failed to send StartCommand message: %v", err)
	}
}

func sendGeneralOnboardingVideo(ctx context.Context, bot *Bot, message *telego.Message) {
	// try getting onboarding video from redis and sending it to the user
	videoFileId := redis.RedisClient.Get(ctx, "onboarding-video").Val()
	if videoFileId == "" || videoFileId == "get onboarding-video: redis: nil" {
		log.Errorf("Failed to get onboarding video from redis: %v", videoFileId)
		return
	}
	log.Infof("Sending onboarding video %s to userID: %s", videoFileId, util.GetChatIDString(message))
	_, err := bot.SendVideo(&telego.SendVideoParams{
		ChatID: util.GetChatID(message),
		Video:  telego.InputFile{FileID: videoFileId},
	})
	if err != nil {
		log.Errorf("Failed to send onboarding video: %v", err)
	}
}
