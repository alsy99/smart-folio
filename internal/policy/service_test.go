package policy

import (
	"context"
	"testing"

	policyv1 "aperture/gen/policy/v1"
	"aperture/pkg/ips"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func req(mut func(*policyv1.IPS)) *policyv1.PutIPSRequest {
	m := ToProto(ips.Default("c-1", 1_000_000))
	if mut != nil {
		mut(m)
	}
	return &policyv1.PutIPSRequest{Ips: m}
}

func TestPutIPSRejectsOutsideWalls(t *testing.T) {
	svc := New(Deps{})
	cases := []struct {
		name string
		mut  func(*policyv1.IPS)
	}{
		{"max_dd 0.20", func(m *policyv1.IPS) { m.MaxDd = 0.20 }},
		{"core 0.5", func(m *policyv1.IPS) { m.CorePct = 0.5; m.SatellitePct = 0.5 }},
		{"missing benchmark", func(m *policyv1.IPS) { m.Benchmark = "" }},
		{"guaranteed 10%", func(m *policyv1.IPS) { m.Goal = "guaranteed 10%" }},
		{"no id", func(m *policyv1.IPS) { m.Id = "" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.PutIPS(context.Background(), req(c.mut))
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("want InvalidArgument, got %v", err)
			}
		})
	}
	if _, err := svc.GetIPS(context.Background(), &policyv1.GetIPSRequest{Id: "c-1"}); status.Code(err) != codes.NotFound {
		t.Fatalf("a rejected statement must not be stored, got %v", err)
	}
}

func TestPutIPSChecksBenchmarkOnTape(t *testing.T) {
	svc := New(Deps{Available: func(b ips.Benchmark) bool { return b == ips.BenchNifty50 }})
	_, err := svc.PutIPS(context.Background(), req(func(m *policyv1.IPS) { m.Benchmark = "SENSEX" }))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SENSEX has no series on this tape, got %v", err)
	}
	if _, err := svc.PutIPS(context.Background(), req(nil)); err != nil {
		t.Fatal(err)
	}
}

func TestHashSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	a := New(Deps{Store: NewFileStore(dir)})
	got, err := a.PutIPS(context.Background(), req(nil))
	if err != nil {
		t.Fatal(err)
	}
	// New process, same directory.
	b := New(Deps{Store: NewFileStore(dir)})
	again, err := b.GetIPS(context.Background(), &policyv1.GetIPSRequest{Id: "c-1"})
	if err != nil {
		t.Fatal(err)
	}
	if again.Hash != got.Hash || again.Hash == "" {
		t.Fatalf("hash %q after restart, was %q", again.Hash, got.Hash)
	}
	if again.Hash != ips.Default("c-1", 1_000_000).Hash() {
		t.Fatal("stored statement must hash like the accepted one")
	}
}

func TestFrozenIPSCannotChange(t *testing.T) {
	bound := ips.Default("c-1", 1_000_000)
	svc := New(Deps{Frozen: func(id string) (string, bool) {
		if id == "c-1" {
			return bound.Hash(), true
		}
		return "", false
	}})
	// Same bytes: idempotent re-put is fine.
	if _, err := svc.PutIPS(context.Background(), req(nil)); err != nil {
		t.Fatal(err)
	}
	// Any change is refused while the campaign is frozen.
	_, err := svc.PutIPS(context.Background(), req(func(m *policyv1.IPS) { m.MaxDd = 0.10 }))
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("want FailedPrecondition, got %v", err)
	}
	// Another client is unaffected.
	if _, err := svc.PutIPS(context.Background(), req(func(m *policyv1.IPS) { m.Id = "c-2"; m.MaxDd = 0.10 })); err != nil {
		t.Fatal(err)
	}
}

func TestFileStoreRejectsPathTricks(t *testing.T) {
	s := NewFileStore(t.TempDir())
	p := ips.Default("../evil", 1)
	if err := s.Put(p); err == nil {
		t.Fatal("id with path separators must be refused")
	}
}
