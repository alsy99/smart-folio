package main

import (
	"context"
	"log"
	"math"
	"net"
	"os"
	"sort"
	"sync"
	"time"

	commonv1 "aperture/go/gen/common/v1"
	learningv1 "aperture/go/gen/learning/v1"
	"aperture/pkg/strategies"

	"google.golang.org/grpc"
)

type server struct {
	learningv1.UnimplementedLearningServiceServer
	mu      sync.Mutex
	journal []*commonv1.JournalEntry
	stats   map[string]*stat
}

type stat struct {
	n, wins int
	pnlEMA  float64
	excEMA  float64
}

func newServer() *server {
	st := map[string]*stat{}
	for _, id := range strategies.IDs() {
		st[id] = &stat{}
	}
	return &server{stats: st}
}

func (s *server) RecordTrade(ctx context.Context, req *learningv1.RecordTradeRequest) (*learningv1.RecordTradeResponse, error) {
	t := req.GetTrade()
	if t == nil {
		return &learningv1.RecordTradeResponse{}, nil
	}
	tags := t.AttributionTags
	lesson := t.Lesson
	if lesson == "" {
		lesson, tags = attribute(t)
		t.AttributionTags = tags
		t.Lesson = lesson
	}
	entry := &commonv1.JournalEntry{
		Id: "j-" + t.Id, Trade: t, Lesson: lesson, Tags: tags, TsUnixMs: time.Now().UnixMilli(),
	}
	s.mu.Lock()
	s.journal = append([]*commonv1.JournalEntry{entry}, s.journal...)
	st := s.stats[t.StrategyId]
	if st == nil {
		st = &stat{}
		s.stats[t.StrategyId] = st
	}
	st.n++
	if t.Pnl > 0 {
		st.wins++
	}
	alpha := 0.2
	st.pnlEMA = (1-alpha)*st.pnlEMA + alpha*t.Pnl
	st.excEMA = (1-alpha)*st.excEMA + alpha*t.ExcessReturn
	s.mu.Unlock()
	return &learningv1.RecordTradeResponse{Entry: entry}, nil
}

func (s *server) ListJournal(ctx context.Context, req *learningv1.ListJournalRequest) (*learningv1.ListJournalResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lim := int(req.Limit)
	if lim <= 0 || lim > len(s.journal) {
		lim = len(s.journal)
	}
	return &learningv1.ListJournalResponse{Entries: s.journal[:lim]}, nil
}

func (s *server) GetWeights(ctx context.Context, _ *learningv1.GetWeightsRequest) (*learningv1.GetWeightsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := strategies.IDs()
	raw := make([]float64, len(ids))
	var weights []*commonv1.StrategyWeight
	sum := 0.0
	for i, id := range ids {
		st := s.stats[id]
		if st == nil {
			st = &stat{}
		}
		wr := 0.5
		if st.n > 0 {
			wr = float64(st.wins) / float64(st.n)
		}
		exp := st.excEMA
		score := math.Exp(3 * (exp*10 + (wr - 0.5)))
		if score < 0.15 {
			score = 0.15
		}
		raw[i] = score
		sum += score
		_ = wr
	}
	for i, id := range ids {
		st := s.stats[id]
		wr := 0.5
		n := 0
		if st != nil {
			n = st.n
			if n > 0 {
				wr = float64(st.wins) / float64(n)
			}
		}
		w := raw[i] / sum
		weights = append(weights, &commonv1.StrategyWeight{
			StrategyId: id, Weight: w, Expectancy: stSafe(st), WinRate: wr, Regime: "mixed",
		})
	}
	sort.Slice(weights, func(i, j int) bool { return weights[i].Weight > weights[j].Weight })
	return &learningv1.GetWeightsResponse{Weights: weights}, nil
}

func stSafe(st *stat) float64 {
	if st == nil {
		return 0
	}
	return st.excEMA
}

func attribute(t *commonv1.PaperTrade) (string, []string) {
	var tags []string
	if t.Pnl > 0 {
		tags = append(tags, "win")
	} else {
		tags = append(tags, "loss")
	}
	if t.ExcessReturn > 0 {
		tags = append(tags, "helped_vs_nifty")
	} else {
		tags = append(tags, "hurt_vs_nifty")
	}
	if len(t.InvestigationIds) > 0 {
		tags = append(tags, "investigation_linked")
	}
	lesson := ""
	if t.Pnl > 0 && t.ExcessReturn > 0 {
		lesson = t.StrategyId + " captured a move that outpaced Nifty — keep weight if regime persists."
	} else if t.Pnl <= 0 && t.ExcessReturn <= 0 {
		lesson = t.StrategyId + " lagged Nifty on this name; downweight until expectancy recovers."
	} else if t.Pnl > 0 {
		lesson = "Absolute profit but no excess vs Nifty — size down versus the index, not just P&L."
	} else {
		lesson = "Loss in rupees but relative tape was not the issue; review stop placement."
	}
	return lesson, tags
}

func main() {
	addr := os.Getenv("LEARNING_BIND")
	if addr == "" {
		addr = ":9083"
	}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	gs := grpc.NewServer()
	learningv1.RegisterLearningServiceServer(gs, newServer())
	log.Printf("learning listening on %s", addr)
	log.Fatal(gs.Serve(lis))
}
