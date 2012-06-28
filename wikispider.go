package main

import "net/url"
import "net/http"
import "log"
import "io/ioutil"
import "os"
import "sync"

type Article struct {
	str []byte
}

func GetArticle(title string) Article {

	path := "pages/" + title
	_, err := os.Stat(path)

	// if a file already exists, return its contents
	if err == nil {
		file, _ := os.Open(path)
		bytes, _ := ioutil.ReadAll(file)
		file.Close()
		log.Printf("Got page %s from disk", title)
		return Article{bytes}
	}
	
	url := "http://en.wikipedia.org/w/index.php?title=" + url.QueryEscape(title) + "&action=raw"

	resp, err := http.Get(url)
	if err != nil {
		log.Panicf("Couldn't get page %s", title)
		return Article{}
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		log.Panicf("Couldn't read body of page %s", title)
		return Article{}
	}

	file, err := os.Create(path)
	if err == nil { 
		file.Write(body)
		file.Close()
	} else {
		log.Panicf("Couldn't write body of page %s to disk", title)
	}

	return Article{body}
}

// return all non-template internal wikipedia links in a given article 
func (a* Article) Links() [][]byte {
	slices := make([][]byte, 0, 32)

outer: for s := a.str ; len(s) > 0 ; s = s[1:] {
		if s[0] == '[' && s[1] == '[' {
			var i int
			for i = 0; s[i] != ']' && s[i] != '|' ; i++ {
				if s[i] == ':' {
					continue outer
				}
			}
			slices = append(slices, s[2:i])
		}
	}

	return slices
}

type Spider struct {
	wait sync.WaitGroup
	maxdepth int
}

func (s *Spider) Step(title string, depth int) {

	tabs := ""
	for i := 0 ; i < depth; i++ { tabs += "\t" }
	log.Printf("%s[%s]\n", tabs, title)
	
	art := GetArticle(title)
	links := art.Links()

	if depth < s.maxdepth {
		for i, a := range links {
			if i > 3 { break }
			s.wait.Add(1)
			go s.Step(string(a), depth+1)
		}
	}

	s.wait.Done()
}

func (s *Spider) Start(title string) {
	s.wait.Add(1)
	s.Step(title, 0)
	s.wait.Wait()
}

func main() {

	var spider Spider

	spider.maxdepth = 3
	spider.Start("Napoleon")
}