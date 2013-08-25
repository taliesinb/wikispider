package wikispider

import (
	"net/url"
	"net/http"
	"io/ioutil"
	"strings"
	"time"
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
	kinds []string
	links []string
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

func (art *Article) Download(cachePath string, client http.Client, limiter chan bool, staleTime time.Time) ( cached, ok bool ) {

	if art.body != "" {
		panic("Article already loaded")
	}
	
retry_redirect:

	escaped := url.QueryEscape(art.title)
	filePath := filepath.Join(cachePath, escaped + ".wiki")
	stat, err := os.Stat(filePath)

	// either load from cache or download, redirect logic happens afterwards
	if err == nil && stat.ModTime().After(staleTime) {
		
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
		if err != nil { 
			print(err.Error())
			return false, false 
		}

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
		log.Printf("\t⟲ %-64q", art.title)
		art.title = redirect
		goto retry_redirect
	}

	// sometimes redirects and normalization form a loop, guard against this
	if art.redirects <= 1 {		
		if normalized := NormalizeTitle(art.title); normalized != art.title {
			art.Write(filePath) // we want to store both versions
			art.title = normalized
		}
	}
	return 
}

// return all non-template internal wikipedia links in a given article 
func (a* Article) Links(n int, rank bool) (links []string) {
	
	if a.links == nil {
		links = make([]string, 0, 32)

	outer: for s := a.body ; len(s) > 1 ; s = s[1:] {
			if s[0] == '[' && s[1] == '[' {
				var i int
				for i = 2; s[i] != ']' && s[i] != '|' ; i++ {
					if s[i] == ':' || s[i] == '\n' || i >= 64 {
						continue outer
					}
				}
				link := string(s[2:i])
				links = append(links, link)
			}
		}

		a.links = links

		if n == -1 || n >= len(links) {
			return
		}
	} else {
		links = a.links
	}

	if rank {
		return MostCommon(a.body, links, n)
	} else if n == -1 || n >= len(links) { 
		return links
	} else {
		return links[:n]
	}
}

func (art *Article) Kinds() (kinds []string) {

	if art.kinds != nil { return art.kinds }

	str := art.body
	kinds = make([]string,0,32)

	if strings.Contains(str, "{{Persondata") {
		kinds = append(kinds, "person")
	}

	for {
		index := strings.Index(str, "{{Infobox")
		if index == -1 {break}
		str = str[index+10:]
		index2 := strings.IndexAny(str, "<|\n")
		if index2 == -1 || index2 >= 32 { continue }
		kind := strings.ToLower(strings.Trim(string(str[0:index2]), " "))
		kinds = append(kinds, kind)
	}

	
	art.kinds = kinds
	return
}

func NormalizeTitle(title string) string {
	if title == "" { return "" }
	first := true
	res := strings.Map(
		func(r rune) rune {
		if first {
			first = false
			return unicode.ToUpper(r)
		} 
		if r == ' ' { first = true; return '_' }
		return r
	}, title)
	res = strings.Replace(res, "_Of_", "_of_", -1)
	ind := strings.Index(res, "#")
	if ind != -1 {
		res = res[:ind]
	}
	return res
}
