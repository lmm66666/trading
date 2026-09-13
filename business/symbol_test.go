package business

import "testing"

func TestToSymbol(t *testing.T) {
	for _, test := range []struct{ code, want string }{{"600000", "sh600000"}, {"000001", "sz000001"}} {
		got, err := toSymbol(test.code)
		if err != nil || got != test.want {
			t.Fatalf("toSymbol(%q)=(%q,%v)", test.code, got, err)
		}
	}
	for _, code := range []string{"", "123", "abcdef", "999999"} {
		if _, err := toSymbol(code); err == nil {
			t.Fatalf("expected %q rejected", code)
		}
	}
}
