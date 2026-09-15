package fundamentals

import (
	"time"

	"aperture/pkg/asof"
	"aperture/pkg/universe"
)

// Card is a qualitative reference sheet — not live filings or a Bloomberg print.
// RevisedAt is the as-of timestamp; a revision after the bar is not used.
type Card struct {
	Symbol    string
	Name      string
	Sector    string
	Model     string
	Drivers   string
	Watch     string
	Risks     string
	RevisedAt time.Time
}

var cardAsOf = time.Date(2024, 1, 1, 9, 15, 0, 0, time.UTC)

func Lookup(symbol string) Card {
	return LookupAsOf(symbol, time.Now())
}

// LookupAsOf returns the card only if its revision is known at the bar.
func LookupAsOf(symbol string, bar time.Time) Card {
	inst, _ := universe.Lookup(symbol)
	fallback := Card{
		Symbol: symbol, Name: inst.Name, Sector: inst.Sector,
		Model:   "India cash equity on the Aperture universe.",
		Drivers: "Tape, news investigations, and peer relative strength vs Nifty.",
		Watch:   "Event type, corroboration, and whether the setup is trend or mean-reversion.",
		Risks:   "No live 10-Q; treat this card as a desk briefing not a filing.",
		RevisedAt: cardAsOf,
	}
	c, ok := cards[symbol]
	if !ok {
		c = fallback
	} else {
		c.RevisedAt = cardAsOf
		c.Name = inst.Name
		c.Sector = inst.Sector
	}
	if bar.IsZero() {
		return c
	}
	if !asof.KnownAt(c.RevisedAt, bar) {
		return Card{
			Symbol: symbol, Name: inst.Name, Sector: inst.Sector,
			RevisedAt: c.RevisedAt,
			Model:     "No fundamental card as-of this bar.",
			Drivers:   "Card revision is after the bar — ignored.",
			Watch:     "Wait for a dated revision at or before the bar.",
			Risks:     "Look-ahead: do not use a later briefing on an earlier fill.",
		}
	}
	return c
}

func (c Card) Summary() string {
	return c.Name + " (" + c.Sector + "). Model: " + c.Model + " Drivers: " + c.Drivers + " Watch: " + c.Watch + " Risks: " + c.Risks
}

var cards = map[string]Card{
	"RELIANCE": {
		Symbol:  "RELIANCE",
		Model:   "Conglomerate: refining/petchem, Jio telecom, retail, and new energy optionality.",
		Drivers: "GRM/crack spreads, Jio ARPU and capex cycle, retail SSS, oil-to-chemicals integration.",
		Watch:   "Jio IPO/fundraising headlines, refining margin tape, promoter/group capex guidance.",
		Risks:   "Energy cycle, regulatory on telecom, execution on new energy, conglomerate discount.",
	},
	"TCS": {
		Symbol:  "TCS",
		Model:   "Global IT services: large-deal TCV, pyramid utilisation, and discretionary vs run-the-business mix.",
		Drivers: "Deal wins, attrition, pricing, BFSI/retail client spend, USD/INR.",
		Watch:   "Large-deal TCV, guidance band, hiring freeze vs ramp, vertical commentary.",
		Risks:   "Client budget cuts, vendor consolidation, wage inflation, rupee strength.",
	},
	"INFY": {
		Symbol:  "INFY",
		Model:   "IT services peer to TCS; more guidance-sensitive and large-deal dependent.",
		Drivers: "Guidance, large-deal TCV, utilisation, digital mix.",
		Watch:   "Guidance raises/cuts, deal pipeline, employee utilisation.",
		Risks:   "Same as TCS plus higher perceived guidance risk vs Street.",
	},
	"HDFCBANK": {
		Symbol:  "HDFCBANK",
		Model:   "Private-sector deposit franchise; NIM, CASA, and unsecured mix dominate earnings.",
		Drivers: "Deposit growth vs credit, CASA mix, NIM, slippages, LDR after HDFC Ltd merger digestion.",
		Watch:   "Fortnightly deposit data, NIM commentary, unsecured/microfinance stress, CD ratio.",
		Risks:   "Liability franchise rebuild, NIM compression, retail unsecured cycle, RBI measures.",
	},
	"ICICIBANK": {
		Symbol:  "ICICIBANK",
		Model:   "Diversified private bank with stronger recent asset-quality optics vs some peers.",
		Drivers: "Loan growth mix, NIM, fee income, credit costs.",
		Watch:   "Slippage ratio, deposit pricing, retail vs corporate mix.",
		Risks:   "Credit cycle, deposit competition, market-linked treasury swings.",
	},
	"SBIN": {
		Symbol:  "SBIN",
		Model:   "Systemically important PSU bank; credit growth, bond book, and operating leverage.",
		Drivers: "Loan growth vs system, NIM, slippage, treasury given duration of G-Sec book.",
		Watch:   "RBI liquidity/rate path, credit growth prints, PSU recap/dividend chatter.",
		Risks:   "Rate-driven MTM, corporate stress, political/PSU governance overlay.",
	},
	"AXISBANK": {
		Symbol:  "AXISBANK",
		Model:   "Private bank still earning back a quality/funding premium; wholesale and retail mix.",
		Drivers: "Deposit traction, NIM, unsecured, fee lines, occasional promoter/block flow.",
		Watch:   "CASA, wholesale book growth, one-off provisions, block-deal headlines.",
		Risks:   "Funding cost, credit costs, event-driven flow around residual promoter stakes.",
	},
	"KOTAKBANK": {
		Symbol:  "KOTAKBANK",
		Model:   "Conservative private bank + securities/AMC ecosystem; growth vs premium valuation.",
		Drivers: "Loan growth (esp. unsecured/consumer), NIM, capital markets cycle.",
		Watch:   "Management growth stance, wholesale vs retail mix, competitive pricing.",
		Risks:   "Growth gap vs peers, valuation de-rate, capital-markets downturn.",
	},
	"BHARTIARTL": {
		Symbol:  "BHARTIARTL",
		Model:   "India wireless + Africa; ARPU, 4G/5G mix, and tariff-hike absorption.",
		Drivers: "ARPU, subscriber quality, capex intensity, Africa FX, tower/infra.",
		Watch:   "Tariff hikes holding, 5G traffic mix, regulatory AGR/spectrum.",
		Risks:   "Price war relapse, spectrum/AGR, leverage, Africa currency.",
	},
	"ITC": {
		Symbol:  "ITC",
		Model:   "Cigarettes cash engine funding FMCG/hotels/paper; demerger optionality.",
		Drivers: "Cigarette volume/tax, FMCG margin, hotels cycle, any hotel demerger timeline.",
		Watch:   "Excise/tax, rural volume, hotel demerger rumors vs board notes.",
		Risks:   "Sin-tax, ESG ownership constraints, rumours without filings.",
	},
	"LT": {
		Symbol:  "LT",
		Model:   "Infra + hydrocarbon + services; order inflow and execution margins.",
		Drivers: "Domestic + Middle East order inflow, working capital, margin mix.",
		Watch:   "Order inflow surprises, guidance, execution delays, Hyderabad/metro-type projects.",
		Risks:   "Working-capital stretch, commodity inflation, project delays.",
	},
	"HINDUNILVR": {
		Symbol:  "HINDUNILVR",
		Model:   "Staples: volume vs mix, rural recovery, premium vs mass, gross margin vs RM.",
		Drivers: "Volume growth, mix, commodity deflation/inflation, rural vs urban.",
		Watch:   "Rural commentary, competitive intensity (local/D2C), RM basket.",
		Risks:   "Volume miss, downtrading, competitive pricing, valuation.",
	},
}
