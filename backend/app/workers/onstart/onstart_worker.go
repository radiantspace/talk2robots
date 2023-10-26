// Run on start
package onstart

import (
	"context"
	"talk2robots/m/v2/app/db/mongo"

	log "github.com/sirupsen/logrus"
)

func Run() {
	// this was one time migration, but keeping it here for future reference
	// migrateFreePlus()
}

// migrate users from free_plus to free+ in mongo
func migrateFreePlus() {
	log.Info("migrating free_plus to free+..")
	err := mongo.MongoDBClient.MigrateUsersToSubscription(context.Background(), "free_plus", "free+")
	if err != nil {
		log.Errorf("failed to migrate free_plus to free+: %s", err)
		return
	}
	log.Info("finished migrating free_plus to free+")
}
