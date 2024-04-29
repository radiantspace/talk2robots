package config

import (
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
)

var CONFIG *Config

type Config struct {
	AssistantGpt4Id        string
	AssistantGpt35Id       string
	BotName                string
	BotUrl                 string
	ClaudeAPIKey           string
	DataDogClient          *statsd.Client
	Environment            string
	FireworksAPIKey        string
	MongoDBName            string
	MongoDBConnection      string
	OpenAIAPIKey           string
	Redis                  Redis
	SlackBotToken          string
	SlackSigningSecret     string
	StatusWorkerInterval   time.Duration
	StripeEndpointSecret   string
	StripeEndpointSuffix   string
	StripeToken            string
	TelegramBotToken       string
	TelegramSystemBotToken string
	TelegramSystemTo       string
	WhisperAPIEndpoint     string
}

type Redis struct {
	Host     string
	Port     string
	Password string
}
