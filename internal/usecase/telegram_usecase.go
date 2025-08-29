package usecase

import (
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type TelegramUsecase interface {
	HandleStartCommand(chatId, userId int64) error
	TakeUpdate(update tgbotapi.Update) error
}
type telegramUsecase struct {
	bot *tgbotapi.BotAPI
}

// StartCommand implements TelegramUsecase.
func (t *telegramUsecase) HandleStartCommand(chatId, userId int64) error {
	photoUrl := "https://firebasestorage.googleapis.com/v0/b/rent-ffb49.appspot.com/o/photos%2Fvictory-contest-log.png?alt=media&token=477e8229-07ad-4447-9ffa-3e16835b5d2a"
	message := "<b>Welcome! 👋 </b>\nPress the button below and take one step to the journey"
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonWebApp("Open Contest", tgbotapi.WebAppInfo{URL: "https://victory-contest.vercel.app"}),
		),
	)
	err := t.sendMessage(chatId, message, photoUrl, keyboard)
	if err != nil {
		return err
	}
	
	return nil
}

// TakeUpdate implements TelegramUsecase.
func (t *telegramUsecase) TakeUpdate(update tgbotapi.Update) error {
	message := update.Message
	chatID := message.Chat.ID
	userID := message.From.ID
	text := message.Text

	if text == "/start" {
		err := t.HandleStartCommand(chatID, userID)
		if err != nil {
			return err
		}
		return nil
	}
	return nil
}
func (t *telegramUsecase) sendMessage(chatID int64, text string, photo string, keyboard tgbotapi.InlineKeyboardMarkup) error {
	photoMsg := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(photo))
	photoMsg.Caption = text
	photoMsg.ParseMode = "HTML"
	photoMsg.ReplyMarkup = keyboard

	if _, err := t.bot.Send(photoMsg); err != nil {
		log.Printf("Error sending photo: %v", err)
	}

	// _, err := t.bot.Send(photoMsg)
	// if err != nil {
	// 	return err
	// }
	return nil
}

func NewTelegramUsecase(bot *tgbotapi.BotAPI) TelegramUsecase {
	return &telegramUsecase{bot: bot}
}
