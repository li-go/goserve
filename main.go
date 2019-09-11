package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/chroma/quick"
)

var (
	port         int
	dir          string
	disableCache bool
)

func main() {
	flag.IntVar(&port, "port", 3000, "port to host")
	flag.StringVar(&dir, "dir", ".", "directory to host")
	flag.BoolVar(&disableCache, "disable-cache", false, "disable http cache")
	flag.Parse()

	fs := http.FileServer(http.Dir(dir))
	if disableCache {
		fs = noCache(fs)
	}
	http.Handle("/", highlight(fs))

	errCh := make(chan error)
	go func() {
		log.Printf("hosting '%s' at ::%d", dir, port)
		errCh <- http.ListenAndServe(":"+strconv.Itoa(port), nil)
	}()

	time.Sleep(time.Second)
	url := "http://localhost:" + strconv.Itoa(port)
	log.Printf("open '%s' in browser", url)
	if err := exec.Command("open", url).Run(); err != nil {
		log.Printf("unable to open %s: %v", url, err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	select {
	case err := <-errCh:
		log.Printf("serve error: %s", err)
	case <-quit:
		log.Println("quit!")
	}
}

var noCacheHeaders = map[string]string{
	"Expires":         time.Unix(0, 0).Format(time.RFC1123),
	"Cache-Control":   "no-cache, private, max-age=0",
	"Pragma":          "no-cache",
	"X-Accel-Expires": "0",
}

var etagHeaders = []string{
	"ETag",
	"If-Modified-Since",
	"If-Match",
	"If-None-Match",
	"If-Range",
	"If-Unmodified-Since",
}

func noCache(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		// Delete any ETag headers that may have been set
		for _, v := range etagHeaders {
			if r.Header.Get(v) != "" {
				r.Header.Del(v)
			}
		}

		// Set our NoCache headers
		for k, v := range noCacheHeaders {
			w.Header().Set(k, v)
		}
		h.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

func highlight(h http.Handler) http.Handler {
	skipSuffixes := []string{"html", "css", "js"}
	fn := func(w http.ResponseWriter, r *http.Request) {
		for _, suffix := range skipSuffixes {
			if strings.HasSuffix(r.RequestURI, suffix) {
				h.ServeHTTP(w, r)
				return
			}
		}

		fp := filepath.Join(dir, r.RequestURI)
		file, err := os.Open(fp)
		if err != nil {
			log.Printf("fail to open - %s: %v", fp, err)
			h.ServeHTTP(w, r)
			return
		}
		stat, err := file.Stat()
		if err != nil {
			log.Printf("fail to stat - %s: %v", fp, err)
			h.ServeHTTP(w, r)
			return
		}
		if stat.IsDir() {
			h.ServeHTTP(w, r)
			return
		}

		bs, err := ioutil.ReadAll(file)
		if err != nil {
			log.Printf("fail to read - %s: %v", fp, err)
			h.ServeHTTP(w, r)
			return
		}

		err = quick.Highlight(w, string(bs), "", "html", "monokai")
		if err != nil {
			log.Printf("fail to highlight - %s: %v", fp, err)
			h.ServeHTTP(w, r)
		}
	}
	return http.HandlerFunc(fn)
}
