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
	if again.Line != got.Line || again.Line == "" {
		t.Fatalf("line %q after restart, was %q", again.Line, got.Line)
	}
	anon, err := b.GetIPS(context.Background(), &policyv1.GetIPSRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if anon.Id != again.Id || anon.Line != again.Line {
		t.Fatalf("GET with no id must return the bound statement, got %+v", anon)
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

func TestBootEmptyCreatesIPSA(t *testing.T) {
	dir := t.TempDir()
	st := NewFileStore(dir)
	p, err := Boot(st, "", 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != ips.IDA {
		t.Fatalf("want IPS A %s, got %s", ips.IDA, p.ID)
	}
	again, err := NewFileStore(dir).Bound()
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != p.ID || again.Line() != p.Line() {
		t.Fatalf("BOUND after restart %+v vs %+v", again, p)
	}
}

func TestBootKeepsPutStatement(t *testing.T) {
	dir := t.TempDir()
	svc := New(Deps{Store: NewFileStore(dir)})
	put, err := svc.PutIPS(context.Background(), req(func(m *policyv1.IPS) { m.Id = "test" }))
	if err != nil {
		t.Fatal(err)
	}
	p, err := Boot(NewFileStore(dir), "", 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "test" || p.Line() != put.Line {
		t.Fatalf("boot dropped the put statement: %+v want line %s", p, put.Line)
	}
}
