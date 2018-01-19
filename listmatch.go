// listmatch lets you compare lists anonymously, using a neutral third-party server.
// This client sends salted hashes to the server, without telling the server the
// salt.
package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func usage() {
	fmt.Fprintf(os.Stderr, `To upload a list to be matched, run

    %s upload MYFILE.csv https://example.com/xyz/

...but using a URL provided to you by the server operator in
place of the example.com one. To match a list, use the unique
"%s match" command the uploader sends you.

The FIRST column of the CSV (or TSV) must be the key you're matching
on. We assume the first row is headers. Matching is case insensitive.
`, os.Args[0], os.Args[0])
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	mode := os.Args[1]

	log.SetFlags(0) // don't print date and time of errors

	if (mode == "match" || mode == "upload") && len(os.Args) >= 3 && os.Args[2] == "MYFILE.csv" {
		log.Fatalf("replace the 'MYFILE.csv' placeholder in the command line with the name of the file you want to match")
	}

	if mode == "upload" {
		if len(os.Args) != 4 {
			usage()
		}
		inName, urlString := os.Args[2], os.Args[3]
		uploadMain(inName, urlString)
	} else if mode == "match" {
		if len(os.Args) != 5 {
			usage()
		}
		inName, urlString, saltString := os.Args[2], os.Args[3], os.Args[4]
		matchMain(inName, urlString, saltString)
	} else {
		usage()
	}
}

// uploadMain generates a salt and name, uploads, and prints the command for
// the matcher to run
func uploadMain(inName, urlString string) {
	// gen salt
	var salt [32]byte
	_, err := rand.Read(salt[:])
	if err != nil {
		log.Fatalln("can't get entropy:", err)
	}
	saltStr := base64string(salt[:]) // for cmdline

	// gen url w/name
	var uploadName string
	var randBytes [16]byte
	_, err = rand.Read(randBytes[:])
	if err != nil {
		log.Fatalln("can't get entropy:", err)
	}
	uploadName = base64string(randBytes[:])
	uploadURL, err := url.Parse(urlString)
	if err != nil {
		log.Fatalln("upload url invalid:", err)
	}
	if uploadURL.Host == "example.com" {
		log.Fatalln("Sorry, the example.com URL in the usage info is just a placeholder. Use a login provided to you by someone running a server.")
	}
	vals := uploadURL.Query()
	vals["name"] = []string{uploadName}
	uploadURL.RawQuery = vals.Encode()
	uploadURL.Path += "upload"
	urlString = uploadURL.String()

	// send hashes
	resp := send(inName, urlString, salt)

	// print out response
	if len(resp.Header["Location"]) == 0 {
		log.Fatalf("server didn't send back a match link")
	}
	matchURL, err := url.Parse(resp.Header["Location"][0])
	if err != nil {
		log.Fatal("server sent back bad match link:", err)
	}
	matchURL = uploadURL.ResolveReference(matchURL) // get host and schema back
	log.Printf("Done! Ask the matcher to run:\n\t%s match MYFILE.csv %s %s\n", os.Args[0], matchURL.String(), saltStr)
}

func matchMain(inName, urlString, saltStr string) {
	// parse salt
	var salt [32]byte
	decoder := base64.NewDecoder(base64.StdEncoding, bytes.NewBufferString(saltStr))
	_, err := io.ReadFull(decoder, salt[:])
	if err != nil {
		log.Fatalln("trying to decode salt, got", err)
	}

	// send hashes to server
	resp := send(inName, urlString, salt)

	// pick an output filename
	var matchesOut *os.File
	dir, file := filepath.Split(inName)
	filePieces := strings.Split(file, ".")
	filePieces[0] += "-matches"
	file = strings.Join(filePieces, ".")
	matchPath := filepath.Join(dir, file)
	matchesOut, err = os.OpenFile(matchPath, os.O_CREATE|os.O_WRONLY, 0700)
	if err != nil {
		log.Fatalln("can't make output file", matchesOut, "due to error", err)
	}

	// reopen CSV and print matching rows
	masks, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln("Error receiving matches from server:", err)
	}
	in, err := os.Open(inName)
	if err != nil {
		log.Fatalf("can't reopen input file '%s': %v", inName, err)
	}
	s := bufio.NewScanner(in)
	if !s.Scan() {
		log.Fatalln("Couldn't reread header from CSV file")
	}
	fmt.Fprintln(matchesOut, s.Text()) // copy header row
	matches := 0
	for _, mask := range masks {
		for i := 7; i >= 0; i-- {
			if !s.Scan() { // err or EOF
				if s.Err() != nil {
					log.Fatalln("error reading CSV:", s.Err())
				}
				break
			}
			if (mask>>uint(i))&1 == 1 {
				fmt.Fprintln(matchesOut, s.Text())
				matches++
			}
		}
	}
	fmt.Println("Wrote", matches, "matches to", matchesOut.Name())
}

// send hashes a file and sends it to the server, returning the
// response. Both upload and match use it.
func send(inName, urlString string, salt [32]byte) *http.Response {
	in, err := os.Open(inName)
	if err != nil {
		log.Fatalf("can't open input file '%s': %v", inName, err)
	}

	hashReader, hashWriter := io.Pipe()
	go writeHashes(in, hashWriter, salt)

	req, err := http.NewRequest("PUT", urlString, hashReader)
	if err != nil {
		log.Fatal("error creating request:", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal("error doing request:", err)
	}
	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		log.Println("server returned bad status", resp.StatusCode)
		log.Println("response follows:")
		io.Copy(os.Stdout, resp.Body)
		os.Stdout.Write([]byte{'\n'})
		log.Println("server error, exiting")
	}

	return resp
}

func writeHashes(in io.Reader, w io.WriteCloser, salt [32]byte) {
	defer w.Close()

	bw := bufio.NewWriter(w)

	// skip headers and make sure we can read the file
	s := bufio.NewScanner(in)
	// read and throw out headers
	_, _, err := nextLine(s)
	if err != nil {
		log.Fatalf("couldn't read input file: %v", err)
	}

	fmt.Fprintln(os.Stderr, "Each . is 100k hashes written")

	hashBuf := [32]byte{}
	hasher := sha256.New()
	i := 0
	for {
		var keyCol []byte
		keyCol, _, err = nextLine(s)
		if err != nil {
			if err != io.EOF {
				log.Fatalf("couldn't read file: %v", err)
			}
			break
		}
		hasher.Reset()
		hasher.Write(salt[:])
		hasher.Write(keyCol)
		hash := hasher.Sum(hashBuf[:0])
		_, err = bw.Write(hash[:8])
		if err != nil {
			log.Fatalf("couldn't write hash: %v", err)
		}
		i++
		if i%100000 == 0 {
			os.Stderr.WriteString(".")
		}
	}

	err = bw.Flush()
	if err != nil {
		log.Fatalf("couldn't finish output: %v", err)
	}

	fmt.Fprintln(os.Stderr, "all sent! (waiting for reply)")
}

const delimiters = "\r\n\t ,"

// parses the next line of an input file
func nextLine(s *bufio.Scanner) (keyCol, wholeLine []byte, err error) {
	if !s.Scan() {
		err := s.Err()
		if err == nil {
			err = io.EOF
		}
		return nil, nil, err
	}
	line := append([]byte(nil), s.Bytes()...) // copy
	keyCol = line
	// cut off other cols
	colBreak := bytes.IndexAny(keyCol, delimiters)
	if colBreak != -1 {
		keyCol = line[:colBreak]
	}
	// trim quotes and normalize case
	keyCol = bytes.Trim(keyCol, "'\"")
	keyCol = bytes.ToUpper(keyCol)
	return keyCol, line, nil
}

func base64string(b []byte) string {
	buf := new(bytes.Buffer)
	encoder := base64.NewEncoder(base64.StdEncoding, buf)
	encoder.Write(b)
	encoder.Close()
	return buf.String()
}
