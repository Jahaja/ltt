package loadtest

import (
	"context"
	"errors"
)

type storageContextKeyType int

var storageContextKey storageContextKeyType

// Local storage for each user instance
type Storage struct {
	data map[string]interface{}
}

var ErrInvalidStorageValue = errors.New("invalid storage value")

func (s *Storage) GetInt(key string) (int, error) {
	if v, ok := s.Get(key).(int); ok {
		return v, nil
	}

	return 0, ErrInvalidStorageValue
}

func (s *Storage) GetInt64(key string) (int64, error) {
	if v, ok := s.Get(key).(int64); ok {
		return v, nil
	}

	return 0, ErrInvalidStorageValue
}

func (s *Storage) GetString(key string) (string, error) {
	if v, ok := s.Get(key).(string); ok {
		return v, nil
	}

	return "", ErrInvalidStorageValue
}

func (s *Storage) Get(key string) interface{} {
	v, _ := s.data[key]
	return v
}

func (s *Storage) Set(key string, value interface{}) {
	s.data[key] = value
}

func NewStorageContext(ctx context.Context, s *Storage) context.Context {
	return context.WithValue(ctx, storageContextKey, s)
}

func StorageFromContext(ctx context.Context) *Storage {
	if s, ok := ctx.Value(storageContextKey).(*Storage); ok {
		return s
	}

	return nil
}

func NewStorage() *Storage {
	return &Storage{data: make(map[string]interface{})}
}
