package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/db/mongo"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/models"
	"talk2robots/m/v2/app/payments"
	"talk2robots/m/v2/app/util"
	"time"

	"github.com/fasthttp/router"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
	log "github.com/sirupsen/logrus"
	"github.com/stripe/stripe-go/v78/customer"
)

const (
	SYSTEMBanUserCommand              Command = "/banuser"
	SYSTEMUnbanUserCommand            Command = "/unbanuser"
	SYSTEMStatusCommand               Command = "/status"
	SYSTEMStripeResetCommand          Command = "/stripereset"
	SYSTEMUsageResetCommand           Command = "/usagereset"
	SYSTEMUserCommand                 Command = "/user"
	SYSTEMUsersCountCommand           Command = "/userscount"
	SYSTEMUsersForSubscriptionCommand Command = "/usersforsubscription"
	SYSTEMSendMessageToUsers          Command = "/sendmessagetousers"
	SYSTEMSendMessageToAUser          Command = "/sendmessagetoauser"
)

var SystemCommandHandlers CommandHandlers = CommandHandlers{}
var SystemBOT *Bot

func NewSystemBot(rtr *router.Router, cfg *config.Config) (*Bot, error) {
	if cfg.TelegramSystemBotToken == "" {
		return nil, fmt.Errorf("system bot token is empty")
	}
	newBot, err := telego.NewBot(cfg.TelegramSystemBotToken, util.GetBotLoggerOption(cfg))
	if err != nil {
		return nil, fmt.Errorf("failed to create system bot: %w", err)
	}
	setupSystemCommandHandlers()
	updates, err := signBotForUpdates(newBot, rtr)
	if err != nil {
		return nil, fmt.Errorf("failed to sign system bot for updates: %w", err)
	}
	bh, err := th.NewBotHandler(newBot, updates, th.WithStopTimeout(time.Second*10))
	if err != nil {
		return nil, fmt.Errorf("failed to setup system bot handler: %w", err)
	}

	chatId, _ := strconv.ParseInt(cfg.TelegramSystemTo, 10, 64)
	SystemBOT = &Bot{
		Bot:        newBot,
		BotHandler: bh,
		ChatID:     tu.ID(chatId),
		Name:       "system",
	}

	bh.HandleMessage(handleSystemMessage)

	go bh.Start()

	return SystemBOT, nil
}

func NewStubSystemBot(cfg *config.Config) *Bot {
	chatId, _ := strconv.ParseInt(cfg.TelegramSystemTo, 10, 64)
	SystemBOT = &Bot{
		Dummy:  true,
		Bot:    newStubBot(cfg),
		ChatID: tu.ID(chatId),
	}
	return SystemBOT
}

// newStubBot creates new stub bot instance, that can be used for testing
func newStubBot(cfg *config.Config) *telego.Bot {
	stubBot, err := telego.NewBot(generateStubToken(), telego.WithHTTPClient(&http.Client{
		Transport: models.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"ok": true, "result": {}}`)),
			}, nil
		}),
	}), util.GetBotLoggerOption(cfg))
	if err != nil {
		log.Fatalf("Failed to create stub bot: %v", err)
	}
	return stubBot
}

// stub token that matches the pattern ^\d{9,10}:[\w-]{35}$
func generateStubToken() string {
	const digits = "0123456789"
	const alphaNum = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"

	tokenBuilder := strings.Builder{}
	for i := 0; i < 9; i++ {
		tokenBuilder.WriteByte(digits[rand.Intn(len(digits))])
	}
	tokenBuilder.WriteString(":")
	for i := 0; i < 35; i++ {
		tokenBuilder.WriteByte(alphaNum[rand.Intn(len(alphaNum))])
	}
	return tokenBuilder.String()
}

func handleSystemMessage(bot *telego.Bot, message telego.Message) {
	if SystemBOT.ChatID != tu.ID(message.Chat.ID) {
		log.Errorf("System bot received message from chat %d, but expected from %d", message.Chat.ID, SystemBOT.ChatID.ID)
		return
	}

	err := bot.SendChatAction(&telego.SendChatActionParams{ChatID: SystemBOT.ChatID, Action: telego.ChatActionTyping})
	if err != nil {
		log.Errorf("Failed to send chat action: %s", err)
	}

	// process commands
	if message.Text == string(EmptyCommand) || strings.HasPrefix(message.Text, "/") || strings.HasPrefix(message.Caption, "/") {
		log.Infof("System bot received message: %+v", message) // audit
		SystemCommandHandlers.handleCommand(context.Background(), SystemBOT, &message)
		return
	}
}

func setupSystemCommandHandlers() {
	SystemCommandHandlers = CommandHandlers{
		newCommandHandler(EmptyCommand, func(ctx context.Context, bot *Bot, message *telego.Message) {
			bot.SendMessage(tu.Message(SystemBOT.ChatID, "There is no message provided to correct or comment on. If you have a message you would like me to review, please provide it."))
		}),
		newCommandHandler(SYSTEMStatusCommand, handleStatus),
		newCommandHandler(SYSTEMUserCommand, handleUser),
		newCommandHandler(SYSTEMStripeResetCommand, handleStripeReset),
		newCommandHandler(SYSTEMUsersCountCommand, handleUsersCount),
		newCommandHandler(SYSTEMUsageResetCommand, handleUsageReset),
		newCommandHandler(SYSTEMUsersForSubscriptionCommand, handleUsersForSubscription),
		newCommandHandler(SYSTEMBanUserCommand, handleBanUser),
		newCommandHandler(SYSTEMUnbanUserCommand, handleUnbanUser),
		newCommandHandler(SYSTEMSendMessageToUsers, handleSendMessageToUsers),
		newCommandHandler(SYSTEMSendMessageToAUser, handleSendMessageToAUser),
	}
}

func handleStatus(ctx context.Context, bot *Bot, message *telego.Message) {
	systemStatus := redis.RedisClient.Get(context.Background(), "system-status")
	bot.SendMessage(tu.Message(SystemBOT.ChatID, systemStatus.Val()))
}

func handleUser(ctx context.Context, bot *Bot, message *telego.Message) {
	commandArray := strings.Split(message.Text, " ")
	if len(commandArray) < 2 {
		bot.SendMessage(tu.Message(SystemBOT.ChatID, "Please provide user id"))
		return
	}
	userId := commandArray[1]
	user, err := mongo.MongoDBClient.GetUser(context.WithValue(context.Background(), models.UserContext{}, userId))
	if err != nil {
		bot.SendMessage(tu.Message(SystemBOT.ChatID, fmt.Sprintf("Failed to get user: %s", err)))
		return
	}
	userJson, err := json.Marshal(user)
	if err != nil {
		log.Errorf("Failed to marshal user: %s", err)
	}
	userString := "DB:\n" + string(userJson) + "\n\n"

	userString += "Redis:\ntotal cost            - " + redis.RedisClient.Get(ctx, lib.UserTotalCostKey(userId)).Val() + "\n"
	userString += "total audio minutes   - " + redis.RedisClient.Get(ctx, lib.UserTotalAudioMinutesKey(userId)).Val() + "\n"
	userString += "total tokens          - " + redis.RedisClient.Get(ctx, lib.UserTotalTokensKey(userId)).Val() + "\n\n"
	userString += "current thread        - " + redis.RedisClient.Get(ctx, lib.UserCurrentThreadKey(userId, "")).Val() + "\n"
	userString += "current prompt tokens - " + redis.RedisClient.Get(ctx, lib.UserCurrentThreadPromptKey(userId, "")).Val()

	bot.SendMessage(tu.Message(SystemBOT.ChatID, fmt.Sprintf("User: %+v", userString)))
}

func handleStripeReset(ctx context.Context, bot *Bot, message *telego.Message) {
	commandArray := strings.Split(message.Text, " ")
	if len(commandArray) < 2 {
		bot.SendMessage(tu.Message(SystemBOT.ChatID, "Please provide user id"))
		return
	}
	userId := commandArray[1]
	ctx = context.WithValue(ctx, models.UserContext{}, userId)
	user, err := mongo.MongoDBClient.GetUser(ctx)
	if err != nil {
		bot.SendMessage(tu.Message(SystemBOT.ChatID, fmt.Sprintf("Failed to get user: %s", err)))
		return
	}
	stripeCustomerId := user.StripeCustomerId
	if stripeCustomerId != "" {
		payments.StripeCancelSubscription(ctx, stripeCustomerId)
		customer, err := customer.Del(stripeCustomerId, nil)
		if err != nil {
			bot.SendMessage(tu.Message(SystemBOT.ChatID, fmt.Sprintf("Failed to delete customer: %s", err)))
		} else {
			bot.SendMessage(tu.Message(SystemBOT.ChatID, fmt.Sprintf("Customer deleted from Stripe: %+v", customer)))
		}
		err = mongo.MongoDBClient.UpdateUserStripeCustomerId(ctx, "")
		if err != nil {
			bot.SendMessage(tu.Message(SystemBOT.ChatID, fmt.Sprintf("Failed to update user: %s", err)))
			return
		}
		//  downgrade engine to GPT-4o Mini
		redis.SaveModel(userId, models.ChatGpt4oMini)
	}

	bot.SendMessage(tu.Message(SystemBOT.ChatID, fmt.Sprintf("Stripe Reset for: %+v", user)))
}

func handleUsersCount(ctx context.Context, bot *Bot, message *telego.Message) {
	users, err := mongo.MongoDBClient.GetUsersCount(context.Background())
	if err != nil {
		bot.SendMessage(tu.Message(SystemBOT.ChatID, fmt.Sprintf("Failed to get users: %s", err)))
		return
	}
	bot.SendMessage(tu.Message(SystemBOT.ChatID, fmt.Sprintf("Users: %+v", users)))
}

func handleUsageReset(ctx context.Context, bot *Bot, message *telego.Message) {
	commandArray := strings.Split(message.Text, " ")
	if len(commandArray) < 2 {
		bot.SendMessage(tu.Message(SystemBOT.ChatID, "Please provide user id"))
		return
	}
	userId := commandArray[1]
	redis.RedisClient.Del(ctx, lib.UserTotalCostKey(userId))
	redis.RedisClient.Del(ctx, lib.UserTotalAudioMinutesKey(userId))
	redis.RedisClient.Del(ctx, lib.UserTotalTokensKey(userId))
	bot.SendMessage(tu.Message(SystemBOT.ChatID, "Usage reset for user: "+userId))
}

func handleUsersForSubscription(ctx context.Context, bot *Bot, message *telego.Message) {
	commandArray := strings.Split(message.Text, " ")
	if len(commandArray) < 2 {
		bot.SendMessage(tu.Message(SystemBOT.ChatID, "Please provide subscription name"))
		return
	}
	subscriptionName := commandArray[1]
	usersCount, err := mongo.MongoDBClient.GetUsersCountForSubscription(context.Background(), subscriptionName)
	if err != nil {
		bot.SendMessage(tu.Message(SystemBOT.ChatID, fmt.Sprintf("Failed to get users: %s", err)))
		return
	}
	bot.SendMessage(tu.Message(SystemBOT.ChatID, fmt.Sprintf("Users count for %s subscription: %d", subscriptionName, usersCount)))
}

func handleBanUser(ctx context.Context, bot *Bot, message *telego.Message) {
	commandArray := strings.Split(message.Text, " ")
	if len(commandArray) < 2 {
		bot.SendMessage(tu.Message(SystemBOT.ChatID, "Please provide user id"))
		return
	}
	userId := commandArray[1]
	err := redis.RedisClient.Set(ctx, userId+":banned", "true", 0).Err()
	if err != nil {
		bot.SendMessage(tu.Message(SystemBOT.ChatID, fmt.Sprintf("Failed to ban user: %v", err)))
		return
	}
	bot.SendMessage(tu.Message(SystemBOT.ChatID, "User "+userId+" banned"))
}

func handleUnbanUser(ctx context.Context, bot *Bot, message *telego.Message) {
	commandArray := strings.Split(message.Text, " ")
	if len(commandArray) < 2 {
		bot.SendMessage(tu.Message(SystemBOT.ChatID, "Please provide user id"))
		return
	}
	userId := commandArray[1]
	err := redis.RedisClient.Del(ctx, userId+":banned").Err()
	if err != nil {
		bot.SendMessage(tu.Message(SystemBOT.ChatID, fmt.Sprintf("Failed to unban user: %v", err)))
		return
	}
	bot.SendMessage(tu.Message(SystemBOT.ChatID, "User "+userId+" unbanned"))
}

func handleSendMessageToUsers(ctx context.Context, bot *Bot, message *telego.Message) {
	commandUsage := fmt.Sprintf("Usage: %s <maximum_users_to_notify> <days_after_last_notification> <message>", SYSTEMSendMessageToUsers)
	commandArray := strings.Split(message.Text, " ")
	if len(commandArray) < 4 {
		bot.SendMessage(tu.Message(SystemBOT.ChatID, commandUsage))
		return
	}
	maximumUsers, err := strconv.Atoi(commandArray[1])
	if err != nil {
		bot.SendMessage(tu.Message(SystemBOT.ChatID, commandUsage))
		return
	}
	// skip users who were notified after this amount of days ago
	daysAfterLastNotification, err := strconv.Atoi(commandArray[2])
	if err != nil {
		bot.SendMessage(tu.Message(SystemBOT.ChatID, commandUsage))
		return
	}
	messageText := strings.Join(commandArray[3:], " ")

	page := 0
	pageSize := 10
	sleepBetweenPages := 1 * time.Second
	notifiedUsers := 0.0
	skippedNonTelegramUsers := 0.0
	skippedGroupUsers := 0.0
	skippedErrorUsers := 0.0
	notifiedUsersIds := []string{}

	defer func() {
		message := fmt.Sprintf("Message sent to %.f users, %.f groups skipped, %.f error users skipped, %.f non-telegram users skipped", notifiedUsers, skippedGroupUsers, skippedErrorUsers, skippedNonTelegramUsers)
		bot.SendMessage(tu.Message(SystemBOT.ChatID, message))
		log.Info("[SYSTEM] " + message)
		config.CONFIG.DataDogClient.Incr("custom_message_sent_total", []string{}, notifiedUsers)
		config.CONFIG.DataDogClient.Incr("custom_message_sent_groups_skipped", []string{}, skippedGroupUsers)
		config.CONFIG.DataDogClient.Incr("custom_message_sent_non_telegram_users_skipped", []string{}, skippedNonTelegramUsers)
		config.CONFIG.DataDogClient.Incr("custom_message_sent_error_users_skipped", []string{}, skippedErrorUsers)

		err := mongo.MongoDBClient.UpdateUsersNotified(context.Background(), notifiedUsersIds)
		if err != nil {
			log.Errorf("[SYSTEM] Failed to update users notified: %s", err)
		} else {
			log.Infof("[SYSTEM] %d users notified updated", len(notifiedUsersIds))
		}
	}()

	for {
		users, err := mongo.MongoDBClient.GetUserIdsNotifiedBefore(context.Background(), time.Now().AddDate(0, 0, -1*daysAfterLastNotification), page, pageSize)
		if err != nil {
			bot.SendMessage(tu.Message(SystemBOT.ChatID, fmt.Sprintf("Failed to get users page %d (page size %d): %s", page, pageSize, err)))
			return
		}
		if len(users) == 0 {
			break
		}
		for _, user := range users {
			if user == "" || strings.HasPrefix(user, "SYSTEM:STATUS") || strings.HasPrefix(user, "U") || strings.HasPrefix(user, "slack") {
				// skip non telegram users
				skippedNonTelegramUsers++
				continue
			}

			if strings.HasPrefix(user, "-") {
				// skip group users
				skippedGroupUsers++
				continue
			}

			userId, err := strconv.ParseInt(user, 10, 64)
			if err != nil {
				log.Errorf("Failed to convert user id %s to int: %s", user, err)
				skippedErrorUsers++
				continue
			}

			_, err = BOT.SendMessage(tu.Message(tu.ID(userId), messageText))
			if err != nil {
				skippedErrorUsers++

				if strings.Contains(err.Error(), "bot was blocked by the user") || strings.Contains(err.Error(), "user is deactivated") {
					notifiedUsersIds = append(notifiedUsersIds, user)
				} else {
					log.Errorf("[SYSTEM] Failed to send message to user %d: %v", userId, err)
				}
			} else {
				notifiedUsers++
				notifiedUsersIds = append(notifiedUsersIds, user)
				log.Debugf("Custom message sent to user %d", userId)
			}

			if notifiedUsers >= float64(maximumUsers) {
				log.Infof("[SYSTEM] Maximum users reached with custom message: %.f", notifiedUsers)
				return
			}
		}
		time.Sleep(sleepBetweenPages)
		page++
	}
}

func handleSendMessageToAUser(ctx context.Context, bot *Bot, message *telego.Message) {
	commandUsage := fmt.Sprintf("Usage: %s <user_id> <message>", SYSTEMSendMessageToAUser)
	commandArray := strings.Split(message.Text, " ")
	if len(commandArray) < 3 {
		bot.SendMessage(tu.Message(SystemBOT.ChatID, commandUsage))
		return
	}
	userId := commandArray[1]
	userIdInt, err := strconv.ParseInt(userId, 10, 64)
	if err != nil {
		bot.SendMessage(tu.Message(SystemBOT.ChatID, commandUsage))
		return
	}
	messageText := strings.Join(commandArray[2:], " ")

	_, err = BOT.SendMessage(tu.Message(tu.ID(userIdInt), messageText))
	if err != nil {
		log.Errorf("Failed to send message to user %s: %v", userId, err)
		bot.SendMessage(tu.Message(SystemBOT.ChatID, fmt.Sprintf("Failed to send message to user %s: %v", userId, err)))
		return
	}

	log.Infof("[SYSTEM] Message sent to user %s", userId)
	bot.SendMessage(tu.Message(SystemBOT.ChatID, fmt.Sprintf("Message sent to user %s", userId)))
}
