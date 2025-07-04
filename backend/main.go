package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/db/mongo"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/payments"
	"talk2robots/m/v2/app/slack"
	"talk2robots/m/v2/app/telegram"
	"talk2robots/m/v2/app/util"
	"talk2robots/m/v2/app/workers"
	"talk2robots/m/v2/app/workers/clearusage"
	"talk2robots/m/v2/app/workers/onstart"
	"talk2robots/m/v2/app/workers/status"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/fasthttp/router"
	tu "github.com/mymmrac/telego/telegoutil"
	log "github.com/sirupsen/logrus"
	"github.com/stripe/stripe-go/v78"
	"github.com/valyala/fasthttp"
)

func main() {
	done := make(chan struct{}, 1)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	env := util.Env("ENV", "dev")
	dataDogClient, err := statsd.New("datadog-agent.default.svc.cluster.local:8125", statsd.WithNamespace("talk2robots."))
	if err != nil && env == "production" {
		log.Fatalf("error creating main DataDog client: %v", err)
	}

	config.CONFIG = &config.Config{
		BotUrl:          "https://t.me/gienjibot?start=s=home",
		DataDogClient:   dataDogClient,
		Environment:     env,
		OpenAIAPIKey:    util.Env("OPENAI_API_KEY"),
		FireworksAPIKey: util.Env("FIREWORKS_API_KEY"),
		ClaudeAPIKey:    util.Env("CLAUDE_API_KEY"),
		GrokAPIKey:      util.Env("GROK_API_KEY"),
		Redis: config.Redis{
			Host:     util.Env("REDIS_HOST"),
			Port:     "6379",
			Password: util.Env("REDIS_PASSWORD"),
		},
		SlackBotToken:          util.Env("SLACK_BOT_TOKEN"),
		SlackSigningSecret:     util.Env("SLACK_SIGNING_SECRET"),
		StatusWorkerInterval:   time.Minute,
		StripeEndpointSecret:   util.Env("STRIPE_ENDPOINT_SECRET"),
		StripeEndpointSuffix:   util.Env("STRIPE_ENDPOINT_SUFFIX"),
		StripeToken:            util.Env("STRIPE_TOKEN"),
		TelegramBotToken:       util.Env("TELEGRAM_BOT_TOKEN"),
		TelegramSystemBotToken: util.Env("TELEGRAM_SYSTEM_TOKEN"),
		TelegramSystemTo:       util.Env("TELEGRAM_SYSTEM_TO"),
		WhisperAPIEndpoint:     util.Env("WHISPER_API_ENDPOINT", "https://api.openai.com/v1/audio/"),
		MongoDBConnection:      util.Env("MONGO_DB_CONNECTION_STRING"),
		MongoDBName:            util.Env("MONGO_DB_NAME", "talk2robots"),
	}

	err = dataDogClient.Count("main.start", 1, []string{"env:" + config.CONFIG.Environment}, 1)
	if err != nil {
		log.Errorf("error sending metric: %v", err)
	}
	if config.CONFIG.Environment == "production" {
		log.SetFormatter(&log.JSONFormatter{
			DisableTimestamp: true,
		})
	} else {
		log.SetFormatter(&log.TextFormatter{
			FullTimestamp: true,
			DisableColors: false,
		})
		log.SetLevel(log.TraceLevel)
	}

	redis.RedisClient = redis.NewClient(config.CONFIG.Redis)
	mongo.MongoDBClient = mongo.NewClient(config.CONFIG.MongoDBConnection)

	// create and setup main telegram bot
	var telegramBot *telegram.Bot
	if config.CONFIG.TelegramBotToken != "" {
		telegramBot, err = telegram.NewBot(config.CONFIG)
		if err != nil {
			log.Fatalf("ERROR creating bot: %v", err)
		}

		// payments bot used for notifications
		payments.PaymentsBot = telegramBot.Bot
	}

	// create system bot for alerts, etc
	var systemBot *telegram.Bot
	if env == "production" {
		systemBot, err = telegram.NewSystemBot(config.CONFIG)
		if err != nil {
			log.Fatalf("ERROR creating system bot: %v", err)
		}
	} else {
		systemBot = telegram.NewStubSystemBot(config.CONFIG)
	}

	rtr := router.New()
	rtr.GET("/", func(ctx *fasthttp.RequestCtx) {
		ctx.Redirect(config.CONFIG.BotUrl, fasthttp.StatusFound)
	})
	rtr.GET("/health", func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		_, _ = ctx.WriteString("❤️ from robots")
	})
	rtr.GET("/miniapp", func(ctx *fasthttp.RequestCtx) {
		ctx.Redirect("https://t.me/gienjibot?start=s=miniapp", fasthttp.StatusFound)
	})

	// stripe webhook
	stripe.Key = config.CONFIG.StripeToken
	stripe.SetAppInfo(&stripe.AppInfo{
		Name:    "talk2robots",
		Version: "0.0.1",
		URL:     config.CONFIG.BotUrl,
	})
	rtr.POST(fmt.Sprintf("/stripe_%s", config.CONFIG.StripeEndpointSuffix), payments.StripeWebhook)

	// slack bot setup
	var slackBot *slack.Bot
	if config.CONFIG.SlackBotToken != "" {
		slackBot, err = slack.NewBot(rtr, config.CONFIG)
		if err != nil {
			log.Fatalf("ERROR creating slack bot: %v", err)
		}
		payments.PaymentsSlackClient = slackBot.Client
	}

	// run onstart worker once
	onstart.Run(config.CONFIG)

	// create status worker
	status.WORKER = workers.NewWorker(telegramBot.API, systemBot.Bot, config.CONFIG, config.CONFIG.StatusWorkerInterval, status.Run, false)
	go status.WORKER.Start()

	// create usage clearing worker
	clearusage.WORKER = workers.NewWorker(telegramBot.API, systemBot.Bot, config.CONFIG, time.Hour*23, clearusage.Run, true)
	go clearusage.WORKER.Start()

	go TearDown(sigs, done, slackBot, telegramBot, systemBot, status.WORKER, clearusage.WORKER)

	telegramBot.Server.Handler = fasthttp.TimeoutHandler(func(ctx *fasthttp.RequestCtx) {
		switch string(ctx.Path()) {
		case "/bot":
			telegramBot.Handler(ctx)
		case "/sbot":
			systemBot.Handler(ctx)
		default:
			rtr.Handler(ctx)
		}
	}, time.Second*30, "Request timeout")
	go func() {
		err = telegramBot.Server.ListenAndServe(util.Env("BACKEND_LISTEN_ADDRESS"))
		util.Assert(err == nil, "ListenAndServe:", err)
	}()

	chatId, _ := strconv.ParseInt(config.CONFIG.TelegramSystemTo, 10, 64)
	successfulStartMessage := fmt.Sprintf("🤖 %s started successfully 🚀 inside %s", config.CONFIG.BotName, util.Env("POD_NAME", "unknown"))
	_, err = systemBot.Bot.SendMessage(context.Background(), tu.Message(tu.ID(chatId), successfulStartMessage))
	if err != nil {
		log.Errorf("Failed to send start message to systemBot: %s", err)
	}
	log.Info(successfulStartMessage)

	<-done
	log.Info("Done")
}

func TearDown(sigs chan os.Signal, done chan struct{}, slackBot *slack.Bot, telegramBot *telegram.Bot, systemBot *telegram.Bot, statusWorker *workers.Worker, clearUsageWorker *workers.Worker) {
	<-sigs
	exitMessage := fmt.Sprintf("🤖 %s bids farewell ❌ inside %s", config.CONFIG.BotName, util.Env("POD_NAME", "unknown"))
	log.Info(exitMessage)
	chatId, _ := strconv.ParseInt(config.CONFIG.TelegramSystemTo, 10, 64)
	systemBot.Bot.SendMessage(context.Background(), tu.Message(tu.ID(chatId), exitMessage))
	statusWorker.StopWorker()
	clearUsageWorker.StopWorker()
	err := telegramBot.BotHandler.Stop()
	if err != nil {
		log.Errorf("TearDown: BotHandler.Stop for bot: %v", err)
	}
	err = telegramBot.Stop()
	if err != nil {
		log.Errorf("TearDown: Stop for bot: %v", err)
	}
	err = systemBot.BotHandler.Stop()
	if err != nil {
		log.Errorf("TearDown: BotHandler.Stop for system bot: %v", err)
	}
	err = systemBot.Stop()
	if err != nil {
		log.Errorf("TearDown: Stop for system bot: %v", err)
	}

	err = mongo.MongoDBClient.Disconnect(context.Background())
	if err != nil {
		log.Errorf("TearDown: Disconnecting from MongoDB: %v", err)
	}
	done <- struct{}{}
}
