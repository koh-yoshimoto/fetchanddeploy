package main

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config はツール全体の設定。
type Config struct {
	// Listen は待受アドレス（例: ":9000"）。リバースプロキシ配下でHTTPを想定。
	Listen string `yaml:"listen"`
	// Path はwebhookを受け付けるHTTPパス。
	Path string `yaml:"path"`
	// LogFile は指定するとstdoutに加えてファイルにも追記する（任意）。
	LogFile string `yaml:"log_file"`
	// Notify はデプロイ結果通知の既定設定（任意）。リポジトリ側で上書き可能。
	Notify *NotifyConfig `yaml:"notify"`
	// Repositories は対象リポジトリの一覧。
	Repositories []*Repository `yaml:"repositories"`
}

// NotifyConfig はデプロイ結果通知の設定（任意機能）。
type NotifyConfig struct {
	// SlackWebhook はSlack Incoming WebhookのURL。空なら通知しない。
	SlackWebhook string `yaml:"slack_webhook"`
	// On は通知タイミング。"always"（既定）= 成功/失敗とも、"failure" = 失敗時のみ。
	On string `yaml:"on"`
}

const (
	notifyAlways  = "always"
	notifyFailure = "failure"
)

// Repository は1リポジトリ分のデプロイ設定。
type Repository struct {
	// Name はGitHubのフルネーム（"owner/repo"）。payloadのrepository.full_nameと一致させる。
	Name string `yaml:"name"`
	// Branch は反応するブランチ名（例: "main"）。空なら全ブランチ。
	Branch string `yaml:"branch"`
	// Path はデプロイコマンドを実行する作業ディレクトリ。
	Path string `yaml:"path"`
	// Secret はGitHub webhookのSecret。HMAC-SHA256検証に使う。
	Secret string `yaml:"secret"`
	// Deploy はpull/デプロイのために順次実行するシェルコマンド列。
	Deploy []string `yaml:"deploy"`
	// Timeout はデプロイ全体のタイムアウト。0なら無制限。
	Timeout time.Duration `yaml:"timeout"`
	// Notify はこのリポジトリ専用の通知設定（任意）。指定するとグローバルのnotifyを上書きする。
	Notify *NotifyConfig `yaml:"notify"`
}

// LoadConfig は設定ファイルを読み込み検証する。
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("設定ファイル読み込み: %w", err)
	}

	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("設定ファイル解析: %w", err)
	}

	if cfg.Listen == "" {
		cfg.Listen = ":9000"
	}
	if cfg.Path == "" {
		cfg.Path = "/webhook"
	}
	if len(cfg.Repositories) == 0 {
		return nil, fmt.Errorf("repositories が空です")
	}

	seen := make(map[string]bool)
	for i, r := range cfg.Repositories {
		if r.Name == "" {
			return nil, fmt.Errorf("repositories[%d]: name は必須です", i)
		}
		if seen[r.Name] {
			return nil, fmt.Errorf("repositories[%d]: name %q が重複しています", i, r.Name)
		}
		seen[r.Name] = true
		if r.Path == "" {
			return nil, fmt.Errorf("%s: path は必須です", r.Name)
		}
		if r.Secret == "" {
			return nil, fmt.Errorf("%s: secret は必須です", r.Name)
		}
		if len(r.Deploy) == 0 {
			return nil, fmt.Errorf("%s: deploy コマンドが空です", r.Name)
		}
		if err := normalizeNotify(r.Notify); err != nil {
			return nil, fmt.Errorf("%s: notify: %w", r.Name, err)
		}
	}
	if err := normalizeNotify(cfg.Notify); err != nil {
		return nil, fmt.Errorf("notify: %w", err)
	}

	return &cfg, nil
}

// normalizeNotify はNotifyConfigの既定値補完と検証を行う。nilは何もしない。
func normalizeNotify(n *NotifyConfig) error {
	if n == nil {
		return nil
	}
	if n.On == "" {
		n.On = notifyAlways
	}
	if n.On != notifyAlways && n.On != notifyFailure {
		return fmt.Errorf("on は %q または %q を指定してください（指定値: %q）", notifyAlways, notifyFailure, n.On)
	}
	return nil
}

// find はフルネームから該当リポジトリ設定を返す。
func (c *Config) find(fullName string) *Repository {
	for _, r := range c.Repositories {
		if r.Name == fullName {
			return r
		}
	}
	return nil
}

// notifyFor はリポジトリに適用する通知設定を返す。
// リポジトリ固有設定があればそれを、無ければグローバル設定を使う。
func (c *Config) notifyFor(r *Repository) *NotifyConfig {
	if r.Notify != nil {
		return r.Notify
	}
	return c.Notify
}
