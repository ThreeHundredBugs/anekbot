# anekbot

Бот для рассылки анекдотов

Напиши `анек!` и наслаждайся результатом

Сам бот: `@thb_anekbot`

## Implementations

- `src/anekbotpy/` — the original Python implementation, deployed as a Yandex Cloud Function behind a webhook (see `infra/terraform/` and `.github/workflows/deploy-cloud-function.yaml`).
- `src/anekbot-go/` — a Go rewrite meant to run as a long-lived process (e.g. on a VPS), documented below.

## anekbot-go

Same behavior as the Python bot: replies with a random joke when a message contains `анек!`, and reacts with 🤬 to messages containing Russian profanity.

Deployment/infra for anekbot-go is intentionally out of scope of this repo.

### Build

```sh
cd src/anekbot-go
go build ./cmd/anekbot-go
```

### Test

```sh
cd src/anekbot-go
go test ./...
```

### Run — webhook mode (production)

Runs an HTTP server that accepts Telegram webhook updates.

```sh
BOT_TOKEN=<your-bot-token> ./anekbot-go -mode=webhook -port=8080
```

Telegram must be told where to send updates. This is a manual, one-time step — the binary does not call `setWebhook` itself:

```sh
curl "https://api.telegram.org/bot<your-bot-token>/setWebhook?url=https://<your-public-host>/webhook"
```

Optionally set `WEBHOOK_SECRET_TOKEN` (or `-webhook-secret`) and pass the same value as `secret_token` in the `setWebhook` call above, to have anekbot-go validate Telegram's `X-Telegram-Bot-Api-Secret-Token` header on every request.

### Run — poll mode (local testing)

Long-polls Telegram for updates instead of serving a webhook, so it can run locally without a public HTTPS endpoint. Point it at a separate test bot token to try changes without affecting the production bot:

```sh
BOT_TOKEN=<your-test-bot-token> ./anekbot-go -mode=poll
```

### Configuration

| Flag | Env var | Default | Notes |
|---|---|---|---|
| `-bot-token` | `BOT_TOKEN` | *(required)* | Telegram bot token |
| `-mode` | `ANEKBOT_MODE` | `webhook` | `webhook` or `poll` |
| `-port` | `PORT` | `8080` | webhook mode only |
| `-webhook-path` | `WEBHOOK_PATH` | `/webhook` | webhook mode only |
| `-webhook-secret` | `WEBHOOK_SECRET_TOKEN` | *(disabled)* | optional, validates Telegram's secret token header |

### Releases

Pushing a tag matching `v*` (or running the workflow manually) builds `anekbot-go` for linux/amd64, linux/arm64, darwin/amd64 and darwin/arm64, and publishes them as a GitHub Release.