package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	cp "github.com/otiai10/copy"
)

func check(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

func main() {
	args := os.Args

	if slices.Contains(args, "serve") {

		watcher, err := fsnotify.NewWatcher()
		check(err)
		defer watcher.Close()

		go func() {
			for {
				select {
				case event, ok := <-watcher.Events:
					if !ok {
						return
					}

					if event.Has(fsnotify.Write | fsnotify.Create) {
						if strings.Contains(event.Name, "~") {
							// log.Println("ignoring file:", event.Name)
							continue
						}

						log.Println("modified file:", event.Name)
						time.Sleep(100 * time.Millisecond)
						render()
					}
				case err, ok := <-watcher.Errors:
					if !ok {
						return
					}
					log.Println("error:", err)
				}
			}
		}()

		err = watcher.Add("./content")
		check(err)
		err = watcher.Add("./layouts")
		check(err)

		render()

		fs := http.FileServer(http.Dir("./out"))
		http.Handle("/", fs)

		log.Print("Listening on :3000...")
		err = http.ListenAndServe(":3000", nil)
		check(err)
	} else {
		render()
		log.Println("")
	}
}

func render() {
	err := cp.Copy("./static/", "./out/static", cp.Options{
		Sync: true,
	})
	check(err)

	data, err := os.ReadFile("content/index.md")
	check(err)

	rendered := mdToHtml(data)

	tmpl := template.Must(template.ParseGlob("layouts/*"))
	check(err)

	f, err := os.Create("out/index.html")
	check(err)
	defer f.Close()

	test := struct {
		Title  string
		Author string
		Index  template.HTML
	}{
		Title:  "jens.pub",
		Author: "Jens",
		Index:  template.HTML(string(rendered)),
	}

	err = tmpl.ExecuteTemplate(f, "index.html", test)
	check(err)
}

func mdToHtml(md []byte) []byte {
	// create markdown parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)

	// create HTML renderer with extensions
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	return markdown.Render(doc, renderer)
}
