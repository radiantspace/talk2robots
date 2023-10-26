package telegram

// import (
// 	"testing"
// 	"time"

// 	"github.com/mymmrac/telego"
// 	"github.com/undefinedlabs/go-mpatch"
// )

// func TestEmilyCommandMessage(t *testing.T) {
// 	now := "2023-05-31T00:00:00Z"

// 	mpatch.PatchMethod(time.Now, func() time.Time {
// 		t, _ := time.Parse(time.RFC3339, now)
// 		return t
// 	})
// 	mpatch.PatchMethod(telego.Bot.SendMessage, func(bot *telego.Bot, chatID int64, text string, options ...telego.SendMessageOption) (telego.Message, error) {
// 		// assert.Equal(t, "היי, אעזור עם הטקסטים והודעות בעברית.\n\nאגב, אני בת 17520 שעות, כלומר 730 ימים או 2.0 שנים", text)
// 		return telego.Message{}, nil
// 	}, nil)

// 	// newCommandHandler(EmiliCommand, getModeHandlerFunction(Emili, "היי, אעזור עם הטקסטים והודעות בעברית."+"\n\n"+fmt.Sprintf("אגב, אני בת %.f שעות, כלומר %.f ימים או %.1f שנים", time.Since(EMILY_BIRTHDAY).Hours(), time.Since(EMILY_BIRTHDAY).Hours()/24, time.Since(EMILY_BIRTHDAY).Hours()/24/365))),
// 	setupCommandHandlers()

// 	AllCommandHandlers.handleCommand(nil, nil, &telego.Message{
// 		Text: "/emily",
// 	})
// }
