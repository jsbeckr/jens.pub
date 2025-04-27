package main

import (
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
)

func check(e error) {
	if e != nil {
		// TODO: maybe log.Fatal?
		panic(e)
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
	}
}

func render() {
	data, err := os.ReadFile("content/index.md")
	check(err)

	rendered := mdToHtml(data)

	err = os.WriteFile("out/index.html", rendered, 0644)
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
