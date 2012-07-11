package wikispider

import (
	"net/url"
	"net/http"
	"io/ioutil"
	"strings"
	"path/filepath"
	"unicode"
	"log"
	"os"
)

type Article struct {
	title string
	parent string
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
	
	return string(temp[0])
}

func (art *Article) Write(filePath string) {
	err := ioutil.WriteFile(filePath, []byte(art.body), os.FileMode(0666))
	if err != nil {
		log.Printf("\tCouldn't write body of page %s to disk\n", art.title, err)
	}
}

func (art *Article) Download(cachePath string, client http.Client, limiter chan bool) ( cached, ok bool ) {

	if art.body != "" {
		panic("Article already loaded")
	}
	
retry_redirect:

	escaped := url.QueryEscape(art.title)
	filePath := filepath.Join(cachePath, escaped + ".wiki")
	_, err := os.Stat(filePath)

	// either load from cache or download, redirect logic happens afterwards
	if err == nil {
		
		body, err := ioutil.ReadFile(filePath)
		if err != nil {
			log.Printf("\tCouldn't read %q from disk", art.title)
			return true, false
		}
		art.body = string(body)
		cached = true
		
	} else {
		
		<-limiter

		address := "http://en.wikipedia.org/w/index.php?title=" + escaped + "&action=raw"

		resp, err := client.Get(address)
		if err != nil { return false, false }

		bytes, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil { return false, false }
		
		art.body = string(bytes)

		art.Write(filePath)
	}
	ok = true
	
	if redirect := art.CheckRedirect(); redirect != "" {

		art.redirects++
		
		if art.redirects > 4 {
			log.Printf("\tToo many redirects")
			return false, false
		}
		log.Printf("\tRedirected %q to %q", art.title, redirect)
		art.title = redirect
		goto retry_redirect
	}

	// sometimes redirects and normalization form a loop, guard against this
	if art.redirects == 0 {		
		if normalized := NormalizeTitle(art.title); normalized != art.title {
			art.Write(filePath) // we want to store both versions
			art.title = normalized
		}
	}

	art.GetInfobox()

	return 
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

func NormalizeTitle(title string) string {
	if title == "" { return "" }
	first := true
	return strings.Map(
		func(r rune) rune {
		if first {
			first = false
			return unicode.ToUpper(r)
		} 
		if r == ' ' { return '_' }
		return unicode.ToLower(r)
	}, title)
}
