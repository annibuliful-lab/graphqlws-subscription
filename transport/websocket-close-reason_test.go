package transport

import (
	"context"
	"testing"
)

func TestCloseReasonForContext_NoReason(t *testing.T) {
	ctx := context.Background()

	// Test retrieving from a context without a set reason
	if got := closeReasonForContext(ctx); got != "" {
		t.Errorf("closeReasonForContext() = %v, want empty string", got)
	}
}
