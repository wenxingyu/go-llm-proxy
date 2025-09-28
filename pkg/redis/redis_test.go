package redis

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test configuration with the provided Redis connection details
var testConfig = &Config{
	Addr:     "192.168.70.128:6379",
	Password: "myredissecret",
	DB:       0,
}

func TestNewRedis(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "valid config",
			config:  testConfig,
			wantErr: false,
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "invalid address",
			config: &Config{
				Addr:     "invalid:9999",
				Password: "test",
				DB:       0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewRedis(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				if client != nil {
					defer client.Close()
				}
			}
		})
	}
}

func TestRedis_SetAndGet(t *testing.T) {
	client, err := NewRedis(testConfig)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	t.Run("set and get string", func(t *testing.T) {
		key := "test:string"
		value := "hello world"
		ttl := 5 * time.Minute

		// Set value
		err := client.Set(ctx, key, value, ttl)
		assert.NoError(t, err)

		// Get value
		var result string
		exists, err := client.Get(ctx, key, &result)
		assert.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, value, result)
	})

	t.Run("set and get struct", func(t *testing.T) {
		key := "test:struct"
		type TestStruct struct {
			Name  string `json:"name"`
			Age   int    `json:"age"`
			Email string `json:"email"`
		}
		value := TestStruct{
			Name:  "John Doe",
			Age:   30,
			Email: "john@example.com",
		}
		ttl := 5 * time.Minute

		// Set value
		err := client.Set(ctx, key, value, ttl)
		assert.NoError(t, err)

		// Get value
		var result TestStruct
		exists, err := client.Get(ctx, key, &result)
		assert.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, value, result)
	})

	t.Run("set and get map", func(t *testing.T) {
		key := "test:map"
		value := map[string]interface{}{
			"key1": "value1",
			"key2": 123,
			"key3": true,
		}
		ttl := 5 * time.Minute

		// Set value
		err := client.Set(ctx, key, value, ttl)
		assert.NoError(t, err)

		// Get value
		var result map[string]interface{}
		exists, err := client.Get(ctx, key, &result)
		assert.NoError(t, err)
		assert.True(t, exists)

		// Check individual fields since JSON unmarshaling converts numbers to float64
		assert.Equal(t, value["key1"], result["key1"])
		assert.Equal(t, float64(value["key2"].(int)), result["key2"])
		assert.Equal(t, value["key3"], result["key3"])
	})

	t.Run("set and get slice", func(t *testing.T) {
		key := "test:slice"
		value := []string{"item1", "item2", "item3"}
		ttl := 5 * time.Minute

		// Set value
		err := client.Set(ctx, key, value, ttl)
		assert.NoError(t, err)

		// Get value
		var result []string
		exists, err := client.Get(ctx, key, &result)
		assert.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, value, result)
	})
}

func TestRedis_GetNonExistentKey(t *testing.T) {
	client, err := NewRedis(testConfig)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	key := "test:nonexistent"

	var result string
	exists, err := client.Get(ctx, key, &result)
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestRedis_TTL(t *testing.T) {
	client, err := NewRedis(testConfig)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	t.Run("set with TTL", func(t *testing.T) {
		key := "test:ttl"
		value := "test value"
		ttl := 2 * time.Second

		// Set value with TTL
		err := client.Set(ctx, key, value, ttl)
		assert.NoError(t, err)

		// Verify value exists
		var result string
		exists, err := client.Get(ctx, key, &result)
		assert.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, value, result)

		// Wait for TTL to expire
		time.Sleep(3 * time.Second)

		// Verify value no longer exists
		exists, err = client.Get(ctx, key, &result)
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("set without TTL", func(t *testing.T) {
		key := "test:no_ttl"
		value := "test value"
		ttl := 0 // No expiration

		// Set value without TTL
		err := client.Set(ctx, key, value, time.Duration(ttl))
		assert.NoError(t, err)

		// Verify value exists
		var result string
		exists, err := client.Get(ctx, key, &result)
		assert.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, value, result)

		// Clean up
		client.client.Del(ctx, key)
	})
}

func TestRedis_ConcurrentAccess(t *testing.T) {
	client, err := NewRedis(testConfig)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	numGoroutines := 10
	keyPrefix := "test:concurrent"

	// Test concurrent writes
	t.Run("concurrent writes", func(t *testing.T) {
		done := make(chan bool, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				key := fmt.Sprintf("%s:write:%d", keyPrefix, id)
				value := fmt.Sprintf("value_%d", id)
				ttl := 5 * time.Minute

				err := client.Set(ctx, key, value, ttl)
				assert.NoError(t, err)
				done <- true
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < numGoroutines; i++ {
			<-done
		}
	})

	// Test concurrent reads
	t.Run("concurrent reads", func(t *testing.T) {
		// First set some values
		for i := 0; i < numGoroutines; i++ {
			key := fmt.Sprintf("%s:read:%d", keyPrefix, i)
			value := fmt.Sprintf("read_value_%d", i)
			ttl := 5 * time.Minute

			err := client.Set(ctx, key, value, ttl)
			require.NoError(t, err)
		}

		done := make(chan bool, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				key := fmt.Sprintf("%s:read:%d", keyPrefix, id)
				var result string
				exists, err := client.Get(ctx, key, &result)
				assert.NoError(t, err)
				assert.True(t, exists)
				assert.Equal(t, fmt.Sprintf("read_value_%d", id), result)
				done <- true
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < numGoroutines; i++ {
			<-done
		}
	})
}

func TestRedis_ErrorHandling(t *testing.T) {
	client, err := NewRedis(testConfig)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	t.Run("invalid JSON unmarshaling", func(t *testing.T) {
		key := "test:invalid_json"
		value := "invalid json string"
		ttl := 5 * time.Minute

		// Set raw string value
		err := client.client.Set(ctx, key, value, ttl).Err()
		require.NoError(t, err)

		// Try to unmarshal into struct
		type TestStruct struct {
			Name string `json:"name"`
		}
		var result TestStruct
		exists, err := client.Get(ctx, key, &result)
		assert.Error(t, err)
		assert.False(t, exists) // Key exists but unmarshaling failed, so Get returns false
	})

	t.Run("context cancellation", func(t *testing.T) {
		key := "test:cancel"
		value := "test"
		ttl := 5 * time.Minute

		// Create a cancelled context
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		err := client.Set(cancelCtx, key, value, ttl)
		assert.Error(t, err)
	})
}

func TestRedis_Close(t *testing.T) {
	client, err := NewRedis(testConfig)
	require.NoError(t, err)

	// Test that close doesn't return an error
	err = client.Close()
	assert.NoError(t, err)

	// Test that operations fail after close
	ctx := context.Background()
	err = client.Set(ctx, "test", "value", time.Minute)
	assert.Error(t, err)
}

func TestRedis_Integration(t *testing.T) {
	client, err := NewRedis(testConfig)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	// Clean up any existing test keys
	client.client.FlushDB(ctx)

	t.Run("complete workflow", func(t *testing.T) {
		// Test data
		type User struct {
			ID    int    `json:"id"`
			Name  string `json:"name"`
			Email string `json:"email"`
		}

		user := User{
			ID:    1,
			Name:  "Alice",
			Email: "alice@example.com",
		}

		key := "user:1"
		ttl := 10 * time.Minute

		// 1. Set user data
		err := client.Set(ctx, key, user, ttl)
		assert.NoError(t, err)

		// 2. Get user data
		var retrievedUser User
		exists, err := client.Get(ctx, key, &retrievedUser)
		assert.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, user, retrievedUser)

		// 3. Update user data
		user.Email = "alice.updated@example.com"
		err = client.Set(ctx, key, user, ttl)
		assert.NoError(t, err)

		// 4. Verify update
		var updatedUser User
		exists, err = client.Get(ctx, key, &updatedUser)
		assert.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, user, updatedUser)

		// 5. Test with different data types
		stats := map[string]interface{}{
			"login_count": 42,
			"last_login":  time.Now().Unix(),
			"active":      true,
		}

		statsKey := "user:1:stats"
		err = client.Set(ctx, statsKey, stats, ttl)
		assert.NoError(t, err)

		var retrievedStats map[string]interface{}
		exists, err = client.Get(ctx, statsKey, &retrievedStats)
		assert.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, float64(stats["login_count"].(int)), retrievedStats["login_count"])
		assert.Equal(t, stats["active"], retrievedStats["active"])
	})
}

// Benchmark tests
func BenchmarkRedis_Set(b *testing.B) {
	client, err := NewRedis(testConfig)
	require.NoError(b, err)
	defer client.Close()

	ctx := context.Background()
	value := "benchmark test value"
	ttl := 5 * time.Minute

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench:set:%d", i)
		err := client.Set(ctx, key, value, ttl)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRedis_Get(b *testing.B) {
	client, err := NewRedis(testConfig)
	require.NoError(b, err)
	defer client.Close()

	ctx := context.Background()
	value := "benchmark test value"
	ttl := 5 * time.Minute

	// Pre-populate with test data
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench:get:%d", i)
		err := client.Set(ctx, key, value, ttl)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench:get:%d", i)
		var result string
		_, err := client.Get(ctx, key, &result)
		if err != nil {
			b.Fatal(err)
		}
	}
}
