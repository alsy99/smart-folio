package indstocks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestScorePulsePutHeavy(t *testing.T) {
	ladder := map[string]chainStrike{
		"24500": {
			CE: chainLeg{OI: 100, IV: 10},
			PE: chainLeg{OI: 180, IV: 11},
		},
		"24600": {
			CE: chainLeg{OI: 80, IV: 10.5},
			PE: chainLeg{OI: 160, IV: 11.2},
		},
	}
	p := scorePulse("2026-09-22", 24510, ladder)
	if p.PCR < 1.5 {
		t.Fatalf("pcr %v", p.PCR)
	}
	if p.Tilt >= 0 {
		t.Fatalf("expected defensive tilt, got %v", p.Tilt)
	}
	if p.Headline == "" {
		t.Fatal("headline")
	}
}

func TestPulseOptionChain(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/market/instruments", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("source") == "index" {
			_, _ = w.Write([]byte("EXCH,SEGMENT,SECURITY_ID\nNSE,NIFTY 50,40000001\n"))
			return
		}
		_, _ = w.Write([]byte("EXCH,SEGMENT,SECURITY_ID,INSTRUMENT_NAME,EXPIRY_CODE,TRADING_SYMBOL,LOT_UNITS,CUSTOM_SYMBOL,EXPIRY_DATE,STRIKE_PRICE,OPTION_TYPE,TICK_SIZE,EXPIRY_FLAG,SEM_EXCH_INSTRUMENT_TYPE,SERIES,SYMBOL_NAME\nNSE,E,2885,EQUITY,0,RELIANCE,1,RELIANCE INDUSTRIES LTD,,,,0.05,,ES,EQ,Reliance Industries Ltd\n"))
	})
	mux.HandleFunc("/market/instruments/expiries", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": []string{"2026-09-15", "2026-09-22"}})
	})
	mux.HandleFunc("/market/option-chain", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("underlying-scrip") != "40000001" {
			http.Error(w, "bad scrip", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"underlying_ltp": 24500.0,
				"expiry":         "2026-09-22",
				"strikes": map[string]any{
					"24500": map[string]any{
						"ce": map[string]any{"oi": 1000.0, "iv": 12.0},
						"pe": map[string]any{"oi": 700.0, "iv": 11.5},
					},
				},
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := &Client{http: srv.Client(), baseURL: srv.URL, token: "t", cache: t.TempDir(), quotes: map[string]Quote{}, bars: map[string]barSet{}}
	p, err := c.Pulse(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if p.PCR < 0.6 || p.PCR > 0.8 {
		t.Fatalf("pcr %v", p.PCR)
	}
	if p.Tilt <= 0 {
		t.Fatalf("call-heavy should tilt up, got %v", p.Tilt)
	}
	rep := p.Report()
	if rep.GetHeadline() == "" || len(rep.GetSymbols()) == 0 {
		t.Fatalf("%+v", rep)
	}
}
