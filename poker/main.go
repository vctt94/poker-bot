package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"vctt94/poker-bot/bot"

	"github.com/companyzero/bisonrelay/clientrpc/types"
	"github.com/decred/slog"
	"github.com/jrick/logrotate/rotator"
)

const (
	networkBR     uint16 = 1
	networkMatrix uint16 = 2
)

func realMain() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// Setup logging
	logDir := filepath.Join(cfg.DataDir, "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return err
	}
	logPath := filepath.Join(logDir, "bot.log")
	logFd, err := rotator.New(logPath, 32*1024, true, 0)
	if err != nil {
		return err
	}
	defer logFd.Close()

	logBknd := slog.NewBackend(&logWriter{logFd}, slog.WithFlags(slog.LUTC))
	botLog := logBknd.Logger("BOT")
	gcLog := logBknd.Logger("BRLY")
	mtrxLog := logBknd.Logger("MTRX")
	mtrxLog.SetLevel(slog.LevelDebug)

	bknd := slog.NewBackend(os.Stderr)
	log := bknd.Logger("BRLY")
	log.SetLevel(slog.LevelDebug)

	gcChan := make(chan types.GCReceivedMsg)
	pChan := make(chan types.TipProgressEvent)

	botCfg := bot.Config{
		DataDir: cfg.DataDir,
		Log:     botLog,

		URL:            cfg.URL,
		ServerCertPath: cfg.ServerCertPath,
		ClientCertPath: cfg.ClientCertPath,
		ClientKeyPath:  cfg.ClientKeyPath,

		GCChan: gcChan,
		GCLog:  gcLog,
	}

	bot, err := bot.New(botCfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Launch handler
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case gcm := <-gcChan:
				nick := escapeNick(gcm.Nick)
				if gcm.Msg == nil {
					gcLog.Tracef("empty message from %v", nick)
					continue
				}
				gcidstr := hex.EncodeToString(gcm.Uid[:])
				msg := fmt.Sprintf("[m] <%v> %v", gcm.Nick, gcm.Msg)
				if err := bot.SendGC(ctx, gcidstr, msg); err != nil {
					gcLog.Errorf("failed to send msg to gc %v: %v", gcidstr, err)
				}
			case p := <-pChan:
				nick := escapeNick(p.Nick)
				if p.AmountMatoms == 0 {
					gcLog.Tracef("empty tip from %v", nick)
					continue
				}
				// how to know it is a payment from a game?

			}

		}
	}()

	return bot.Run()
}

func main() {
	err := realMain()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

type logWriter struct {
	r *rotator.Rotator
}

func (l *logWriter) Write(p []byte) (n int, err error) {
	os.Stdout.Write(p)
	return l.r.Write(p)
}
