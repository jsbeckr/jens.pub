package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/fsnotify/fsnotify"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	cp "github.com/otiai10/copy"
)

func check(e error) {
	if e != nil {
		log.Println("Error: " + e.Error())
	}
}

func main() {
	watcher, err := fsnotify.NewWatcher()
	check(err)
	defer watcher.Close()

	reloadBrowser := make(chan bool)

	go watchForChanges(*watcher, reloadBrowser)

	err = watcher.Add("./content")
	check(err)
	err = watcher.Add("./layouts")
	check(err)

	render()

	fs := http.FileServer(http.Dir("./out"))
	http.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		fs.ServeHTTP(w, r)
	}))
	http.Handle(
		"/reload",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := websocket.Accept(w, r, nil)
			if err != nil {
				check(err)
			}
			defer c.CloseNow()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			log.Println("WEBSOCKET CLIENT CONNECTED")

			<-reloadBrowser

			log.Println("SENDING WEBSOCKET RELOAD")

			wsjson.Write(ctx, c, "reload")

			c.Close(websocket.StatusNormalClosure, "")
		}),
	)

	log.Print("Listening on :3000...")
	server := http.Server{Addr: ":3000", Handler: nil}
	server.ListenAndServe()
}

func updateStyles() {
	tailwindCmd := exec.Command(
		"npx",
		"@tailwindcss/cli", "-i", "./static/base.css", "-o", "./out/static/index.css",
	)
	_, err := tailwindCmd.Output()
	check(err)
	log.Println("Recomputed styles")
}

func watchForChanges(watcher fsnotify.Watcher, reloadBrowser chan bool) {
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			if event.Has(fsnotify.Write | fsnotify.Create) {
				if strings.Contains(event.Name, "~") {
					continue
				}

				log.Println("modified file:", event.Name)

				render()
				updateStyles()

				reloadBrowser <- true
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("error:", err)
		}
	}
}

func foobar() {
	// go through all markdown files

	// look at frontmatter and use referenced layout file to render it

	// folders are the same as the md folder
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

	// TODO: Frontmatter much?
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
