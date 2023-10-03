package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"vctt94/poker-bot/bot"

	"github.com/companyzero/bisonrelay/clientrpc/types"
	"github.com/decred/slog"
	"github.com/jrick/logrotate/rotator"
)

const (
	networkBR uint16 = 1
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

	onGoingGames := make(map[string]*PokerGame)
	onGoingGamesMtx := make(map[string]*sync.Mutex)
	// expectedPayments := make(map[string]float64)
	var botId string

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
				msg := gcm.Msg.Message
				if strings.HasPrefix(msg, "!start") {
					game := &PokerGame{}
					var botReply string

					info, err := bot.Info(ctx, "")
					if err != nil {
						log.Warnf("err: %v", err)
						continue
					}
					botId = info.Uid
					gc, err := bot.GetGC(ctx, gcidstr)
					if err != nil {
						log.Warnf("err: %v", err)
						continue
					}

					if gc.NbMembers < 3 {
						botReply = "minimum 2 players to start game"
						err = bot.SendGC(ctx, gcidstr, msg)
						if err != nil {
							log.Warnf("not possible to send message: %s", err)
							continue
						}
						continue
					}
					onGoingGamesMtx[gcidstr].Lock()

					players := make([]Player, gc.NbMembers)
					for i, member := range info.Members {

						// deactivate bot
						if member == botId {
							players[i] = Player{
								ID:       member,
								IsActive: false,
							}
							continue
						}
						resp, err := bot.Info(ctx, member)
						if err != nil {
							log.Warnf("not possible to chat.Info: %s", err)
							continue
						}
						players[i] = Player{
							ID:       member,
							Nick:     resp.Nick,
							Hand:     []Card{},
							IsActive: true,
							HasActed: false,
						}

					}
					game = New(gcidstr, players, 0, 0.005, 0.01)
					game.ShuffleDeck()

					// blinds need to be paid
					game.Players[game.BigBlind].HasActed = false
					game.Players[game.SmallBlind].HasActed = false

					// draw cards
					for i := range players {
						if !players[i].IsActive {
							continue
						}
						players[i].Hand = []Card{game.Draw(), game.Draw()}
						err = bot.SendPM(ctx, players[i].ID, fmt.Sprintf("Hand: %v\n"+
							"___________________________________", players[i].Hand))
						if err != nil {
							log.Warnf("Err: %v", err)
						}
					}
					onGoingGames[gcidstr] = game
					onGoingGamesMtx[gcidstr].Unlock()

					botReply = fmt.Sprintf("\n---------------\n"+
						"Current Stage: %s\n"+
						"Community Cards: %v\n"+
						"Pot: %f\n"+
						"---------------|\n"+
						fmt.Sprintf("Waiting for:\nBB: %f from %s\nSB: %f from %s\n", game.BB, game.Players[game.BigBlind].Nick, game.SB, game.Players[game.SmallBlind].Nick)+
						"Current Player: %s\n",

						game.CurrentStage, game.CommunityCards, game.Pot, game.Players[game.CurrentPlayer].Nick)

					if err != nil {
						log.Warnf("Err: %v", err)
						continue
					}
					err = bot.SendGC(ctx, gcidstr, botReply)
					if err != nil {
						log.Warnf("Err: %v", err)
						continue
					}
					// game started can continue.
					continue
				}

				// if exists {
				// 	// This is a message related to an ongoing game. Handle accordingly.
				// 	// For example, update the game's state based on the message.
				// } else {
				// 	// This is an unrelated message.
				// }
			case p := <-pChan:
				nick := escapeNick(p.Nick)
				if p.AmountMatoms == 0 {
					gcLog.Tracef("empty tip from %v", nick)
					continue
				}
				// xxx
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
