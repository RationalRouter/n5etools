// zipdist packages release files into a zip, using only the Go standard
// library — no dependency on a system `zip` binary being installed on
// whatever machine runs `make release-windows`, consistent with this
// project's pure-Go/no-external-tool packaging philosophy (see main.go's
// "antivirus canary" doc comment for the same reasoning applied elsewhere).
package main

import (
	"archive/zip"
	"io"
	"log"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatal("usage: zipdist <output.zip> <file1> [file2 ...]")
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		log.Fatal(err)
	}
}

func run(outPath string, files []string) error {
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	for _, path := range files {
		if err := addFile(zw, path); err != nil {
			return err
		}
	}
	return zw.Close()
}

func addFile(zw *zip.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	// zip.Writer.Create alone doesn't carry the source file's permission
	// bits into the archive — every entry comes out non-executable
	// regardless of the original file, forcing anyone unzipping a Linux/
	// macOS binary to manually chmod +x it before it'll run. FileInfoHeader
	// captures the real mode (including the executable bit already set on
	// dist/n5e-linux etc. by the release Makefile targets) into the zip's
	// Unix external file attributes, which unzip/Archive Utility/etc. all
	// restore correctly on extract.
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.Base(path)
	header.Method = zip.Deflate

	w, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}
