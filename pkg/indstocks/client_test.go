package indstocks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseEquityAndIndex(t *testing.T) {
	eqCSV := `EXCH,SEGMENT,SECURITY_ID,INSTRUMENT_NAME,EXPIRY_CODE,TRADING_SYMBOL,LOT_UNITS,CUSTOM_SYMBOL,EXPIRY_DATE,STRIKE_PRICE,OPTION_TYPE,TICK_SIZE,EXPIRY_FLAG,SEM_EXCH_INSTRUMENT_TYPE,SERIES,SYMBOL_NAME
NSE,E,2885,EQUITY,0,RELIANCE,1,RELIANCE INDUSTRIES LTD,,,,0.05,,ES,EQ,Reliance Industries Ltd
NSE,E,11536,EQUITY,0,TCS,1,TATA CONSULTANCY SERV LT,,,,0.05,,ES,EQ,Tata Consultancy Services Ltd
BSE,E,500325,EQUITY,0,RELIANCE,1,RELIANCE INDUSTRIES LTD,,,,0.05,,ES,A,Reliance Industries Ltd
`
	eq, err := parseEquityCSV(strings.NewReader(eqCSV))
	if err != nil {
		t.Fatal(err)
	}
	if eq["RELIANCE"].Code != "NSE_2885" {
		t.Fatalf("reliance %q", eq["RELIANCE"].Code)
	}
	idxCSV := `EXCH,SEGMENT,SECURITY_ID
NSE,NIFTY 50,40000001
NSE,NIFTY 500,40000002
BSE,SENSEX,1
`
	idx, err := parseIndexCSV(strings.NewReader(idxCSV))
	if err != nil {
		t.Fatal(err)
	}
	m := mapUniverse(eq, idx)
	if m["RELIANCE"].Code != "NSE_2885" {
		t.Fatalf("mapped reliance")
	}
	if m["NIFTY50"].Code != "NSE_40000001" {
		t.Fatalf("nifty %q", m["NIFTY50"].Code)
	}
	if m["SENSEX"].Code != "BSE_1" {
		t.Fatalf("sensex %q", m["SENSEX"].Code)
	}
}

func TestQuotesAndBars(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/user/profile", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"user_id": "1"}})
	})
	mux.HandleFunc("/market/instruments", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("source") == "index" {
			_, _ = w.Write([]byte("EXCH,SEGMENT,SECURITY_ID\nNSE,NIFTY 50,40000001\n"))
			return
		}
		_, _ = w.Write([]byte("EXCH,SEGMENT,SECURITY_ID,INSTRUMENT_NAME,EXPIRY_CODE,TRADING_SYMBOL,LOT_UNITS,CUSTOM_SYMBOL,EXPIRY_DATE,STRIKE_PRICE,OPTION_TYPE,TICK_SIZE,EXPIRY_FLAG,SEM_EXCH_INSTRUMENT_TYPE,SERIES,SYMBOL_NAME\nNSE,E,2885,EQUITY,0,RELIANCE,1,RELIANCE,,,,0.05,,EQ,EQ,RELIANCE\n"))
	})
	mux.HandleFunc("/market/quotes/full", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "test-token" {
			http.Error(w, `{"status":"error","message":"no auth"}`, http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"NSE_2885": map[string]any{"live_price": 1401.25, "day_change_percentage": 1.2, "volume": 1000},
			},
		})
	})
	mux.HandleFunc("/market/historical/15minute", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"NSE_2885": map[string]any{
					"candles": []map[string]any{
						{"ts": 1700000000, "o": 10, "h": 11, "l": 9, "c": 10.5, "v": 100},
						{"ts": 1700000900, "o": 10.5, "h": 12, "l": 10, "c": 11, "v": 110},
					},
				},
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{
		http:    srv.Client(),
		baseURL: srv.URL,
		token:   "test-token",
		cache:   t.TempDir(),
		quotes:  map[string]Quote{},
	}
	if err := c.Profile(context.Background()); err != nil {
		t.Fatal(err)
	}
	qs, err := c.Quotes(context.Background(), []string{"RELIANCE", "MF_LARGECAP"})
	if err != nil {
		t.Fatal(err)
	}
	if qs["RELIANCE"].LivePrice != 1401.25 {
		t.Fatalf("got %+v", qs["RELIANCE"])
	}
	if _, ok := qs["MF_LARGECAP"]; ok {
		t.Fatal("mf should be unmapped")
	}
	bars, err := c.Bars(context.Background(), "RELIANCE", "15m", 40, time.Unix(1700000900, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 2 || bars[1].Close != 11 {
		t.Fatalf("bars %+v", bars)
	}
}

func TestTOTPLength(t *testing.T) {
	// RFC 6238 appendix B seed "12345678901234567890" as ASCII is not base32;
	// just check we produce 6 digits for a valid base32 secret.
	code, err := totpCode("JBSWY3DPEHPK3PXP", time.Unix(59, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 6 {
		t.Fatalf("got %q", code)
	}
}

func TestFromEnvAlias(t *testing.T) {
	t.Setenv("INDSTOCKS_ACCESS_TOKEN", "")
	t.Setenv("INDMONEY_ACCESS_TOKEN", "")
	t.Setenv("INDMONEY_API_TOKEN", "alias-token")
	c, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c == nil || c.token != "alias-token" {
		t.Fatal("expected INDMONEY_API_TOKEN alias")
	}
}

func TestFromEnvEmpty(t *testing.T) {
	t.Setenv("INDSTOCKS_ACCESS_TOKEN", "")
	t.Setenv("INDMONEY_API_TOKEN", "")
	t.Setenv("INDMONEY_ACCESS_TOKEN", "")
	t.Setenv("INDSTOCKS_API_KEY", "")
	t.Setenv("INDMONEY_CLIENT_ID", "")
	t.Setenv("INDSTOCKS_MPIN", "")
	t.Setenv("INDSTOCKS_TOTP_SECRET", "")
	c, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c != nil {
		t.Fatal("expected nil client")
	}
}

func TestLiveQuotesAndBars(t *testing.T) {
	if os.Getenv("INDSTOCKS_LIVE") != "1" {
		t.Skip("set INDSTOCKS_LIVE=1 to hit api.indstocks.com")
	}
	c, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("expected client from INDMONEY_API_TOKEN")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := c.EnsureScrips(ctx); err != nil {
		t.Fatal(err)
	}
	if c.ScripCount() < 12 {
		t.Fatalf("mapped scrips %d", c.ScripCount())
	}
	qs, err := c.Quotes(ctx, []string{"RELIANCE", "TCS", "HDFCBANK"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sym := range []string{"RELIANCE", "TCS", "HDFCBANK"} {
		if qs[sym].LivePrice <= 0 {
			t.Fatalf("%s missing live price: %+v", sym, qs[sym])
		}
		t.Logf("%s last=%.2f chg=%.2f vol=%.0f", sym, qs[sym].LivePrice, qs[sym].DayChangePct, qs[sym].Volume)
	}
	bars, err := c.Bars(ctx, "RELIANCE", "1d", 8, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) < 3 {
		t.Fatalf("bars %d", len(bars))
	}
	last := bars[len(bars)-1]
	t.Logf("reliance 1d bars=%d last_close=%.2f vol=%.0f", len(bars), last.Close, last.Volume)
}

func TestCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := &Client{cache: dir}
	c.saveCached("equity.csv", map[string]Scrip{"RELIANCE": {Code: "NSE_2885", Symbol: "RELIANCE"}})
	if _, err := os.Stat(filepath.Join(dir, "indstocks-equity.csv")); err != nil {
		t.Fatal(err)
	}
	got := c.loadCached("equity.csv")
	if got["RELIANCE"].Code != "NSE_2885" {
		t.Fatalf("%+v", got)
	}
}
