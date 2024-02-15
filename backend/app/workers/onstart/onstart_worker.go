// Run on start
package onstart

import (
	"context"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/db/mongo"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/models"
	"talk2robots/m/v2/app/openai"

	log "github.com/sirupsen/logrus"
)

var OpenAI *openai.API

func Run(cfg *config.Config) {
	OpenAI = openai.NewAPI(cfg)

	// this was one time migration, but keeping it here for future reference
	// migrateFreePlus()
	migrateAll()

	setupAssistants()
}

// migrate users from free_plus to free+ in mongo
func migrateFreePlus() {
	log.Info("[onstart] migrating free_plus to free+..")
	err := mongo.MongoDBClient.MigrateUsersToSubscription(context.Background(), "free_plus", "free+")
	if err != nil {
		log.Errorf("[onstart] failed to migrate free_plus to free+: %s", err)
		return
	}
	log.Info("[onstart] finished migrating free_plus to free+")
}

func migrateAll() {
	log.Info("[onstart] migrating all users..")

	// that is needed to update limits on all subscriptions
	for _, subscription := range models.Subscriptions {
		err := mongo.MongoDBClient.MigrateUsersToSubscription(context.Background(), string(subscription.Name), string(subscription.Name))
		if err != nil {
			log.Errorf("[onstart] failed to update limits for %s: %s", subscription.Name, err)
		}

	}
	log.Info("[onstart] finished migrating all users")
}

func setupAssistants() {
	log.Info("[onstart] setting up assistants..")

	config.CONFIG.AssistantGpt4Id = setupAssistantForModel(models.ChatGpt4Turbo)
	config.CONFIG.AssistantGpt35Id = setupAssistantForModel(models.ChatGpt35Turbo)

	log.Infof("[onstart] finished setting up assistants (gpt4: %s, gpt35: %s)", config.CONFIG.AssistantGpt4Id, config.CONFIG.AssistantGpt35Id)
}

func setupAssistantForModel(model models.Engine) string {
	var id string
	id, err := redis.RedisClient.Get(context.Background(), string(models.AssistantKeyForModel(model))).Result()
	if id != "" {
		log.Infof("[onstart] found assistant %s for %s in Redis", id, model)

		// validate if assistant exists in OpenAI
		assistant, err := OpenAI.GetAssistant(context.Background(), id)
		if err != nil {
			log.Fatalf("[onstart] failed to validate assistant %s exists: %v", id, err)
		}
		return assistant.ID
	}

	log.Infof("[onstart] no assistant value found in Redis for %s, looking up OpenAI..", model)

	assistants, err := OpenAI.ListAssistants(context.Background(), 0, "", "", "")
	if err != nil {
		log.Fatalf("[onstart] failed to list assistants: %v", err)
	}

	// find assistant by model
	for _, assistant := range assistants.Data {
		if assistant.Model == string(model) {
			id = assistant.ID
		}
	}

	if id != "" {
		log.Infof("[onstart] found assistant %s for %s, saving to Redis..", id, model)
		redis.RedisClient.Set(context.Background(), string(models.AssistantKeyForModel(model)), id, 0)
		return id
	}

	log.Infof("[onstart] no assistant found in OpenAI for %s, try to create one..", model)
	assistant, err := OpenAI.CreateAssistant(context.Background(), &models.AssistantRequest{
		Name:  "Assistant for " + string(model),
		Model: string(model),
	})
	if err != nil {
		log.Fatalf("[onstart] failed to create assistant: %v", err)
	}

	log.Infof("[onstart] created assistant %s for %s, saving to Redis..", assistant.ID, model)
	redis.RedisClient.Set(context.Background(), string(models.AssistantKeyForModel(model)), assistant.ID, 0)
	return assistant.ID
}
