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
	// Repositories は対象リポジトリの一覧。
	Repositories []*Repository `yaml:"repositories"`
}

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
	}

	return &cfg, nil
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
