package beamcore

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func pieceCount(m *sessionMeta) int {
	if m.Size <= 0 || m.PieceLen <= 0 {
		return 0
	}
	return int((m.Size + m.PieceLen - 1) / m.PieceLen)
}
func pieceRange(m *sessionMeta, index int) (int64, int64) {
	s := int64(index) * m.PieceLen
	e := s + m.PieceLen
	if e > m.Size {
		e = m.Size
	}
	return s, e
}
func getBitmap(m *sessionMeta) string {
	n := pieceCount(m)
	if len(m.Bitmap) != n {
		return strings.Repeat("0", n)
	}
	return m.Bitmap
}
func missingPieces(m *sessionMeta) []int {
	bm := getBitmap(m)
	out := []int{}
	for i := 0; i < len(bm); i++ {
		if bm[i] != '1' {
			out = append(out, i)
		}
	}
	return out
}
func rangeAdd(m *sessionMeta, s, e int64) {
	// Clamp to session bounds: ignore invalid/empty spans.
	if m == nil || e <= s {
		return
	}
	if s < 0 {
		s = 0
	}
	if m.Size > 0 {
		if s > m.Size {
			return
		}
		if e > m.Size {
			e = m.Size
		}
	}
	ranges := [][2]int64{}
	for _, r := range m.Ranges {
		ranges = append(ranges, r)
	}
	ranges = append(ranges, [2]int64{s, e})
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i][0] != ranges[j][0] {
			return ranges[i][0] < ranges[j][0]
		}
		return ranges[i][1] < ranges[j][1]
	})
	merged := [][2]int64{}
	for _, r := range ranges {
		if len(merged) > 0 && r[0] <= merged[len(merged)-1][1] {
			if r[1] > merged[len(merged)-1][1] {
				merged[len(merged)-1][1] = r[1]
			}
		} else {
			merged = append(merged, r)
		}
	}
	// Never drop covered bytes: the merged list is already minimal
	// (overlaps coalesced), so every entry is live accounting. Capping
	// the list (e.g. keeping only the last N) forgets early ranges,
	// undercounts received bytes and forces needless re-downloads.
	m.Ranges = merged
}
func rangeCover(m *sessionMeta, s, e int64) bool {
	for _, r := range m.Ranges {
		if r[0] <= s && e <= r[1] {
			return true
		}
	}
	return false
}
func rangeRemove(m *sessionMeta, s, e int64) {
	out := [][2]int64{}
	for _, r := range m.Ranges {
		a, b := r[0], r[1]
		if b <= s || e <= a {
			out = append(out, r)
			continue
		}
		if a < s {
			out = append(out, [2]int64{a, s})
		}
		if e < b {
			out = append(out, [2]int64{e, b})
		}
	}
	m.Ranges = out
}
func receivedRangesBytes(m *sessionMeta) int64 {
	var total int64
	for _, r := range m.Ranges {
		if r[1] > r[0] {
			total += r[1] - r[0]
		}
	}
	return total
}
func contiguousOffset(m *sessionMeta) int64 {
	bm := getBitmap(m)
	var off int64
	for i := 0; i < len(bm); i++ {
		if bm[i] != '1' {
			break
		}
		_, e := pieceRange(m, i)
		off = e
	}
	return off
}
func markPiece(uid string, m *sessionMeta, index int, ok bool) {
	bm := []byte(getBitmap(m))
	if index >= 0 && index < len(bm) {
		if ok {
			bm[index] = '1'
		} else {
			bm[index] = '0'
		}
		m.Bitmap = string(bm)
		saveMeta(uid, m)
	}
}
func verifyPiece(uid string, m *sessionMeta, index int) bool {
	n := pieceCount(m)
	if index < 0 || index >= n {
		return false
	}
	var want string
	if index < len(m.Hashes) && m.Hashes[index] != nil {
		want = strings.ToLower(*m.Hashes[index])
	}
	if len(want) != 64 {
		return false
	}
	s, e := pieceRange(m, index)
	f, err := os.Open(filepath.Join(sessDir(uid), "data.part"))
	if err != nil {
		return false
	}
	defer f.Close()
	if _, err := f.Seek(s, io.SeekStart); err != nil {
		return false
	}
	h := sha256.New()
	remaining := e - s
	buf := getCopyBuf()
	defer putCopyBuf(buf)
	for remaining > 0 {
		want := remaining
		if want > int64(len(buf)) {
			want = int64(len(buf))
		}
		nr, err := f.Read(buf[:want])
		if nr > 0 {
			h.Write(buf[:nr])
			remaining -= int64(nr)
		}
		if err != nil {
			break
		}
	}
	if remaining != 0 {
		return false
	}
	return hex.EncodeToString(h.Sum(nil)) == want
}
func zeroPiece(uid string, m *sessionMeta, index int) {
	if !validSessID(uid) {
		return
	}
	s, e := pieceRange(m, index)
	partPath := filepath.Join(sessDir(uid), "data.part")
	// Refuse to zero through a symlink: data.part must be a regular file.
	if st, err := os.Lstat(partPath); err != nil || st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
		if err == nil {
			return
		}
		// Missing file: nothing to zero, just clear accounting below.
		if !os.IsNotExist(err) {
			return
		}
		markPiece(uid, m, index, false)
		return
	}
	f, err := os.OpenFile(filepath.Join(sessDir(uid), "data.part"), os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err := f.Seek(s, io.SeekStart); err != nil {
		return
	}
	remaining := e - s
	zeros := getCopyBuf()
	defer putCopyBuf(zeros)
	for remaining > 0 {
		n := remaining
		if n > int64(len(zeros)) {
			n = int64(len(zeros))
		}
		if _, err := f.Write(zeros[:n]); err != nil {
			break
		}
		remaining -= n
	}
	markPiece(uid, m, index, false)
}
