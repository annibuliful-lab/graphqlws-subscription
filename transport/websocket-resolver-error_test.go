package transport

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

func TestAddAndGetSubscriptionError(t *testing.T) {
	ctx := context.Background()
	ctx = withSubscriptionErrorContext(ctx) // Initialize the context with error handling

	// Simulate adding errors
	err1 := &gqlerror.Error{Message: "First error"}
	err2 := &gqlerror.Error{Message: "Second error"}
	AddSubscriptionError(ctx, err1)
	AddSubscriptionError(ctx, err2)

	// Retrieve errors
	errs := getSubscriptionError(ctx)

	// Assertions
	assert.Len(t, errs, 2, "There should be two errors recorded")
	assert.Equal(t, err1, errs[0], "The first error should match the added error")
	assert.Equal(t, err2, errs[1], "The second error should match the added error")
}

func TestWithSubscriptionErrorContext(t *testing.T) {
	ctx := context.Background()

	// Context without error setup
	assert.Nil(t, getSubscriptionErrorStruct(ctx), "Expected nil subscription error struct for uninitialized context")

	// Set up error context
	ctx = withSubscriptionErrorContext(ctx)
	assert.NotNil(t, getSubscriptionErrorStruct(ctx), "Expected non-nil subscription error struct after initialization")
}

func TestGetSubscriptionErrorEmpty(t *testing.T) {
	ctx := context.Background()
	ctx = withSubscriptionErrorContext(ctx) // Ensure context is prepared even if no errors are added

	// Retrieve errors from an "empty" error context
	errs := getSubscriptionError(ctx)

	// Assert on the expected empty result
	assert.Empty(t, errs, "Expected no errors in the newly initialized context")
}
