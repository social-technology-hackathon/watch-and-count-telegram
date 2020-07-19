package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"vybar/destenation"
	"vybar/symbol"
	"vybar/tg"
	"vybar/tg/file"
	"vybar/tg/keyboard"
	"vybar/tg/message"

	"github.com/kelseyhightower/envconfig"

	"github.com/davecgh/go-spew/spew"
	"github.com/sirupsen/logrus"
)

const (
	txtVote      = "Иду на участок!"
	txtVolunteer = "Хочу помочь в подсчете!"
)

type Config struct {
	TelegramToken string `envconfig:"TELEGRAM_TOKEN" required:"true"`
	Verbose       bool   `envconfig:"VERBOSE"`
	StorageType   string `envconfig:"STORAGE_TYPE" required:"true"`
	SecretKey     string `envconfig:"SECRET_KEY" required:"true"`
}

type FSStorageParams struct {
	Path string `envconfig:"PATH" required:"true"`
}

type SpacesStorageParams struct {
	S3BaseStorageParams
	Endpoint string `envconfig:"ENDPOINT" required:"true"`
}

type S3BaseStorageParams struct {
	Key    string `envconfig:"KEY" required:"true"`
	Secret string `envconfig:"SECRET" required:"true"`
	Bucket string `envconfig:"BUCKET" required:"true"`
}

type S3StorageParams struct {
	S3BaseStorageParams
	Region string `envconfig:"REGION" required:"true"`
}

func main() {
	var cfg Config
	err := envconfig.Process("", &cfg)
	if err != nil {
		panic(err)
	}

	if cfg.Verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}

	var dst destenation.Destenation
	switch cfg.StorageType {
	case "file":
		var params FSStorageParams
		if err := envconfig.Process("STORAGE", &params); err != nil {
			panic(err)
		}
		wd, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		d, err := destenation.NewFSDestenation(filepath.Join(wd, params.Path))
		if err != nil {
			panic(err)
		}
		dst = d
	case "spaces":
		var params SpacesStorageParams
		if err := envconfig.Process("STORAGE", &params); err != nil {
			panic(err)
		}

		d, err := destenation.NewS3Destenation(params.Bucket, params.Key, params.Secret, "us-east-1", destenation.WithCustomEndpoint(params.Endpoint))
		if err != nil {
			panic(err)
		}
		dst = d
	case "s3":
		var params S3StorageParams
		if err := envconfig.Process("STORAGE", &params); err != nil {
			panic(err)
		}

		d, err := destenation.NewS3Destenation(params.Bucket, params.Key, params.Secret, params.Region)
		if err != nil {
			panic(err)
		}
		dst = d
	}

	api, err := tg.New(cfg.TelegramToken)
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGABRT)

	go func() {
		defer cancel()
		<-sigChan
		logrus.Info("Shutdown an app")
	}()

	gen, err := symbol.New(cfg.SecretKey)
	if err != nil {
		panic(err)
	}

	bot := TGBot{
		api:              api,
		fileStorage:      dst,
		secretKey:        cfg.SecretKey,
		generator:        gen,
		userSalts:        make(map[int64]string),
		userLastVideoURL: make(map[int64]string),
	}
	bot.Run(ctx)
}

type TGBot struct {
	api              *tg.API
	fileStorage      destenation.Destenation
	secretKey        string
	generator        *symbol.Generator
	userSalts        map[int64]string
	userLastVideoURL map[int64]string
}

func (tg *TGBot) Run(ctx context.Context) {
	ch, err := tg.api.GetUpdatesChan(ctx, 0)
	if err != nil {
		panic(err)
	}

	for upd := range ch {
		if upd.Message == nil {
			continue
		}

		txt := ""
		if upd.Message.Text != nil {
			txt = strings.ToLower(strings.TrimSpace(*upd.Message.Text))
		}

		if txt == "/start" {
			logrus.Debug("start working with new user")
			if err := tg.welcomeMessage(upd.Message.Chat.ID); err != nil {
				logrus.Error(err)
			}
			continue
		}

		if txt == strings.ToLower(txtVote) {
			if err := tg.processVoteRequest(ctx, upd.Message.Chat.ID); err != nil {
				logrus.Error(err)
			}
			continue
		}

		if txt == strings.ToLower(txtVolunteer) {
			if err := tg.processModeration(ctx, upd.Message.Chat.ID); err != nil {
				logrus.Error(err)
			}
			continue
		}

		if upd.Message.Photo != nil {
			logrus.Debug("got photo")
			spew.Dump(upd.Message.Photo)
			var maxSizeFile *file.PhotoSize
			maxSize := 0
			for _, ps := range upd.Message.Photo {
				s := ps.Width * ps.Height
				if s > maxSize {
					maxSize = s
					maxSizeFile = ps
				}
			}

			if maxSizeFile != nil {
				if _, err := tg.storeFile(maxSizeFile.ID, "jpg"); err != nil {
					logrus.Error(err)
					continue
				}
			}
		}

		if upd.Message.Video != nil {
			if err := tg.processVideoMessage(ctx, upd.Message); err != nil {
				logrus.Error(err)
			}
			continue
		}
	}
}

func (tg *TGBot) processVoteRequest(ctx context.Context, chatID int64) error {
	logrus.Debug("generator")
	id, err := tg.generator.Generate()
	if err != nil {
		return err
	}
	msg := message.Text(
		chatID,
		fmt.Sprintf(
			`Взяв бюллетень и зайдя в кабинку, возьми свой телефон и включи видеозапись.
Камеру направь на бюллетень, снимать нужно только его.
Поставь в бланк напротив своего кандидата вместо галочки вот эти символы: %s.
Не переживай, это абсолютно законно!
Переверни бюллетень и сними его полностью.
После этого, видеозапись можно завершить. Отправь свой бюллетень в урну.
Обязательно загрузи видео сюда, сделать это можно в любое время. Однако чем раньше, тем лучше.
Спасибо!
			`,
			id,
		),
	)
	if _, err := tg.api.SendMessage(msg); err != nil {
		return err
	}
	tg.userSalts[chatID] = id
	return nil
}

func (tg *TGBot) processVideoMessage(ctx context.Context, msg *message.Message) error {
	logrus.Debug("got video")
	spew.Dump(msg.Video)
	p, err := tg.storeFile(msg.Video.ID, "mp4")
	if err != nil {
		respMsg := message.Text(
			msg.Chat.ID, "При загрузке видео произошла ошибка, попробуйте еще раз",
			message.InReplyTo(msg.ID),
		)
		if _, err := tg.api.SendMessage(respMsg); err != nil {
			return err
		}
		return err
	}
	msgText := "Ваше видео успешно принято"
	s3dst, ok := tg.fileStorage.(*destenation.S3Destenation)
	options := []message.Option{message.InReplyTo(msg.ID)}
	if ok {
		u, err := s3dst.PublicURL(ctx, p)
		if err != nil {
			return err
		}
		options = append(options, message.WithKeyboard(
			&keyboard.InlineMarkup{
				Buttons: [][]keyboard.InlineButton{
					{
						{
							Text: "▶️ Посмотреть",
							URL:  u,
						},
					},
				},
			},
		))
		tg.userLastVideoURL[msg.Chat.ID] = u
	}
	respMsg := message.Text(msg.Chat.ID, msgText, options...)
	if _, err := tg.api.SendMessage(respMsg); err != nil {
		return err
	}
	return nil
}

func (tg *TGBot) storeFile(fileID string, ext string) (string, error) {
	rdr, err := tg.api.GetFD(fileID)
	if err != nil {
		return "", err
	}
	defer rdr.Close()

	p, err := tg.fileStorage.Store(context.Background(), rdr, ext)
	if err != nil {
		return "", err
	}

	if err := rdr.Close(); err != nil {
		return "", err
	}
	return p, nil
}

func (tg *TGBot) welcomeMessage(chatID int64) error {
	msg := message.Text(
		chatID, "Привет! Я знаю, как ты можешь защитить свой голос",
	)
	if _, err := tg.api.SendMessage(msg); err != nil {
		return err
	}

	msg = message.Text(
		chatID,
		"План очень прост! Тебе нужно лишь написать мне перед тем, как пойдешь на участок и я скажу тебе, что надо делать",
	)
	if _, err := tg.api.SendMessage(msg); err != nil {
		return err
	}

	msg = message.Text(
		chatID,
		"А еще, если тебе очень хочется помочь в подсчете голосов - скажи мне об этом обязательно!",
		message.WithKeyboard(
			keyboard.NewReplyKeyboard(
				keyboard.Row(keyboard.Button(txtVote)),
				keyboard.Row(keyboard.Button(txtVolunteer)),
				keyboard.Row(keyboard.Button("Можно подробнее?")),
			),
		),
	)
	if _, err := tg.api.SendMessage(msg); err != nil {
		return err
	}
	return nil
}

func escape(s string) string {
	return strings.ReplaceAll(s, `\`, `\\`)
}

func (tg *TGBot) processModeration(ctx context.Context, chatID int64) error {
	msg := message.Text(chatID, "Спасибо что согласился помочь!")
	if _, err := tg.api.SendMessage(msg); err != nil {
		return err
	}
	noVideo := func() error {
		msg := message.Text(chatID, "На данный момент у нас нет видео для валидации")
		if _, err := tg.api.SendMessage(msg); err != nil {
			return err
		}
		return nil
	}

	salt, ok := tg.userSalts[chatID]
	if !ok {
		return noVideo()
	}
	video, ok := tg.userLastVideoURL[chatID]
	if !ok {
		return noVideo()
	}

	msg = message.Text(
		chatID,
		fmt.Sprintf(`Пожалуйста, посмотри это [видео](%s) и убедись в следующих фактах:

\* В этом видео видно бюллетень с двух сторон
\* На этом бюллетени есть минимум две подписи членов избирательной комиссии
\* В бюллетене отмечен только один кандидат
\* Для отметки использовались символы: %s
\* Ответь ниже, за какого кандидата поставлена отметка, либо сообщи, что видео не соответствует требованиям
`, video, fmt.Sprintf("```%s```", escape(salt))),
		message.Markdown(),
		message.WithKeyboard(&keyboard.InlineMarkup{
			Buttons: [][]keyboard.InlineButton{
				{
					{
						Text: "Кандидат 1",
						URL:  video,
					},
				},
				{
					{
						Text: "Кандидат 2",
						URL:  video,
					},
				},
				{
					{
						Text: "Кандидат 3",
						URL:  video,
					},
				},
				{
					{
						Text: "Кандидат 4",
						URL:  video,
					},
				},
				{
					{
						Text: "Против всех",
						URL:  video,
					},
				},
				{
					{
						Text: "Бюллетень испорчен",
						URL:  video,
					},
				},
				{
					{
						Text: "Видео не соответствует требованиям",
						URL:  video,
					},
				},
			},
		}),
	)
	if _, err := tg.api.SendMessage(msg); err != nil {
		return err
	}
	return nil
}
