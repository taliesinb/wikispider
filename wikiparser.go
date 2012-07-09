package main

import(
  //  "net"
  "net/url"
  "net/http"
  "io/ioutil"
  "os"
  "path/filepath"
  //"sync"
  //"time"
  "log"
  "bytes"
  //"fmt"

)

type Article struct {
	title string
	body []byte
	redirects int
	infobox []string
}

func (art *Article) GetInfobox() (thereis bool) {

	str := art.body
	results := make([]string,0,32)
	for {
		index := bytes.Index(str, []byte("{{Infobox"))
		if index == -1 {break}
		str = str[index+10:]
		index2 := bytes.Index(str, []byte("\n"))
		results = append(results, string(str[0:index2]))
	}
	art.infobox=results
	
	return len(results) > 0
}


func (art *Article) CheckRedirect() (url string) {

	str := art.body
	index := bytes.Index(str, []byte("#REDIRECT"))
	if index == -1 { return }
	index   = bytes.Index(str, []byte("[["))
	index2 := bytes.Index(str, []byte("]]"))
	temp:=bytes.Split(str[index+2:index2], []byte("#"))
	art.title = string(temp[0])
	
	return string(art.title)
}


func (art *Article) Download(cachePath string, client http.Client, limiter chan bool) ( cached, ok bool ) {

	if art.body != nil {
		panic("Article already loaded")
	}
retry_redirect:
// I think the problem is that Join "remembers" where we were directed and
// appends the new article to it. Which it shouldn't.
// FIXED  
	escaped := url.QueryEscape(art.title)
  filePath := filepath.Join(cachePath, escaped + ".wiki")
	_, err := os.Stat(filePath)

	if err == nil {
		file, _ := os.Open(filePath)
		art.body, _ = ioutil.ReadAll(file)
		file.Close()
		return true, true
	}
	
	<-limiter

	address := "http://en.wikipedia.org/w/index.php?title=" + escaped + "&action=raw"

	resp, err := client.Get(address)
	if err != nil {
		return false, false
	}

	defer resp.Body.Close()
	art.body, err = ioutil.ReadAll(resp.Body)

	if err != nil {
		return false, false
	}

	redirect := art.CheckRedirect()
	if redirect != "" {
		art.redirects++
		if art.redirects > 4 {return false, false}
		art.title = url.QueryEscape(redirect)
		log.Printf("Got redirected %d times, now to %s",art.redirects, art.title)
		art.body = nil
		goto retry_redirect
	}

  if art.redirects == 0 {
    art.title = SanitizeTitle(art.title)
  }

	art.GetInfobox()

	file, err := os.Create(filePath)
	if err == nil { 
		file.Write(art.body)
		file.Close()
	} else {
		log.Printf("Couldn't write body of page %s to disk and I have this error:\n\n %s\n\n", art.title, err)
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
