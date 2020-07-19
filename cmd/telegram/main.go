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
	"vybar/tg"
	"vybar/tg/file"
	"vybar/tg/keyboard"
	"vybar/tg/message"

	"github.com/kelseyhightower/envconfig"

	"github.com/davecgh/go-spew/spew"
	"github.com/sirupsen/logrus"
)

type Config struct {
	TelegramToken string `envconfig:"TELEGRAM_TOKEN" required:"true"`
	Verbose       bool   `envconfig:"VERBOSE"`
	StorageType   string `envconfig:"STORAGE_TYPE"`
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

	bot := TGBot{
		api:         api,
		fileStorage: dst,
	}
	bot.Run(ctx)
}

type TGBot struct {
	api         *tg.API
	fileStorage destenation.Destenation
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
			txt = *upd.Message.Text
		}

		if strings.ToLower(strings.TrimSpace(txt)) == "/start" {
			logrus.Debug("start working with new user")
			tg.welcomeMessage(upd.Message.Chat.ID)
			continue
		}

		msg := message.Text(upd.Message.Chat.ID, fmt.Sprintf("echo: %s", txt), message.InReplyTo(upd.Message.ID))
		if _, err := tg.api.SendMessage(msg); err != nil {
			logrus.Error(err)
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
				if err := tg.storeFile(maxSizeFile.ID, "jpg"); err != nil {
					logrus.Error(err)
					continue
				}
			}
		}

		if upd.Message.Video != nil {
			logrus.Debug("got video")
			spew.Dump(upd.Message.Video)
			if err := tg.storeFile(upd.Message.Video.ID, "mp4"); err != nil {
				logrus.Error(err)
				respMsg := message.Text(upd.Message.Chat.ID, "При загрузке видео произошла ошибка, попробуйте еще раз", message.InReplyTo(upd.Message.ID))
				if _, err := tg.api.SendMessage(respMsg); err != nil {
					logrus.Error(err)
				}
				continue
			}
			respMsg := message.Text(upd.Message.Chat.ID, "Ваше видео успешно принято", message.InReplyTo(upd.Message.ID))
			if _, err := tg.api.SendMessage(respMsg); err != nil {
				logrus.Error(err)
				continue
			}
		}
	}
}

func (tg *TGBot) storeFile(fileID string, ext string) error {
	rdr, err := tg.api.GetFD(fileID)
	if err != nil {
		return err
	}
	defer rdr.Close()

	if _, err := tg.fileStorage.Store(context.Background(), rdr, ext); err != nil {
		return err
	}

	if err := rdr.Close(); err != nil {
		return err
	}
	return nil
}

func (tg *TGBot) welcomeMessage(chatID int64) error {
	msg := message.Text(chatID, "Привет! Я знаю, как ты можешь защитить свой голос")
	if _, err := tg.api.SendMessage(msg); err != nil {
		return err
	}

	msg = message.Text(chatID, "План очень прост! Тебе нужно лишь написать мне перед тем, как пойдешь на участок и я скажу тебе, что надо делать")
	if _, err := tg.api.SendMessage(msg); err != nil {
		return err
	}

	msg = message.Text(
		chatID,
		"А еще, если тебе очень хочется помочь в подсчете голосов - скажи мне об этом обязательно!",
		message.WithKeyboard(
			keyboard.NewReplyKeyboard(
				keyboard.Row(keyboard.Button("Иду на участок!")),
				keyboard.Row(keyboard.Button("Хочу помочь в подсчете!")),
				keyboard.Row(keyboard.Button("Можно подробнее?")),
			),
		),
	)
	if _, err := tg.api.SendMessage(msg); err != nil {
		return err
	}
	return nil
}
