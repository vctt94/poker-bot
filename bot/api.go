package bot

import (
	"context"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"

	"github.com/companyzero/bisonrelay/clientrpc/types"
	"github.com/companyzero/bisonrelay/zkidentity"
	"github.com/decred/dcrd/dcrutil/v4"
)

type Info struct {
	Uid     string
	Members []string
	Nick    string
}

func (b *Bot) GetGCs(ctx context.Context) ([]*types.ListGCsResponse_GCInfo, error) {
	var req types.ListGCsRequest
	var rep types.ListGCsResponse

	if err := b.gcService.List(ctx, &req, &rep); err != nil {
		return nil, err
	}
	sort.Sort(GCs(rep.Gcs))

	return rep.Gcs, nil
}

func (b *Bot) GetGC(ctx context.Context, uid string) (*types.ListGCsResponse_GCInfo, error) {
	gcs, err := b.GetGCs(ctx)
	if err != nil {
		return nil, err
	}
	for _, gc := range gcs {
		gcid := hex.EncodeToString(gc.Id)
		if gcid == uid {
			return gc, nil
		}
	}
	return &types.ListGCsResponse_GCInfo{}, nil
}

func (b *Bot) Info(ctx context.Context, uid string) (*Info, error) {
	return &Info{
		Uid:     "bot uid",
		Members: []string{"", ""},
	}, nil
}

func (b *Bot) SendFile(ctx context.Context, uid, filename string) error {
	sfr := types.SendFileRequest{
		User:     uid,
		Filename: filename,
	}

	return b.chatService.SendFile(ctx, &sfr, &types.SendFileResponse{})
}

func (b *Bot) SendPM(ctx context.Context, nick, msg string) error {
	req := &types.PMRequest{
		User: nick,
		Msg: &types.RMPrivateMessage{
			Message: msg,
		},
	}
	var res types.PMResponse
	return b.chatService.PM(ctx, req, &res)
}

func (b *Bot) SendGC(ctx context.Context, gc, msg string) error {
	req := &types.GCMRequest{
		Gc:  gc,
		Msg: msg,
	}
	var res types.GCMResponse
	return b.chatService.GCM(ctx, req, &res)
}

func (b *Bot) SubscribeToUserPosts(ctx context.Context, user zkidentity.ShortID) error {
	var rep types.SubscribeToPostsResponse
	req := types.SubscribeToPostsRequest{
		User: user.String(),
	}
	return b.postService.SubscribeToPosts(ctx, &req, &rep)
}

func (b *Bot) PayTip(ctx context.Context, uid zkidentity.ShortID, tipAmt dcrutil.Amount, maxAttempts int32) error {
	var rep types.TipUserResponse
	req := types.TipUserRequest{
		User:        uid.String(),
		DcrAmount:   tipAmt.ToCoin(),
		MaxAttempts: maxAttempts,
	}
	return b.paymentService.TipUser(ctx, &req, &rep)
}

func (b *Bot) MediateKX(ctx context.Context, mediator, target string) error {
	var mres types.MediateKXResponse
	mreq := types.MediateKXRequest{
		Mediator: mediator,
		Target:   target,
	}
	return b.chatService.MediateKX(ctx, &mreq, &mres)
}

func (b *Bot) AcceptGCInvite(ctx context.Context, id string) error {
	i, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return err
	}
	var res types.AcceptGCInviteResponse
	req := types.AcceptGCInviteRequest{
		InviteId: i,
	}
	return b.gcService.AcceptGCInvite(ctx, &req, &res)
}

func (b *Bot) InviteToGC(ctx context.Context, gc, id string) error {
	var irep types.InviteToGCResponse
	ireq := types.InviteToGCRequest{
		Gc:   gc,
		User: id,
	}
	return b.gcService.InviteToGC(ctx, &ireq, &irep)

}

func (b *Bot) WriteNewInvite(ctx context.Context, amt dcrutil.Amount, gc string) ([]byte, string, error) {
	if amt < 0 {
		return nil, "", fmt.Errorf("negative amount")
	}
	req := types.WriteNewInviteRequest{
		Gc:         gc,
		FundAmount: uint64(amt),
	}
	var rep types.WriteNewInviteResponse

	// Add GC
	err := b.chatService.WriteNewInvite(ctx, &req, &rep)
	if err != nil {
		return nil, "", err
	}
	if len(rep.InviteKey) < 2 {
		return nil, "", fmt.Errorf("invalid invitekey")
	}

	return rep.InviteBytes, rep.InviteKey, nil
}
