package beamcore

import (
	"strconv"
)

// fmtSpeed renders Arabic speed text like fileshare._fmt_speed.
func fmtSpeed(numBytes int64, seconds float64) string {
	if seconds <= 0 {
		seconds = 0.001
	}
	v := float64(numBytes) / seconds
	switch {
	case v < 1024:
		return strconv.FormatInt(int64(v), 10) + " بايت/ث"
	case v < 1048576:
		return ftoa1(v/1024) + " ك.ب/ث"
	case v < 1073741824:
		return ftoa1(v/1048576) + " م.ب/ث"
	default:
		return ftoa2(v/1073741824) + " ج.ب/ث"
	}
}
func ftoa1(v float64) string {
	return ftoaPrec(v, 1)
}
func ftoa2(v float64) string {
	return ftoaPrec(v, 2)
}
func ftoaPrec(v float64, prec int) string {
	// Kept (not strconv.FormatFloat): verified to differ — this rounds
	// half-up like Python % formatting, FormatFloat rounds half-even
	// (e.g. ftoa1(0.15)="0.2" vs "0.1"). See TestStdlibDivergence.
	mult := 1.0
	for i := 0; i < prec; i++ {
		mult *= 10
	}
	rounded := float64(int64(v*mult+0.5)) / mult
	intPart := int64(rounded)
	frac := int64((rounded-float64(intPart))*mult + 0.5)
	s := strconv.FormatInt(intPart, 10) + "."
	fs := strconv.FormatInt(frac, 10)
	for len(fs) < prec {
		fs = "0" + fs
	}
	return s + fs
}
