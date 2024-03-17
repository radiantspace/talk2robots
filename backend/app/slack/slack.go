package slack

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/models"
	"talk2robots/m/v2/app/openai"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/fasthttp/router"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/valyala/fasthttp"
)

var (
	BOT                                 *Bot
	GRAMMAR_CHECK_REACTION              = "eyeglasses"
	SUMMARIZE_REACTION                  = "memo"
	PROCESSED_REACTION                  = "white_check_mark"
	HOME_TAB_MESSAGE                    = "I'm @gienji, your intelligent chatbot. Just DM your question or request, and I'll do my best to provide you with the information you need. You can also add me to a channel and mention @gienji. React a message with :eyeglasses: to check grammar, :memo: to summarize threads.\n\n Btw, did I mention that I'm powered by OpenAI's API and completely open source - https://github.com/radiantspace/talk2robots?"
	TIMEOUT                             = 2 * time.Minute
	THREAD_MESSAGES_LIMIT_FOR_SUMMARIZE = 100
)

type Bot struct {
	*openai.API
	*slack.Client
	Name          string
	SigningSecret string
	WhisperConfig openai.WhisperConfig
}

func NewBot(rtr *router.Router, config *config.Config) (*Bot, error) {
	slackClient := slack.New(config.SlackBotToken)

	// subscribe to slack events
	rtr.POST("/slack/events", slackEventsHandler)

	// subscribe to slack commands
	rtr.POST("/slack/commands", slackCommandsHandler)

	BOT = &Bot{
		API:           openai.NewAPI(config),
		Client:        slackClient,
		SigningSecret: config.SlackSigningSecret,
		Name:          config.BotName,
		WhisperConfig: openai.WhisperConfig{
			APIKey:             config.OpenAIAPIKey,
			WhisperAPIEndpoint: config.WhisperAPIEndpoint,
			Mode:               "transcriptions",
			StopTimeout:        5 * time.Second,
			OnTranscribe:       nil,
		},
	}

	return BOT, nil
}

func slackEventsHandler(ctx *fasthttp.RequestCtx) {
	ok := validateSignature(ctx)
	if !ok {
		return
	}
	eventsAPIEvent, err := slackevents.ParseEvent(json.RawMessage(ctx.PostBody()), slackevents.OptionNoVerifyToken())
	if err != nil {
		log.Errorf("Error parsing slack event: %v", err)
		ctx.Error(err.Error(), http.StatusInternalServerError)
		return
	}

	log.Infof("Received slack event: %v", eventsAPIEvent.InnerEvent.Type)
	if eventsAPIEvent.Type == slackevents.URLVerification {
		handleUrlVerificationEvent(ctx)
		return
	}

	ctx.Response.SetStatusCode(http.StatusOK)
	ctx.Response.SetBodyString("")

	go func() {
		if eventsAPIEvent.Type == slackevents.CallbackEvent {
			innerEvent := eventsAPIEvent.InnerEvent
			switch ev := innerEvent.Data.(type) {
			case *slackevents.AppMentionEvent:
				if ev.User != "" && ev.BotID == "" {
					log.Infof("Received slack app mention event in channel %s, ts: %s", ev.Channel, ev.TimeStamp)
					handleMessageEvent("slack:"+ev.User, ev.Channel, ev.TimeStamp, ev.Text, true, "")
					BOT.AddReaction(PROCESSED_REACTION, slack.ItemRef{
						Channel:   ev.Channel,
						Timestamp: ev.TimeStamp,
					})
				}
				return
			case *slackevents.MessageEvent:
				// only process private messages from users to the app
				if ev.ChannelType == "im" && ev.User != "" && ev.SubType != "bot_message" && ev.BotID == "" {
					log.Infof("Received slack message event from user %s in channel %s, ts: %s", ev.User, ev.Channel, ev.TimeStamp)
					handleMessageEvent("slack:"+ev.User, ev.Channel, ev.TimeStamp, ev.Text, false, "")
				}
				return
			case *slackevents.ReactionAddedEvent:
				if (ev.Reaction != GRAMMAR_CHECK_REACTION && ev.Reaction != SUMMARIZE_REACTION) || ev.Item.Type != "message" {
					return
				}
				log.Infof("Received slack `%s` reaction event from user %s in channel %s, ts: %s", ev.Reaction, ev.User, ev.Item.Channel, ev.Item.Timestamp)
				message := fetchMessage(ev.Item.Channel, ev.Item.Timestamp)
				messageTS := message.Timestamp
				if message.ThreadTimestamp != "" {
					messageTS = message.ThreadTimestamp
				}
				if ev.Reaction == GRAMMAR_CHECK_REACTION {
					handleMessageEvent("slack:"+ev.User, ev.Item.Channel, messageTS, message.Text, true, lib.Grammar)
					BOT.AddReaction(PROCESSED_REACTION, slack.ItemRef{
						Channel:   ev.Item.Channel,
						Timestamp: message.Timestamp,
					})
					return
				} else if ev.Reaction == SUMMARIZE_REACTION {
					summarizeThread("slack:"+ev.User, message.Timestamp, ev.Item.Channel)
					return
				}
				return
			case *slackevents.AppHomeOpenedEvent:
				handleAppHomeOpenedEvent(ev)
				return
			}
		}
	}()
}

func slackCommandsHandler(ctx *fasthttp.RequestCtx) {
	ok := validateSignature(ctx)
	if !ok {
		return
	}
	request, err := lib.ConvertFasthttpRequest(ctx)
	if err != nil {
		log.Errorf("Error converting fasthttp.Request to http.Request: %v", err)
		ctx.Error(err.Error(), http.StatusInternalServerError)
		return
	}
	command, err := slack.SlashCommandParse(request)
	if err != nil {
		log.Errorf("Error parsing slack command: %v", err)
		ctx.Error(err.Error(), http.StatusInternalServerError)
		return
	}

	userId := "slack:" + command.UserID
	log.Infof("Received slack command: %v from user: %v", command.Command, userId)

	ctx.Response.SetStatusCode(http.StatusOK)
	ctx.Response.SetBodyString("")

	go func() {
		switch command.Command {
		case "/grammar":
			lib.SaveMode(userId, "", lib.Grammar, "")
			BOT.SendMessage(command.ChannelID, slack.MsgOptionText("Grammar mode enabled", false), slack.MsgOptionPostEphemeral(command.UserID))
		case "/chatgpt":
			lib.SaveMode(userId, "", lib.ChatGPT, "")
			BOT.SendMessage(command.ChannelID, slack.MsgOptionText("ChatGPT mode enabled", false), slack.MsgOptionPostEphemeral(command.UserID))
		case "/upgrade":
			_, currentContext, _, err := lib.SetupUserAndContext(userId, lib.SlackClientName, command.ChannelID, "")
			if err != nil {
				log.Errorf("Error setting up user and context: %v", err)
				return
			}
			upgradeCommandHandler(currentContext, BOT)
		}
	}()
}

func handleUrlVerificationEvent(ctx *fasthttp.RequestCtx) {
	var r *slackevents.ChallengeResponse
	err := json.Unmarshal(ctx.PostBody(), &r)
	if err != nil {
		ctx.Error(err.Error(), http.StatusInternalServerError)
		return
	}
	ctx.SetContentType("text")
	ctx.Write([]byte(r.Challenge))
}

func handleAppHomeOpenedEvent(ev *slackevents.AppHomeOpenedEvent) {
	userId := "slack:" + ev.User
	user, _, cancelFunc, err := lib.SetupUserAndContext(userId, lib.SlackClientName, ev.Channel, "")
	if err != nil {
		if err == lib.ErrUserBanned {
			log.Infof("User %s is banned", userId)
			return
		}

		log.Errorf("Error setting up user and context: %v", err)
		return
	}
	defer cancelFunc()

	usage := user.Usage
	productName := user.SubscriptionType.Name
	hasFreePlan := productName == models.FreeSubscriptionName // TODO: basic subscription: || productName == lib.FreePlusSubscriptionName

	// Row 1: Current Plan and Upgrade Button
	currentPlanSection := slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("✅ *%s*", productName), false, false), nil, nil)
	if hasFreePlan {
		currentPlanSection.Accessory = slack.NewAccessory(slack.NewButtonBlockElement("", "upgrade_plan", slack.NewTextBlockObject("plain_text", "Upgrade", false, false)))
		currentPlanSection.Accessory.ButtonElement.URL = "https://radiant.space"
	}

	// Info section
	infoSection := slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", HOME_TAB_MESSAGE, false, false), nil, nil)

	row1Blocks := []slack.Block{
		currentPlanSection,
		slack.NewDividerBlock(),
		infoSection,
		slack.NewDividerBlock(),
	}

	usageSection := slack.NewContextBlock("", slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("Current $ usage: %f", usage), false, false))
	row1Blocks = append(row1Blocks, usageSection)

	contactSupportButton := slack.NewButtonBlockElement("", "email_support", slack.NewTextBlockObject("plain_text", "✉️ Contact support", false, false))
	contactSupportButton.URL = "mailto:free+support@radiant.space"

	row1Blocks = append(row1Blocks, slack.NewActionBlock("", contactSupportButton))

	homeTabContent := slack.HomeTabViewRequest{
		Type: "home",
		Blocks: slack.Blocks{
			BlockSet: row1Blocks,
		},
	}

	_, err = BOT.Client.PublishView(ev.User, homeTabContent, "")

	if err != nil {
		log.Errorf("Failed to publish home tab view: %v", err)
	}
}

func handleMessageEvent(userId string, channel string, messageTS string, messageText string, replyInThread bool, mode lib.ModeName) {
	// throw err if current user doesn't start with "slack:"
	if !strings.HasPrefix(userId, "slack:") {
		log.Errorf("Invalid user: %s", userId)
		return
	}
	_, currentContext, cancelFunc, err := lib.SetupUserAndContext(userId, lib.SlackClientName, channel, "")
	if err != nil {
		if err == lib.ErrUserBanned {
			log.Infof("User %s is banned", userId)
			return
		}

		log.Errorf("Error setting up user and context: %v", err)
		return
	}
	defer cancelFunc()

	// user usage exceeded monthly limit, send message and return
	ok := lib.ValidateUserUsage(currentContext)
	if !ok {
		BOT.SendMessage(channel, slack.MsgOptionText("Your monthly usage limit has been exceeded. Please /upgrade your plan.", false), slack.MsgOptionPostEphemeral(userId))
		config.CONFIG.DataDogClient.Incr("usage_exceeded", []string{"client:slack"}, 1)
		return
	}

	config.CONFIG.DataDogClient.Incr("text_message_received", []string{"client:slack"}, 1)
	var seedData []models.Message
	var userMessagePrimer string

	if mode == "" {
		mode, _ = lib.GetMode(userId, "")
	}
	seedData, userMessagePrimer = lib.GetSeedDataAndPrimer(mode)

	log.Infof("Received message in slack chat: %s, user: %s, mode: %s, initiating request to OpenAI", channel, userId, mode)
	engineModel := redis.GetChatEngine(userId)

	ProcessStreamingMessage(currentContext, channel, messageTS, messageText, seedData, userMessagePrimer, mode, engineModel, cancelFunc, replyInThread)
}

func summarizeThread(userId string, messageTS string, channelId string) {
	// throw err if current user doesn't start with "slack:"
	if !strings.HasPrefix(userId, "slack:") {
		log.Errorf("Invalid user: %s", userId)
		return
	}
	_, currentContext, cancelFunc, err := lib.SetupUserAndContext(userId, lib.SlackClientName, channelId, "")
	if err != nil {
		if err == lib.ErrUserBanned {
			log.Infof("User %s is banned", userId)
			return
		}

		log.Errorf("Error setting up user and context: %v", err)
		return
	}
	defer cancelFunc()

	// user usage exceeded monthly limit, send message and return
	ok := lib.ValidateUserUsage(currentContext)
	if !ok {
		BOT.SendMessage(channelId, slack.MsgOptionText("Your monthly usage limit has been exceeded. Please /upgrade your plan.", false), slack.MsgOptionPostEphemeral(userId))
		config.CONFIG.DataDogClient.Incr("usage_exceeded", []string{"client:slack"}, 1)
		return
	}

	config.CONFIG.DataDogClient.Incr("summarize_thread", []string{"client:slack"}, 1)
	var seedData []models.Message
	var userMessagePrimer string
	seedData, userMessagePrimer = lib.GetSeedDataAndPrimer(lib.Summarize)
	messageText := fetchMessageThread(channelId, messageTS)

	if messageText == "" {
		BOT.SendMessage(channelId, slack.MsgOptionText("Error fetching message thread", false), slack.MsgOptionPostEphemeral(userId))
		return
	}

	log.Infof("Received summarize request in slack chat: %s, user: %s, initiating request to OpenAI", channelId, userId)
	engineModel := redis.GetChatEngine(userId)

	summarizeInThread := true
	if messageTS == "" {
		summarizeInThread = false
	}
	ProcessStreamingMessage(currentContext, channelId, messageTS, messageText, seedData, userMessagePrimer, lib.Summarize, engineModel, cancelFunc, summarizeInThread)
}
