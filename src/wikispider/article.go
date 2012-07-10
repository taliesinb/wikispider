package wikispider

import (
	"net/url"
	"net/http"
	"io/ioutil"
	"strings"
	"path/filepath"
	"log"
	"os"
)


type Article struct {
	title string
	body string
	redirects int
	infobox []string
}

func (art *Article) GetInfobox() (thereis bool) {

	str := art.body
	results := make([]string,0,32)
	for {
		index := strings.Index(str, "{{Infobox")
		if index == -1 {break}
		str = str[index+10:]
		index2 := strings.Index(str, "\n")
		results = append(results, string(str[0:index2]))
	}
	art.infobox = results
	
	return len(results) > 0
}


func (art *Article) CheckRedirect() (url string) {

	str := art.body
	index := strings.Index(str, "#REDIRECT")
	if index == -1 { return }
	index   = strings.Index(str, "[[")
	index2 := strings.Index(str, "]]")
	temp := strings.Split(str[index+2:index2], "#")
	art.title = string(temp[0])
	
	return string(art.title)
}


func (art *Article) Download(cachePath string, client http.Client, limiter chan bool) ( cached, ok bool ) {

	if art.body != "" {
		panic("Article already loaded")
	}
retry_redirect:
	escaped := url.QueryEscape(art.title)
	filePath := filepath.Join(cachePath, escaped + ".wiki")
	_, err := os.Stat(filePath)

	if err == nil {
		body, err := ioutil.ReadFile(filePath)
		if err != nil {
			log.Printf("Couldn't read from disk")
			return true, false
		}
		art.body = string(body)
		return true, true
	}
	
	<-limiter

	address := "http://en.wikipedia.org/w/index.php?title=" + escaped + "&action=raw"

	resp, err := client.Get(address)
	if err != nil {
		return false, false
	}

	bytes, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()

	if err != nil {
		return false, false
	}
	art.body = string(bytes)

	redirect := art.CheckRedirect()
	if redirect != "" {
		art.redirects++
		if art.redirects > 4 {return false, false}
		art.title = redirect
		log.Printf("\tRedirected to %q", art.title)
		goto retry_redirect
	}

	if art.redirects == 0 {
		art.title = SanitizeTitle(art.title)
	}

	art.GetInfobox()

	err = ioutil.WriteFile(filePath, []byte(art.body), os.FileMode(0666))
	if err != nil {
		log.Printf("\tCouldn't write body of page %s to disk\n", art.title, err)
		return false, false
	}

	return false, true
}

// return all non-template internal wikipedia links in a given article 
func (a* Article) Links(n int, rank bool) (links []string) {
	
	links = make([]string, 0, 32)

outer: for s := a.body ; len(s) > 0 ; s = s[1:] {
		if s[0] == '[' && s[1] == '[' {
			var i int
			for i = 0; s[i] != ']' && s[i] != '|' ; i++ {
				if s[i] == ':' {
					continue outer
				}
			}
			links = append(links, string(s[2:i]))
		}
	}

	if n >= len(links) {
		return
	}

	if rank {
		links = MostCommon(a.body, links, n)
	} else {
		links = links[:n]
	}
	return 
}
