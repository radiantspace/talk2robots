package mongo

import (
	"context"
	"fmt"
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

type MongoClient interface {
	Disconnect(ctx context.Context) error
	GetUser(ctx context.Context) (*models.MongoUser, error)
	GetUsersCount(ctx context.Context) (int64, error)
	GetUsersCountForSubscription(ctx context.Context, subscription string) (int64, error)
	MigrateUsersToSubscription(ctx context.Context, from, to string) error
	Ping(ctx context.Context, rp *readpref.ReadPref) error
	UpdateUserContacts(ctx context.Context, name, phone, email string) error
	UpdateUserSubscription(ctx context.Context, subscription models.MongoSubscription) error
	UpdateUserUsage(ctx context.Context, userTotalCost float64) error
	UpdateUserStripeCustomerId(ctx context.Context, stripeCustomerId string) error
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
	collection := c.Database(config.CONFIG.MongoDBName).Collection("users")
	filter := bson.M{"_id": userId}
	var user models.MongoUser
	err := collection.FindOne(ctx, filter).Decode(&user)
	if err != nil {
		return nil, fmt.Errorf("GetUser: failed to find user: %w", err)
	}
	return &user, nil
}

func (c *Client) GetUsersCount(ctx context.Context) (int64, error) {
	collection := c.Database(config.CONFIG.MongoDBName).Collection("users")
	count, err := collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("GetUsersCount: failed to get users count: %w", err)
	}
	return count, nil
}

func (c *Client) GetUsersCountForSubscription(ctx context.Context, subscription string) (int64, error) {
	collection := c.Database(config.CONFIG.MongoDBName).Collection("users")
	count, err := collection.CountDocuments(ctx, bson.M{"subscription.name": subscription})
	if err != nil {
		return 0, fmt.Errorf("GetUsersCountForSubscription: failed to get users count: %w", err)
	}
	return count, nil
}

func (c *Client) UpdateUserSubscription(ctx context.Context, subscription models.MongoSubscription) error {
	userId := ctx.Value(models.UserContext{}).(string)
	collection := c.Database(config.CONFIG.MongoDBName).Collection("users")

	filter := bson.M{"_id": userId}
	var update bson.M

	if subscription.Name != "" {
		update = bson.M{
			"$set": bson.M{
				"subscription":      subscription,
				"subscription_date": time.Now().Format("2006-01-02"),
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
	collection := c.Database(config.CONFIG.MongoDBName).Collection("users")

	filter := bson.M{"_id": userId}
	var update bson.M

	if newUsage != 0 {
		update = bson.M{
			"$set": bson.M{
				"usage": newUsage,
			},
		}
	} else {
		return nil
	}

	options := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(ctx, filter, update, options)
	return err
}

func (c *Client) UpdateUserContacts(ctx context.Context, name, phone, email string) error {
	userId := ctx.Value(models.UserContext{}).(string)
	collection := c.Database(config.CONFIG.MongoDBName).Collection("users")

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
	collection := c.Database(config.CONFIG.MongoDBName).Collection("users")
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
	collection := c.Database(config.CONFIG.MongoDBName).Collection("users")

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
