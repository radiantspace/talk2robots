// Run every month to clear the monthly usage of the users
package clearusage

import (
	"context"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/db/redis"
	"talk2robots/m/v2/app/lib"
	"talk2robots/m/v2/app/workers"

	log "github.com/sirupsen/logrus"
)

var WORKER *workers.Worker

func Run() {
	clearByWildcard(lib.UserTotalCostKey("*"))
	clearByWildcard(lib.UserTotalAudioMinutesKey("*"))
	clearByWildcard(lib.UserTotalTokensKey("*"))
	log.Info("finished usage clearing")
}

func clearByWildcard(wildcard string) {
	log.Infof("clearing %s..", wildcard)
	keys := redis.RedisClient.Keys(context.Background(), wildcard)
	config.CONFIG.DataDogClient.Gauge("clear_usage_worker.keys", float64(len(keys.Val())), []string{"wildcard:" + wildcard}, 1)
	log.Infof("clearing %s, keys count: %d", wildcard, len(keys.Val()))

	if len(keys.Val()) == 0 {
		log.Infof("no keys to clear for %s", wildcard)
		return
	}
	cmd := redis.RedisClient.Del(context.Background(), keys.Val()...)
	if cmd.Err() != nil {
		log.Errorf("failed to clear %s: %s", wildcard, cmd.Err())
		return
	}
	count, _ := cmd.Result()
	log.Infof("cleared %d keys for %s", count, wildcard)
}
