package universe

type Instrument struct {
	Symbol      string
	Name        string
	Exchange    string
	Sector      string
	Currency    string
	BasePrice   float64
	IsBenchmark bool
}

func Equities() []Instrument {
	return []Instrument{
		{Symbol: "RELIANCE", Name: "Reliance Industries", Exchange: "NSE", Sector: "Energy", Currency: "INR", BasePrice: 2984},
		{Symbol: "TCS", Name: "Tata Consultancy Services", Exchange: "NSE", Sector: "IT", Currency: "INR", BasePrice: 3921},
		{Symbol: "HDFCBANK", Name: "HDFC Bank", Exchange: "NSE", Sector: "Banks", Currency: "INR", BasePrice: 1724},
		{Symbol: "INFY", Name: "Infosys", Exchange: "NSE", Sector: "IT", Currency: "INR", BasePrice: 1512},
		{Symbol: "ICICIBANK", Name: "ICICI Bank", Exchange: "NSE", Sector: "Banks", Currency: "INR", BasePrice: 1288},
		{Symbol: "SBIN", Name: "State Bank of India", Exchange: "NSE", Sector: "Banks", Currency: "INR", BasePrice: 812},
		{Symbol: "BHARTIARTL", Name: "Bharti Airtel", Exchange: "NSE", Sector: "Telecom", Currency: "INR", BasePrice: 1654},
		{Symbol: "ITC", Name: "ITC", Exchange: "NSE", Sector: "FMCG", Currency: "INR", BasePrice: 462},
		{Symbol: "LT", Name: "Larsen & Toubro", Exchange: "NSE", Sector: "Industrials", Currency: "INR", BasePrice: 3610},
		{Symbol: "KOTAKBANK", Name: "Kotak Mahindra Bank", Exchange: "NSE", Sector: "Banks", Currency: "INR", BasePrice: 1788},
		{Symbol: "AXISBANK", Name: "Axis Bank", Exchange: "NSE", Sector: "Banks", Currency: "INR", BasePrice: 1096},
		{Symbol: "HINDUNILVR", Name: "Hindustan Unilever", Exchange: "NSE", Sector: "FMCG", Currency: "INR", BasePrice: 2418},
	}
}

func Benchmarks() []Instrument {
	return []Instrument{
		{Symbol: "NIFTY50", Name: "Nifty 50", Exchange: "NSE", Sector: "Index", Currency: "INR", BasePrice: 24980, IsBenchmark: true},
		{Symbol: "NIFTY500", Name: "Nifty 500", Exchange: "NSE", Sector: "Index", Currency: "INR", BasePrice: 22840, IsBenchmark: true},
		{Symbol: "SENSEX", Name: "S&P BSE Sensex", Exchange: "BSE", Sector: "Index", Currency: "INR", BasePrice: 81720, IsBenchmark: true},
		{Symbol: "MF_LARGECAP", Name: "Large-cap MF peer (proxy)", Exchange: "MF", Sector: "Fund", Currency: "INR", BasePrice: 142.8, IsBenchmark: true},
		{Symbol: "MF_FLEXI", Name: "Flexi-cap MF peer (proxy)", Exchange: "MF", Sector: "Fund", Currency: "INR", BasePrice: 98.4, IsBenchmark: true},
	}
}

func All() []Instrument {
	out := Equities()
	return append(out, Benchmarks()...)
}

func EquitySymbols() []string {
	eq := Equities()
	out := make([]string, len(eq))
	for i, e := range eq {
		out[i] = e.Symbol
	}
	return out
}

func Keywords(symbol string) []string {
	m := map[string][]string{
		"RELIANCE":    {"reliance", "ril", "mukesh ambani", "jio", "jamnagar"},
		"TCS":         {"tcs", "tata consultancy", "tata"},
		"HDFCBANK":    {"hdfc bank", "hdfc"},
		"INFY":        {"infosys", "infy", "nandan nilekani"},
		"ICICIBANK":   {"icici", "icici bank"},
		"SBIN":        {"sbi", "state bank"},
		"BHARTIARTL":  {"airtel", "bharti"},
		"ITC":         {"itc limited", "itc"},
		"LT":          {"larsen", "l&t", "larsen & toubro"},
		"KOTAKBANK":   {"kotak"},
		"AXISBANK":    {"axis bank"},
		"HINDUNILVR":  {"hul", "unilever", "hindustan unilever"},
		"NIFTY50":     {"nifty", "nse"},
		"SENSEX":      {"sensex", "bse"},
	}
	return m[symbol]
}

func Lookup(symbol string) (Instrument, bool) {
	for _, i := range All() {
		if i.Symbol == symbol {
			return i, true
		}
	}
	return Instrument{}, false
}
