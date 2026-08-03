package app

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestMessageSenderUsername_NilFrom(t *testing.T) {
	msg := &tgbotapi.Message{}
	if got := messageSenderUsername(msg); got != "" {
		t.Errorf("got %q, want empty string for nil From", got)
	}
}

func TestMessageSenderUsername_ReturnsUsername(t *testing.T) {
	msg := &tgbotapi.Message{From: &tgbotapi.User{UserName: "alice"}}
	if got := messageSenderUsername(msg); got != "alice" {
		t.Errorf("got %q, want %q", got, "alice")
	}
}
