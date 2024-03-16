package mongo

import (
	"context"
	"errors"
	"talk2robots/m/v2/app/models"
)

// MockMongoDBClient is a mock for the MongoDB client in the mongo package.
type MockMongoDBClient struct {
	MongoClient
	User models.MongoUser
}

func NewMockMongoDBClient(user models.MongoUser) *MockMongoDBClient {
	return &MockMongoDBClient{
		User: user,
	}
}

func (m *MockMongoDBClient) UpdateUserUsage(ctx context.Context, userTotalCost float64) error {
	return nil
}

func (m *MockMongoDBClient) UpdateUserSubscription(ctx context.Context, subscription models.MongoSubscription) error {
	return nil
}

func (m *MockMongoDBClient) GetUser(ctx context.Context) (*models.MongoUser, error) {
	if m.User.ID == "" {
		return &models.MongoUser{}, errors.New("user not found")
	}
	return &m.User, nil
}
