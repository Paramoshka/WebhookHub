// Package web exposes the embedded UI templates and static assets.
package web

import (
	"embed"
	"io/fs"
)

//go:embed templates static
var files embed.FS

// Templates is the root of the embedded HTML templates.
var Templates = subFS(files, "templates")

// Static is the root of the embedded static assets served under /static/.
var Static = subFS(files, "static")

func subFS(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
