package indicator

import (
	"math"
	"testing"
)

func TestRelativePanel(t *testing.T) {
	primary, comparison := make([]float64, 90), make([]float64, 90)
	for i := range primary {
		comparison[i] = math.Exp(float64(i*i) / 10000)
		primary[i] = comparison[i] * math.Exp(float64(i)/100)
	}
	panel, err := RelativePanel(primary, comparison, 20, 5, 40)
	if err != nil {
		t.Fatal(err)
	}
	z, ok := panel.Z.At(19)
	if !ok || math.Abs(z-1.647508942095828) > 1e-9 {
		t.Fatalf("population z %v %v", z, ok)
	}
	if panel.Z.Valid(18) || panel.Regime.Valid(38) || panel.Correlation.Valid(23) {
		t.Fatal("premature warmup")
	}
	smooth, _ := panel.Smooth.At(19)
	if smooth != z {
		t.Fatal("EMA seed")
	}
	relative, ok := panel.Performance.At(63)
	if !ok || math.Abs(relative-(math.Exp(.63)-1)*100) > 1e-9 {
		t.Fatal(relative)
	}
	corr, ok := panel.Correlation.At(40)
	if !ok || math.Abs(corr-1) > 1e-9 {
		t.Fatal(corr, ok)
	}
	for n := 41; n < len(primary); n++ {
		prefix, _ := RelativePanel(primary[:n], comparison[:n], 20, 5, 40)
		for j, s := range []Series{panel.Z, panel.Smooth, panel.Regime, panel.Performance, panel.Correlation} {
			p := []Series{prefix.Z, prefix.Smooth, prefix.Regime, prefix.Performance, prefix.Correlation}[j]
			for i := 0; i < n; i++ {
				a, av := s.At(i)
				b, bv := p.At(i)
				if av != bv || math.Abs(a-b) > 1e-10 {
					t.Fatal("future leak")
				}
			}
		}
	}
	for i := range primary {
		primary[i] = comparison[i] * 3
	}
	flat, _ := RelativePanel(primary, comparison, 20, 5, 40)
	if flat.Z.Valid(89) || flat.Regime.Valid(89) {
		t.Fatal("constant ratio must have no Z")
	}
}
func TestRelativePanelInvalid(t *testing.T) {
	for _, p := range [][3]int{{1, 5, 40}, {20, 0, 40}, {20, 5, 20}} {
		if _, err := RelativePanel(nil, nil, p[0], p[1], p[2]); err == nil {
			t.Fatal(p)
		}
	}
	if _, err := RelativePanel([]float64{1}, nil, 2, 1, 3); err == nil {
		t.Fatal("length")
	}
	if _, err := RelativePanel([]float64{0}, []float64{1}, 2, 1, 3); err == nil {
		t.Fatal("price")
	}
	flat, _ := RelativePanel(makePrices(80, 1), makePrices(80, 1), 2, 1, 3)
	if flat.Correlation.Valid(79) {
		t.Fatal("undefined correlation")
	}
}
func makePrices(n int, v float64) []float64 {
	r := make([]float64, n)
	for i := range r {
		r[i] = v
	}
	return r
}
