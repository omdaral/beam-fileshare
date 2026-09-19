package beamcore

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// insecureTLSConfig skips verification for loopback self-probes only.
// Browsers still show the normal self-signed warning and the owner
// verifies via the displayed SHA-256 fingerprint.
func insecureTLSConfig() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- loopback self-probe only
}

// TLS: self-signed HTTPS for LAN/hotspot (no internet, no CA).
// Cert is generated once and persisted under SharedDir/.beam-tls/
// (hidden dot dir — never listed by walkShared, never downloadable).
// Fingerprint (SHA-256) is shown to the owner for verification.

var (
	TLSEnabled     bool
	TLSFingerprint string

	tlsMu       sync.RWMutex
	tlsCertPath string
	tlsKeyPath  string
)

func tlsDir() string {
	if SharedDir == "" {
		return ""
	}
	return filepath.Join(SharedDir, ".beam-tls")
}

// TLSScheme returns "https" when TLS is on, else "http".
func TLSScheme() string {
	tlsMu.RLock()
	on := TLSEnabled
	tlsMu.RUnlock()
	if on {
		return "https"
	}
	return "http"
}

func tlsFingerprintOfDER(der []byte) string {
	sum := sha256.Sum256(der)
	h := strings.ToUpper(hex.EncodeToString(sum[:]))
	// Colon-separated groups like browsers show.
	out := make([]byte, 0, len(h)+len(h)/2)
	for i := 0; i < len(h); i += 2 {
		if i > 0 {
			out = append(out, ':')
		}
		out = append(out, h[i], h[i+1])
	}
	return string(out)
}

// EnsureTLSCert generates or loads the persistent self-signed cert.
// Must be called after SharedDir is set, before Run().
func EnsureTLSCert() (certFile, keyFile string, err error) {
	dir := tlsDir()
	if dir == "" {
		return "", "", os.ErrNotExist
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", "", err
	}
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	tlsMu.Lock()
	tlsCertPath = certFile
	tlsKeyPath = keyFile
	tlsMu.Unlock()

	// Reuse if both exist and parse.
	if certPEM, err1 := os.ReadFile(certFile); err1 == nil {
		if keyPEM, err2 := os.ReadFile(keyFile); err2 == nil {
			if pair, err3 := tls.X509KeyPair(certPEM, keyPEM); err3 == nil && len(pair.Certificate) > 0 {
				if leaf, err4 := x509.ParseCertificate(pair.Certificate[0]); err4 == nil {
					// Regenerate when expired or expiring within 30 days.
					if time.Until(leaf.NotAfter) > 30*24*time.Hour {
						TLSFingerprint = tlsFingerprintOfDER(pair.Certificate[0])
						return certFile, keyFile, nil
					}
				}
			}
		}
	}
	// Generate fresh ECDSA P-256.
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}
	now := time.Now().Add(-5 * time.Minute)
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "Beam LAN", Organization: []string{"Beam"}},
		NotBefore:    now,
		NotAfter:     now.Add(825 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	// SANs: loopback + LAN IPs + friendly names.
	tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	seen := map[string]bool{"127.0.0.1": true, "::1": true}
	for _, ip := range GetLANIPs() {
		if seen[ip] {
			continue
		}
		if parsed := net.ParseIP(ip); parsed != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, parsed)
			seen[ip] = true
		}
	}
	names := []string{"localhost", "beam.local", "beam"}
	if h, err := os.Hostname(); err == nil && h != "" {
		names = append(names, h)
		short := h
		if i := strings.Index(short, "."); i > 0 {
			short = short[:i]
		}
		if short != h {
			names = append(names, short)
		}
	}
	seenDNS := map[string]bool{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seenDNS[strings.ToLower(n)] {
			continue
		}
		seenDNS[strings.ToLower(n)] = true
		tmpl.DNSNames = append(tmpl.DNSNames, n)
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return "", "", err
	}
	certOut, err := os.OpenFile(certFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return "", "", err
	}
	_ = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	certOut.Close()
	keyOut, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return "", "", err
	}
	privDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		keyOut.Close()
		return "", "", err
	}
	_ = pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER})
	keyOut.Close()
	TLSFingerprint = tlsFingerprintOfDER(der)
	return certFile, keyFile, nil
}

// TLSConfigured reports whether cert files are ready.
func TLSConfigured() bool {
	tlsMu.RLock()
	defer tlsMu.RUnlock()
	return tlsCertPath != "" && tlsKeyPath != ""
}
func tlsCertFiles() (string, string) {
	tlsMu.RLock()
	defer tlsMu.RUnlock()
	return tlsCertPath, tlsKeyPath
}

// DropTLSCert deletes the persisted cert so the next EnsureTLSCert
// generates a fresh one (used by --tls-regen).
func DropTLSCert() {
	dir := tlsDir()
	if dir == "" {
		return
	}
	_ = os.Remove(filepath.Join(dir, "cert.pem"))
	_ = os.Remove(filepath.Join(dir, "key.pem"))
	tlsMu.Lock()
	tlsCertPath = ""
	tlsKeyPath = ""
	tlsMu.Unlock()
	TLSFingerprint = ""
}
