package lib

import (
	"context"
	"strings"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/models"

	log "github.com/sirupsen/logrus"
)

type ModeName string

const (
	Grammar    ModeName = "grammar"
	Teacher    ModeName = "teacher"
	Emili      ModeName = "emili"
	Vasilisa   ModeName = "vasilisa"
	ChatGPT    ModeName = "chatgpt"
	VoiceGPT   ModeName = "voicegpt"
	Transcribe ModeName = "transcribe"
	Summarize  ModeName = "summarize"
	Translate  ModeName = "translate"
)

func SaveMode(chatID string, topicID string, mode ModeName, params string) {
	log.Infof("Setting mode to %s for chat %s with params %s", mode, chatID, params)
	key := chatID + ":mode"
	redis.RedisClient.Set(context.Background(), key, string(mode)+","+params, 0)
	if topicID != "" && topicID != "0" {
		key = chatID + ":" + topicID + ":mode"
		redis.RedisClient.Set(context.Background(), key, string(mode)+","+params, 0)
	}
}

func GetMode(chatID string, topicID string) (mode ModeName, params string) {
	modeAndParams := ""
	if topicID != "" && topicID != "0" {
		modeAndParams, _ = redis.RedisClient.Get(context.Background(), chatID+":"+topicID+":mode").Result()
	}

	// if no mode set for topic, check for general chat mode
	if modeAndParams == "" {
		modeAndParams, _ = redis.RedisClient.Get(context.Background(), chatID+":mode").Result()
	}

	// still no mode set, return+set default
	if modeAndParams == "" {
		log.Infof("No mode set for chat %s (topic: %s), setting to default", chatID, topicID)
		SaveMode(chatID, topicID, ChatGPT, "")
		return ChatGPT, ""
	}
	modeAndParamsArray := strings.Split(modeAndParams, ",")
	modeString := modeAndParamsArray[0]

	if len(modeAndParamsArray) > 1 {
		params = modeAndParamsArray[1]
	}

	// once ChatGPT mode was called "free"
	if modeString == "free" {
		return ChatGPT, ""
	}
	return ModeName(modeString), params
}

var grammarSeed = []models.Message{
	{
		Role:    "system",
		Content: "You will correct grammar only. You keep the language, style and voice of the original text. Your response will only include the updated text. If no corrections needed, you will just say exacly - [correct]",
	},
	{
		Role:    "user",
		Content: "Waass up bro? can you hepl me wiht my homework",
	},
	{
		Role:    "assistant",
		Content: `What's up, bro? Can you help me with my homework?`,
	},
	{
		Role:    "user",
		Content: "בדיקה של דיקדוק, האם זה נכון או לא?",
	},
	{
		Role:    "assistant",
		Content: `[correct]`,
	},
	{
		Role:    "user",
		Content: "Чё как другг? Можешь мен помочь с домашкой?",
	},
	{
		Role:    "assistant",
		Content: `Как дела, друг? Можешь мне помочь с домашкой?`,
	},
	{
		Role:    "user",
		Content: "What's up, bro?",
	},
	{
		Role:    "assistant",
		Content: `[correct]`,
	},
}

var teacherSeed = []models.Message{
	{
		Role:    "system",
		Content: "You are a helpful teacher. You will correct grammar of user and explain which mistakes were made. You keep the language, style and voice of the original text. You will make it in a concise way first replying with a block of corrected message. Then you will explain why it is wrong. If no corrections required, you will just reply with - [correct]",
	},
	{
		Role:    "user",
		Content: "Waass up bro? can you hepl me wiht my homework",
	},
	{
		Role: "assistant",
		Content: `What's up, bro? Can you help me with my homework?
	Explanation:
	Here are the corrections made to the message:
	
	"Waass" is misspelled; it should be "What's."
	"hepl" is misspelled; it should be "help."
	The message ends with a question, so it should end with a question mark.
	Additionally, while the use of "bro" is informal, it may be appropriate in some contexts, such as talking with a close friend. However, in more formal or professional situations, it may be better to use a more formal greeting.`,
	},
}

var emiliSeed = []models.Message{
	{
		Role:    "system",
		Content: "את מורה אדיבה ומועילה אמילי. אתה חברה טובה. את תתקני את הדקדוק של המשתמש ותסבירי אילו טעויות נעשו. את תעשי את זה בצורה תמציתית ותשיב תחילה עם בלוק של הודעה מתוקנת. אז תסבירי מה לא היה בסדר. אם לא בוצעו תיקונים, פשוט תגידי בקצרה שההודעה נכונה.",
	},
	{
		Role:    "user",
		Content: "מהנישמה כלטוב? יכלו להזור לי עם שיעורי בית?",
	},
	{
		Role: "assistant",
		Content: `מה נשמע? האם תוכלו לעזור לי עם שיעורי בית?
		התיקון:
		שימוש במילה "נשמע" במקום "מהנישמה".
השתמשות בשאלה ישירה וסדר המילים הנכון: "האם" לפני "תוכלו" ולאחר מכן הנושא "אתם", ולבסוף הפועל "לעזור" והמקום "לי".
השימוש במילת "הזור" במקום "לעזור" וזו טעות בשימוש של המילה.
	`,
	},
}

var vasilisaSeed = []models.Message{
	{
		Role:    "system",
		Content: "Вы полезная и добрая учительница Василиса. Вы хорошая подруга. Вы исправите грамматику пользователя и объясните, какие ошибки были допущены. Вы сделаете это лаконично, сначала ответив блоком исправленного сообщения. Потом объясните, что было исправлено. Если никаких исправлений сделано не было, вы просто кратко скажете, что сообщение верное.",
	},
	{
		Role:    "user",
		Content: "Чё как другг? Можешь мен помочь с домашкой?",
	},
	{
		Role: "assistant",
		Content: `Как дела, друг? Можешь мне помочь с домашним заданием?
		Объяснение:
		Вот исправления, сделанные в сообщении:

	1. Вместо "чё как" лучше использовать "как дела" - это более формальное и правильное обращение.
	2. Вместо "другг" нужно использовать "друг" - это правильное написание. Кроме того обращение выделено запятой.
    3. Вместо "мен" нужно использовать "мне" - это правильный падеж.
    4. "Домашка" - это не совсем корректное сокращение слова "домашнее задание". Лучше использовать полное название, чтобы было понятнее, о чем идет речь.`,
	},
}

var chatGPTSeed = []models.Message{}

var summarizeSeed = []models.Message{
	{
		Role:    "system",
		Content: "You will summarize a conversation provided as a transcript. Summarize the conversation in a few paragraphs. Structure should include context, participants/actors (<@mentions>), and the main points of the conversation. If there were major decisions and/or action items, you will add them as a bullet list. Keep the mentions in slack format, i.e. <@U12345678>.",
	},
}

func GetSeedDataAndPrimer(mode ModeName) ([]models.Message, string) {
	var seedData []models.Message
	userMessagePrimer := "Text to correct:\n" //default
	switch mode {
	case Grammar:
		seedData = grammarSeed
	case Teacher:
		seedData = teacherSeed
	case Emili:
		seedData = emiliSeed
		userMessagePrimer = "טקסט לתיקון:\n"
	case Vasilisa:
		seedData = vasilisaSeed
		userMessagePrimer = "Текст для исправления:\n"
	case ChatGPT:
		seedData = chatGPTSeed
		userMessagePrimer = ""
	case Summarize:
		seedData = summarizeSeed
		userMessagePrimer = ""
	default:
		seedData = chatGPTSeed
		userMessagePrimer = ""
	}
	return seedData, userMessagePrimer
}
