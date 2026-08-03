# Telegram User Allow-List Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `telegram.users` config option listing allowed Telegram usernames; requests from any other user (or from users with no Telegram username) are silently ignored.

**Architecture:** `MessageHandler` (pkg/telegram) owns a lower-cased allow-list built from config and exposes `IsAllowed(username string) bool`. `internal/app`'s `runBot` is the single loop handling every incoming message (commands and downloads alike), so it calls `IsAllowed` once, right after resolving the message's `From`, before dispatching to either the command branch or `MessageHandler.Handle`.

**Tech Stack:** Go, `github.com/go-telegram-bot-api/telegram-bot-api/v5`, `github.com/rs/zerolog`.

## Global Constraints

- Empty/omitted `telegram.users` list = no restriction (must not change behavior for existing configs).
- Username matching is case-insensitive.
- A message with no `From` or an empty `From.UserName` is denied whenever the allow-list is non-empty.
- Denied requests get no reply of any kind (not even to `/start`).

---

### Task 1: Allow-list on `MessageHandler`

**Files:**
- Modify: `pkg/telegram/handler.go:24-41` (interface/struct/constructor block)
- Test: `pkg/telegram/handler_test.go` (new)

**Interfaces:**
- Consumes: nothing new — this task is self-contained.
- Produces:
  - `func NewMessageHandler(bot *tgbotapi.BotAPI, dl Downloader, users []string, l zerolog.Logger) *MessageHandler` (signature change: `users []string` inserted before `l`)
  - `func (h *MessageHandler) IsAllowed(username string) bool`

- [ ] **Step 1: Write the failing tests**

Create `pkg/telegram/handler_test.go`:

```go
package telegram

import (
	"testing"

	"github.com/rs/zerolog"
)

func TestIsAllowed_EmptyListAllowsAny(t *testing.T) {
	h := NewMessageHandler(nil, nil, nil, zerolog.Nop())

	if !h.IsAllowed("someone") {
		t.Error("expected empty allow-list to allow any username")
	}
	if !h.IsAllowed("") {
		t.Error("expected empty allow-list to allow an empty username")
	}
}

func TestIsAllowed_MatchesCaseInsensitively(t *testing.T) {
	h := NewMessageHandler(nil, nil, []string{"Alice", "bob"}, zerolog.Nop())

	if !h.IsAllowed("alice") {
		t.Error("expected case-insensitive match for 'alice'")
	}
	if !h.IsAllowed("BOB") {
		t.Error("expected case-insensitive match for 'BOB'")
	}
}

func TestIsAllowed_DeniesUnknownAndEmptyUsername(t *testing.T) {
	h := NewMessageHandler(nil, nil, []string{"alice"}, zerolog.Nop())

	if h.IsAllowed("carol") {
		t.Error("expected an unlisted username to be denied")
	}
	if h.IsAllowed("") {
		t.Error("expected an empty username to be denied when the list is non-empty")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/telegram/... -run TestIsAllowed -v`
Expected: FAIL to compile — `NewMessageHandler` doesn't accept a `users` argument and `IsAllowed` doesn't exist yet.

- [ ] **Step 3: Implement the allow-list**

In `pkg/telegram/handler.go`, add `"strings"` is already imported. Replace the `MessageHandler` struct and `NewMessageHandler` (lines 29-41) with:

```go
type MessageHandler struct {
	bot          *tgbotapi.BotAPI
	dl           Downloader
	allowedUsers map[string]struct{}
	l            zerolog.Logger
}

func NewMessageHandler(bot *tgbotapi.BotAPI, dl Downloader, users []string, l zerolog.Logger) *MessageHandler {
	allowed := make(map[string]struct{}, len(users))
	for _, u := range users {
		allowed[strings.ToLower(u)] = struct{}{}
	}

	return &MessageHandler{
		bot:          bot,
		dl:           dl,
		allowedUsers: allowed,
		l:            l,
	}
}

// IsAllowed reports whether username may use the bot. An empty allow-list
// means no restriction is configured, so every username is allowed.
func (h *MessageHandler) IsAllowed(username string) bool {
	if len(h.allowedUsers) == 0 {
		return true
	}

	_, ok := h.allowedUsers[strings.ToLower(username)]
	return ok
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/telegram/... -run TestIsAllowed -v`
Expected: PASS (3 tests)

- [ ] **Step 5: Commit**

```bash
git add pkg/telegram/handler.go pkg/telegram/handler_test.go
git commit -m "Add allow-list check to MessageHandler"
```

---

### Task 2: Config option and wiring into the bot loop

**Files:**
- Modify: `internal/app/config.go:3-5` (`Telegram` struct)
- Modify: `internal/app/app.go:19-30` (`Run`) and `:42-68` (`runBot`)

**Interfaces:**
- Consumes: `telegram.NewMessageHandler(bot, dl, users []string, l)` and `(*MessageHandler).IsAllowed(username string) bool` from Task 1.
- Produces: nothing consumed by later tasks — this is the last task.

- [ ] **Step 1: Add the config field**

In `internal/app/config.go`, change:

```go
type Telegram struct {
	Token string `yaml:"token"`
}
```

to:

```go
type Telegram struct {
	Token string   `yaml:"token"`
	Users []string `yaml:"users"`
}
```

- [ ] **Step 2: Wire the config into `NewMessageHandler`**

In `internal/app/app.go`, change:

```go
	runBot(ctx, tgAPI, telegram.NewMessageHandler(tgAPI, dl, l), l)
```

to:

```go
	runBot(ctx, tgAPI, telegram.NewMessageHandler(tgAPI, dl, cfg.Telegram.Users, l), l)
```

- [ ] **Step 3: Gate `runBot` on the allow-list**

In `internal/app/app.go`, inside `runBot`'s `case upd := <-updates:` branch, right after the existing:

```go
			if upd.Message == nil {
				continue
			}
```

add:

```go
			var userName string
			if upd.Message.From != nil {
				userName = upd.Message.From.UserName
			}
			if !msgHandler.IsAllowed(userName) {
				l.Debug().
					Int64("chat_id", upd.Message.Chat.ID).
					Str("user_name", userName).
					Msg("ignoring request from disallowed user")
				continue
			}
```

This runs before the `IsCommand()` branch, so both `/start` and download requests are silently dropped for disallowed users.

- [ ] **Step 4: Build and verify no regressions**

Run: `go build ./...`
Expected: builds cleanly with no errors.

Run: `go test ./...`
Expected: all existing tests still pass (including the Task 1 tests).

- [ ] **Step 5: Manually verify config parses**

Add a `users:` entry under `telegram:` in `config.yml` locally (do not commit real usernames if this is a shared file — use a throwaway value), run the bot, and confirm in the logs that an unlisted username's message is logged as "ignoring request from disallowed user" and produces no reply. Remove the local test edit afterward if it was only for manual verification.

- [ ] **Step 6: Commit**

```bash
git add internal/app/config.go internal/app/app.go
git commit -m "Restrict Telegram bot to an allow-list of usernames"
```

## Self-Review Notes

- Spec coverage: config option (Task 2 Step 1), case-insensitive matching and empty-list passthrough (Task 1), gating `/start` + downloads via the single `runBot` call site (Task 2 Step 3), denial of empty/absent username (Task 1 tests + Task 2 Step 3's `userName` zero-value default). All covered.
- No placeholders: every step has literal code.
- Type consistency: `NewMessageHandler(bot, dl, users []string, l)` and `IsAllowed(username string) bool` are identical across Task 1's produces and Task 2's consumes.
