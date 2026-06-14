package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxBodyBytes = 25 << 20 // GitHubのpayloadは最大25MB

// pushPayload はpushイベントから必要なフィールドだけ取り出す。
type pushPayload struct {
	Ref        string `json:"ref"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	HeadCommit struct {
		ID string `json:"id"`
	} `json:"head_commit"`
}

// Handler はwebhookを処理するHTTPハンドラ。
type Handler struct {
	cfg      *Config
	deployer *Deployer
	notifier *Notifier
	// locks はリポジトリ単位の排他ロック。同一リポジトリのデプロイ重複を防ぐ。
	locks sync.Map
}

func NewHandler(cfg *Config, d *Deployer, n *Notifier) *Handler {
	return &Handler{cfg: cfg, deployer: d, notifier: n}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	// 署名検証より前にリポジトリを特定するため、まず軽くパースする。
	var p pushPayload
	if err := json.Unmarshal(body, &p); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	repo := h.cfg.find(p.Repository.FullName)
	if repo == nil {
		// 未知のリポジトリ。secretが分からないので検証もできない。
		log.Printf("未設定のリポジトリからのリクエスト: %q", p.Repository.FullName)
		http.Error(w, "unknown repository", http.StatusNotFound)
		return
	}

	// HMAC-SHA256で署名検証。
	if !validSignature(r.Header.Get("X-Hub-Signature-256"), body, repo.Secret) {
		log.Printf("%s: 署名検証に失敗しました", repo.Name)
		http.Error(w, "signature mismatch", http.StatusUnauthorized)
		return
	}

	switch event {
	case "ping":
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "pong")
		return
	case "push":
		// 続行
	default:
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "ignored event: %s", event)
		return
	}

	// ブランチ判定。refは "refs/heads/main" 形式。
	branch := strings.TrimPrefix(p.Ref, "refs/heads/")
	if repo.Branch != "" && branch != repo.Branch {
		log.Printf("%s: ブランチ %q はスキップ（対象: %q）", repo.Name, branch, repo.Branch)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "skipped branch: %s", branch)
		return
	}

	// GitHubの10秒タイムアウトを避けるため、即座に応答してデプロイは非同期実行。
	log.Printf("%s: push受信 branch=%s commit=%s — デプロイを開始します", repo.Name, branch, shortSHA(p.HeadCommit.ID))
	go h.runDeploy(repo, branch, p.HeadCommit.ID)

	w.WriteHeader(http.StatusAccepted)
	_, _ = io.WriteString(w, "accepted")
}

// runDeploy はリポジトリ単位ロックを取りデプロイを実行し、結果を通知する。
func (h *Handler) runDeploy(repo *Repository, branch, commit string) {
	muIface, _ := h.locks.LoadOrStore(repo.Name, &sync.Mutex{})
	mu := muIface.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	start := time.Now()
	err := h.deployer.Run(repo, branch, commit)
	dur := time.Since(start)
	if err != nil {
		log.Printf("%s: デプロイ失敗: %v", repo.Name, err)
	} else {
		log.Printf("%s: デプロイ完了 (%s)", repo.Name, dur.Round(time.Millisecond))
	}

	h.notify(DeployResult{Repo: repo.Name, Branch: branch, Commit: commit, Err: err, Duration: dur}, repo)
}

// notify はデプロイ結果を通知する（設定があれば）。失敗してもデプロイ自体には影響させない。
func (h *Handler) notify(res DeployResult, repo *Repository) {
	if h.notifier == nil {
		return
	}
	nc := h.cfg.notifyFor(repo)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := h.notifier.Notify(ctx, nc, res); err != nil {
		log.Printf("%s: 通知の送信に失敗しました: %v", repo.Name, err)
	}
}

// validSignature はGitHubのX-Hub-Signature-256を検証する。
func validSignature(header string, body []byte, secret string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	want, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(want, mac.Sum(nil))
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}
