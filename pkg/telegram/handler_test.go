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
