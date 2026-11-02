package controller

import (
	"errors"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"

	"github.com/kuehnleon/uptimerobot-operator/internal/uptimerobot"
)

func TestFailureReason(t *testing.T) {
	for err, want := range map[error]string{
		&uptimerobot.APIError{StatusCode: 403}: "Rejected",
		&uptimerobot.APIError{StatusCode: 429}: "RateLimited",
		&uptimerobot.APIError{StatusCode: 502}: "Failed",
		errors.New("secret not found"):         "Failed",
	} {
		if got := failureReason(err); got != want {
			t.Errorf("%v: got %s, want %s", err, got, want)
		}
	}
}

func TestRetries(t *testing.T) {
	var r retries
	key := types.NamespacedName{Name: "a"}
	if d := r.failed(key, 1, time.Minute); d != time.Minute {
		t.Fatalf("first retry after %v", d)
	}
	if d := r.failed(key, 1, time.Minute); d != 2*time.Minute {
		t.Fatalf("second retry after %v", d)
	}
	if r.wait(key, 1) <= time.Minute {
		t.Fatal("events before the next retry must be suppressed")
	}
	if r.wait(key, 2) != 0 {
		t.Fatal("spec changes must bypass the backoff")
	}
	r.reset(key)
	if r.wait(key, 1) != 0 {
		t.Fatal("reset must clear the backoff")
	}
}
