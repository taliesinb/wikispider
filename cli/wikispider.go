package main

import (
	"github.com/taliesinb/wikispider"
	"flag"
	"strings"
	"os"
	"log"
)


func main() {

	var depth, width, pool, limit int
	var path, kind string
	var rank, force bool
	
	flag.IntVar(&depth, "depth", 2, "Depth to traverse to")
	flag.IntVar(&width, "width", 3, "Number of links to get from each page")
	flag.BoolVar(&rank, "rank", true, "Rank links beforing limiting them")
	flag.BoolVar(&force, "force", false, "Force redownload of old articles")
	flag.IntVar(&pool, "pool", 4, "Number of simultaneous downloads")
	flag.IntVar(&limit, "limit", 300, "Delay between downloads, in milliseconds (global to pool)")
	flag.StringVar(&path, "path", "pages", "Directory in which to put the visited pages")
	flag.StringVar(&kind, "kind", "", "Which kind of infoboxes we want to download")
	flag.Parse()

	if flag.NArg() == 0 {
		flag.PrintDefaults()
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
	
	kinds := strings.Split(kind, ",")
	if len(kinds) == 0 { kinds = nil }
	wikispider.Spider(flag.Args(), path, depth+1, width, pool, limit, kinds, rank, force)
}
