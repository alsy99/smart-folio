package campaign

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aperture/pkg/ips"
)

// SpecFile names the per-campaign policy file. The legacy satellite-only
// campaign has none; a policy campaign binds one IPS to its ledger here.
const SpecFile = "campaign.json"

// Spec is a policy campaign's checked-in definition.
type Spec struct {
	Name string `json:"name"`
	// IPS is the client policy the book ran under; its hash lands in the
	// manifest and freezes with the ledger.
	IPS *ips.IPS `json:"ips,omitempty"`
	// Note is the one-paragraph "what this book is" for the README table.
	Note string `json:"note,omitempty"`
	// Shock is an optional documented synthetic move applied to bars.json
	// at load: every series' O/H/L/C on and after From is multiplied by
	// Factor. It exists to exercise the halt path on a tape that did not
	// fall enough on its own, and the manifest says so in Tape and
	// Synthetic. bars.json itself is untouched and keeps its sha256.
	Shock *Shock `json:"shock,omitempty"`
}

type Shock struct {
	From   string  `json:"from"`
	Factor float64 `json:"factor"`
	Note   string  `json:"note,omitempty"`
}

// Synthetic is the tape suffix a shocked replay carries.
const Synthetic = "+synthetic-shock"

func specPath(dir string) string { return filepath.Join(Dir(dir), SpecFile) }

var ErrSpec = errors.New("campaign: bad campaign.json")

// LoadSpec reads campaign.json; a missing file is the legacy campaign
// (nil spec, nil error).
func LoadSpec(dir string) (*Spec, error) {
	b, err := os.ReadFile(specPath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var s Spec
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSpec, err)
	}
	if strings.TrimSpace(s.Name) == "" {
		return nil, fmt.Errorf("%w: name is required", ErrSpec)
	}
	if s.IPS != nil {
		if err := s.IPS.Validate(); err != nil {
			return nil, fmt.Errorf("%w: ips: %v", ErrSpec, err)
		}
	}
	if s.Shock != nil {
		if s.Shock.Factor <= 0 || s.Shock.Factor > 2 || len(s.Shock.From) != 10 {
			return nil, fmt.Errorf("%w: shock needs from=YYYY-MM-DD and 0<factor≤2", ErrSpec)
		}
	}
	return &s, nil
}

func SaveSpec(dir string, s Spec) error {
	d := Dir(dir)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, SpecFile), append(b, '\n'), 0o644)
}

// Apply multiplies every series' prices on and after From by Factor. It
// returns a new Bars; the input is not modified.
func (s *Shock) Apply(in *Bars) *Bars {
	if s == nil || in == nil {
		return in
	}
	out := *in
	out.Series = make(map[string][]DailyBar, len(in.Series))
	for sym, bars := range in.Series {
		cp := make([]DailyBar, len(bars))
		for i, b := range bars {
			if b.Date >= s.From {
				b.Open *= s.Factor
				b.High *= s.Factor
				b.Low *= s.Factor
				b.Close *= s.Factor
			}
			cp[i] = b
		}
		out.Series[sym] = cp
	}
	return &out
}

// Describe is the manifest's one-line record of the shock.
func (s *Shock) Describe() string {
	if s == nil {
		return ""
	}
	d := fmt.Sprintf("all series ×%.2f from %s", s.Factor, s.From)
	if s.Note != "" {
		d += " — " + s.Note
	}
	return d
}
