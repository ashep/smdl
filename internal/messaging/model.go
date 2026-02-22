package messaging

import (
	"database/sql"
	"time"
)

type MessageType int
type UserRole string
type TextFormat int

const (
	MessageTypeRegular MessageType = iota
	MessageTypeToolCallResponse
)

const (
	UserRoleUser UserRole = "user"
	UserRoleBot  UserRole = "bot"
)

const (
	TextFormatPlain TextFormat = iota
	TextFormatMarkdown
	TextFormatMarkdownV2
	TextFormatHTML
)

type Message struct {
	Type          MessageType
	ID            string
	ChatID        string
	UserID        string
	UserName      string
	UserRealName  string
	UserRole      UserRole
	ReplyToID     sql.NullString
	ReplyToChatID sql.NullString
	ReplyToUserID sql.NullString
	TextFormat    TextFormat
	Text          string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Chat struct {
	ID       string
	Messages []Message
}

type Thread struct {
	ID            string
	ChatID        string
	Messages      []Message
	IsBotInvolved bool
}
