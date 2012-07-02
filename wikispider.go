package main

import (
	"net"
	"net/url"
	"net/http"
	"log"
	"io/ioutil"
	"os"
	"flag"
	"path/filepath"
	"sync"
	"time"
)

type Article struct {
	title string
	body []byte
}

func (art *Article) Download(cachePath string, client http.Client, limiter chan bool) ( cached, ok bool ) {

	if art.body != nil {
		panic("Article already loaded")
	}

	escaped := url.QueryEscape(art.title)
	cachePath = filepath.Join(cachePath, escaped + ".wiki")
	_, err := os.Stat(cachePath)

	if err == nil {
		file, _ := os.Open(cachePath)
		art.body, _ = ioutil.ReadAll(file)
		file.Close()
		return true, true
	}
	
	<-limiter

	url := "http://en.wikipedia.org/w/index.php?title=" + escaped + "&action=raw"

	resp, err := client.Get(url)
	if err != nil {
		return false, false
	}

	defer resp.Body.Close()
	art.body, err = ioutil.ReadAll(resp.Body)

	if err != nil {
		return false, false
	}

	file, err := os.Create(cachePath)
	if err == nil { 
		file.Write(art.body)
		file.Close()
	} else {
		log.Printf("Couldn't write body of page %s to disk", art.title)
		return false, false
	}

	return false, true
}

// return all non-template internal wikipedia links in a given article 
func (a* Article) Links() []string {
	slices := make([]string, 0, 32)

outer: for s := a.body ; len(s) > 0 ; s = s[1:] {
		if s[0] == '[' && s[1] == '[' {
			var i int
			for i = 0; s[i] != ']' && s[i] != '|' ; i++ {
				if s[i] == ':' {
					continue outer
				}
			}
			slices = append(slices, string(s[2:i]))
		}
	}

	return slices
}

func RateLimiter(delay int) chan bool {
	
	limiter := make(chan bool)
	go func() {
		if delay == 0 {
			for {
				limiter <- true
			}
		} else {
			ticker := time.NewTicker(time.Duration(delay) * time.Millisecond)
			for {
				<- ticker.C
				limiter <- true
			}
		}	
	}()
	return limiter
}

func Spider(titles []string, path string, maxdepth, maxwidth, pool, delay int) {

	var total, total_new, total_errors int

	// articles to be downloaded
	download := make(chan *Article, 32)

	// rate limiter, shared by all download workers
	limiter := RateLimiter(delay)
	
	// how many articles are currently downloading, used as a barrier to next generation
	counter := new(sync.WaitGroup)
	
	// which articles have we already visited
	visited := make(map[string]bool, 8192)

	// out-edges for each article
	graph :=  make(map[string][]string, 8192)

	// virtual channel with infinite buffer, modified from http://play.golang.org/p/AiHBsxTFpj
	visitWriter := make(chan *Article)
	visitReader := make(chan *Article)
	visitBuffer := make([]*Article, 0, 64)
	go func() {
		for {
			if len(visitBuffer) == 0 { visitBuffer = append(visitBuffer, <-visitWriter) } 
			select {
			case visitReader <- visitBuffer[0]: visitBuffer = visitBuffer[1:]; 
			case page := <- visitWriter: visitBuffer = append(visitBuffer, page); 
			}
		} 
	}()

	// pool of download workers, each with their own http client
	// workers consume pages from the download queue, download them, and send them to the visit queue
	for i := 0; i < pool; i++ {
		go func(){
			// timeout code modified from https://groups.google.com/group/golang-nuts/browse_thread/thread/d9e86a6fab79e240
			client := http.Client{Transport: &http.Transport{
				Dial: func(netw, addr string) (net.Conn, error) { 
						c, err := net.DialTimeout(netw, addr, time.Duration(2e9))
						if err != nil { return nil, err } 
						return c, nil
					},
				},
			}
			for page := range download {
				
				cached, ok := page.Download(path, client, limiter)
				
				if ok {
					total++
					if !cached {
						log.Printf("\tDownloaded \"%s\"", page.title)
						total_new++
					} else {
						log.Printf("\tAlready have \"%s\"", page.title)
					}
					visitWriter <- page	
				} else {
					log.Printf("\tError downloading \"%s\"", page.title)
					total_errors++
				}
				
				counter.Done()
			}
		}()
	}

	// kickstart the first generation
	// nil signals the end of a generation; the first generation is entirely empty
	depth := 0
	visitWriter <- nil 
	for _, t := range titles {
		counter.Add(1)
		download <- &Article{title:t}
	}

	// loop through the visit queue, which consists of the downloaded pages in breadth-first order
	// successive depths ('generations') are separated by 'nil' pages
	for page := range visitReader {

		if page != nil && depth <= maxdepth { // did we get a downloaded page to process?

			links := page.Links()
			width := 0

			graph[page.title] = links
			
			for _, link := range links {

				// articles are cached to disk, but this allows us to avoid
				// counting links we've already seen towards the width limit
				if visited[link] { continue } 
				visited[link] = true
				
				counter.Add(1)
				download <- &Article{title:link}
				
				width++
				if width > maxwidth { break }
			}

		} else { // did we hit this generation's cap?

			if depth <= maxdepth {
				log.Printf("Downloading generation %d\n", depth)
			}
			
			// wait for the next generation's visits are all on the queue by 
			// waiting for the current generation to finish downloading
			counter.Wait()

			if depth > maxdepth { break }
			depth++

			// cap the next generation
			visitWriter <- nil
		}
	}

	// write the graph
	graphpath := filepath.Join(path, "graph.tsv")
	file, err := os.Create(graphpath)
	if err == nil {
		for title, links := range graph {
			str := url.QueryEscape(title)
			for _, link := range links { str += "\t" + url.QueryEscape(link) }
			str += "\n"
			file.Write([]byte(str))
		}
		file.Close()
	} else {
		log.Printf("Couldn't write graph to disk", graphpath)
	}

	// TODO: Properly clean up infinite channel, download workers
	
	log.Printf("Visited %d unique pages, downloaded %d pages, %d errors", total, total_new, total_errors)
}

func main() {

	var depth, width, pool, limit int
	var path string
	
	flag.IntVar(&depth, "depth", 1, "Depth to traverse to")
	flag.IntVar(&width, "width", 3, "Number of links to get from each page")
	flag.IntVar(&pool, "pool", 4, "Number of simultaneous downloads")
	flag.IntVar(&limit, "limit", 300, "Delay between downloads, in milliseconds (global to pool)")
	flag.StringVar(&path, "path", "pages", "Directory in which to put the visited pages")
	flag.Parse()

	if flag.NArg() == 0 {
		panic("Require one or more starting articles")
	}

	p, ok := os.Stat(path)
	if ok != nil {
		log.Printf("Creating output directory \"%s\"\n", path)
		wd, _ := os.Getwd()
		wdi, _ := os.Stat(wd)
		if os.Mkdir(path, wdi.Mode()) != nil {
			panic("Couldn't create output directory")
		}
	} else if !p.IsDir() {
		panic("Output path is not a directory")
	}

	Spider(flag.Args(), path, depth, width, pool, limit)
}
