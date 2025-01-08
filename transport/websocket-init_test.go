package transport

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithInitPayload(t *testing.T) {
	ctx := context.Background()
	payload := InitPayload{
		"Authorization": "Bearer token",
	}

	ctxWithPayload := withInitPayload(ctx, payload)
	retrievedPayload := GetInitPayload(ctxWithPayload)

	assert.NotNil(t, retrievedPayload, "Expected non-nil payload")
	assert.Equal(t, "Bearer token", retrievedPayload["Authorization"], "Expected Authorization token to match")
}

func TestGetInitPayload(t *testing.T) {
	ctx := context.Background()

	// Context without payload
	emptyPayload := GetInitPayload(ctx)
	assert.Nil(t, emptyPayload, "Expected nil payload for context without init payload")

	// Context with payload
	payload := InitPayload{
		"authorization": "Bearer token",
	}
	ctxWithPayload := withInitPayload(ctx, payload)
	retrievedPayload := GetInitPayload(ctxWithPayload)

	assert.NotNil(t, retrievedPayload, "Expected non-nil payload")
	assert.Equal(t, "Bearer token", retrievedPayload.Authorization(), "Expected to retrieve the authorization token")
}

func TestGetString(t *testing.T) {
	payload := InitPayload{
		"user":          "admin",
		"Authorization": "Bearer xyz",
	}

	// Valid key
	assert.Equal(t, "admin", payload.GetString("user"), "Expected to retrieve user value correctly")

	// Non-existent key
	assert.Equal(t, "", payload.GetString("nonexistent"), "Expected empty string for nonexistent key")

	// Nil payload
	var nilPayload InitPayload
	assert.Equal(t, "", nilPayload.GetString("anything"), "Expected empty string for nil payload")
}

func TestAuthorization(t *testing.T) {
	payload := InitPayload{
		"Authorization": "Bearer xyz",
	}

	// Check case sensitivity
	assert.Equal(t, "Bearer xyz", payload.Authorization(), "Expected to retrieve the correct Authorization value")

	payloadLower := InitPayload{
		"authorization": "Bearer abc",
	}

	assert.Equal(t, "Bearer abc", payloadLower.Authorization(), "Expected to retrieve the correct lowercase authorization value")

	// No authorization
	payloadNone := InitPayload{}
	assert.Equal(t, "", payloadNone.Authorization(), "Expected empty string when no authorization is present")
}
