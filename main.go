package main

import (
	"bytes"
	"context"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/fsnotify/fsnotify"
	cp "github.com/otiai10/copy"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"go.abhg.dev/goldmark/frontmatter"
)

func check(e error) {
	if e != nil {
		panic("Error: " + e.Error())
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
	updateStyles()

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

			<-reloadBrowser

			log.Println("Browser reload")

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

const (
	OUT_DIR     = "out/"
	STATIC_DIR  = "static/"
	CONTENT_DIR = "content/"
	LAYOUTS_DIR = "layouts/"
)

func render() {
	err := os.RemoveAll(OUT_DIR)
	check(err)

	err = os.Mkdir(OUT_DIR, 0755)
	check(err)

	err = cp.Copy(STATIC_DIR, OUT_DIR+STATIC_DIR, cp.Options{
		Sync: true,
	})
	check(err)

	data, err := os.ReadFile(CONTENT_DIR + "index.md")
	check(err)

	rendered := mdToHtml(data)

	tmpl := template.Must(template.ParseGlob(LAYOUTS_DIR + "*"))
	check(err)

	index, err := os.Create(OUT_DIR + "index.html")
	check(err)
	defer index.Close()

	dirs, err := os.ReadDir(CONTENT_DIR)
	check(err)

	for _, dir := range dirs {
		if dir.IsDir() {
			files, err := os.ReadDir(CONTENT_DIR + dir.Name())
			check(err)

			err = os.Mkdir(OUT_DIR+dir.Name(), 0755)
			check(err)

			for _, file := range files {
				filename := OUT_DIR + dir.Name() + "/" + strings.TrimSuffix(
					file.Name(),
					".md",
				) + ".html"
				file, err := os.Create(filename)
				check(err)
				defer file.Close()

				data := struct {
					Title   string
					Content string
				}{
					Title:   file.Name(),
					Content: filename,
				}

				err = tmpl.ExecuteTemplate(file, "index.html", data)
				check(err)
			}

		}
	}

	test := struct {
		Title   string
		Content template.HTML
	}{
		Title:   "jens.pub",
		Content: template.HTML(rendered),
	}

	err = tmpl.ExecuteTemplate(index, "index.html", test)
	index.WriteString(reloadJS)
	check(err)
}

type PostMeta struct {
	Title string    `yaml:"title"`
	Tags  []string  `yaml:"tags"`
	Desc  string    `yaml:"desc"`
	Date  time.Time `yaml:"date"`
}

func mdToHtml(source []byte) string {
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

	meta := PostMeta{}

	d := frontmatter.Get(ctx)
	if d != nil {
		err = d.Decode(&meta)
		check(err)
	}

	return buf.String()
}

const reloadJS = `
<script>
  var socket = new WebSocket("ws://localhost:3000/reload");
  socket.onmessage = function (e) {
    location.reload();
  };
</script>
	`
