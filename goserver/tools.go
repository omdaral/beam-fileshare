//go:build tools

// Tool dependencies, build-tagged out of every real build so the desktop
// binary stays stdlib-only. Keeps golang.org/x/mobile resolvable for
// `gomobile bind` (which shells out to `go list` on it).
package beamcore

import _ "golang.org/x/mobile/bind"
