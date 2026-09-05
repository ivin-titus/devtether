package proxy

import (
	_ "embed"
	"html/template"
)

//go:embed error_page.html
var errorPageHTML string

// errorPageTemplate is strictly parsed using html/template to natively
// contextualize and escape all variables (Host, Target). This completely
// mitigates Reflected XSS vulnerabilities if a malicious Host header is passed.
var errorPageTemplate = template.Must(template.New("502").Parse(errorPageHTML))

// ErrorPageData holds the context for rendering the 502 error overlay.
type ErrorPageData struct {
	Host       string
	Target     string
	TargetPort string
	ErrorTrace string
}
