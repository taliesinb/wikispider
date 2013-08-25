package wikispider

import (
	"strings"
	"unicode"
	"sort"
)

type sorter struct {
	str []string
	num []int
}

func (t sorter) Len() int {
	return len(t.str)
}

func (t sorter) Less(i, j int) bool {
	return t.num[i] > t.num[j]
}

func (t sorter) Swap(i, j int) {
	t.num[i], t.num[j] = t.num[j], t.num[i]
	t.str[i], t.str[j] = t.str[j], t.str[i]
}

func MostCommon(text string, words []string, n int) []string {

	counts := make(map[string]int, 128)
	
	tokens := strings.FieldsFunc(text,
		func (r rune) bool { return !unicode.IsLetter(r) })

	for _, t := range tokens {
		counts[t]++
	}

	tally := sorter{
		make([]string, 0, len(words)),
		make([]int, 0, len(words)),
	}

	dedup := make(map[string]bool, len(words))
	for _, word := range words {
		if dedup[word] { continue }
		dedup[word] = true
		tally.str = append(tally.str, word)
		tally.num = append(tally.num, counts[word])
	}

	sort.Sort(tally)

	if n == -1 || n >= len(words) {
		return tally.str
	}
	return tally.str[:n]
}
