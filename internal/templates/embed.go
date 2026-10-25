package templates

import (
	"embed"
	"html/template"
)

//go:embed layout.html dashboard.html distributors.html events.html event_detail.html
var Files embed.FS

var Pages map[string]*template.Template

func init() {
	shared := []string{"layout.html"} // add nav.html, footer.html etc. here later

	pages := []string{
		"dashboard.html",
		"distributors.html",
		"events.html",
		"event_detail.html",
	}

	Pages = make(map[string]*template.Template, len(pages))

	for _, page := range pages {
		files := append(shared, page)
		Pages[page] = template.Must(
			template.ParseFS(Files, files...),
		)
	}
}
