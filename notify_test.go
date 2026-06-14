package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// slackServer は受信したペイロードを記録するテスト用Slackエンドポイント。
func slackServer(t *testing.T, calls *atomic.Int32, last *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		*last = string(body)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestNotifyAlwaysOnSuccess(t *testing.T) {
	var calls atomic.Int32
	var last string
	srv := slackServer(t, &calls, &last)

	n := NewNotifier()
	nc := &NotifyConfig{SlackWebhook: srv.URL, On: notifyAlways}
	res := DeployResult{Repo: "o/r", Branch: "main", Commit: "abc1234def", Duration: 1500 * time.Millisecond}
	if err := n.Notify(context.Background(), nc, res); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}

	var msg slackMessage
	if err := json.Unmarshal([]byte(last), &msg); err != nil {
		t.Fatalf("payload parse: %v", err)
	}
	if len(msg.Attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(msg.Attachments))
	}
	att := msg.Attachments[0]
	if !strings.Contains(att.Title, "succeeded") {
		t.Errorf("title = %q, want success", att.Title)
	}
	if att.Color != "#2eb886" {
		t.Errorf("color = %q, want green", att.Color)
	}
	// コミットは短縮表示される。
	if joined := fieldValue(att, "Commit"); joined != "abc1234" {
		t.Errorf("commit field = %q, want abc1234", joined)
	}
}

func TestNotifyFailureOnlySkipsSuccess(t *testing.T) {
	var calls atomic.Int32
	var last string
	srv := slackServer(t, &calls, &last)

	n := NewNotifier()
	nc := &NotifyConfig{SlackWebhook: srv.URL, On: notifyFailure}

	// 成功時は送らない。
	if err := n.Notify(context.Background(), nc, DeployResult{Repo: "o/r"}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("成功なのに通知された: calls = %d", calls.Load())
	}

	// 失敗時は送る。
	res := DeployResult{Repo: "o/r", Err: errors.New("boom")}
	if err := n.Notify(context.Background(), nc, res); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("失敗時に通知されない: calls = %d", calls.Load())
	}
	if !strings.Contains(last, "failed") || !strings.Contains(last, "boom") {
		t.Errorf("失敗ペイロードに情報が無い: %s", last)
	}
}

func TestNotifyNoConfigNoop(t *testing.T) {
	n := NewNotifier()
	// nil でも webhook空 でもエラーにならず送らない。
	if err := n.Notify(context.Background(), nil, DeployResult{}); err != nil {
		t.Fatalf("nil notify: %v", err)
	}
	if err := n.Notify(context.Background(), &NotifyConfig{On: notifyAlways}, DeployResult{}); err != nil {
		t.Fatalf("empty webhook notify: %v", err)
	}
}

func fieldValue(att slackAttachment, title string) string {
	for _, f := range att.Fields {
		if f.Title == title {
			return f.Value
		}
	}
	return ""
}
