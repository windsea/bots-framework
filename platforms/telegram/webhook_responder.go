package telegram_bot

import (
	"encoding/json"
	"fmt"
	"github.com/strongo/bots-api-telegram"
	"github.com/strongo/bots-framework/core"
	"golang.org/x/net/context"
	"net/http"
)

type TelegramWebhookResponder struct {
	w   http.ResponseWriter
	whc *TelegramWebhookContext
}

var _ bots.WebhookResponder = (*TelegramWebhookResponder)(nil)

func NewTelegramWebhookResponder(w http.ResponseWriter, whc *TelegramWebhookContext) TelegramWebhookResponder {
	responder := TelegramWebhookResponder{w: w, whc: whc}
	whc.responder = responder
	return responder
}

func (r TelegramWebhookResponder) SendMessage(c context.Context, m bots.MessageFromBot, channel bots.BotApiSendMessageChannel) (resp bots.OnMessageSentResponse, err error) {
	if channel != bots.BotApiSendMessageOverHTTPS && channel != bots.BotApiSendMessageOverResponse {
		panic(fmt.Sprintf("Unknown channel: [%v]. Expected either 'https' or 'response'.", channel))
	}
	//ctx := tc.Context()
	logger := r.whc.Logger()

	var chattable tgbotapi.Chattable
	botApi := &tgbotapi.BotAPI{
		Token:  r.whc.BotContext.BotSettings.Token,
		Debug:  true,
		Client: r.whc.GetHttpClient(),
	}
	if m.TelegramCallbackAnswer != nil {
		logger.Debugf(c, "Inline answer")
		input := r.whc.Input().(TelegramWebhookUpdateProvider)
		m.TelegramCallbackAnswer.CallbackQueryID = input.TgUpdate().CallbackQuery.ID

		chattable = m.TelegramCallbackAnswer
		jsonStr, err := json.Marshal(chattable)
		if err == nil {
			logger.Infof(c, "CallbackAnswer for sending to Telegram: %v", string(jsonStr))
		} else {
			logger.Errorf(c, "Failed to marshal message config to json: %v\n\tInput: %v", err, chattable)
		}
		apiResponse, err := botApi.Send(chattable)

		if s, err2 := json.Marshal(apiResponse); err2 != nil {
			logger.Debugf(c, "apiResponse: %v", s)
		}
		return resp, err
	} else if m.TelegramEditMessageText != nil {
		if m.TelegramEditMessageText.ReplyMarkup == nil && m.TelegramKeyboard != nil {
			m.TelegramEditMessageText.ReplyMarkup = m.TelegramKeyboard.(*tgbotapi.InlineKeyboardMarkup)
		}
		chattable = m.TelegramEditMessageText
	} else if m.TelegramEditMessageMarkup != nil {
		chattable = m.TelegramEditMessageMarkup
	} else if m.TelegramInlineConfig != nil {
		chattable = m.TelegramInlineConfig
	} else if m.Text != "" {
		if m.Text == bots.NoMessageToSend {
			return
		}
		messageConfig := r.whc.NewTgMessage(m.Text)
		if m.TelegramChatID != 0 {
			messageConfig.ChatID = m.TelegramChatID
		}
		messageConfig.DisableWebPagePreview = m.DisableWebPagePreview
		switch m.Format {
		case bots.MessageFormatHTML:
			messageConfig.ParseMode = "HTML"
		case bots.MessageFormatMarkdown:
			messageConfig.ParseMode = "Markdown"
		}
		messageConfig.ReplyMarkup = m.TelegramKeyboard

		chattable = messageConfig
	} else {
		switch r.whc.InputType() {
		case bots.WebhookInputInlineQuery: // pass
		case bots.WebhookInputChosenInlineResult: // pass
		default:
			mBytes, err := json.Marshal(m)
			if err != nil {
				logger.Errorf(c, "Failed to marshal MessageFromBot to JSON: %v", err)
			}
			inputTypeName := bots.WebhookInputTypeNames[r.whc.InputType()]
			logger.Warningf(c, "Not inline answer, Not inline, Not edit inline, Text is empty. r.whc.InputType(): %v\nMessageFromBot:\n%v", inputTypeName, string(mBytes))
		}
		return
	}

	if jsonStr, err := json.Marshal(chattable); err != nil {
		logger.Errorf(c, "Failed to marshal message config to json: %v\n\tJSON: %v\n\tchattable: %v", err, jsonStr, chattable)
		return resp, err
	} else {
		vals, err := chattable.Values()
		if err != nil {
			//pass?
		}
		logger.Infof(c, "Sending to Telegram, Text: %v\n------------------------\nAs JSON: %v------------------------\nAs URL values: %v", m.Text, string(jsonStr), vals.Encode())
	}

	//if values, err := chattable.Values(); err != nil {
	//	logger.Errorf(c, "Failed to marshal message config to url.Values: %v", err)
	//	return resp, err
	//} else {
	//	logger.Infof(c, "Message for sending to Telegram as URL values: %v", values)
	//}

	switch channel {
	case bots.BotApiSendMessageOverResponse:
		if _, err := tgbotapi.ReplyToResponse(chattable, r.w); err != nil {
			logger.Errorf(c, "Failed to send message to Telegram throw HTTP response: %v", err)
		}
		return resp, err
	case bots.BotApiSendMessageOverHTTPS:
		if message, err := botApi.Send(chattable); err != nil {
			logger.Errorf(c, "Failed to send message to Telegram using HTTPS API: %v", err)
			return resp, err
		} else {
			logger.Debugf(c, "Telegram API: MessageID=%v", message.MessageID)
			//if messageJson, err := json.Marshal(message); err != nil {
			//	logger.Warningf(c, "Telegram API response as raw: %v", message)
			//} else {
			//	logger.Debugf(c, "Telegram API: MessageID=%v", message.MessageID)
			//	logger.Debugf(c, "Telegram API response as JSON: %v", string(messageJson))
			//}
			return bots.OnMessageSentResponse{TelegramMessage: message}, nil
		}
	default:
		panic(fmt.Sprintf("Unknown channel: %v", channel))
	}
}
