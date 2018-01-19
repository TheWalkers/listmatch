// listmatch-server collects lists of truncated hash values and matches them
// against each other.
package main

import (
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

func serveUsage() {
	fmt.Fprintf(os.Stderr, `
To start a list-matching server, run

    listmatch-server https://example.com/xyz/

To test on your own computer, use a URL like http://localhost:8080/.

For built-in HTTPS support, your server has to have ports 80 and 443
exposed, and this program needs to be allowed to listen on them, e.g. 
via
    
	sudo setcap cap_net_bind_service=+ep %s

on Linux. Clients require HTTPS except on localhost; if you don't use
the built-in support, you need to provide it through a reverse proxy 
like nginx or Caddy.
`, os.Args[0])
}

func main() {
	isRoot := os.Getuid() == 0
	if len(os.Args) != 2 || isRoot {
		serveUsage()
		if isRoot {
			log.Printf("\nERROR: don't run the server as root; use `setcap` to open low ports.")
		}
		os.Exit(1)
	}
	serve(os.Args[1])
}

// we use 8-10b mem/hash + program + uncollected garbage
const maxHashes = 1e8
const maxTotalHashes int = 1e8

// if you try a billion hashes, you're not really matching
const maxMatchesPerUpload int = 1e8

var errTooManyHashes = errors.New("too many hashes")

// Sorts hashes.
func sortHashes(result []uint64) {
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
}

// Finds a hash (needle) in a sorted list (haystack)
func findHash(needle uint64, haystack []uint64) bool {
	foundAt := sort.Search(len(haystack), func(i int) bool { return haystack[i] >= needle })
	if foundAt == len(haystack) || haystack[foundAt] != needle {
		return false
	}
	return true
}

// Makes a mask saying which values in needles have a match in haystack.
func findHashes(needles []uint64, haystack []uint64) (out []byte) {
	out = make([]byte, 0, (len(needles)+7)/8)
	for i := 0; i < len(needles); {
		var mask byte
		for j := 7; i < len(needles) && j >= 0; i, j = i+1, j-1 {
			if findHash(needles[i], haystack) {
				mask |= 1 << uint(j)
			}
		}
		out = append(out, mask)
	}
	return out
}

// pathPrefix exists so random Internet folks can't use your server
var pathPrefix string

func serve(desiredURL string) {
	u, err := url.Parse(desiredURL)
	if err != nil {
		log.Fatalln("Desired serving url", desiredURL, "did not parse:", err)
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	pathPrefix = u.Path
	fmt.Fprintf(os.Stderr, "To use this server, uploaders can run:\n\t%s upload MYFILE.csv %s\n", os.Args[0], u.String())

	http.HandleFunc(u.Path+"upload", handleUpload)
	http.HandleFunc(u.Path+"match", handleMatch)
	http.HandleFunc("/", handleBrowser)
	if u.Scheme == "https" {
		certManager := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(u.Hostname()),
			Cache:      autocert.DirCache("."),
		}
		if u.Port() == "" {
			u.Host += ":443"
		}
		server := &http.Server{
			Addr:    u.Host,
			Handler: http.DefaultServeMux,
			TLSConfig: &tls.Config{
				GetCertificate: certManager.GetCertificate,
			},
		}
		log.Fatal(server.ListenAndServeTLS("", ""))
	} else {
		if u.Port() == "" {
			u.Host += ":80"
		}

		log.Fatal(http.ListenAndServe(u.Host, http.DefaultServeMux))
	}
}

var uploads = map[string][]uint64{}
var uploadMatchCounts = map[string]int{}
var uploadHistory = []string{}
var uploadLock sync.Mutex

// drops uploads until total upload size is under maxTotalHashes;
// hold uploadLock while calling
func cleanOldUploads() {
	size := 0
	for _, upload := range uploads {
		size = len(upload)
	}

	for size > maxTotalHashes && len(uploadHistory) > 0 {
		var oldest string
		oldest, uploadHistory = uploadHistory[0], uploadHistory[1:]
		size -= len(uploads[oldest])
		delete(uploads, oldest)
	}
}

func allowCORS(rw http.ResponseWriter) {
	h := rw.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "PUT, OPTIONS")
}

func handleUpload(rw http.ResponseWriter, r *http.Request) {
	allowCORS(rw)
	if r.Method != "PUT" {
		handleBrowser(rw, r)
		return
	}

	nameList := r.URL.Query()["name"]
	if len(nameList) != 1 {
		writeError(rw, errors.New("need a name arg"))
		return
	}
	name := nameList[0]
	uploadLock.Lock()
	if _, ok := uploads[name]; ok {
		writeError(rw, errors.New("that name's taken"))
		return
	}
	uploadLock.Unlock()

	upload, err := readUint64s(r.Body)
	if err != nil {
		writeError(rw, err, "reading")
		return
	}
	sortHashes(upload)

	uploadLock.Lock()
	uploads[name] = upload
	uploadHistory = append(uploadHistory, name)
	cleanOldUploads()
	uploadLock.Unlock()

	time.AfterFunc(24*time.Hour, func() {
		uploadLock.Lock()
		delete(uploads, name)
		uploadLock.Unlock()
	})

	match := url.URL(*r.URL) // copy to change path
	match.Path = pathPrefix + "match"
	rw.Header().Add("Location", match.String())
	rw.WriteHeader(204)
}

func handleMatch(rw http.ResponseWriter, r *http.Request) {
	allowCORS(rw)
	if r.Method != "PUT" {
		handleBrowser(rw, r)
		return
	}

	nameList := r.URL.Query()["name"]
	if len(nameList) != 1 {
		writeError(rw, errors.New("need a name arg"))
		return
	}
	name := nameList[0]

	uploadLock.Lock()
	upload := uploads[name]
	uploadLock.Unlock()

	if len(upload) == 0 {
		if upload == nil {
			writeError(rw, errors.New("upload missing"))
		} else {
			writeError(rw, errors.New("whoops, upload empty"))
		}
		return
	}

	matches, err := readUint64s(r.Body)
	if err != nil {
		writeError(rw, err, "reading")
		return
	}
	uploadLock.Lock()
	uploadMatchCounts[name] += len(matches)
	if uploadMatchCounts[name] > maxMatchesPerUpload {
		delete(uploads, name)
		uploadLock.Unlock()
		writeError(rw, errors.New("too many matches"))
		return
	}
	uploadLock.Unlock()
	masks := findHashes(matches, upload)

	rw.WriteHeader(200)
	_, _ = rw.Write(masks)
}

func handleBrowser(rw http.ResponseWriter, r *http.Request) {
	rw.WriteHeader(200)
	rw.Write([]byte("Sorry, you need to use the command-line program to access this server."))
}

func writeError(rw http.ResponseWriter, err error, rest ...interface{}) {
	rw.WriteHeader(500)
	rw.Write([]byte(err.Error()))
	if len(rest) != 0 {
		rw.Write([]byte{'\n'})
		fmt.Fprintln(rw, rest...)
	}
}

// Reads uint64s, big-endian. err==nil if it reaches EOF.
func readUint64s(r io.Reader) (result []uint64, err error) {
	for {
		var u uint64
		err = binary.Read(r, binary.BigEndian, &u)
		if err != nil {
			if err == io.EOF {
				return result, nil
			}
			return result, err
		}
		result = append(result, u)
		if len(result) > maxHashes {
			return nil, errTooManyHashes
		}
	}
}
