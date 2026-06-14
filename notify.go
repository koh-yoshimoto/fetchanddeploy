package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DeployResult はデプロイ1回分の結果。通知の材料になる。
type DeployResult struct {
	Repo     string
	Branch   string
	Commit   string
	Err      error // nilなら成功
	Duration time.Duration
}

// Notifier はデプロイ結果を外部サービスへ通知する。
type Notifier struct {
	client *http.Client
}

func NewNotifier() *Notifier {
	return &Notifier{client: &http.Client{Timeout: 10 * time.Second}}
}

// Notify は設定に従いデプロイ結果を送信する。
// nc が nil / Webhook未設定 / 条件不一致 の場合は何もしない（nilを返す）。
func (n *Notifier) Notify(ctx context.Context, nc *NotifyConfig, res DeployResult) error {
	if nc == nil || nc.SlackWebhook == "" {
		return nil
	}
	if nc.On == notifyFailure && res.Err == nil {
		return nil
	}
	return n.sendSlack(ctx, nc.SlackWebhook, res)
}

// slackMessage はSlack Incoming Webhookのペイロード（attachments形式）。
type slackMessage struct {
	Attachments []slackAttachment `json:"attachments"`
}

type slackAttachment struct {
	Color  string       `json:"color"`
	Title  string       `json:"title"`
	Text   string       `json:"text,omitempty"`
	Fields []slackField `json:"fields,omitempty"`
	Footer string       `json:"footer,omitempty"`
}

type slackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

func (n *Notifier) sendSlack(ctx context.Context, url string, res DeployResult) error {
	msg := buildSlackMessage(res)
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("Slack応答 %d: %s", resp.StatusCode, bytes.TrimSpace(b))
	}
	return nil
}

func buildSlackMessage(res DeployResult) slackMessage {
	color, title := "#2eb886", "✅ Deploy succeeded" // good (green)
	if res.Err != nil {
		color, title = "#cc0000", "❌ Deploy failed" // danger (red)
	}

	fields := []slackField{
		{Title: "Repository", Value: res.Repo, Short: true},
		{Title: "Branch", Value: orDash(res.Branch), Short: true},
		{Title: "Commit", Value: orDash(shortSHA(res.Commit)), Short: true},
		{Title: "Duration", Value: res.Duration.Round(time.Millisecond).String(), Short: true},
	}

	att := slackAttachment{
		Color:  color,
		Title:  title,
		Fields: fields,
		Footer: "fetchanddeploy",
	}
	if res.Err != nil {
		att.Text = "```" + res.Err.Error() + "```"
	}
	return slackMessage{Attachments: []slackAttachment{att}}
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
