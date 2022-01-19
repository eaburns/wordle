package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
)

// wordListPath is the path to a word list file.
// The format is each line has two space separated fields:
// the first field is the word, and the second field is the frequency
// of the word in whatever corpus.
//
// The word list that I am using is the Wikipedia word/frequency list from
// https://github.com/IlyaSemenov/wikipedia-word-frequency/blob/master/results/enwiki-20190320-words-frequency.txt
const wordListPath = "./freq.txt"

// minFrequency is the minimum allowed frequency for the initial candidate list.
// Words must appearing in the input word list with a lower frequency
// are immediately eliminated from consideration.
//
// Increasing minFrequency will reduce the initial word candidate pool size,
// leaving only more common words.
// This will make the inital suggestion time faster,
// and it will reduce the number of rare, unlikely suggestions.
// However it will also increase the chance that the targe word
// is not among the suggestions at all.
const minFrequency = 1000

// nExpectedNextSetSize is number of candidates for which
// to compute the full expected next set size.
// Candidates are first ordered by a heuristic score
// based on letter frequency. We then compute the full
// expected next set size for the top nExpectedNextSetSize
// of the candidates to show for the suggestions at each step.
const nExpectedNextSetSize = 20

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to `file`")

func main() {
	flag.Parse()
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			fmt.Printf("could not create CPU profile: ", err)
			os.Exit(1)
		}
		defer f.Close() // error handling omitted for example
		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Printf("could not start CPU profile: ", err)
			os.Exit(1)
		}
		defer pprof.StopCPUProfile()
	}

	words := initialCandidates()
	suggest(words)
	scanner := bufio.NewScanner(os.Stdin)
	for {
		if !scanner.Scan() {
			break
		}
		if scanner.Text() == "" {
			break
		}
		c := inputConstraints(scanner.Text())
		if c == nil {
			fmt.Println("Enter 5 fields of the form XY where X is -, +, or ~ and Y is a letter a-z.")
			fmt.Println("	- means wrong letter; doesn't appear in the word")
			fmt.Println("	+ means correct letter")
			fmt.Println("	~ means letter appears in the word in a different position")
			continue
		}
		words = filter(c, words)
		suggest(words)
	}
}

type word struct {
	word  string
	freq  int
	score int
	exp   float64
}

func initialCandidates() []word {
	data, err := ioutil.ReadFile(wordListPath)
	if err != nil {
		fmt.Printf("failed to read frequency file: %s", err)
		os.Exit(1)
	}
	words := make([]word, 0, 4096)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		w := fields[0]
		if len(w) != 5 || strings.IndexFunc(w, func(r rune) bool {
			return r < 'a' || r > 'z'
		}) >= 0 {
			continue
		}
		freq, err := strconv.Atoi(fields[1])
		if err != nil {
			fmt.Printf("failed to parse word frequency: %s", err)
			os.Exit(1)
		}
		if freq < minFrequency {
			continue
		}
		words = append(words, word{word: w, freq: freq})
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("error reading frequency file: %s", err)
		os.Exit(1)
	}
	return words
}

type constraints struct {
	position    [5]byte
	notPosition [5][]byte
	contains    []byte
	notContains []byte
}

func newConstraints() *constraints {
	return &constraints{
		position: [5]byte{},
		notPosition: [5][]byte{
			[]byte{},
			[]byte{},
			[]byte{},
			[]byte{},
			[]byte{},
		},
		contains:    nil,
		notContains: nil,
	}
}

// inputConstraints returns constraints based on the user input line.
func inputConstraints(line string) *constraints {
	c := newConstraints()
	fields := strings.Fields(line)
	if len(fields) != 5 {
		return nil
	}
	for _, field := range fields {
		if len(field) != 2 {
			return nil
		}
		op := field[0]
		b := field[1]
		if op != '-' && op != '+' && op != '~' || b < 'a' || b > 'z' {
			return nil
		}
	}
	for i, field := range fields {
		op := field[0]
		b := field[1]
		switch op {
		case '-':
			c.notContains = append(c.notContains, b)
		case '+':
			c.position[i] = b
		case '~':
			c.notPosition[i] = append(c.notPosition[i], b)
			c.contains = append(c.contains, b)
		}
	}
	return c
}

// filter returns words, filtered to only those words that satisfy the constraints.
func filter(c *constraints, words []word) []word {
	var i int
	for _, w := range words {
		if satisfies(c, w.word) {
			words[i] = w
			i++
		}
	}
	return words[0:i]
}

// satisfies returns whether a word satisifes the constraints.
func satisfies(c *constraints, word string) bool {
	for i := 0; i < 5; i++ {
		got := word[i]
		if want := c.position[i]; want != 0 && got != want {
			return false
		}
		for _, b := range c.notPosition[i] {
			if got == b {
				return false
			}
		}
	}
	for _, b := range c.contains {
		if word[0] != b && word[1] != b && word[2] != b && word[3] != b && word[4] != b {
			return false
		}
	}
	for _, b := range c.notContains {
		if word[0] == b || word[1] == b || word[2] == b || word[3] == b || word[4] == b {
			return false
		}
	}
	return true
}

// suggest suggests 10 words from the candidate set, words,
// printing the most preferred choice last.
func suggest(words []word) {
	posFreq := letterFreqByPosition(words)
	posScore := letterScoreByPosition(posFreq)

	// Compute word scores as the sum of the letter frequency ranks.
	for i := range words {
		words[i].score = score(posScore, words[i].word)
	}
	sort.Slice(words, func(i, j int) bool {
		scorei := words[i].score
		scorej := words[j].score
		if scorei == scorej {
			return words[i].freq > words[j].freq
		}
		return scorei > scorej
	})

	// Computed expected next set size for the top N words by score.
	// We do top N here by score, because this computation is O(n^2)
	// in the number of candidates words.
	n := nExpectedNextSetSize
	if len(words) < n {
		n = len(words)
	}
	top := words[0:n]
	for i := range top {
		top[i].exp = expectedNextSetSize(words, top[i].word)
	}
	sort.Slice(top, func(i, j int) bool {
		expi := top[i].exp
		expj := top[j].exp
		if expi == expj {
			freqi := top[i].freq
			freqj := top[j].freq
			if freqi == freqj {
				return top[i].score < top[j].score
			}
			return freqi < freqj
		}
		return expi > expj
	})

	// Print the top 10 words in decreasing order of expected set size.
	for _, ws := range top {
		fmt.Printf("freq: %-8d score: %-5d exp: %-5.2f: %s\n", ws.freq, ws.score, ws.exp, ws.word)
	}
	fmt.Printf("%d candidates\n", len(words))
}

// Computes the frequency of each letter in each position.
func letterFreqByPosition(words []word) [5][255]int {
	var freq [5][255]int
	for i := range words {
		for i, r := range words[i].word {
			freq[i][r]++
		}
	}
	return freq
}

// Computes a letter frequency rank by position.
// The score is for each position, for each letter in said position,
// the rank of that letter among all letters sorted in increasing order
// of their frequency in the given position.
//
// We are sloppy and ignore the fact that letters are a-z,
// and instead just compute across all ASCII 0-255.
// Of course most of these will have frequency 0, but that's fine.
//
// So, for example, the most frequent letter in a given position
// will have a score of 255, the second most frequent
// will have a score of 254, and so on.
func letterScoreByPosition(posFreq [5][255]int) [5][255]int {
	order := make([]byte, 255)
	var posScore [5][255]int
	for i := 0; i < 5; i++ {
		for j := 0; j < len(order); j++ {
			order[j] = byte(j)
		}
		sort.Slice(order, func(k, l int) bool {
			return posFreq[i][order[k]] < posFreq[i][order[l]]
		})
		for j := 0; j < len(order); j++ {
			posScore[i][order[j]] = j
		}
	}
	return posScore
}

// score computes a score for the word
// as the sum of the letter frequency ranks by position.
func score(posScore [5][255]int, word string) int {
	score := 0
	for i, r := range word {
		score += posScore[i][r]
	}
	return score
}

// expectedNextSetSize computes the expected next set size;
// the expecteded number of candidates left after guessing guess
// given the candidate pool words.
func expectedNextSetSize(words []word, guess string) float64 {
	c := newConstraints()
	var avg float64
	for i := range words {
		clearConstraints(c)
		applyDiffConstraint(c, guess, words[i].word)
		var n int
		for j := range words {
			if satisfies(c, words[j].word) {
				n++
			}
		}
		avg = avg + (float64(n)-avg)/float64(i+1)
	}
	return avg
}

func clearConstraints(c *constraints) {
	for i := range c.position {
		c.position[i] = 0
	}
	for i := range c.notPosition {
		c.notPosition[i] = c.notPosition[i][:0]
	}
	c.contains = c.contains[:0]
	c.notContains = c.notContains[:0]
}

// applyDiffConstraint adds constraints to c assuming we guessed guess
// but the answer was actually answer.
func applyDiffConstraint(c *constraints, guess string, answer string) {
	for i := 0; i < 5; i++ {
		if guess[i] == answer[i] {
			c.position[i] = guess[i]
			continue
		}
		c.notPosition[i] = append(c.notPosition[i], guess[i])
		if !strings.ContainsRune(answer, rune(guess[i])) {
			c.notContains = append(c.notContains, guess[i])
		} else {
			c.contains = append(c.contains, guess[i])
		}
	}
}
