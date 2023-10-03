package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/chehsunliu/poker"
	"github.com/companyzero/bisonrelay/clientrpc/types"
)

var (
	suits  = []string{"Hearts ♥", "Diamonds ♦", "Clubs ♣", "Spades ♠"}
	values = []string{"2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K", "A"}
)

const (
	Draw     = "draw"
	PreFlop  = "pre-flop"
	Flop     = "flop"
	Turn     = "turn"
	River    = "river"
	Showdown = "showdown"
)

type PokerGame struct {
	ID             string   `json:"id"` //gcid
	Players        []Player `json:"players"`
	CommunityCards []Card   `json:"communitycards"`
	CurrentStage   string   `json:"currentstage"`
	CurrentPlayer  int      `json:"currentplayer"`
	// Winner is an array with indexes of players. If a tie happens can have
	// multiple winners.
	Winner         []int   `json:"winner"`
	DealerPosition int     `json:"dealerposition"`
	BigBlind       int     `json:"bigblind"`
	SmallBlind     int     `json:"smallblind"`
	CurrentBet     float64 `json:"currentbet"`
	Pot            float64 `json:"pot"`
	BB             float64 `json:"bb"`
	SB             float64 `json:"sb"`
	Deck           []Card  `json:"deck"`
	// the bot client responsible to managing the game
	Bot string `json:"bot"`
}

type Player struct {
	ID       string
	Nick     string
	Hand     []Card
	Chips    int
	Folded   bool
	Bet      float64
	IsActive bool
	HasActed bool
}

type Card struct {
	Value string
	Suit  string
}

// Helper function to get the next active position that player hasn't folded.
func nextActiveNotFoldedPosition(pos int, players []Player) int {
	for {
		pos = (pos + 1) % len(players)
		if players[pos].IsActive && !players[pos].Folded {
			break
		}
	}
	return pos
}

// Helper function to get the next active position.
func nextActivePosition(pos int, players []Player) int {
	for {
		pos = (pos + 1) % len(players)
		if players[pos].IsActive {
			break
		}
	}
	return pos
}

func New(id string, players []Player, dealerPosition int, sb, bb float64) *PokerGame {
	smallBlindPosition := nextActivePosition(dealerPosition, players)
	bigBlindPosition := nextActivePosition(smallBlindPosition, players)
	// currentPlayer := nextActivePosition(smallBlindPosition, players)
	// as we start the current stage on the draw, the first player to receive cards
	// is the small blind. In others stages, the first player to act is the one
	// after the big blind.
	currentPlayer := smallBlindPosition

	game := &PokerGame{
		ID: id,
		// Bot:            botId,
		CurrentStage:   Draw,
		Pot:            0,
		DealerPosition: dealerPosition,
		CurrentPlayer:  currentPlayer,
		SmallBlind:     smallBlindPosition,
		BigBlind:       bigBlindPosition,
		BB:             bb,
		SB:             sb,
		Players:        players,
	}

	return game
}

func (g *PokerGame) ProgressPokerGame() {
	if g.AllPlayersActed() {
		switch g.CurrentStage {
		case Draw:
			g.CurrentStage = PreFlop
			g.CurrentPlayer = nextActiveNotFoldedPosition(g.BigBlind, g.Players)
			g.ResetPlayerActions()
		case PreFlop:
			g.Flop()
			g.CurrentStage = Flop
			g.CurrentPlayer = nextActiveNotFoldedPosition(g.BigBlind, g.Players)
			g.ResetPlayerActions()
		case Flop:
			g.Turn()
			g.CurrentStage = Turn
			g.CurrentPlayer = nextActiveNotFoldedPosition(g.BigBlind, g.Players)
			g.ResetPlayerActions()
		case Turn:
			g.River()
			g.CurrentStage = River
			g.CurrentPlayer = nextActiveNotFoldedPosition(g.BigBlind, g.Players)
			g.ResetPlayerActions()
		case River:
			g.Showdown()
			g.CurrentStage = Showdown
			// XXX
			// g.CurrentPlayer after showdown needs to progress blinds and button
		case Showdown:
			g.DetermineWinner()
			g.DistributePot()
			// g.ResetPokerGame()
		}
	} else {
		g.CurrentPlayer = nextActiveNotFoldedPosition(g.CurrentPlayer, g.Players)
	}
}

func (g *PokerGame) Draw() Card {
	card := g.Deck[len(g.Deck)-1]
	g.Deck = g.Deck[:len(g.Deck)-1]
	return card
}

func (g *PokerGame) ShuffleDeck() {
	if g.Deck == nil {
		var cards []Card
		for _, suit := range suits {
			for _, value := range values {
				cards = append(cards, Card{Suit: suit, Value: value})
			}
		}
		g.Deck = cards
	}
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(g.Deck), func(i, j int) { g.Deck[i], g.Deck[j] = g.Deck[j], g.Deck[i] })
}

func (g *PokerGame) Deal() {
	// Deal two cards to each player
	for _, player := range g.Players {
		player.Hand = append(player.Hand, g.Deck[len(g.Deck)-2], g.Deck[len(g.Deck)-1])
		g.Deck = g.Deck[:len(g.Deck)-2]
	}
}

func (g *PokerGame) Flop() {
	// Deal the flop
	g.CommunityCards = append(g.CommunityCards, g.Deck[len(g.Deck)-3], g.Deck[len(g.Deck)-2], g.Deck[len(g.Deck)-1])
	g.Deck = g.Deck[:len(g.Deck)-3]
}

func (g *PokerGame) Turn() {
	card := g.Deck[0]
	g.CommunityCards = append(g.CommunityCards, card)
	g.Deck = g.Deck[1:]
}

func (g *PokerGame) River() {
	card := g.Deck[0]
	g.CommunityCards = append(g.CommunityCards, card)
	g.Deck = g.Deck[1:]
}

func (g *PokerGame) Raise(value float64) {
	g.CurrentBet = value
	for i := range g.Players {
		if g.Players[i].IsActive && !g.Players[i].Folded {
			g.Players[i].HasActed = false
		}
	}
	g.Players[g.CurrentPlayer].HasActed = true
	g.Players[g.CurrentPlayer].Bet = value
	g.Pot += value
	g.ProgressPokerGame()
}

func (g *PokerGame) Bet(betAmount float64) {
	// Update the player's bet and the game's current bet
	g.Players[g.CurrentPlayer].Bet = betAmount
	g.CurrentBet = betAmount
	g.Pot += betAmount
	g.Players[g.CurrentPlayer].HasActed = true
	g.ProgressPokerGame()
}

func (g *PokerGame) Call() {
	g.Players[g.CurrentPlayer].HasActed = true
	g.Pot += g.CurrentBet
	g.ProgressPokerGame()
}

func (g *PokerGame) ProgressDealer(ctx context.Context, payment types.PaymentsServiceClient) error {
	g.DealerPosition = nextActivePosition(g.DealerPosition, g.Players)
	g.SmallBlind = nextActivePosition(g.DealerPosition, g.Players)
	g.BigBlind = nextActivePosition(g.DealerPosition, g.Players)

	return nil
}

func (g *PokerGame) DetermineWinner() []int {
	var winners []int
	var bestRank int32 = math.MaxInt32 // Initialize to the maximum possible value

	for i, player := range g.Players {
		if !player.IsActive {
			continue
		}

		// Convert player and community cards to poker.Card
		var cards []poker.Card
		for _, card := range append(player.Hand, g.CommunityCards...) {
			// Convert to the string representation expected by the poker package
			cardStr := card.Value + strings.ToLower(card.Suit[:1])
			cards = append(cards, poker.NewCard(cardStr))
		}

		fmt.Printf("Hand: %v\ncards: %v\n", player.Hand, cards)
		// Evaluate the hand
		rank := poker.Evaluate(cards)
		fmt.Printf("rank: %s\n\n", poker.RankString(rank))
		if rank < bestRank {
			fmt.Printf("Best Rank: %s player: %s\n", poker.RankString(rank), player.Nick)

			bestRank = rank
			winners = []int{i} // Reset winners list as we have a new best hand
		} else if rank == bestRank {
			winners = append(winners, i) // Add to winners list as hands are equal
		}
	}

	g.Winner = winners
	return winners
}

func (g *PokerGame) DistributePot() {
	winners := g.DetermineWinner()
	if len(winners) == 0 {
		return // No active players
	}

	// Divide the pot equally among the winners in case of a tie
	share := g.Pot / float64(len(winners))
	for _, winner := range winners {
		g.Players[winner].Chips += int(share)
	}
}

func (g *PokerGame) ResetPokerGame() {
	// g.Deck = NewDeck().Cards
	g.Deck = nil
	g.CommunityCards = []Card{}
	g.Pot = 0
	g.CurrentStage = "pre-flop"
	for _, player := range g.Players {
		// player.Hand = []Card{}
		player.HasActed = false
	}
	g.ShuffleDeck()
	g.Deal()
}

func (g *PokerGame) Showdown() {
	// Show all players' hands
	for _, player := range g.Players {
		if !player.IsActive {
			continue
		}
		fmt.Printf("%s's hand: %v and %v\n", player.Nick, player.Hand[0], player.Hand[1])
	}
	// Determine the winner, distribute the pot and reset the PokerGame
	g.DistributePot()
}

func (g *PokerGame) AllPlayersActed() bool {
	for _, player := range g.Players {
		if !player.IsActive {
			continue
		}
		if !player.HasActed {
			return false
		}
	}
	return true
}

func (g *PokerGame) ResetPlayerActions() {
	for i, _ := range g.Players {
		g.Players[i].HasActed = false
	}
	for i := range g.Players {
		g.Players[i].Bet = 0
	}
	g.CurrentBet = 0
}
