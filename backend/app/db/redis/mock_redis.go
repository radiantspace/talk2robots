package redis

import (
	"context"
	"errors"
	"fmt"

	r "github.com/go-redis/redis/v8"
)

// MockRedisClient is a mock for the Redis client in the redis package.
type MockRedisClient struct {
	Client
	data map[string]interface{}
}

func NewMockRedisClient() *MockRedisClient {
	return &MockRedisClient{
		data: make(map[string]interface{}),
	}
}

func (m *MockRedisClient) IncrByFloat(ctx context.Context, key string, value float64) *r.FloatCmd {
	if v, ok := m.data[key]; ok {
		if f, ok := v.(float64); ok {
			m.data[key] = f + value
		} else {
			m.data[key] = value
		}
	} else {
		m.data[key] = value
	}
	cmd := r.NewFloatCmd(ctx)
	cmd.SetVal(m.data[key].(float64))
	return cmd
}

func (m *MockRedisClient) IncrBy(ctx context.Context, key string, value int64) *r.IntCmd {
	if v, ok := m.data[key]; ok {
		if i, ok := v.(int64); ok {
			m.data[key] = i + value
		} else {
			m.data[key] = value
		}
	} else {
		m.data[key] = value
	}
	return nil
}

func (m *MockRedisClient) Get(ctx context.Context, key string) *r.StringCmd {
	cmd := r.NewStringCmd(ctx)
	if value, ok := m.data[key]; ok {
		strValue := fmt.Sprintf("%v", value) // Convert the value to a string
		cmd.SetVal(strValue)
	} else {
		cmd.SetVal("")
		cmd.SetErr(errors.New("key not found"))
	}
	return cmd
}
