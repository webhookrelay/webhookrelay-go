package client

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func getIntegrationTestClient() (*API, error) {
	return New(os.Getenv("RELAY_KEY"), os.Getenv("RELAY_SECRET"))

}

func TestListBuckets(t *testing.T) {
	client, err := getIntegrationTestClient()
	if err != nil {
		t.Fatalf("failed to get API client: %s", err)
	}

	buckets, err := client.ListBuckets(&BucketListOptions{})
	assert.Nil(t, err)
	assert.True(t, len(buckets) > 0)
}
