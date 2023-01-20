package webui

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"github.com/korrel8r/korrel8r/pkg/korrel8r"
	"github.com/korrel8r/korrel8r/pkg/uri"
)

type storeHandler struct{ ui *WebUI }

var storePath = regexp.MustCompile(`/stores/(?P<domain>[^/]+)/(?P<class>[^/]+)/(?P<path>.+)`)

func (h *storeHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	m := storePath.FindStringSubmatch(req.URL.Path)
	if m == nil {
		http.Error(w, fmt.Sprintf("bad store uri: %v", req.URL), http.StatusNotFound)
		return
	}
	d, c, p := m[1], m[2], m[3]
	store, err := h.ui.Engine.Store(d)
	if httpError(w, err, http.StatusNotFound) {
		return
	}
	domain, err := h.ui.Engine.Domain(d)
	if httpError(w, err, http.StatusNotFound) {
		return
	}
	ref := uri.Reference{Path: p, RawQuery: req.URL.RawQuery}
	class := domain.Class(c)
	if class == nil {
		http.Error(w, fmt.Sprintf("class not found %v/%v", d, c), http.StatusNotFound)
		return
	}
	result := korrel8r.NewResult(class)
	err = store.Get(context.Background(), ref, result)
	data := map[string]any{
		"class":  class,
		"ref":    ref,
		"err":    err,
		"result": result.List(),
	}
	t, err := h.ui.Page("stores").Parse(`
{{define "body"}}
    Request {{.class}}: {{.ref}}<br>
    <hr>
    {{if .err}}
        Error: {{.err}}<br>
    {{else}}
        Results ({{len .result}})<br>
            {{range .result}}<hr><pre>{{toYAML .}}</pre>{{end}}
        </pre>
    {{end}}
{{end}}
    `)
	if !httpError(w, err, http.StatusInternalServerError) {
		httpError(w, t.Execute(w, data), http.StatusInternalServerError)
	}
}
