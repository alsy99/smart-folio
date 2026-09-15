package indstocks

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"aperture/pkg/universe"
)

// Scrip is SEGMENT_SECURITYID, e.g. NSE_2885.
type Scrip struct {
	Code     string
	Symbol   string
	Exchange string
	Token    string
}

func (s Scrip) Valid() bool { return s.Code != "" }

func parseEquityCSV(r io.Reader) (map[string]Scrip, error) {
	cr := csv.NewReader(r)
	cr.ReuseRecord = true
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	head, err := cr.Read()
	if err != nil {
		return nil, err
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.ToUpper(strings.TrimSpace(h))] = i
	}
	need := []string{"EXCH", "SECURITY_ID", "SERIES", "TRADING_SYMBOL"}
	for _, n := range need {
		if _, ok := col[n]; !ok {
			return nil, fmt.Errorf("instruments csv missing %s", n)
		}
	}
	out := map[string]Scrip{}
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		get := func(k string) string {
			i, ok := col[k]
			if !ok || i >= len(rec) {
				return ""
			}
			return strings.TrimSpace(rec[i])
		}
		if !strings.EqualFold(get("EXCH"), "NSE") {
			continue
		}
		series := strings.ToUpper(get("SERIES"))
		if series != "EQ" && series != "E" && series != "BE" {
			continue
		}
		sym := strings.ToUpper(get("TRADING_SYMBOL"))
		if i := strings.IndexByte(sym, '-'); i > 0 {
			sym = sym[:i]
		}
		if sym == "" {
			sym = strings.ToUpper(get("SYMBOL_NAME"))
		}
		id := get("SECURITY_ID")
		if sym == "" || id == "" {
			continue
		}
		if _, exists := out[sym]; exists {
			continue
		}
		out[sym] = Scrip{Code: "NSE_" + id, Symbol: strings.Clone(sym), Exchange: "NSE", Token: strings.Clone(id)}
	}
	return out, nil
}

func parseIndexCSV(r io.Reader) (map[string]Scrip, error) {
	cr := csv.NewReader(r)
	cr.ReuseRecord = true
	_, err := cr.Read() // header
	if err != nil {
		return nil, err
	}
	out := map[string]Scrip{}
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) < 3 {
			continue
		}
		exch := strings.ToUpper(strings.TrimSpace(rec[0]))
		name := strings.ToUpper(strings.TrimSpace(rec[1]))
		id := strings.TrimSpace(rec[2])
		if id == "" || name == "" {
			continue
		}
		out[name] = Scrip{Code: exch + "_" + id, Symbol: strings.Clone(name), Exchange: exch, Token: strings.Clone(id)}
	}
	return out, nil
}

var indexAliases = map[string][]string{
	"NIFTY50":  {"NIFTY 50", "NIFTY50", "NIFTY"},
	"NIFTY500": {"NIFTY 500", "NIFTY500"},
	"SENSEX":   {"SENSEX", "BSE SENSEX", "S&P BSE SENSEX", "BSE SENSEX 30"},
}

func mapUniverse(equity, index map[string]Scrip) map[string]Scrip {
	out := map[string]Scrip{}
	for _, inst := range universe.All() {
		if inst.IsBenchmark {
			if aliases, ok := indexAliases[inst.Symbol]; ok {
				for _, a := range aliases {
					if s, ok := index[a]; ok {
						s.Symbol = inst.Symbol
						out[inst.Symbol] = s
						break
					}
				}
			}
			continue
		}
		if s, ok := equity[inst.Symbol]; ok {
			out[inst.Symbol] = s
		}
	}
	return out
}

func quoteable(symbol string) bool {
	inst, ok := universe.Lookup(symbol)
	return ok && !inst.IsBenchmark
}
