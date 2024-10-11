package cmd

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	_ "embed"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

//go:embed js/reload.js
var script []byte

var change = make(chan string)

var upgrader = websocket.Upgrader{}

var conns = []*websocket.Conn{}

func serveWs(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade:", err)
		return
	}
	conns = append(conns, ws)
	ws.SetCloseHandler(func(code int, text string) error {
		index := 0
		for i, c := range conns {
			if c == ws {
				index = i
			}
		}
		conns = remove(conns, index)
		fmt.Println(conns)
		return nil
	})

}

func injectReloadScript(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	_ := strings.Index(string(b), "</body>")
}

func remove[T any](s []T, i int) []T {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}

var port int

var rootCmd = &cobra.Command{
	Use:   "lserv",
	Short: "live basic http server",
	Long:  "an http server with hot reloading",
	Run: func(cmd *cobra.Command, args []string) {

		http.HandleFunc("/ws", serveWs)
		path := ""
		if len(args) > 0 {
			path = args[0]
		} else {
			p, err := os.Getwd()
			if err != nil {
				log.Fatal(err)
			}
			path = p
		}

		// create watcher
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			log.Fatal(err)
		}
		defer watcher.Close()

		// inject reload scripts into all html files
		filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			fmt.Println(p)
			if strings.HasSuffix(p, "htm") || strings.HasSuffix(p, "html") {
				injectReloadScript(p)
			}
			if d.IsDir() {
				err := watcher.Add(p)
				if err != nil {
					panic(err)
				}
			}
			return nil
		})

		// Start listening for events.
		go func() {
			for {
				select {
				case event, ok := <-watcher.Events:
					if !ok {
						return
					}
					// if new html file is created inject reload script
					if event.Op == fsnotify.Create {
						if strings.HasSuffix(event.Name, "htm") || strings.HasSuffix(event.Name, "html") {
							injectReloadScript(event.Name)
						}
					}
					log.Println("modified file:", event.Op)
					for index, i := range conns {
						fmt.Println("Change!!")
						err := i.WriteMessage(websocket.TextMessage, []byte("reload"))
						if err != nil {
							// presumably connection closed so remove from slice
							conns = remove(conns, index)

						}
						fmt.Println("conns", len(conns))
					}
				case err, ok := <-watcher.Errors:
					if !ok {
						return
					}
					log.Println("error:", err)
				}
			}
		}()

		// Add a path.

		http.Handle("/", http.StripPrefix("/", http.FileServer(http.Dir(path))))

		go http.ListenAndServe(fmt.Sprintf(":%v", port), nil)
		err = watcher.Add(path)
		if err != nil {
			log.Fatal(err)
		}

		// Block main goroutine forever.
		<-make(chan struct{})

	},
}

func init() {
	rootCmd.Flags().IntVarP(&port, "port", "p", 8080, "select port")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
