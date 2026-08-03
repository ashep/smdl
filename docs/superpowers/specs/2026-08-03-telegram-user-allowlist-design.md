# Telegram user allow-list — design

## Goal

Restrict who the Telegram bot responds to. Add a `users` config option
listing allowed Telegram usernames; requests from anyone else are silently
ignored (no reply, no error message).

## Scope

- New config option: `telegram.users` — a list of Telegram usernames.
- Applies to every interaction: `/start`, unknown commands, and download
  requests.
- Empty/omitted list = no restriction (current behavior, unchanged for
  existing configs).
- Users with no Telegram username set are denied whenever the list is
  non-empty (there's nothing to match).
- Username comparison is case-insensitive, matching Telegram's own username
  rules.

Out of scope: per-user rate limiting, group/chat-based restrictions, remote
management of the list (it's static config, reloaded only on restart).

## Components

### Config (`internal/app/config.go`)

```go
type Telegram struct {
    Token string   `yaml:"token"`
    Users []string `yaml:"users"`
}
```

### `MessageHandler` (`pkg/telegram/handler.go`)

`NewMessageHandler` gains a `users []string` parameter and stores it as a
lower-cased set:

```go
func NewMessageHandler(bot *tgbotapi.BotAPI, dl Downloader, users []string, l zerolog.Logger) *MessageHandler
```

```go
func (h *MessageHandler) IsAllowed(username string) bool
```

Returns `true` if the set is empty, or the lower-cased `username` is a member.

### Enforcement (`internal/app/app.go`)

`runBot` is the single call site for both command handling and
`msgHandler.Handle`, so the check goes there: right after `upd.Message == nil`
is ruled out and before the command branch, check
`msgHandler.IsAllowed(upd.Message.From.UserName)` (treating a nil `From` as
disallowed when a list is configured). If not allowed, log at debug level and
`continue` — no reply is sent.

### Wiring

`app.Run` passes `cfg.Telegram.Users` into `telegram.NewMessageHandler(...)`.

## Error handling

No new error paths — this is a pure allow/deny check with no I/O. An
unconfigured (empty) list must not change any existing behavior.

## Testing

- `IsAllowed`: empty list allows any username (including empty); non-empty
  list allows an exact case-insensitive match and denies everything else,
  including an empty username.
