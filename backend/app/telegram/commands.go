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

var EMILY_BIRTHDAY = time.Date(2023, 5, 25, 0, 18, 0, 0, time.FixedZone("UTC+3", 3*60*60))
var VASILISA_BIRTHDAY = time.Date(2007, 12, 13, 23, 45, 0, 0, time.FixedZone("UTC+3", 3*60*60))
var ONBOARDING_TEXT = `Hi, I'm a bot powered by OpenAI! I can:
- Default: chat with or answer any questions (/chatgpt)
- Correct grammar (/grammar)
- Explain grammar and mistakes (/teacher)
- New feature ✨: transcribe voice/audio/video messages (/transcribe)
- New feature ✨: summarize text/voice/audio/video messages (/summarize)
- New feature ✨: explain pictures/photos in (/chatgpt) mode. That works for Basic subscription only, since it's expensive to run. Please /upgrade to use this feature.

Also, I will never store your messages, or any other private information.`

const (
	CancelSubscriptionCommand Command = "/downgrade"
	EmiliCommand              Command = "/emily"
	EmptyCommand              Command = ""
	ChatGPTCommand            Command = "/chatgpt"
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
chatgpt - 🧠 ask AI anything
grammar - 👀 grammar checking mode only, no explanations
teacher - 🧑‍🏫 grammar correction and explanations
transcribe - 🎙 transcribe voice/audio/video
summarize - 📝 summarize text/voice/audio/video
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
			_, err := bot.SendMessage(tu.Message(util.GetChatID(message), ONBOARDING_TEXT).WithReplyMarkup(GetStatusKeyboard(ctx)))
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
		newCommandHandler(ChatGPTCommand, getModeHandlerFunction(lib.ChatGPT, "🚀 ChatGPT is now fully unleashed! Just tell me or ask me anything you want. Previous messages will not be taken into account.")),
		newCommandHandler(GrammarCommand, getModeHandlerFunction(lib.Grammar, "Will only correct your grammar without any explainations. If you want to get explainations, use /teacher command.")),
		newCommandHandler(TeacherCommand, getModeHandlerFunction(lib.Teacher, "Will correct your grammar and explain any mistakes found.")),
		newCommandHandler(TranscribeCommand, getModeHandlerFunction(lib.Transcribe, "Will transcribe your voice/audio/video messages only.")),
		newCommandHandler(SummarizeCommand, getModeHandlerFunction(lib.Summarize, "Will summarize your text/voice/audio/video messages.")),
		newCommandHandler(StatusCommand, statusCommandHandler),
		newCommandHandler(UpgradeCommand, upgradeCommandHandler),
		newCommandHandler(CancelSubscriptionCommand, cancelSubscriptionCommandHandler),
		newCommandHandler(SupportCommand, supportCommandHandler),
		newCommandHandler(TermsCommand, func(ctx context.Context, bot *Bot, message *telego.Message) {
			log.Infof("Terms command received from userID: %s", util.GetChatIDString(message))
			bot.SendMessage(tu.Message(util.GetChatID(message), USAGE_TERMS_URL))
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
				bot.SendMessage(tu.Message(SystemBOT.ChatID, "Please provide video"))
				return
			}
			err := redis.RedisClient.Set(ctx, "onboarding-video", message.Video.FileID, 0).Err()
			if err != nil {
				bot.SendMessage(tu.Message(SystemBOT.ChatID, fmt.Sprintf("Failed to save video: %v", err)))
				return
			}
			bot.SendMessage(tu.Message(SystemBOT.ChatID, "Onboarding video saved"))
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
	command := Command(commandArray[0])
	commandHandler := c.getCommandHandler(command)
	if commandHandler != nil {
		config.CONFIG.DataDogClient.Incr("command", []string{"command:" + string(command), "bot_name:" + bot.Name}, 1)
		commandHandler.Handler(ctx, bot, message)
	} else {
		config.CONFIG.DataDogClient.Incr("unknown_command", nil, 1)
		bot.SendMessage(tu.Message(util.GetChatID(message), "Unknown command \U0001f937"))
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
		bot.SendMessage(tu.Message(util.GetChatID(message), response))
		lib.SaveMode(util.GetChatIDString(message), mode)
	}
}

func upgradeCommandHandler(ctx context.Context, bot *Bot, message *telego.Message) {
	chatID := util.GetChatID(message)
	if lib.IsUserFree(ctx) {
		_, err := bot.SendMessage(tu.Message(chatID, "Upgrading to free+ gives you 5x monthly usage limits, effective immediately 🎉"))
		if err != nil {
			log.Errorf("Failed to send upgrade message: %v", err)
		}
		err = mongo.MongoDBClient.UpdateUserSubscription(ctx, lib.Subscriptions[lib.FreePlusSubscriptionName])
		if err != nil {
			log.Errorf("Failed to update user subscription: %v", err)
			bot.SendMessage(tu.Message(chatID, "Failed to upgrade your account to free+ plan. Please try again later."))
			return
		}
		bot.SendMessage(tu.Message(chatID, "You are now a free+ user 🥳! Thanks for trying the bot and the wish to support it's development! 🙏"))
		return
	}
	if lib.IsUserFreePlus(ctx) {
		_, err := bot.SendMessage(tu.Message(chatID, "Upgrading account to basic paid plan.."))
		if err != nil {
			log.Errorf("Failed to send paid plan upgrade message: %v", err)
		}
		// fetch userStripeID from DB
		user, err := mongo.MongoDBClient.GetUser(ctx)
		if err != nil {
			log.Errorf("Failed to get user: %v", err)
			return
		}
		stripeCustomerId := user.StripeCustomerId
		if stripeCustomerId == "" {
			customer, err := payments.StripeCreateCustomer(ctx, bot.Bot, message)
			if err != nil {
				log.Errorf("Failed to create customer: %v", err)
				return
			}
			stripeCustomerId = customer.ID
			err = mongo.MongoDBClient.UpdateUserStripeCustomerId(ctx, stripeCustomerId)
			if err != nil {
				log.Errorf("Failed to update user stripe customer id: %v", err)
				return
			}
		}

		// create new checkout session
		session, err := payments.StripeCreateCheckoutSession(ctx, bot.Bot, message, stripeCustomerId, payments.BasicPlanPriceId)
		if err != nil {
			log.Errorf("Failed to create checkout session: %v", err)
			return
		}

		// send link to customer as a button in telegram
		bot.SendMessage(
			tu.Message(chatID, "Press the subscribe button below and navigate to our partner, Stripe, to proceed with the payment. You will upgrade to the basic paid plan, which includes:\n💪 200x usage limits compared to the Free+ plan of OpenAI tokens and voice recognition\n🧠 Access to more intelligent GPT-4 model (and more models that OpenAI will release)\n💁🏽 Priority support\n\nBy proceeding with the payments, you agree to /terms of usage.\n\nCancel your subscription at any time with the /downgrade command.").WithReplyMarkup(
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
		bot.SendMessage(tu.Message(chatID, "You are already a basic paid plan user! Premium upgrade plans are not available yet. Stay tuned for updates!"))
		return
	}

	log.Errorf("upgradeCommandHandler: unknown user subscription: %v", ctx.Value(models.SubscriptionContext{}))
}

func cancelSubscriptionCommandHandler(ctx context.Context, bot *Bot, message *telego.Message) {
	chatID := util.GetChatID(message)
	if lib.IsUserFree(ctx) {
		bot.SendMessage(tu.Message(chatID, "You are already a free user!"))
		return
	}
	if lib.IsUserFreePlus(ctx) {
		err := mongo.MongoDBClient.UpdateUserSubscription(ctx, lib.Subscriptions[lib.FreeSubscriptionName])
		if err != nil {
			log.Errorf("Failed to downgrade user subscription: to free %v", err)
			bot.SendMessage(tu.Message(chatID, "Failed to downgrade your account to free plan. Please try again later."))
			return
		}
		bot.SendMessage(tu.Message(chatID, "You are now a free user!"))
		return
	}
	if lib.IsUserBasic(ctx) {
		user, err := mongo.MongoDBClient.GetUser(ctx)
		if err != nil {
			log.Errorf("Failed to get user: %v", err)
			return
		}
		stripeCustomerId := user.StripeCustomerId
		if stripeCustomerId == "" {
			err := mongo.MongoDBClient.UpdateUserSubscription(ctx, lib.Subscriptions[lib.FreePlusSubscriptionName])
			if err != nil {
				log.Errorf("Failed to downgrade user subscription: to free+ %v", err)
				bot.SendMessage(tu.Message(chatID, "Failed to downgrade your account to free+ plan. Please try again later."))
			}
			bot.SendMessage(tu.Message(chatID, "You are now a free+ user!"))
			return
		}

		// cancel subscriptions
		payments.StripeCancelSubscription(ctx, stripeCustomerId)
		return
	}
	log.Errorf("cancelSubscriptionCommandHandler: unknown user subscription: %v", ctx.Value(models.SubscriptionContext{}))
}

func emptyCommandHandler(ctx context.Context, bot *Bot, message *telego.Message) {
	_, err := bot.SendMessage(tu.Message(util.GetChatID(message), "There is no message provided to correct or comment on. If you have a message you would like me to review, please provide it."))
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
	)
	_, err := bot.SendMessage(supportMessage)
	if err != nil {
		log.Errorf("Failed to send SupportCommand message: %v", err)
	}
}

func statusCommandHandler(ctx context.Context, bot *Bot, message *telego.Message) {
	_, err := bot.SendMessage(tu.Message(util.GetChatID(message), GetUserStatus(ctx)).WithReplyMarkup(GetStatusKeyboard(ctx)))
	if err != nil {
		log.Errorf("Failed to send StatusCommand message: %v", err)
	}
}

func clearThreadCommandHandler(ctx context.Context, bot *Bot, message *telego.Message) {
	chatID := util.GetChatID(message)
	chatIDString := util.GetChatIDString(message)
	threadId, err := redis.RedisClient.Get(ctx, chatIDString+":current-thread").Result()
	if threadId == "" {
		_, err = bot.SendMessage(tu.Message(chatID, "There is no thread to clear."))
		if err != nil {
			log.Errorf("Failed to send ClearThreadCommand message: %v", err)
		}
		return
	}

	_, err = BOT.API.DeleteThread(ctx, threadId)
	if err != nil {
		log.Errorf("Failed to clear thread: %v", err)
	}
	_, err = bot.SendMessage(tu.Message(chatID, "Thread cleared."))
	if err != nil {
		log.Errorf("Failed to send ClearThreadCommand message: %v", err)
	}
}
