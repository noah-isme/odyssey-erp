package app

import (
	"log"
	"mime"
)

func init() {
	ensureMimeType(".css", "text/css; charset=utf-8")
}

func ensureMimeType(ext, typ string) {
	if mime.TypeByExtension(ext) != "" {
		return
	}
	if err := mime.AddExtensionType(ext, typ); err != nil {
		log.Printf("app: failed to register MIME type for %s: %v", ext, err)
	}
}
