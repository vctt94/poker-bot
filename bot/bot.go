package bot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/companyzero/bisonrelay/clientrpc/jsonrpc"
	"github.com/companyzero/bisonrelay/clientrpc/types"
	"github.com/decred/slog"
	"golang.org/x/sync/errgroup"
)

type Config struct {
	DataDir string
	Log     slog.Logger

	URL            string
	ServerCertPath string
	ClientCertPath string
	ClientKeyPath  string

	GCChan     chan<- types.GCReceivedMsg
	GCLog      slog.Logger
	InviteChan chan<- types.ReceivedGCInvite

	PMChan chan<- types.ReceivedPM
	PMLog  slog.Logger

	PostChan chan<- types.ReceivedPost
	PostLog  slog.Logger

	PostStatusChan chan<- types.ReceivedPostStatus
	PostStatusLog  slog.Logger

	TipProgressChan chan<- types.TipProgressEvent
	TipLog          slog.Logger

	KXChan chan<- types.KXCompleted
	KXLog  slog.Logger
}

type Bot struct {
	wsc *jsonrpc.WSClient
	ctx context.Context

	wl     map[string]int64
	wlFile string
	wlMtx  sync.Mutex

	gcLog      slog.Logger
	gcChan     chan<- types.GCReceivedMsg
	inviteChan chan<- types.ReceivedGCInvite

	pmLog  slog.Logger
	pmChan chan<- types.ReceivedPM

	postLog  slog.Logger
	postChan chan<- types.ReceivedPost

	postStatusLog  slog.Logger
	postStatusChan chan<- types.ReceivedPostStatus

	tipLog  slog.Logger
	tipChan chan<- types.TipProgressEvent

	kxLog  slog.Logger
	kxChan chan<- types.KXCompleted

	chatService    types.ChatServiceClient
	gcService      types.GCServiceClient
	paymentService types.PaymentsServiceClient
	postService    types.PostsServiceClient
}

type GCs []*types.ListGCsResponse_GCInfo

func (g GCs) Len() int {
	return len(g)
}

func (g GCs) Less(a, b int) bool {
	// Most members first
	return g[a].NbMembers > g[b].NbMembers
}

func (g GCs) Swap(a, b int) {
	g[a], g[b] = g[b], g[a]
}

func (b *Bot) Close() error {
	return b.wsc.Close()
}

func (b *Bot) Run() error {
	g, gctx := errgroup.WithContext(b.ctx)

	if b.gcChan != nil {
		g.Go(func() error {
			return b.gcNtfns(gctx)
		})
	}

	if b.inviteChan != nil {
		g.Go(func() error {
			return b.inviteNtfns(gctx)
		})
	}

	if b.pmChan != nil {
		g.Go(func() error {
			return b.pmNtfns(gctx)
		})
	}

	if b.kxChan != nil {
		g.Go(func() error {
			return b.kxNtfns(gctx)
		})
	}

	if b.postChan != nil {
		g.Go(func() error {
			return b.postNtfns(gctx)
		})
	}

	if b.postStatusChan != nil {
		g.Go(func() error {
			return b.postStatusNtfns(gctx)
		})
	}

	if b.tipChan != nil {
		g.Go(func() error {
			return b.tipProgress(gctx)
		})
	}

	return g.Wait()
}

func New(cfg Config) (*Bot, error) {
	brLog := cfg.Log

	wsc, err := jsonrpc.NewWSClient(
		jsonrpc.WithWebsocketURL(cfg.URL),
		jsonrpc.WithServerTLSCertPath(cfg.ServerCertPath),
		jsonrpc.WithClientTLSCert(cfg.ClientCertPath, cfg.ClientKeyPath),
		jsonrpc.WithClientLog(brLog),
	)
	if err != nil {
		return nil, err
	}

	wl := make(map[string]int64)
	wlFile := filepath.Join(cfg.DataDir, "whitelist.json")
	wlBytes, err := os.ReadFile(wlFile)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return nil, err
	default:
		if err = json.Unmarshal(wlBytes, &wl); err != nil {
			return nil, err
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// XXX - kill everything if websocket returns
		err := wsc.Run(ctx)
		brLog.Errorf("websocket run ended: %v", err)
		cancel()
	}()

	return &Bot{
		wsc: wsc,
		ctx: ctx,

		gcChan:     cfg.GCChan,
		gcLog:      cfg.GCLog,
		inviteChan: cfg.InviteChan,

		pmChan: cfg.PMChan,
		pmLog:  cfg.PMLog,

		tipChan: cfg.TipProgressChan,
		tipLog:  cfg.TipLog,

		kxChan: cfg.KXChan,
		kxLog:  cfg.KXLog,

		postChan: cfg.PostChan,
		postLog:  cfg.PostLog,

		postStatusChan: cfg.PostStatusChan,
		postStatusLog:  cfg.PostStatusLog,

		wl:     wl,
		wlFile: wlFile,

		chatService:    types.NewChatServiceClient(wsc),
		gcService:      types.NewGCServiceClient(wsc),
		paymentService: types.NewPaymentsServiceClient(wsc),
		postService:    types.NewPostsServiceClient(wsc),
	}, nil
}
