package templates

import _ "embed"

//go:embed install.sh.tmpl
var Shell []byte
