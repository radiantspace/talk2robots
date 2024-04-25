package workers

import (
	"strconv"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"talk2robots/m/v2/app/ai"
	"talk2robots/m/v2/app/config"
)

const DAY_FOR_MONTHLY_RUNS = 1

type Worker struct {
	Interval             time.Duration
	MainBotName          string
	Monthly              bool
	AI                   *ai.API
	Run                  func()
	Stop                 chan struct{}
	SystemTelegramChatID telego.ChatID
	TelegramSystemBot    *telego.Bot
}

func NewWorker(ai *ai.API, systemBot *telego.Bot, cfg *config.Config, interval time.Duration, run func(), monthly bool) *Worker {
	chatId, _ := strconv.ParseInt(cfg.TelegramSystemTo, 10, 64)
	return &Worker{
		Interval:             interval,
		MainBotName:          cfg.BotName,
		Monthly:              monthly,
		AI:                   ai,
		Run:                  run,
		Stop:                 make(chan struct{}),
		SystemTelegramChatID: tu.ID(chatId),
		TelegramSystemBot:    systemBot,
	}
}

func (w *Worker) Start() {
	if !w.Monthly || time.Now().Day() == DAY_FOR_MONTHLY_RUNS {
		w.Run()
	}
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if !w.Monthly || time.Now().Day() == DAY_FOR_MONTHLY_RUNS {
				w.Run()
			}
		case <-w.Stop:
			return
		}
	}
}

func (w *Worker) StopWorker() {
	w.Stop <- struct{}{}
}
