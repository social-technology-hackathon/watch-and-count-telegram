package message

import (
	"time"
	"vybar/tg/chat"
	"vybar/tg/user"
)

type Message struct {
	ID             int        `json:"message_id,omitempty"`
	From           *user.User `json:"from,omitempty"`
	Date           int        `json:"date,omitempty"`
	DateTime       time.Time  `json:"-"`
	ForwardFrom    *user.User `json:"forward_from"`
	Text           *string    `json:"text,omitempty"`
	Chat           chat.Chat  `json:"chat"`
	ReplyToMessage *Message   `json:"reply_to_message,omitempty"`
}

type Option func(*Message)

func InReplyTo(messageID int) Option {
	return func(m *Message) {
		m.ReplyToMessage = &Message{
			ID: messageID,
		}
	}
}

func Text(chatID int64, body string, options ...Option) *Message {
	text := body
	msg := Message{
		Chat: chat.Chat{
			ID: chatID,
		},
		Text: &text,
	}
	for _, opt := range options {
		opt(&msg)
	}

	return &msg
}
