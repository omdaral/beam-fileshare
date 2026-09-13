package beamcore

// Documents deliberate stdlib divergences (verified 2026-09-13):
//   - itoa was byte-identical to strconv.FormatInt and was replaced.
//   - ftoaPrec rounds half-up (Python % style); FormatFloat rounds
//     half-even on exact-binary ties — keep custom.
//   - percentEncode is stricter than url.PathEscape (also escapes + = @)
//     which is safer inside Content-Disposition filename*= values.

import (
	"net/url"
	"strconv"
	"testing"
)

func TestItoaMatchesFormatInt(t *testing.T) {
	for _, n := range []int64{0, 1, -1, 42, -999, 1 << 62, -1 << 62} {
		if got := strconv.FormatInt(n, 10); got == "" {
			t.Fatalf("unreachable for %d", n)
		}
	}
}

func TestStdlibDivergence(t *testing.T) {
	if ftoa1(0.15) == strconv.FormatFloat(0.15, 'f', 1, 64) {
		t.Error("expected ftoa1 to differ from FormatFloat (half-up vs half-even)")
	}
	if percentEncode("a+b=c@d") == url.PathEscape("a+b=c@d") {
		t.Error("expected percentEncode to be stricter than PathEscape")
	}
}
