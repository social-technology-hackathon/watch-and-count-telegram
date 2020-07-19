package keyboard

import "encoding/json"

type KeyboardButton struct {
	Text string `json:"text"`
}

type ReplyKeyboard struct {
	Keyboard        [][]KeyboardButton `json:"keyboard"`
	ResizeKeyboard  bool               `json:"resize_keyboard"`
	OneTimeKeyboard bool               `json:"one_time_keyboard"`
	Selective       bool               `json:"selective"`
}

type ButtonRow []KeyboardButton

func Button(text string) KeyboardButton {
	return KeyboardButton{
		Text: text,
	}
}

func Row(buttons ...KeyboardButton) ButtonRow {
	return ButtonRow(buttons)
}

func NewReplyKeyboard(rows ...ButtonRow) *ReplyKeyboard {
	rowsToSend := make([][]KeyboardButton, 0, len(rows))
	for _, row := range rows {
		rowsToSend = append(rowsToSend, []KeyboardButton(row))
	}
	return &ReplyKeyboard{
		Keyboard:       rowsToSend,
		ResizeKeyboard: true,
	}
}

func (kb *ReplyKeyboard) Serialize() ([]byte, error) {
	return json.Marshal(kb)
}
