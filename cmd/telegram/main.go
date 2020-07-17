package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"vybar/tg"
	"vybar/tg/message"

	"github.com/sirupsen/logrus"
)

var (
	token   = flag.String("token", "", "Bot API token")
	verbose = flag.Bool("verbose", false, "turns verbosing on")
)

func main() {
	flag.Parse()

	if *verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}

	if token == nil || *token == "" {
		panic("no token specified")
	}

	api, err := tg.New(*token)
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

	ch, err := api.GetUpdatesChan(ctx, 0)
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

		msg := message.Text(upd.Message.Chat.ID, fmt.Sprintf("echo: %s", txt), message.InReplyTo(upd.Message.ID))
		if _, err := api.SendMessage(msg); err != nil {
			logrus.Error(err)
		}
	}
}
