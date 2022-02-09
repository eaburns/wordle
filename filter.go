// Filter filters a word-frequency list by a word list.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/kljensen/snowball/english"
)

const (
	dictPath = "/usr/share/dict/words"
	freqPath = "./freq2.txt"
)

func main() {
	dict := loadDict()
	data, err := ioutil.ReadFile(freqPath)
	if err != nil {
		fmt.Printf("failed to read frequency file: %s", err)
		os.Exit(1)
	}
	freq := make(map[string]int, len(dict))
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		freqWord := fields[0]
		if strings.IndexFunc(freqWord, func(r rune) bool {
			return r < 'a' || r > 'z'
		}) >= 0 {
			continue
		}
		stem := english.Stem(freqWord, true)
		if dictWord, ok := dict[stem]; ok {
			f, err := strconv.Atoi(fields[1])
			if err != nil {
				fmt.Printf("failed to parse frequency: %s", err)
				os.Exit(1)
			}
			if len(freqWord) == 5 {
				freq[freqWord] = freq[freqWord] + f
			} else if len(dictWord) == 5 {
				freq[dictWord] = freq[dictWord] + f
			}
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("error reading frequency file: %s", err)
		os.Exit(1)
	}
	sorted := make([]string, 0, len(freq))
	for w := range freq {
		sorted = append(sorted, w)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return freq[sorted[i]] > freq[sorted[j]]
	})
	for _, w := range sorted {
		fmt.Println(w, freq[w])
	}
}

func loadDict() map[string]string {
	data, err := ioutil.ReadFile(dictPath)
	if err != nil {
		fmt.Printf("failed to read dictionary file: %s", err)
		os.Exit(1)
	}
	dict := make(map[string]string, 4096)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		w := scanner.Text()
		if strings.IndexFunc(w, func(r rune) bool {
			return r < 'a' || r > 'z'
		}) >= 0 {
			continue
		}
		stem := english.Stem(w, true)
		if prev, ok := dict[stem]; !ok || len(prev) != 5 {
			dict[stem] = w
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("error reading dictionary file: %s", err)
		os.Exit(1)
	}
	return dict
}
