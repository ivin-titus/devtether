package proxy

import (
	_ "embed"
	"encoding/base64"
	"html/template"
	"strings"
)

//go:embed error_page.html
var errorPageHTML string

//go:embed logo.png
var logoBytes []byte

// errorPageTemplate is strictly parsed using html/template to natively
// contextualize and escape all variables (Host, Target). This completely
// mitigates Reflected XSS vulnerabilities if a malicious Host header is passed.
var errorPageTemplate *template.Template

func init() {
	logoBase64 := base64.StdEncoding.EncodeToString(logoBytes)
	finalHTML := strings.ReplaceAll(errorPageHTML, "{{LOGO_BASE64}}", logoBase64)
	errorPageTemplate = template.Must(template.New("error_page").Parse(finalHTML))
}

type ErrorDetail struct {
	Label string
	Value string
}

type ErrorHint struct {
	Title   string
	Message string
}

// ErrorPageData holds the context for rendering the error overlay.
type ErrorPageData struct {
	StatusCode int
	StatusText string
	Message    string
	Details    []ErrorDetail // Optional key-value rows
	Hint       *ErrorHint    // Optional hint block
	ErrorTrace string        // Optional diagnostic trace
}
