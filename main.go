package main

import (
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

// Configuration
const (
	linksPerPageMin   = 5
	linksPerPageMax   = 10
	lengthOfLinksMin  = 3
	lengthOfLinksMax  = 20
	defaultPort       = "8000"
	delayMilliseconds = 350
	charSpace         = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890_-/"
)

var (
	webpages []string
	chars    = []rune(charSpace)
	port     string
	endpoint string
)

func generatePage() string {
	var html string
	html += "<html>\n<body>\n"
	numPages := rand.Intn(linksPerPageMax-linksPerPageMin+1) + linksPerPageMin
	for i := 0; i < numPages; i++ {
		address := ""
		if len(webpages) > 0 {
			address = webpages[rand.Intn(len(webpages))]
		} else {
			address = randString(rand.Intn(lengthOfLinksMax-lengthOfLinksMin+1) + lengthOfLinksMin)
		}
		html += "<a href=\"" + address + "\">" + address + "</a><br>\n"
	}
	if endpoint != "" {
		html += `<form action="` + endpoint + `" method="get">
		<input type="text" name="param">
		<button type="submit">Submit</button>
		</form>`
	}
	html += "</body>\n</html>"
	return html
}

func randString(length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	time.Sleep(time.Duration(delayMilliseconds) * time.Millisecond)
	w.Header().Set("Content-Type", "text/html")
	io.WriteString(w, generatePage())
}

func printUsage() {
	fmt.Println("Usage:", os.Args[0], "[-p PORT] -a HTML_FILE -w WORDLIST_FILE [-e ENDPOINT]")
	fmt.Println()
	fmt.Println("-p   Port to run the server on (default: 8000)")
	fmt.Println("-a   HTML file input, replace <a href> links")
	fmt.Println("-e   Endpoint to point form GET requests to (optional)")
	fmt.Println("-w   Wordlist to use for links")
}

func main() {
	var htmlFile string
	var wordlistFile string

	flag.StringVar(&port, "p", defaultPort, "Port to run the server on")
	flag.StringVar(&htmlFile, "a", "", "HTML file containing links to be replaced")
	flag.StringVar(&wordlistFile, "w", "", "Wordlist file to use for links")
	flag.StringVar(&endpoint, "e", "", "Endpoint to point form GET requests to")
	flag.Usage = printUsage
	flag.Parse()

	if wordlistFile != "" {
		file, err := os.Open(wordlistFile)
		if err != nil {
			fmt.Println("Can't read wordlist file. Using randomly generated links.")
		} else {
			defer file.Close()
			buf := make([]byte, 1024)
			for {
				n, err := file.Read(buf)
				if err != nil && err != io.EOF {
					fmt.Println("Error reading wordlist file.")
					break
				}
				if n == 0 {
					break
				}
				content := string(buf[:n])
				lines := strings.Split(content, "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if line != "" {
						webpages = append(webpages, line)
					}
				}
			}
			if len(webpages) == 0 {
				fmt.Println("No links found in the wordlist file. Using randomly generated links.")
			}
		}
	}

	http.HandleFunc("/", handleRequest)
	fmt.Println("Starting server on port", port, "...")
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		fmt.Println("Error starting HTTP server on port", port+".", err)
	}
}

