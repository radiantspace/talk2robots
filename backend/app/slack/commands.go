package slack

import (
	"context"
	"strings"
	"talk2robots/m/v2/app/db/mongo"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/models"

	log "github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
)

func upgradeCommandHandler(ctx context.Context, bot *Bot) {
	userString := ctx.Value(models.UserContext{}).(string)
	userId := strings.TrimPrefix(userString, "slack:")
	channelId := ctx.Value(models.ChannelContext{}).(string)
	log.Infof("upgradeCommandHandler: userId: %s, channelId: %s", userString, channelId)
	if lib.IsUserFree(ctx) {
		_, _, _, err := bot.SendMessage(channelId,
			slack.MsgOptionText("Upgrading to free+ gives you 5x monthly usage limits, effective immediately 🎉", false),
			slack.MsgOptionPostEphemeral(userId))
		if err != nil {
			log.Errorf("Failed to send upgrade message to user %s, %v", userString, err)
		}
		err = mongo.MongoDBClient.UpdateUserSubscription(ctx, models.Subscriptions[models.FreePlusSubscriptionName])
		if err != nil {
			log.Errorf("Failed to update user %s subscription: %v", userString, err)
			bot.SendMessage(channelId,
				slack.MsgOptionText("Failed to upgrade your account to free+ plan. Please try again later.", false),
				slack.MsgOptionPostEphemeral(userId))
			return
		}
		bot.SendMessage(channelId,
			slack.MsgOptionText("You are now a free+ user 🥳! Thanks for trying the bot and the wish to support it's development! 🙏", false),
			slack.MsgOptionPostEphemeral(userId))
		return
	}
	if lib.IsUserFreePlus(ctx) || lib.IsUserBasic(ctx) {
		_, _, _, err := bot.SendMessage(channelId,
			slack.MsgOptionText("You are already a free+ plan user! Premium upgrade plans are not available yet. Stay tuned for updates!", false),
			slack.MsgOptionPostEphemeral(userId))
		if err != nil {
			log.Errorf("Failed to send already upgraded message to user %s, %v", userString, err)
		}
		return
	}

	log.Errorf("upgradeCommandHandler: unknown user %s subscription: %v", userString, ctx.Value(models.SubscriptionContext{}))
}
