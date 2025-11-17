package webhookrelay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func getIntegrationTestClient() (*API, error) {

	key := os.Getenv("RELAY_KEY")
	secret := os.Getenv("RELAY_SECRET")

	if key == "" {
		return nil, errors.New("RELAY_KEY must be set")
	}
	if secret == "" {
		return nil, errors.New("RELAY_SECRET must be set")
	}

	return New(key, secret)
}

func TestListBuckets(t *testing.T) {
	client, err := getIntegrationTestClient()
	if err != nil {
		t.Fatalf("failed to get API client: %s", err)
	}

	buckets, err := client.ListBuckets(&BucketListOptions{})
	assert.Nil(t, err)
	assert.True(t, len(buckets) > 0)

	found := false

	// Look for "test-bucket" in the buckets
	for _, bucket := range buckets {
		if bucket.Name == "test-bucket-1" {
			found = true
		}
	}

	assert.True(t, found, "test-bucket-1 not found in buckets")

}

func TestListBuckets_TimeFormats(t *testing.T) {
	requestCount := 0
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/buckets", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		var buckets []map[string]interface{}

		if requestCount == 0 {
			buckets = []map[string]interface{}{
				{
					"id":          "bucket-1",
					"name":        "Test Bucket 1",
					"description": "First bucket",
					"created_at":  testTime.Unix(),
					"updated_at":  testTime.Unix(),
					"stream":      false,
					"ephemeral":   false,
					"auth":        map[string]interface{}{"type": "none"},
					"inputs":      []interface{}{},
					"outputs":     []interface{}{},
				},
			}
			requestCount++
		} else {
			buckets = []map[string]interface{}{
				{
					"id":          "bucket-2",
					"name":        "Test Bucket 2",
					"description": "Second bucket",
					"created_at":  testTime.Format(time.RFC3339),
					"updated_at":  testTime.Format(time.RFC3339),
					"stream":      true,
					"ephemeral":   true,
					"auth":        map[string]interface{}{"type": "basic", "username": "user", "password": "pass"},
					"inputs":      []interface{}{},
					"outputs":     []interface{}{},
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(buckets)
	}))
	defer server.Close()

	client, err := New("test-key", "test-secret", WithAPIEndpointURL(server.URL))
	assert.NoError(t, err)

	buckets, err := client.ListBuckets(&BucketListOptions{})
	assert.NoError(t, err)
	assert.Len(t, buckets, 1)
	assert.Equal(t, "bucket-1", buckets[0].ID)
	assert.Equal(t, "Test Bucket 1", buckets[0].Name)
	assert.Equal(t, testTime.Unix(), buckets[0].CreatedAt.Unix())
	assert.Equal(t, testTime.Unix(), buckets[0].UpdatedAt.Unix())

	buckets, err = client.ListBuckets(&BucketListOptions{})
	assert.NoError(t, err)
	assert.Len(t, buckets, 1)
	assert.Equal(t, "bucket-2", buckets[0].ID)
	assert.Equal(t, "Test Bucket 2", buckets[0].Name)
	assert.Equal(t, testTime.Unix(), buckets[0].CreatedAt.Unix())
	assert.Equal(t, testTime.Unix(), buckets[0].UpdatedAt.Unix())
}
