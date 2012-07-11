package wikispider

import (
	"strings"
	"net"
	"net/url"
	"net/http"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
	"fmt" 
)


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

func Spider(titles []string, path string, maxdepth, maxwidth, pool, delay int, kind string, rank bool) {

	var total, total_new, total_errors int

	// articles to be downloaded
	download := make(chan *Article, 32)

	// rate limiter, shared by all download workers
	limiter := RateLimiter(delay)
	
	// how many articles are currently downloading, used as a barrier to next generation
	counter := new(sync.WaitGroup)

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
					log.Printf("\tError dowloading \"%s\"", page.title)
					total_errors++
				}
				
				counter.Done()
			}
		}()
	}

	// normalize all the titles
	for i := range titles {
		titles[i] = NormalizeTitle(titles[i])
	}

	// kickstart the first generation
	// nil signals the end of a generation; the first generation is entirely empty
	depth := 0
	visitWriter <- nil 
	for _, t := range titles {
		counter.Add(1)
		download <- &Article{title:t, parent:""}
	}

	// a graph file will be continually appended with parent<tab>child pairs
	escapedTitles := url.QueryEscape(strings.Join(titles, "_"))
	graphfilename := fmt.Sprintf("graph-%d-%d-%s.tsv", maxdepth, maxwidth, escapedTitles)
	graphpath := filepath.Join(path, graphfilename)
	graphfile, err := os.Create(graphpath)

	if err != nil {
		panic("Can't open graph file \"" + graphpath + "\"")
	}

	// loop through the visit queue, which consists of the downloaded pages in breadth-first order
	// successive depths ('generations') are separated by 'nil' pages
	for page := range visitReader {

		if page != nil && depth <= maxdepth { // did we get a downloaded page to process?

			links := page.Links(maxwidth, rank)
			
			if kind != "" && !Intersect(page.infobox, kind) {
				continue
			}

			fmt.Fprintf(graphfile, "%s\t%s\n",
				url.QueryEscape(page.parent),
				url.QueryEscape(page.title))

			if depth < maxdepth {
				for _, link := range links {

					counter.Add(1)
					download <- &Article{title:link, parent:page.title}
				}
			}

		} else { // did we hit this generation's cap?

			if depth < maxdepth {
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

	// TODO: Properly clean up infinite channel, download workers
	
	log.Printf("Visited %d unique pages, downloaded %d pages, %d errors", total, total_new, total_errors)
	
	graphfile.Close()
}

func Intersect(infoboxes []string , kind string ) bool {
	for _,v := range(infoboxes){
		if v == kind {
			return true
		}
	}
	return false
}
