package webhookrelay

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListAccessTokens(t *testing.T) {
	client := integrationClient(t)

	// Smoke test: listing works against whatever account the credentials
	// belong to (the account always has at least the credential's own token),
	// and every returned token is well-formed. End-to-end CRUD coverage lives
	// in TestIntegrationBucketLifecycle.
	tokens, err := client.ListAccessTokens(&AccessTokenListOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, tokens)

	for _, token := range tokens {
		assert.NotEmpty(t, token.ID, "access token missing ID")
	}
}
