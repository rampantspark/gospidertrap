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
	port              = ":8000"
	delayMilliseconds = 350
	charSpace         = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890_-/"
)

var (
	webpages []string
	chars    = []rune(charSpace)
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

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
	fmt.Println("Usage:", os.Args[0], "[-a HTML_FILE]\n")
	fmt.Println("HTML_FILE is an HTML file containing links to be replaced with random ones.")
}

func main() {
	var htmlFile string
	flag.StringVar(&htmlFile, "a", "", "HTML file containing links to be replaced")
	flag.Parse()

	if htmlFile != "" {
		file, err := os.Open(htmlFile)
		if err != nil {
			fmt.Println("Can't read input file. Using randomly generated links.")
		} else {
			defer file.Close()
			buf := make([]byte, 1024)
			for {
				n, err := file.Read(buf)
				if err != nil && err != io.EOF {
					fmt.Println("Error reading input file.")
					break
				}
				if n == 0 {
					break
				}
				content := string(buf[:n])
				lines := strings.Split(content, "\n")
				for _, line := range lines {
					if strings.Contains(line, "<a href=\"") {
						// Extract link from <a href=""> tag
						linkStart := strings.Index(line, "<a href=\"") + len("<a href=\"")
						linkEnd := strings.Index(line[linkStart:], "\"") + linkStart
						if linkStart >= len("<a href=\"") && linkEnd > linkStart {
							link := line[linkStart:linkEnd]
							webpages = append(webpages, link)
						}
					}
				}
			}
			if len(webpages) == 0 {
				fmt.Println("No links found in the HTML file. Using randomly generated links.")
			}
		}
	}

	http.HandleFunc("/", handleRequest)
	fmt.Println("Starting server on port", port, "...")
	err := http.ListenAndServe(port, nil)
	if err != nil {
		fmt.Println("Error starting HTTP server on port", port+".", err)
	}
}

