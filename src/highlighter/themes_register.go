package highlighter

import (
	"embed"
	"io/fs"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
)

//go:embed themes/*.xml
var customThemesFS embed.FS

func init() {
	entries, err := fs.ReadDir(customThemesFS, "themes")
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		f, err := customThemesFS.Open("themes/" + e.Name())
		if err != nil {
			continue
		}
		style, err := chroma.NewXMLStyle(f)
		f.Close()
		if err != nil {
			continue
		}
		styles.Register(style)
	}
}
