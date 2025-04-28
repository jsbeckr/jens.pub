package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

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
	restartChannel := make(chan bool)

	go watchForChanges(*watcher, restartChannel)

	err = watcher.Add("./content")
	check(err)
	err = watcher.Add("./layouts")
	check(err)

	render()

	// TODO: split ServerMux to have the webserver running the ganze Zeit
	fs := http.FileServer(http.Dir("./out"))
	http.Handle("/", fs)
	http.Handle("/reload", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			check(err)
		}
		defer c.CloseNow()

		// Set the context as needed. Use of r.Context() is not recommended
		// to avoid surprising behavior (see http.Hijacker).
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		<-reloadBrowser

		log.Println("SENDING WEBSOCKET RELOAD")

		wsjson.Write(ctx, c, "reload")

		c.Close(websocket.StatusNormalClosure, "")
	}))

	log.Print("Listening on :3000...")
	server := http.Server{Addr: ":3000", Handler: nil}

	go func() {
		for {
			if <-restartChannel {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

				if err := server.Shutdown(ctx); err != nil {
					panic(err)
				}

				time.Sleep(500 * time.Millisecond)

				server = http.Server{Addr: ":3000", Handler: nil}
				go func() {
					log.Println("New Server listening!")
					reloadBrowser <- true
					server.ListenAndServe()
				}()

				cancel()
			}
		}
	}()

	go func() {
		server.ListenAndServe()
	}()

	block := make(chan struct{})
	<-block
}

func watchForChanges(watcher fsnotify.Watcher, restartChannel chan bool) {
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
				restartChannel <- true
				render()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("error:", err)
		}
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
