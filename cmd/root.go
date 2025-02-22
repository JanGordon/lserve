package cmd

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	_ "embed"

	"github.com/fatih/color"
	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

var warn = color.New(color.FgHiYellow)
var e = color.New(color.BgHiRed)
var success = color.New(color.FgGreen).Add(color.Underline)

//go:embed js/reload.js
var script []byte

var upgrader = websocket.Upgrader{}

var conns = []*websocket.Conn{}

func serveWs(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		e.Println("upgrade:", err)
		return
	}
	reloadCount += 2
	cleanConns()

	conns = append(conns, ws)

	printReloadCount()

	// ws.SetCloseHandler(func(code int, text string) error {
	// 	index := 0
	// 	for i, c := range conns {
	// 		if c == ws {
	// 			index = i
	// 		}
	// 	}
	// 	conns = remove(conns, index)
	// 	fmt.Println(conns)
	// 	return nil
	// })

}

func injectReloadScript(pat string) []byte {
	b, err := os.ReadFile(pat)
	if err != nil {
		panic(err)
	}
	index := strings.Index(string(b), "</body>")
	if index < 0 {
		relPath, err := filepath.Rel(path, pat)
		if err != nil {
			warn.Println(err)
			warn.Printf("cannot hot reload with no body: %v \n", pat)

		} else {
			warn.Printf("cannot hot reload with no body: %v \n", relPath)

		}
		return b
	}
	newString := string(b[:index]) + "<script>" + string(script) + "</script>" + string(b[index:])

	return []byte(newString)
}

func remove[T any](s []T, i int) []T {

	s[i] = s[len(s)-1]
	return s[:len(s)-1]

}

func cleanConns() {
	toDelete := []*websocket.Conn{}
	for index := 0; index < len(conns); index++ {
		i := conns[index]
		err := i.WriteMessage(websocket.TextMessage, []byte("ping"))
		if err != nil {
			// presumably connection closed so remove from slice
			toDelete = append(toDelete, i)

		}
	}
	for _, c := range toDelete {
		conns = remove(conns, slices.Index(conns, c))
	}
}

func printReloadCount() {
	if reloadCount > 2 {
		fmt.Print("\033[1A\033[K")

	}
	success.Printf("reloaded on %v clients (x%v)\n", len(conns), reloadCount/2)
}

var path = ""
var reloadCount = 0 // divide by 2 becasue file modifies is called x2

var port int

var rootCmd = &cobra.Command{
	Use:   "lserv",
	Short: "live basic http server",
	Long:  "an http server with hot reloading",
	Run: func(cmd *cobra.Command, args []string) {

		if len(args) > 0 {
			path = args[0]
		} else {
			p, err := os.Getwd()
			if err != nil {
				log.Fatal(err)
			}
			path = p
		}

		http.HandleFunc("/ws", serveWs)
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			p, _ := strings.CutPrefix(r.URL.String(), "/")
			if filepath.IsLocal(p) {
				// safe
				if strings.HasSuffix(p, ".html") || strings.HasSuffix(p, ".htm") {
					r.Header.Add("Content-Type", "text/html")
					w.Write(injectReloadScript(p))
				} else {
					http.ServeFile(w, r, p)
				}
			}

			if p == "" {
				//look for root html
				files, err := os.ReadDir(path)
				if err != nil {
					panic(err)
				}
				for _, f := range files {
					if !f.IsDir() && (strings.HasSuffix(f.Name(), ".html") || strings.HasSuffix(f.Name(), ".htm")) {
						r.Header.Add("Content-Type", "text/html")
						w.Write(injectReloadScript(filepath.Join(path, f.Name())))
						break
					}
				}
			}
		})

		// create watcher
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			log.Fatal(err)
		}
		defer watcher.Close()

		// inject reload scripts into all html files
		filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			if strings.HasSuffix(p, ".htm") || strings.HasSuffix(p, ".html") {
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
					// error may occur if
					toDelete := []*websocket.Conn{}
					for index := 0; index < len(conns); index++ {
						i := conns[index]
						err := i.WriteMessage(websocket.TextMessage, []byte("reload"))
						if err != nil {
							// presumably connection closed so remove from slice
							toDelete = append(toDelete, i)

						}
					}
					for _, c := range toDelete {
						conns = remove(conns, slices.Index(conns, c))
					}
					// reloadCount++
					printReloadCount()
				case err, ok := <-watcher.Errors:
					if !ok {
						return
					}
					log.Println("error:", err)
				}
			}
		}()

		// Add a path.

		go func() {
			err := http.ListenAndServe(fmt.Sprintf(":%v", port), nil)
			if err != nil {
				log.Fatal("server failed to start: ", err)
			}
		}()
		fmt.Print("server started on ")
		success.Printf("http://127.0.0.1:%v", port)
		fmt.Println()
		err = watcher.Add(path)
		if err != nil {
			log.Fatal("failed to add file watcher path: ", err)
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
