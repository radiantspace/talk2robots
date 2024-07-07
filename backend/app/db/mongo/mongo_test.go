package mongo

import (
	"context"
	"runtime"
	"strings"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/models"
	"testing"

	"github.com/tryvium-travels/memongo"
	"go.mongodb.org/mongo-driver/bson"
)

var MockMongoServer *memongo.Server

func TestMain(m *testing.M) {
	opts := &memongo.Options{
		MongoVersion: "6.0.13",
	}
	if runtime.GOARCH == "arm64" {
		if runtime.GOOS == "darwin" {
			// Only set the custom url as workaround for arm64 macs
			opts.DownloadURL = "https://fastdl.mongodb.org/osx/mongodb-macos-x86_64-6.0.13.tgz"
		}
	}

	MockMongoServer, _ = memongo.Start("6.0.13")
	defer MockMongoServer.Stop()
	m.Run()
}

func TestGetUser(t *testing.T) {
	user := `{"_id":"292902807","subscription":"{
    \"name\": \"basic\",
    \"maximum_usage\": 9.99
}","subscription_date":"2024-07-07","last_used_at":"2024-07-07T12:46:05","usage":0.2007462}`
	uri := MockMongoServer.URIWithRandomDB()

	// parse db name from uri
	dbName := uri[strings.LastIndex(uri, "/")+1:]
	config.CONFIG = &config.Config{
		MongoDBName: dbName,
	}

	MockMongoDBClient := NewClient(uri)

	userBson := bson.M{}
	err := bson.UnmarshalExtJSON([]byte(user), true, &userBson)
	if err != nil {
		t.Fatalf("error unmarshalling user: %v", err)
	}

	_, err = MockMongoDBClient.Database(dbName).Collection(MongoUserCollection).InsertOne(context.Background(), userBson)

	if err != nil {
		t.Fatalf("error inserting user: %v", err)
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, models.UserContext{}, "292902807")
	userFromDB, err := MockMongoDBClient.GetUser(ctx)
	if err != nil {
		t.Fatalf("error getting user: %v", err)
	}

	if userFromDB.ID != "292902807" {
		t.Fatalf("expected user id to be 292902807, got %s", userFromDB.ID)
	}

	if userFromDB.SubscriptionType.Name != "free+" {
		t.Fatalf("expected user subscription to be free+, got %s", userFromDB.SubscriptionType.Name)
	}

	if userFromDB.Usage != 0.2004772 {
		t.Fatalf("expected user usage to be 0.2004772, got %f", userFromDB.Usage)
	}
}

func TestAddToUserThread_NewThread(t *testing.T) {
	uri := MockMongoServer.URIWithRandomDB()

	// parse db name from uri
	dbName := uri[strings.LastIndex(uri, "/")+1:]
	config.CONFIG = &config.Config{
		MongoDBName: dbName,
	}
	MockMongoDBClient := NewClient(uri)

	// setup
	_, err := MockMongoDBClient.Database(dbName).Collection(MongoUserCollection).InsertOne(context.Background(), bson.M{
		"_id":                "19291",
		"subscription":       bson.M{"name": "basic", "maximum_usage": 9.99},
		"subscription_at":    "2024-04-30T07:50:07.210Z",
		"updated_at":         "2024-04-31T20:41:41.400Z",
		"usage":              1.230651,
		"stripe_customer_id": "cus_QB9nhUlj5amdIV",
		"email":              "sdfg@fg.h",
		"name":               "alifecoachbot:19291:radiantspace",
		"phone":              "",
		"followup_at":        "",
	})

	if err != nil {
		t.Fatalf("error inserting user: %v", err)
	}

	// test
	ctx := context.Background()
	ctx = context.WithValue(ctx, models.UserContext{}, "19291")
	err = MockMongoDBClient.AddToUserThread(ctx, nil, &models.MultimodalMessage{Role: "user", Content: []models.MultimodalContent{{Type: "text", Text: "some message"}}}, "some user info")
	if err != nil {
		t.Fatalf("error adding to user thread: %v", err)
	}

	// verify
	userThread, err := MockMongoDBClient.GetUserThread(ctx)
	if err != nil {
		t.Fatalf("error getting user thread: %v", err)
	}

	if userThread.ThreadJson == "" || userThread.ThreadJson == "[]" {
		t.Fatalf("expected user thread to have messages, got %s", userThread.ThreadJson)
	}

	if userThread.UserId != "19291" {
		t.Fatalf("expected user thread user id to be 19291, got %s", userThread.UserId)
	}

	if !strings.Contains(userThread.ThreadJson, "some message") {
		t.Fatalf("expected user thread to contain message, got %s", userThread.ThreadJson)
	}

	if !strings.Contains(userThread.ThreadJson, "some user info") {
		t.Fatalf("expected user thread to contain user info, got %s", userThread.ThreadJson)
	}

	if strings.Contains(userThread.ThreadJson, "image_url") {
		t.Fatalf("expected user thread to not contain image url, got %s", userThread.ThreadJson)
	}

	// expect created_at to be set
	if !strings.Contains(userThread.CreatedAt, "T") && !strings.Contains(userThread.CreatedAt, "Z") {
		t.Fatalf("expected user thread to contain created_at, got %s", userThread.ThreadJson)
	}
}

func TestAddToUserThread_ExistingThreadWOPointer(t *testing.T) {
	uri := MockMongoServer.URIWithRandomDB()

	// parse db name from uri
	dbName := uri[strings.LastIndex(uri, "/")+1:]
	config.CONFIG = &config.Config{
		MongoDBName: dbName,
	}
	MockMongoDBClient := NewClient(uri)

	// setup
	_, err := MockMongoDBClient.Database(dbName).Collection(MongoUserCollection).InsertOne(context.Background(), bson.M{
		"_id":                "19291",
		"subscription":       bson.M{"name": "basic", "maximum_usage": 9.99},
		"subscription_at":    "2024-04-30T07:50:07.210Z",
		"updated_at":         "2024-04-31T20:41:41.400Z",
		"usage":              1.230651,
		"stripe_customer_id": "cus_QB9nhUlj5amdIV",
		"email":              "sdfg@fg.h",
		"name":               "alifecoachbot:19291:radiantspace",
		"phone":              "",
		"followup_at":        "",
	})

	if err != nil {
		t.Fatalf("error inserting user: %v", err)
	}

	// test
	ctx := context.Background()
	ctx = context.WithValue(ctx, models.UserContext{}, "19291")
	err = MockMongoDBClient.UpdateUserThread(ctx, &models.MongoUserThread{UserId: "19291", ThreadJson: `[{"role":"system","content":[{"type":"text","text":"some system message"}]}]`})
	if err != nil {
		t.Fatalf("error updating user thread: %v", err)
	}

	err = MockMongoDBClient.AddToUserThread(ctx, nil, &models.MultimodalMessage{Role: "user", Content: []models.MultimodalContent{{Type: "text", Text: "some message"}}}, "some user info")
	if err != nil {
		t.Fatalf("error adding to user thread: %v", err)
	}

	// verify
	userThread, err := MockMongoDBClient.GetUserThread(ctx)
	if err != nil {
		t.Fatalf("error getting user thread: %v", err)
	}

	// doesn't update created_at
	if strings.Contains(userThread.CreatedAt, "T") || strings.Contains(userThread.CreatedAt, "Z") {
		t.Fatalf("expected user thread to not contain created_at, got %s", userThread.ThreadJson)
	}

	// keeps existing messages
	if !strings.Contains(userThread.ThreadJson, "some system message") {
		t.Fatalf("expected user thread to contain system message, got %s", userThread.ThreadJson)
	}

	// adds message to existing thread
	if !strings.Contains(userThread.ThreadJson, "some message") {
		t.Fatalf("expected user thread to contain new message, got %s", userThread.ThreadJson)
	}

	// doesn't add user info to existing thread
	if strings.Contains(userThread.ThreadJson, "some user info") {
		t.Fatalf("expected user thread to not contain user info, got %s", userThread.ThreadJson)
	}
}
