package main

import (
	"bytes"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	cp "github.com/otiai10/copy"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"go.abhg.dev/goldmark/frontmatter"
)

const (
	OUT_DIR     = "out/"
	STATIC_DIR  = "static/"
	CONTENT_DIR = "content/"
	LAYOUTS_DIR = "layouts/"
)

func dirWithoutSlash(dir string) string {
	return strings.TrimSuffix(dir, "/")
}

func getFiles(dir string, ext string) []string {
	var files []string

	dirs, err := os.ReadDir(dir)
	check(err)

	for _, readDir := range dirs {
		if readDir.IsDir() {
			addFiles := getFiles(dirWithoutSlash(dir)+"/"+readDir.Name(), ext)
			files = append(files, addFiles...)
		} else {
			filename := readDir.Name()
			log.Println(filename)
			if strings.Split(filename, ".")[1] == ext {
				files = append(files, dirWithoutSlash(dir)+"/"+filename)
			}
		}
	}

	return files
}

func must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}

	return t
}

var tmpl *template.Template

func newRender() {
	prepareOutDir()

	mdFiles := getFiles(CONTENT_DIR, "md")

	data := make(map[string]any)
	data["Posts"] = make([]Meta, 0)

	for _, mdFile := range mdFiles {
		content := must(os.ReadFile(mdFile))
		log.Println("#####")
		log.Println("Process md file:", mdFile)

		rendered, meta := processMd(content)

		data["Title"] = "jens.pub"
		data["Markdown"] = template.HTML(rendered)

		switch meta.Type {
		case "post":
			log.Println("FOUND POST", meta)
			data["Posts"] = append(data["Posts"].([]Meta), meta)
		}

		outDir := filepath.Join(
			OUT_DIR,
			filepath.Join(strings.Split(filepath.Dir(mdFile), "/")[1:]...),
		)
		outFilename := strings.Split(filepath.Base(mdFile), ".")[0] + ".html"

		if meta.Filename != "" {
			outFilename = meta.Filename
		}

		os.Mkdir(outDir, 0755)

		outFile := must(os.Create(filepath.Join(outDir, outFilename)))
		defer outFile.Close()

		log.Println("Rendering", outFile.Name(), "with", meta.Template)
		myTmpl := must(tmpl.Clone())
		myTmpl.ParseFiles(LAYOUTS_DIR + meta.Template)
		log.Println("data", data)
		myTmpl.ExecuteTemplate(outFile, meta.Template, data)

		outFile.WriteString(reloadJS)
	}
}

func prepareOutDir() {
	err := os.RemoveAll(OUT_DIR)
	check(err)

	err = os.Mkdir(OUT_DIR, 0755)
	check(err)

	err = cp.Copy(STATIC_DIR, OUT_DIR+STATIC_DIR, cp.Options{
		Sync: true,
	})
	check(err)
}

type Meta struct {
	Title    string    `yaml:"title"`
	Tags     []string  `yaml:"tags"`
	Desc     string    `yaml:"desc"`
	Date     time.Time `yaml:"date"`
	Template string    `yaml:"template"`
	Type     string    `yaml:"type"`
	Filename string    `yaml:"filename"`
}

func processMd(source []byte) (string, Meta) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, &frontmatter.Extender{}),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
		),
	)

	ctx := parser.NewContext()

	var buf bytes.Buffer
	err := md.Convert(source, &buf, parser.WithContext(ctx))
	check(err)

	meta := Meta{}

	d := frontmatter.Get(ctx)
	if d != nil {
		err = d.Decode(&meta)
		check(err)
	}

	return buf.String(), meta
}
