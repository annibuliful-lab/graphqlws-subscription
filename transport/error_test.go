package transport

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

func TestSendError(t *testing.T) {
	recorder := httptest.NewRecorder()
	errs := []*gqlerror.Error{
		{Message: "First error"},
		{Message: "Second error"},
	}

	SendError(recorder, http.StatusBadRequest, errs...)

	response := recorder.Result()
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)

	var resp gqlResponse
	err := json.Unmarshal(recorder.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Len(t, resp.Errors, 2)
	assert.Equal(t, "First error", resp.Errors[0].Message)
	assert.Equal(t, "Second error", resp.Errors[1].Message)
}

func TestSendErrorf(t *testing.T) {
	recorder := httptest.NewRecorder()
	SendErrorf(recorder, http.StatusInternalServerError, "Formatted %s", "error")

	response := recorder.Result()
	assert.Equal(t, http.StatusInternalServerError, response.StatusCode)

	var resp gqlResponse
	err := json.Unmarshal(recorder.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Len(t, resp.Errors, 1)
	assert.Equal(t, "Formatted error", resp.Errors[0].Message)
}

func TestToGQLError(t *testing.T) {
	stdErr := errors.New("Standard error")
	gqlErr := toGQLError(stdErr)

	assert.Equal(t, stdErr.Error(), gqlErr.Message)
}
