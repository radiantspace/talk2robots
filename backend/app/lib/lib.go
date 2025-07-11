package lib

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"

	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/db/mongo"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/models"
	"time"
)

var (
	TIMEOUT       = 2 * time.Minute
	ErrUserBanned = fmt.Errorf("user is banned")
)

type ClientName string

const (
	SlackClientName    ClientName = "slack"
	TelegramClientName ClientName = "telegram"
)

func SetupUserAndContext(userId string, client ClientName, channelId string, topicId string) (user *models.MongoUser, currentContext context.Context, cancelContext context.CancelFunc, err error) {
	if redis.IsUserBanned(userId) {
		return nil, nil, nil, ErrUserBanned
	}

	currentContext = context.WithValue(context.Background(), models.UserContext{}, userId)
	currentContext = context.WithValue(currentContext, models.ClientContext{}, string(client))
	currentContext = context.WithValue(currentContext, models.ChannelContext{}, channelId)
	currentContext = context.WithValue(currentContext, models.TopicContext{}, topicId)
	currentContext, cancelContext = context.WithTimeout(currentContext, TIMEOUT)

	log.Infof("Fetching subscription from DB for user: %s", userId)
	currentSubscriptionName := models.FreeSubscriptionName
	defaultSubscription := models.Subscriptions[currentSubscriptionName]
	user, err = mongo.MongoDBClient.GetUser(currentContext)
	if err != nil {
		if strings.Contains(err.Error(), "mongo: no documents in result") {
			log.Infof("User %s not found, creating a new one", userId)
		} else {
			log.Errorf("Failed to get a user (%s): %v", userId, err)
			return nil, nil, cancelContext, err
		}
	}

	if user == nil || user.SubscriptionType.Name == "" {
		// No user found, create a new one
		config.CONFIG.DataDogClient.Incr("new_user", []string{"client:" + string(client)}, 1)
		err = mongo.MongoDBClient.UpdateUserSubscription(currentContext, defaultSubscription)
		if err != nil {
			log.Errorf("Failed to update user subscription: %v", err)
		}
	} else {
		currentSubscriptionName = user.SubscriptionType.Name
	}
	currentContext = context.WithValue(currentContext, models.SubscriptionContext{}, currentSubscriptionName)
	log.Infof("User %s subscription: %s", userId, currentSubscriptionName)
	return user, currentContext, cancelContext, err
}

func ValidateUserUsage(ctx context.Context) (bool, models.MongoSubscriptionName) {
	userId := ctx.Value(models.UserContext{}).(string)
	currentSubscriptionName := ctx.Value(models.SubscriptionContext{}).(models.MongoSubscriptionName)
	currentSubscription := models.Subscriptions[currentSubscriptionName]
	if currentSubscription.MaximumUsage > 0 {
		userTotalCost, err := redis.RedisClient.Get(context.Background(), UserTotalCostKey(userId)).Float64()
		if err != nil && err.Error() != "redis: nil" {
			log.Errorf("Error getting user %s total cost: %v", userId, err)
		}
		if userTotalCost >= currentSubscription.MaximumUsage {
			log.Infof("User %s usage limit exceeded", userId)
			return false, currentSubscriptionName
		}
	}
	return true, currentSubscriptionName
}

// ConvertFasthttpRequest converts a fasthttp.Request to a net/http.Request
func ConvertFasthttpRequest(ctx *fasthttp.RequestCtx) (*http.Request, error) {
	// Create an http.Request
	req, err := http.NewRequest(
		string(ctx.Method()),
		string(ctx.RequestURI()),
		bytes.NewReader(ctx.PostBody()))
	if err != nil {
		return nil, err
	}

	// Copy headers
	req.Header = ConvertFasthttpHeader(&ctx.Request.Header)

	return req, nil
}

// ConvertFasthttpHeader converts a fasthttp.RequestHeader to a net/http.Header
func ConvertFasthttpHeader(fh *fasthttp.RequestHeader) http.Header {
	h := make(http.Header)

	for _, kv := range fh.All() {
		sKey := string(kv[0])
		sValue := string(kv[1])
		h.Add(sKey, sValue)
	}

	return h
}

func AddBotSuffixToGroupCommands(ctx context.Context, message string) string {
	chatString := ctx.Value(models.UserContext{}).(string)
	client := ctx.Value(models.ClientContext{}).(string)
	if client == string(TelegramClientName) && strings.HasPrefix(chatString, "-") {
		commands := []string{
			"/chatgpt", "/voicegpt", "/clear", "/downgrade", "/grammar",
			"/start", "/status", "/summarize", "/support", "/teacher",
			"/terms", "/transcribe", "/upgrade", "/translate", "/billing",
		}

		for _, command := range commands {
			re := regexp.MustCompile(command + `(\s|$)`)
			message = re.ReplaceAllString(message, command+"@"+config.CONFIG.BotName+"$1")
		}
	}
	return message
}
