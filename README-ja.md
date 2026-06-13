# fetchanddeploy

[English](README.md) | 日本語

GitHub の push を webhook で受け取り、対象リポジトリを pull して任意のデプロイコマンドを実行するワンバイナリツール。VPS 等での自動デプロイ用。

- 単一バイナリ（Go）で動作。配布が簡単
- 複数リポジトリを1プロセスで処理
- `X-Hub-Signature-256`（HMAC-SHA256）で署名検証
- リポジトリ単位の排他ロックでデプロイ重複を防止
- GitHub の 10 秒タイムアウトを避けるため即時 202 応答 → デプロイは非同期実行

## ビルド

```sh
go build -o fetchanddeploy .
# バージョン埋め込み:
go build -ldflags "-X main.version=$(git describe --tags --always)" -o fetchanddeploy .

# Linux VPS 向けクロスコンパイル（macOSから）:
GOOS=linux GOARCH=amd64 go build -o fetchanddeploy .
```

## 設定

`config.example.yaml` をコピーして編集する。

```yaml
listen: ":9000"
path: "/webhook"
repositories:
  - name: "owner/repo"          # GitHubのフルネーム
    branch: "main"               # 反応するブランチ（空=全ブランチ）
    path: "/var/www/app"         # 作業ディレクトリ
    secret: "webhookのSecret"
    timeout: 5m
    deploy:
      - "git fetch --all --prune"
      - "git reset --hard origin/main"
      - "docker compose up -d --build"
```

デプロイコマンド内では以下の環境変数が使える: `FAD_REPO` / `FAD_BRANCH` / `FAD_COMMIT`。

## 起動

```sh
./fetchanddeploy -config /etc/fetchanddeploy/config.yaml
```

`-version` でバージョン表示。`/healthz` でヘルスチェック可能。

## GitHub 側の設定

リポジトリの **Settings → Webhooks → Add webhook**:

- Payload URL: `https://your-domain/webhook`
- Content type: `application/json`
- Secret: 設定ファイルの `secret` と同じ値
- イベント: **Just the push event**

## リバースプロキシ（nginx 例）

本ツールは HTTP のみ。TLS 終端はプロキシ側で行う。

```nginx
location /webhook {
    proxy_pass http://127.0.0.1:9000/webhook;
    proxy_set_header Host $host;
}
```

## systemd

`fetchanddeploy.service` を `/etc/systemd/system/` に置く。

```sh
sudo cp fetchanddeploy /usr/local/bin/
sudo mkdir -p /etc/fetchanddeploy
sudo cp config.example.yaml /etc/fetchanddeploy/config.yaml  # 編集する
sudo cp fetchanddeploy.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now fetchanddeploy
sudo journalctl -u fetchanddeploy -f
```

## 注意

- 初回は対象 `path` に対象リポジトリを git clone 済みにしておくこと（本ツールは clone は行わず、設定の deploy コマンドを実行するだけ）。
- pull に SSH を使う場合、サービス実行ユーザーの SSH 鍵 / known_hosts を用意する。
- deploy コマンドは `/bin/sh -c` で実行されるため、設定ファイルの取り扱いに注意（任意コマンド実行になる）。
