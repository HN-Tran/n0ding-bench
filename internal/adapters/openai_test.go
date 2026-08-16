package adapters

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hn-tran/n0ding-bench/internal/bench"
)

func TestOpenAICompatible(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("auth absent")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer s.Close()
	a := OpenAICompatible{Endpoint: s.URL, Model: "m", APIKey: "secret"}
	got, e := a.Invoke(context.Background(), bench.TargetRequest{Input: "hi"})
	if e != nil || got.Output != "ok" {
		t.Fatalf("%#v %v", got, e)
	}
}

func TestOpenAICompatibleMalformedTimeoutAndRetryClassification(t *testing.T) {
	t.Run("malformed", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"choices":[]}`)) }))
		defer s.Close()
		_, err := (&OpenAICompatible{Endpoint: s.URL, Model: "m"}).Invoke(context.Background(), bench.TargetRequest{Input: "q"})
		if err == nil {
			t.Fatal("malformed response accepted")
		}
	})
	t.Run("timeout", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"late"}}]}`))
		}))
		defer s.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		_, err := (&OpenAICompatible{Endpoint: s.URL, Model: "m"}).Invoke(ctx, bench.TargetRequest{Input: "q"})
		if err == nil {
			t.Fatal("timeout ignored")
		}
	})
	t.Run("retryable", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "busy", http.StatusTooManyRequests) }))
		defer s.Close()
		_, err := (&OpenAICompatible{Endpoint: s.URL, Model: "m"}).Invoke(context.Background(), bench.TargetRequest{Input: "q"})
		var temporary TemporaryError
		if err == nil || !errors.As(err, &temporary) {
			t.Fatalf("not retryable: %v", err)
		}
	})
}

func TestOpenAICompatibleRejectsPrivateAndMetadataTargets(t *testing.T) {
	for _, endpoint := range []string{"https://10.0.0.1/v1", "https://169.254.169.254/latest"} {
		a := OpenAICompatible{Endpoint: endpoint, Model: "m"}
		if _, err := a.Invoke(context.Background(), bench.TargetRequest{Input: "hi"}); err == nil {
			t.Fatalf("accepted prohibited endpoint %s", endpoint)
		}
	}
}
