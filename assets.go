package main

import (
	"embed"
	"io/fs"
)

func setupAssets() (fs.FS, error) {
	return fs.Sub(webFS, "web/dist")
}

var _ = embed.FS{}
