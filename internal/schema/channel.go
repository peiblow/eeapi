package schema

import (
	"encoding/json"
	"fmt"
)

type Channel interface {
	GetChannelType() string
}

type WhatsAppChannel struct {
	PhoneNumber string `json:"phone_number"`
}

func (WhatsAppChannel) GetChannelType() string {
	return "whatsapp"
}

type EmailChannel struct {
	Email string `json:"email"`
}

func (EmailChannel) GetChannelType() string {
	return "email"
}

type TelegramChannel struct {
	ChatID string `json:"chat_id"`
}

func (TelegramChannel) GetChannelType() string {
	return "telegram"
}

var registry = map[string]func() Channel{
	"whatsapp": func() Channel { return &WhatsAppChannel{} },
	"email":    func() Channel { return &EmailChannel{} },
	"telegram": func() Channel { return &TelegramChannel{} },
}

func ParseChannel(channelType string, payload json.RawMessage) (Channel, error) {
	factory, ok := registry[channelType]
	if !ok {
		return nil, fmt.Errorf("invalid channel type: %s", channelType)
	}

	ch := factory()

	if err := json.Unmarshal(payload, ch); err != nil {
		return nil, fmt.Errorf("invalid %s payload: %w", channelType, err)
	}

	return ch, nil
}
