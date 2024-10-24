package mongo

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/models"
	"time"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// Client is a mongo client
type Client struct {
	*mongo.Client
}

const (
	// MongoUserCollection is the name of the collection that stores user data
	MongoUserCollection = "users"

	// MongoUserThreadCollection is the name of the collection that stores user thread data
	MongoUserThreadCollection = "user_threads"
)

type MongoClient interface {
	Disconnect(ctx context.Context) error
	GetUser(ctx context.Context) (*models.MongoUser, error)
	GetUserIds(ctx context.Context, page int, pageSize int) ([]string, error)
	GetUsersCount(ctx context.Context) (int64, error)
	GetUsersCountForSubscription(ctx context.Context, subscription string) (int64, error)
	GetUserIdsUsedSince(ctx context.Context, since time.Time, page int, pageSize int) ([]string, error)
	GetUserIdsNotifiedBefore(ctx context.Context, before time.Time, page int, pageSize int) ([]string, error)
	MigrateUsersToSubscription(ctx context.Context, from, to string) error
	Ping(ctx context.Context, rp *readpref.ReadPref) error
	UpdateUserContacts(ctx context.Context, name, phone, email string) error
	UpdateUserSubscription(ctx context.Context, subscription models.MongoSubscription) error
	UpdateUserUsage(ctx context.Context, userTotalCost float64) error
	UpdateUserStripeCustomerId(ctx context.Context, stripeCustomerId string) error
	UpdateUsersNotified(ctx context.Context, userIds []string) error

	// threads
	AddToUserThread(ctx context.Context, thread *models.MongoUserThread, message *models.MultimodalMessage, userInfo string) error
	DeleteUserThread(ctx context.Context) error
	GetUserThread(ctx context.Context) (*models.MongoUserThread, error)
	UpdateUserThread(ctx context.Context, thread *models.MongoUserThread) error

	UpdateUserSourceModeLanguage(ctx context.Context, source string, mode string, language string) error
}

var MongoDBClient MongoClient

// NewClient creates a new mongo client
func NewClient(connection string) *Client {
	return &Client{
		Client: mustConnect(connection),
	}
}

// mustConnect connects to mongo and panics on error
func mustConnect(connection string) *mongo.Client {
	client, err := mongo.NewClient(options.Client().ApplyURI(connection).SetMaxConnecting(25))
	if err != nil {
		logrus.WithError(err).Panic("failed to create mongo client")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = client.Connect(ctx)
	if err != nil {
		logrus.WithError(err).Panic("failed to connect to mongo")
	}

	return client
}

func (c *Client) GetUser(ctx context.Context) (*models.MongoUser, error) {
	userId := ctx.Value(models.UserContext{}).(string)
	collection := c.Database(config.CONFIG.MongoDBName).Collection(MongoUserCollection)
	filter := bson.M{"_id": userId}
	var user models.MongoUser
	err := collection.FindOne(ctx, filter).Decode(&user)
	if err != nil {
		return nil, fmt.Errorf("GetUser: failed to find user: %w", err)
	}
	return &user, nil
}

func (c *Client) GetUsersCount(ctx context.Context) (int64, error) {
	collection := c.Database(config.CONFIG.MongoDBName).Collection(MongoUserCollection)
	count, err := collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("GetUsersCount: failed to get users count: %w", err)
	}
	return count, nil
}

func (c *Client) GetUsersCountForSubscription(ctx context.Context, subscription string) (int64, error) {
	collection := c.Database(config.CONFIG.MongoDBName).Collection(MongoUserCollection)
	count, err := collection.CountDocuments(ctx, bson.M{"subscription.name": subscription})
	if err != nil {
		return 0, fmt.Errorf("GetUsersCountForSubscription: failed to get users count: %w", err)
	}
	return count, nil
}

func (c *Client) UpdateUserSubscription(ctx context.Context, subscription models.MongoSubscription) error {
	userId := ctx.Value(models.UserContext{}).(string)
	collection := c.Database(config.CONFIG.MongoDBName).Collection(MongoUserCollection)

	filter := bson.M{"_id": userId}
	var update bson.M

	if subscription.Name != "" {
		update = bson.M{
			"$set": bson.M{
				"subscription":      subscription,
				"subscription_date": time.Now().UTC().Format("2006-01-02"),
			},
		}
	} else {
		return nil
	}

	options := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(ctx, filter, update, options)
	return err
}

func (c *Client) UpdateUserUsage(ctx context.Context, newUsage float64) error {
	userId := ctx.Value(models.UserContext{}).(string)
	collection := c.Database(config.CONFIG.MongoDBName).Collection(MongoUserCollection)

	filter := bson.M{"_id": userId}
	var update bson.M

	if newUsage != 0 {
		update = bson.M{
			"$set": bson.M{
				"usage":        newUsage,
				"last_used_at": time.Now().UTC().Format("2006-01-02T15:04:05"),
			},
		}
	} else {
		return nil
	}

	options := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(ctx, filter, update, options)
	return err
}

func (c *Client) UpdateUsersNotified(ctx context.Context, userIds []string) error {
	if len(userIds) == 0 {
		return nil
	}

	collection := c.Database(config.CONFIG.MongoDBName).Collection(MongoUserCollection)

	filter := bson.M{"_id": bson.M{"$in": userIds}}
	update := bson.M{
		"$set": bson.M{
			"last_notified_at": time.Now().UTC().Format("2006-01-02T15:04:05"),
		},
	}

	options := options.Update().SetUpsert(true)
	_, err := collection.UpdateMany(ctx, filter, update, options)
	return err
}

func (c *Client) UpdateUserContacts(ctx context.Context, name, phone, email string) error {
	userId := ctx.Value(models.UserContext{}).(string)
	collection := c.Database(config.CONFIG.MongoDBName).Collection(MongoUserCollection)

	filter := bson.M{"_id": userId}
	update := bson.M{
		"$set": bson.M{
			"name":  name,
			"phone": phone,
			"email": email,
		},
	}

	options := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(ctx, filter, update, options)
	return err
}

func (c *Client) MigrateUsersToSubscription(ctx context.Context, from, to string) error {
	collection := c.Database(config.CONFIG.MongoDBName).Collection(MongoUserCollection)
	filter := bson.M{"subscription.name": from}

	var update bson.M
	if to != "" {
		toName := models.MongoSubscriptionName(to)
		toSubscription := models.Subscriptions[toName]
		update = bson.M{
			"$set": bson.M{
				"subscription": toSubscription,
			},
		}
	} else {
		return nil
	}

	_, err := collection.UpdateMany(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("MigrateUsersToSubscription: failed to migrate users to subscription: %w", err)
	}
	return nil
}

func (c *Client) UpdateUserStripeCustomerId(ctx context.Context, stripeCustomerId string) error {
	userId := ctx.Value(models.UserContext{}).(string)
	collection := c.Database(config.CONFIG.MongoDBName).Collection(MongoUserCollection)

	filter := bson.M{"_id": userId}
	update := bson.M{
		"$set": bson.M{
			"stripe_customer_id": stripeCustomerId,
		},
	}

	options := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(ctx, filter, update, options)
	return err
}

func (c *Client) GetUserIds(ctx context.Context, page int, pageSize int) ([]string, error) {
	collection := c.Database(config.CONFIG.MongoDBName).Collection(MongoUserCollection)
	findOptions := options.Find()
	findOptions.SetSkip(int64(page * pageSize))
	findOptions.SetLimit(int64(pageSize))
	cursor, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return nil, fmt.Errorf("GetUserIds: failed to find users: %w", err)
	}
	defer cursor.Close(ctx)

	var userIds []string
	for cursor.Next(ctx) {
		var user models.MongoUser
		err := cursor.Decode(&user)
		if err != nil {
			return nil, fmt.Errorf("GetUserIds: failed to decode user: %w", err)
		}
		userIds = append(userIds, user.ID)
	}
	return userIds, nil
}

func (c *Client) GetUserIdsUsedSince(ctx context.Context, since time.Time, page int, pageSize int) ([]string, error) {
	collection := c.Database(config.CONFIG.MongoDBName).Collection(MongoUserCollection)
	findOptions := options.Find()
	findOptions.SetSkip(int64(page * pageSize))
	findOptions.SetLimit(int64(pageSize))
	// last_used_at or subscription_date is gte than since
	cursor, err := collection.Find(ctx, bson.M{"$or": []bson.M{
		{"last_used_at": bson.M{"$gte": since.Format("2006-01-02T15:04:05")}},
		{"subscription_date": bson.M{"$gte": since.Format("2006-01-02")}},
	}}, findOptions)
	if err != nil {
		return nil, fmt.Errorf("GetUserIdsSince: failed to find users: %w", err)
	}
	defer cursor.Close(ctx)

	var userIds []string
	for cursor.Next(ctx) {
		var user models.MongoUser
		err := cursor.Decode(&user)
		if err != nil {
			return nil, fmt.Errorf("GetUserIdsSince: failed to decode user: %w", err)
		}
		userIds = append(userIds, user.ID)
	}
	return userIds, nil
}

func (c *Client) GetUserIdsNotifiedBefore(ctx context.Context, before time.Time, page int, pageSize int) ([]string, error) {
	collection := c.Database(config.CONFIG.MongoDBName).Collection(MongoUserCollection)
	findOptions := options.Find()
	findOptions.SetSkip(int64(page * pageSize))
	findOptions.SetLimit(int64(pageSize))
	// where last_notified_at is lte than before or empty/nil
	cursor, err := collection.Find(ctx, bson.M{"$or": []bson.M{
		{"last_notified_at": bson.M{"$lte": before.Format("2006-01-02T15:04:05")}},
		{"last_notified_at": nil},
		{"last_notified_at": ""},
	}}, findOptions)
	if err != nil {
		return nil, fmt.Errorf("GetUserIdsNotifiedBefore: failed to find users: %w", err)
	}
	defer cursor.Close(ctx)

	var userIds []string
	for cursor.Next(ctx) {
		var user models.MongoUser
		err := cursor.Decode(&user)
		if err != nil {
			return nil, fmt.Errorf("GetUserIdsNotifiedBefore: failed to decode user: %w", err)
		}
		userIds = append(userIds, user.ID)
	}
	return userIds, nil
}

func (c *Client) GetUserThread(ctx context.Context) (*models.MongoUserThread, error) {
	userId := ctx.Value(models.UserContext{}).(string)
	if userId == "" {
		return nil, fmt.Errorf("GetUserThread: user ID is required")
	}
	collection := c.Database(config.CONFIG.MongoDBName).Collection(MongoUserThreadCollection)
	filter := bson.M{"user_id": userId}
	var userThread models.MongoUserThread
	err := collection.FindOne(ctx, filter).Decode(&userThread)
	if err != nil {
		return nil, fmt.Errorf("GetUserThread: failed to find user thread: %w", err)
	}
	return &userThread, nil
}

func (c *Client) UpdateUserThread(ctx context.Context, thread *models.MongoUserThread) error {
	userId := ctx.Value(models.UserContext{}).(string)
	if thread.UserId == "" {
		thread.UserId = userId
	}

	if thread == nil || thread.ThreadJson == "" {
		return nil
	}

	if thread.UserId == "" {
		return fmt.Errorf("UpdateUserThread: user ID is required")
	}

	collection := c.Database(config.CONFIG.MongoDBName).Collection(MongoUserThreadCollection)
	filter := bson.M{"user_id": thread.UserId}
	update := bson.M{
		"$set": bson.M{
			"updated_at":  time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
			"thread_json": thread.ThreadJson,
		},
	}

	// only update created_at if it doesn't exist
	if thread.CreatedAt != "" {
		update["$set"].(bson.M)["created_at"] = thread.CreatedAt
	}

	options := options.Update().SetUpsert(true)
	_, err := collection.UpdateMany(ctx, filter, update, options)
	return err
}

func (c *Client) DeleteUserThread(ctx context.Context) error {
	userId := ctx.Value(models.UserContext{}).(string)
	if userId == "" {
		return fmt.Errorf("DeleteUserThread: user ID is required")
	}
	collection := c.Database(config.CONFIG.MongoDBName).Collection(MongoUserThreadCollection)
	filter := bson.M{"user_id": userId}
	_, err := collection.DeleteMany(ctx, filter)
	return err
}

func (c *Client) AddToUserThread(ctx context.Context, thread *models.MongoUserThread, message *models.MultimodalMessage, userInfo string) error {
	userId := ctx.Value(models.UserContext{}).(string)
	if message == nil {
		return fmt.Errorf("AddToUserThread: message is required")
	}

	var messages []models.MultimodalMessage
	if thread == nil {
		// fetch the thread
		dbThread, err := c.GetUserThread(ctx)
		if err == nil {
			thread = dbThread
		} else {
			// follow up even if there is no thread
			if strings.Contains(err.Error(), "no documents in result") {
				messages = []models.MultimodalMessage{
					{
						Role:    "system",
						Content: []models.MultimodalContent{{Type: "text", Text: config.AI_INSTRUCTIONS}},
					},
				}
				if userInfo != "" {
					messages[0].Content = append(messages[0].Content, models.MultimodalContent{Type: "text", Text: userInfo})
				}
				threadBytes, err := json.Marshal(messages)
				if err != nil {
					return fmt.Errorf("AddToUserThread: failed to marshal messages: %w", err)
				}
				thread = &models.MongoUserThread{
					UserId:     userId,
					ThreadJson: string(threadBytes),
					CreatedAt:  time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
				}
			} else {
				return fmt.Errorf("AddToUserThread: failed to get user thread: %w", err)
			}
		}
	}

	err := json.Unmarshal([]byte(thread.ThreadJson), &messages)
	if err != nil {
		return fmt.Errorf("AddToUserThread: failed to unmarshal thread: %w", err)
	}

	messages = append(messages, *message)

	threadBytes, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("AddToUserThread: failed to marshal messages: %w", err)
	}
	thread.ThreadJson = string(threadBytes)

	return c.UpdateUserThread(ctx, thread)
}

func (c *Client) UpdateUserSourceModeLanguage(ctx context.Context, source string, mode string, language string) error {
	userId := ctx.Value(models.UserContext{}).(string)
	collection := c.Database(config.CONFIG.MongoDBName).Collection(MongoUserCollection)

	filter := bson.M{"_id": userId}
	update := bson.M{
		"$set": bson.M{
			"source":   sanitize(source),
			"mode":     sanitize(mode),
			"language": sanitize(language),
		},
	}

	options := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(ctx, filter, update, options)
	return err
}

func sanitize(s string) string {
	// use regex to keep only english letters and digits
	return regexp.MustCompile("[^a-zA-Z0-9]+").ReplaceAllString(s, "")
}
