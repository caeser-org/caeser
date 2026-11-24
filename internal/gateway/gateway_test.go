package gateway

import (
	"net/http"
	"testing"
)

func TestRetryableStatusDoesNotRetryQuotaResponses(t *testing.T) {
	for _, tt := range []struct {
		status int
		want   bool
	}{
		{status: http.StatusTooManyRequests, want: false},
		{status: http.StatusServiceUnavailable, want: true},
		{status: http.StatusBadGateway, want: true},
		{status: http.StatusBadRequest, want: false},
	} {
		if got := retryableStatus(tt.status); got != tt.want {
			t.Errorf("retryableStatus(%d) = %t, want %t", tt.status, got, tt.want)
		}
	}
}
