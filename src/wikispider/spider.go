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

func Spider(titles []string, path string, maxdepth, maxwidth, pool, delay int, kind string) {

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
					log.Printf("\tError dowloading \"%s\"", page.title)
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
		t = SanitizeTitle(t)
		counter.Add(1)
		download <- &Article{title:t}
	}

	// loop through the visit queue, which consists of the downloaded pages in breadth-first order
	// successive depths ('generations') are separated by 'nil' pages
	for page := range visitReader {

		if page != nil && depth <= maxdepth { // did we get a downloaded page to process?

			links := page.Links()
			if kind != "" && !Intersect(page.infobox, kind) {
				continue
			}
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
	escapedTitles := url.QueryEscape(strings.Join(titles,"_"))
	graphfilename := fmt.Sprintf("graph_%s_%d_%d.tsv", escapedTitles, maxdepth, maxwidth)
	graphpath := filepath.Join(path,graphfilename)
	file, err := os.Create(graphpath)
	if err == nil {
		for title, links := range graph {
			str := url.QueryEscape(title)
			for _, link := range links { str += "\t" + url.QueryEscape(link) }
			str += "\n"
			file.Write([]byte(str))
		}
	} else {
		log.Printf("Couldn't write graph to disk, err is %s", graphpath,err)
	}

	// TODO: Properly clean up infinite channel, download workers
	
	log.Printf("Visited %d unique pages, downloaded %d pages, %d errors", total, total_new, total_errors)
	file.Close()
}

func SanitizeTitle(oldtitle string) (newtitle string) {
	return strings.Replace(strings.ToUpper(string(oldtitle[0])) + strings.ToLower(oldtitle[1:]), " ", "_", -1)

}

func Intersect(infoboxes []string , kind string ) bool {
	for _,v := range(infoboxes){
		if v == kind {
			return true
		}
	}
	return false
}
