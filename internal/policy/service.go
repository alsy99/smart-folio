// Package policy turns an Investment Policy Statement into targets. It
// decides; trading executes. Nothing here reads a headline, calls a
// strategy, or talks to an LLM.
package policy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	policyv1 "aperture/gen/policy/v1"
	"aperture/pkg/ips"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Frozen answers "is this IPS bound to a frozen campaign, and under which
// hash?". A frozen statement cannot change; that mirrors the weight
// freeze on the public campaign.
type Frozen func(ipsID string) (hash string, frozen bool)

// BookSource is the live paper book seen as allocator input. Trading
// implements it; policy reads it for PreviewTargets and never writes.
type BookSource interface {
	CoreInput(ctx context.Context) (Input, error)
}

// Bound is told when a statement is accepted so the paper book can start
// under it the same day.
type Bound func(ips.IPS)

var ErrNoIPS = errors.New("policy: no IPS bound to the book")

type Deps struct {
	Store     Store
	Available ips.Available // benchmark has a series on the active tape
	Frozen    Frozen
	Book      BookSource
	OnPut     Bound
	Now       func() time.Time
	Log       *slog.Logger
}

type Service struct {
	policyv1.UnimplementedPolicyServiceServer
	store  Store
	avail  ips.Available
	frozen Frozen
	book   BookSource
	onPut  Bound
	now    func() time.Time
	log    *slog.Logger
}

func New(d Deps) *Service {
	if d.Store == nil {
		d.Store = NewMemStore()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Frozen == nil {
		d.Frozen = func(string) (string, bool) { return "", false }
	}
	return &Service{store: d.Store, avail: d.Available, frozen: d.Frozen, book: d.Book, onPut: d.OnPut, now: d.Now, log: d.Log}
}

const LogIPSPut = "IPS_PUT"

// PutIPS validates and stores a statement. Invalid statements are
// InvalidArgument; a statement bound to a frozen campaign is
// FailedPrecondition unless the bytes are identical.
func (s *Service) PutIPS(_ context.Context, req *policyv1.PutIPSRequest) (*policyv1.IPS, error) {
	if req == nil || req.Ips == nil {
		return nil, status.Error(codes.InvalidArgument, "ips required")
	}
	p := FromProto(req.Ips)
	if strings.TrimSpace(p.ID) == "" {
		return nil, status.Error(codes.InvalidArgument, ips.ErrID.Error())
	}
	if err := p.ValidateOn(s.avail); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if hash, frozen := s.frozen(p.ID); frozen && hash != p.Hash() {
		return nil, status.Errorf(codes.FailedPrecondition,
			"ips %s is bound to a frozen campaign (hash %s); a statement cannot change mid-campaign", p.ID, hash)
	}
	if err := s.store.Put(p); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	s.log.Info(LogIPSPut, "id", p.ID, "hash", p.Hash(), "line", p.Line())
	if s.onPut != nil {
		s.onPut(p)
	}
	return ToProto(p), nil
}

// PreviewTargets shows what the core wants against what the book holds:
// targets, drift per name, whether today is a rebalance session, and the
// next one. It reads the book; it never writes a ticket.
func (s *Service) PreviewTargets(ctx context.Context, req *policyv1.PreviewTargetsRequest) (*policyv1.Targets, error) {
	p, err := s.store.Get(req.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("ips %q not found", req.GetId()))
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	in := Input{IPS: p, Now: s.now(), SatelliteEmpty: true, CoreQty: map[string]float64{}, CoreHeld: map[string]float64{}}
	if s.book != nil {
		if live, err := s.book.CoreInput(ctx); err == nil && live.IPS.ID == p.ID {
			in = live
			in.IPS = p
		}
	}
	plan := Preview(in)
	out := &policyv1.Targets{
		IpsId: p.ID, IpsHash: p.Hash(), AsOfUnixMs: in.Now.UnixMilli(),
		CorePct: plan.Targets.CorePct, SatelliteCap: p.SatellitePct, Cash: plan.Targets.Cash,
		Reason: plan.Reason, NextRebalance: plan.NextRebalance, RebalanceSession: plan.Session,
	}
	for _, d := range plan.Drifts {
		out.Core = append(out.Core, &policyv1.CoreWeight{Symbol: d.Symbol, Target: d.Target, Actual: d.Actual, Drift: d.Drift, Ticket: d.Ticket})
	}
	return out, nil
}

func (s *Service) GetIPS(_ context.Context, req *policyv1.GetIPSRequest) (*policyv1.IPS, error) {
	p, err := s.store.Get(req.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("ips %q not found", req.GetId()))
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return ToProto(p), nil
}

func FromProto(m *policyv1.IPS) ips.IPS {
	return ips.IPS{
		ID:            strings.TrimSpace(m.GetId()),
		Goal:          ips.Goal(strings.TrimSpace(m.GetGoal())),
		HorizonYears:  int(m.GetHorizonYears()),
		MaxDD:         m.GetMaxDd(),
		Benchmark:     ips.Benchmark(strings.ToUpper(strings.TrimSpace(m.GetBenchmark()))),
		CorePct:       m.GetCorePct(),
		SatellitePct:  m.GetSatellitePct(),
		Rebalance:     ips.Rebalance(strings.TrimSpace(m.GetRebalance())),
		StartCash:     m.GetStartCash(),
		FoldSatellite: m.GetFoldSatellite(),
	}
}

func ToProto(p ips.IPS) *policyv1.IPS {
	return &policyv1.IPS{
		Id: p.ID, Goal: string(p.Goal), HorizonYears: int32(p.HorizonYears), MaxDd: p.MaxDD,
		Benchmark: string(p.Benchmark), CorePct: p.CorePct, SatellitePct: p.SatellitePct,
		Rebalance: string(p.Rebalance), StartCash: p.StartCash, FoldSatellite: p.FoldSatellite,
		Hash: p.Hash(), Line: p.Line(),
	}
}
