// Package openapi embeds the hand-authored OpenAPI 3 specification for the /v1
// HTTP surface so the API can serve it (and the docs page) without shipping any
// extra files. A test in the httpapi package keeps the spec in sync with the
// router.
package openapi

import (
	"bytes"
	_ "embed"
	"regexp"
)

// Spec is the raw openapi.yaml document.
//
//go:embed openapi.yaml
var Spec []byte

var infoVersion = regexp.MustCompile(`(?m)^  version: .*$`)

// SpecFor returns the spec with info.version set to the running build's
// version, so a served spec always names the release it describes. A
// development build ("dev", or empty) serves the file as written.
func SpecFor(version string) []byte {
	if version == "" || version == "dev" {
		return Spec
	}
	loc := infoVersion.FindIndex(Spec)
	if loc == nil {
		return Spec
	}
	var b bytes.Buffer
	b.Grow(len(Spec) + 16)
	b.Write(Spec[:loc[0]])
	b.WriteString("  version: " + version)
	b.Write(Spec[loc[1]:])
	return b.Bytes()
}
