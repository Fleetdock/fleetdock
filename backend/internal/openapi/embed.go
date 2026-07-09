// Package openapi embeds the hand-authored OpenAPI 3 specification for the /v1
// HTTP surface so the API can serve it (and the docs page) without shipping any
// extra files. A test in the httpapi package keeps the spec in sync with the
// router.
package openapi

import _ "embed"

// Spec is the raw openapi.yaml document.
//
//go:embed openapi.yaml
var Spec []byte
