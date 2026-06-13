package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func newTestHandler(t *testing.T, repo *Repository) *Handler {
	t.Helper()
	cfg := &Config{Path: "/webhook", Repositories: []*Repository{repo}}
	return NewHandler(cfg, NewDeployer())
}

func post(h *Handler, event string, body []byte, sig string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(string(body)))
	req.Header.Set("X-GitHub-Event", event)
	if sig != "" {
		req.Header.Set("X-Hub-Signature-256", sig)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPushDeploys(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "deployed.txt")
	repo := &Repository{
		Name:   "koh-yoshimoto/myapp",
		Branch: "main",
		Path:   dir,
		Secret: "s3cr3t",
		Deploy: []string{"echo $FAD_COMMIT > " + marker},
	}
	h := newTestHandler(t, repo)

	body := []byte(`{"ref":"refs/heads/main","repository":{"full_name":"koh-yoshimoto/myapp"},"head_commit":{"id":"abc1234"}}`)
	rec := post(h, "push", body, sign(body, "s3cr3t"))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}

	// 非同期デプロイの完了待ち。
	waitFor(t, func() bool { _, err := os.Stat(marker); return err == nil })
	got, _ := os.ReadFile(marker)
	if strings.TrimSpace(string(got)) != "abc1234" {
		t.Fatalf("marker = %q, want abc1234", got)
	}
}

func TestBadSignatureRejected(t *testing.T) {
	repo := &Repository{Name: "o/r", Branch: "main", Path: t.TempDir(), Secret: "right", Deploy: []string{"true"}}
	h := newTestHandler(t, repo)
	body := []byte(`{"ref":"refs/heads/main","repository":{"full_name":"o/r"}}`)
	rec := post(h, "push", body, sign(body, "wrong"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestWrongBranchSkipped(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "x")
	repo := &Repository{Name: "o/r", Branch: "main", Path: dir, Secret: "k", Deploy: []string{"touch " + marker}}
	h := newTestHandler(t, repo)
	body := []byte(`{"ref":"refs/heads/dev","repository":{"full_name":"o/r"}}`)
	rec := post(h, "push", body, sign(body, "k"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("対象外ブランチなのにデプロイが走った")
	}
}

func TestUnknownRepo(t *testing.T) {
	repo := &Repository{Name: "o/r", Path: t.TempDir(), Secret: "k", Deploy: []string{"true"}}
	h := newTestHandler(t, repo)
	body := []byte(`{"ref":"refs/heads/main","repository":{"full_name":"other/repo"}}`)
	rec := post(h, "push", body, sign(body, "k"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestPing(t *testing.T) {
	repo := &Repository{Name: "o/r", Path: t.TempDir(), Secret: "k", Deploy: []string{"true"}}
	h := newTestHandler(t, repo)
	body := []byte(`{"repository":{"full_name":"o/r"}}`)
	rec := post(h, "ping", body, sign(body, "k"))
	if rec.Code != http.StatusOK || rec.Body.String() != "pong" {
		t.Fatalf("ping応答が不正: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

// concurrentDeploys がロックで直列化されることを確認。
func TestPerRepoSerialized(t *testing.T) {
	dir := t.TempDir()
	repo := &Repository{Name: "o/r", Branch: "main", Path: dir, Secret: "k",
		Deploy: []string{"sleep 0.2"}}
	h := newTestHandler(t, repo)

	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); h.runDeploy(repo, "main", "x") }()
	}
	wg.Wait()
	if elapsed := time.Since(start); elapsed < 350*time.Millisecond {
		t.Fatalf("直列化されていない: %v", elapsed)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("条件が時間内に満たされませんでした")
}
