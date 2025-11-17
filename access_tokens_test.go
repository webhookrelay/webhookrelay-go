package webhookrelay

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListAccessTokens(t *testing.T) {
	client, err := getIntegrationTestClient()
	if err != nil {
		t.Fatalf("failed to get API client: %s", err)
	}

	tokens, err := client.ListAccessTokens(&AccessTokenListOptions{})
	assert.Nil(t, err)
	assert.True(t, len(tokens) > 0)

	found := false

	for _, token := range tokens {
		if token.Description == "test-token-1" {
			found = true
		}
	}

	assert.True(t, found, "test-token-1 not found in tokens")
}
