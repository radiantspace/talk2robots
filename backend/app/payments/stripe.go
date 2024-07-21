package payments

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/db/mongo"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/models"
	"talk2robots/m/v2/app/util"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	log "github.com/sirupsen/logrus"

	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/checkout/session"
	"github.com/stripe/stripe-go/v78/customer"
	"github.com/stripe/stripe-go/v78/subscription"
	"github.com/stripe/stripe-go/v78/webhook"
	"github.com/valyala/fasthttp"
)

const (
	// TODO: move to config
	// TestBasicPlanPriceId = "price_1N9MltLiy4WJgIwVyfxmfGGG"
	BasicPlanPriceId = "price_1N9MjhLiy4WJgIwVLXPc41OQ"
	ProPlanPriceId   = "price_1OoMCDLiy4WJgIwVhQml2Kxe"
	TelegramChatID   = "telegram_chat_id"
	AppID            = "app_id"
)

func StripeWebhook(ctx *fasthttp.RequestCtx) {
	payload := ctx.Request.Body()
	event := stripe.Event{}

	if err := json.Unmarshal(payload, &event); err != nil {
		log.Errorf("Webhook error while parsing Stripe request. %v", err)
		ctx.Response.Header.SetStatusCode(http.StatusBadRequest)
		return
	}

	endpointSecret := config.CONFIG.StripeEndpointSecret
	signatureHeader := string(ctx.Request.Header.PeekAll("Stripe-Signature")[0])
	event, err := webhook.ConstructEvent(payload, signatureHeader, endpointSecret)
	if err != nil {
		log.Errorf("Webhook signature verification failed. %v", err)
		ctx.Response.Header.SetStatusCode(http.StatusBadRequest) // Return a 400 error on a bad signature
		return
	}
	config.CONFIG.DataDogClient.Incr("stripe.webhook", []string{"event_type:" + string(event.Type)}, 1)

	// Unmarshal the event data into an appropriate struct depending on its Type
	switch event.Type {
	case "checkout.session.expired", "invoice.payment_succeeded", "payment_intent.succeeded":
		return
	case "customer.subscription.created":
		// we handle this event in handleCheckoutSessionCompleted
		log.Debugf("Customer subscription created: %v", event.Data.Raw)
		return
	case "checkout.session.completed":
		var session stripe.CheckoutSession
		err := json.Unmarshal(event.Data.Raw, &session)
		if err != nil {
			log.Errorf("Error parsing %s webhook JSON: %v", event.Type, err)
			ctx.Response.Header.SetStatusCode(http.StatusBadRequest)
			return
		}
		handleCheckoutSessionCompleted(session)
	case "customer.subscription.deleted":
		var subscription stripe.Subscription
		err := json.Unmarshal(event.Data.Raw, &subscription)
		if err != nil {
			log.Errorf("Error parsing webhook JSON: %v", err)
			ctx.Response.Header.SetStatusCode(http.StatusBadRequest)
			return
		}
		log.Infof("Canceled subscription for %s.", subscription.Customer.ID)
		handleCustomerSubscriptionDeleted(subscription)
	default:
		log.Errorf("Unhandled Stripe event type: %s, payload: %v", event.Type, event)
	}

	ctx.Response.Header.SetStatusCode(http.StatusOK)
}

func StripeCreateCustomer(ctx context.Context, bot *telego.Bot, message *telego.Message) (*stripe.Customer, error) {
	userIDString := util.GetChatIDString(message)
	userName := config.CONFIG.BotName + ":" + userIDString
	description := config.CONFIG.BotName + " telegram bot created customer: " + userIDString
	if message.From != nil && message.From.Username != "" {
		userName = userName + ":" + message.From.Username
		description = message.From.FirstName + " " + message.From.LastName + ", " + config.CONFIG.BotName + " telegram bot created customer: " + userIDString
	}
	params := &stripe.CustomerParams{
		Name:        stripe.String(userName),
		Description: stripe.String(description),
	}
	params.AddMetadata(TelegramChatID, userIDString)
	params.AddMetadata(AppID, config.CONFIG.BotName)

	c, err := customer.New(params)
	if err != nil {
		log.Errorf("StripeCreateCustomer: %v", err)
		return nil, err
	}
	return c, nil
}

func StripeCreateCheckoutSession(ctx context.Context, bot *telego.Bot, message *telego.Message, customerId, priceId string) (*stripe.CheckoutSession, error) {
	chatID := util.GetChatID(message)
	params := &stripe.CheckoutSessionParams{
		CancelURL:         stripe.String(config.CONFIG.BotUrl),
		ClientReferenceID: stripe.String(util.GetChatIDString(message)),
		Customer:          stripe.String(customerId),
		Mode:              stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		SuccessURL:        stripe.String(config.CONFIG.BotUrl),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceId),
				Quantity: stripe.Int64(1),
			},
		},
	}
	params.AddMetadata(TelegramChatID, util.GetChatIDString(message))
	params.AddMetadata(AppID, config.CONFIG.BotName)
	s, err := session.New(params)
	if err != nil {
		bot.SendMessage(tu.Message(chatID, "Couldn't reach Stripe. Please try again later."))
		log.Errorf("StripeCreateCheckoutSession: %v", err)
		return nil, err
	}

	return s, nil
}

func StripeGetCustomer(ctx context.Context, bot *telego.Bot, message *telego.Message, customerId string) (*stripe.Customer, error) {
	chatID := util.GetChatID(message)
	c, err := customer.Get(customerId, nil)
	if err != nil {
		bot.SendMessage(tu.Message(chatID, "Couldn't reach Stripe. Please try again later."))
		log.Errorf("StripeGetCustomer: %v", err)
		return nil, err
	}
	return c, nil
}

func StripeCancelSubscription(ctx context.Context, customerId string) error {
	params := &stripe.SubscriptionListParams{
		Customer: stripe.String(customerId),
		Status:   stripe.String(string(stripe.SubscriptionStatusActive)),
	}
	params.AddExpand("data.default_payment_method")
	i := subscription.List(params)
	for i.Next() {
		sub := i.Subscription()
		if sub.Status == "active" {
			cancelParams := &stripe.SubscriptionCancelParams{
				Prorate: stripe.Bool(true),
			}

			_, err := subscription.Cancel(sub.ID, cancelParams)
			if err != nil {
				log.Errorf("Failed cancelling subscription for customer %s error: %v", customerId, err)
			} else {
				log.Infof("Canceled subscription for customer %s, sending refund if needed..", customerId)
			}

			// TODO: refund remaining balance
		}
	}
	return nil
}

func handleCheckoutSessionCompleted(session stripe.CheckoutSession) {
	// ignore sessions for other apps, if any
	if session.Metadata[AppID] != config.CONFIG.BotName && session.Metadata[AppID] != "" {
		log.Infof("Ignoring checkout session %s for app %s", session.ID, session.Metadata[AppID])
		return
	}
	log.Infof("Successful checkout session for %d.", session.AmountTotal)
	log.Infof("Processing checkout session %s, user_id: %s", session.ID, session.Metadata[TelegramChatID])
	config.CONFIG.DataDogClient.Incr("stripe.checkout_session_completed", []string{"payment_status:" + string(session.PaymentStatus)}, 1)
	chatIDString := session.Metadata[TelegramChatID]
	ctx := context.WithValue(context.Background(), models.UserContext{}, chatIDString)
	ctx = context.WithValue(ctx, models.ClientContext{}, "telegram") //  TODO: fix for slack/other clients
	chatIDInt64, err := strconv.ParseInt(chatIDString, 10, 64)
	if err != nil {
		log.Errorf("Failed to convert string to int64 and process successful payment: %v, user_id: %s", err, chatIDString)
		return
	}
	chatID := tu.ID(chatIDInt64)

	failedNotification := "Failed to upgrade your account to basic paid plan. Please contact /support for help."
	failedNotification = lib.AddBotSuffixToGroupCommands(ctx, failedNotification)
	if session.PaymentStatus != "paid" {
		log.Errorf("Checkout session %s payment status is not paid: %s, user_id: %s", session.ID, session.PaymentStatus, chatIDString)
		PaymentsBot.SendMessage(tu.Message(chatID, failedNotification))
		return
	}

	err = mongo.MongoDBClient.UpdateUserSubscription(ctx, models.Subscriptions[models.BasicSubscriptionName])
	if err != nil {
		log.Errorf("Failed to update MongoDB record and process successful payment: %v, user_id: %s", err, chatIDString)
		PaymentsBot.SendMessage(tu.Message(chatID, failedNotification))
		return
	}

	PaymentsBot.SendMessage(tu.Message(chatID, "Your account has been upgraded to basic paid plan! Thanks for your support and enjoy using the bot!"))

	go func() {
		// update subscription metadata to include telegram chat id
		// this way we can immediately correlate subscription events to a user
		params := &stripe.SubscriptionParams{}
		params.AddMetadata(TelegramChatID, chatIDString)
		params.AddMetadata(AppID, config.CONFIG.BotName)
		_, err = subscription.Update(session.Subscription.ID, params)
		if err != nil {
			log.Errorf("Failed to update subscription metadata to add telegram chat id: %v, user_id: %s", err, chatIDString)
		}

		if customerDetails := session.CustomerDetails; customerDetails != nil {
			err = mongo.MongoDBClient.UpdateUserContacts(ctx, customerDetails.Name, customerDetails.Phone, customerDetails.Email)
			if err != nil {
				log.Errorf("Failed to update user contacts: %v, user_id: %s", err, chatIDString)
			}
		}

		if err == nil {
			log.Infof("Successfully processed checkout session %s, user_id: %s", session.ID, chatIDString)
		} else {
			log.Errorf("Failed to process checkout session %s, user_id: %s", session.ID, chatIDString)
		}
	}()
}

func handleCustomerSubscriptionDeleted(subscription stripe.Subscription) {
	// ignore subscriptions for other apps, if any
	if subscription.Metadata[AppID] != config.CONFIG.BotName && subscription.Metadata[AppID] != "" {
		log.Infof("Ignoring customer subscription %s for app %s", subscription.ID, subscription.Metadata[AppID])
		return
	}
	log.Infof("Processing customer subscription deleted: %+v", subscription)
	config.CONFIG.DataDogClient.Incr("stripe.customer_subscription_deleted", nil, 1)

	chatIDString := subscription.Metadata[TelegramChatID]
	if chatIDString == "" {
		log.Errorf("handleCustomerSubscriptionDeleted: failed to get telegram_chat_id from Stripe subscription metadata, try to get from customer %s", subscription.Customer.ID)
		customer, err := customer.Get(subscription.Customer.ID, nil)
		if err != nil {
			log.Errorf("handleCustomerSubscriptionDeleted: failed to get customer %s from Stripe: %v", subscription.Customer.ID, err)
			return
		}
		chatIDString = customer.Metadata[TelegramChatID]
	}

	chatIDInt64, err := strconv.ParseInt(chatIDString, 10, 64)
	if err != nil {
		log.Errorf("handleCustomerSubscriptionDeleted: failed to convert string to int64, user id: %s: %v", chatIDString, err)
		return
	}
	ctx := context.WithValue(context.Background(), models.UserContext{}, chatIDString)
	ctx = context.WithValue(ctx, models.ClientContext{}, "telegram") //  TODO: fix for slack/other clients
	chatID := tu.ID(chatIDInt64)
	err = mongo.MongoDBClient.UpdateUserSubscription(ctx, models.Subscriptions[models.FreePlusSubscriptionName])
	if err != nil {
		log.Errorf("handleCustomerSubscriptionDeleted: failed to update MongoDB record, user id: %s: %v", chatIDString, err)
		notification := "Failed to cancel your subscription. Please contact /support for help."
		notification = lib.AddBotSuffixToGroupCommands(ctx, notification)
		PaymentsBot.SendMessage(tu.Message(chatID, notification))
		return
	}

	//  downgrade engine to GPT 4o Mini
	go redis.SaveModel(chatIDString, models.ChatGpt4oMini)

	PaymentsBot.SendMessage(tu.Message(chatID, "Your subscription has been canceled and the account downgraded to free+. No further charges will be made. If you were using GPT-4 it was downgraded to GPT-3.5 Turbo."))
	log.Infof("Successfully deleted customer %s subscription %s, user id: %s", subscription.Customer.ID, subscription.ID, chatIDString)
}
