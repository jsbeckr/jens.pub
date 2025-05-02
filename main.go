package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/fsnotify/fsnotify"
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

	tmpl = template.Must(template.ParseGlob(LAYOUTS_DIR+"_*.html"))

	reloadBrowser := make(chan bool)

	go watchForChanges(*watcher, reloadBrowser)

	err = filepath.WalkDir(CONTENT_DIR, func(path string, d os.DirEntry, err error) error {
		return watcher.Add(path)
	})
	check(err)

	err = filepath.WalkDir(LAYOUTS_DIR, func(path string, d os.DirEntry, err error) error {
		return watcher.Add(path)
	})
	check(err)

	newRender()
	updateStyles()

	fs := http.FileServer(http.Dir(OUT_DIR))
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

			log.Println("Browser connected")

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

				newRender()
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

func updateStyles() {
	tailwindCmd := exec.Command(
		"npx",
		"@tailwindcss/cli", "-i", "./static/base.css", "-o", "./out/static/index.css",
	)
	_, err := tailwindCmd.Output()
	check(err)
	log.Println("Recomputed styles")
}

const reloadJS = `
<script>
  var socket = new WebSocket("ws://localhost:3000/reload");
  socket.onmessage = function (e) {
    location.reload();
  };
</script>
	`
