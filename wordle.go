package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"
)

// freqListPath is the path to a list of word-frequency pairs,
// one pair per-line, separated by space.
const freqListPath = "./freq2_filtered_dedup.txt"

// smallSetSize is the size threshold to consider a candidate set size small.
// For small candidate sets, compute expected next-set size for all words.
const smallSetSize = 500

// topSetSize is number of candidates for which
// to compute the full expected next-set size
// if the total candidate list is larger than smallSetSize.
const topSetSize = 20

var answer = flag.String("answer", "", "simulates play to find the specified answer")
var verbose = flag.Bool("v", false, "verbose printing when simulating play")
var guess0 = flag.String("guess0", "", "first guess to try when simulating play")

func main() {
	flag.Parse()

	words := initialCandidates()

	if *answer != "" {
		c := newConstraints()
		n := 0
		pass := false
		for len(words) > 0 {
			var guess string
			if n == 0 && *guess0 != "" {
				// The first call to sortWords is very slow,
				// allow specifying the hard-coded guess
				// from the command-line to speed up.
				guess = *guess0
			} else {
				sortWords(words)
				guess = words[len(words)-1].word
			}
			if *verbose {
				fmt.Printf("guess: %s\n", guess)
			}
			n++
			if guess == *answer {
				pass = true
				break
			}
			clearConstraints(c)
			applyDiffConstraint(c, guess, *answer)
			if *verbose {
				fmt.Printf("%s\n", c)
			}
			words = filter(c, words)
		}
		if pass {
			fmt.Printf("passed in ")
		} else {
			fmt.Printf("failed in ")
		}
		fmt.Printf("%d guesses\n", n)
		return
	}

	scanner := bufio.NewScanner(os.Stdin)
	suggest(words)
	for len(words) > 1 {
		fmt.Printf("> ")
		if !scanner.Scan() || scanner.Text() == "quit" {
			break
		}
		c := inputConstraints(scanner.Text())
		if *verbose {
			fmt.Printf("%s\n", c)
		}
		if c == nil {
			fmt.Println("Enter 5 fields of the form XY where X is -, +, or ~ and Y is a letter a-z.")
			fmt.Println("	- means wrong letter; doesn't appear in the word")
			fmt.Println("	+ means correct letter")
			fmt.Println("	~ means letter appears in the word in a different position")
			fmt.Println("'quit' to quit.")
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
	data, err := ioutil.ReadFile(freqListPath)
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
	notPosition [5][26]bool
	contains    []byte
}

func newConstraints() *constraints {
	return &constraints{
		position:    [5]byte{},
		notPosition: [5][26]bool{[26]bool{}, [26]bool{}, [26]bool{}, [26]bool{}, [26]bool{}},
		contains:    nil,
	}
}

func (c *constraints) String() string {
	var s strings.Builder
	for i := 0; i < 5; i++ {
		if c.position[i] != 0 {
			fmt.Fprintf(&s, "+%c ", c.position[i])
		}
		for j, not := range c.notPosition[i] {
			if not {
				fmt.Fprintf(&s, "-%c ", j+'a')
			}
		}
		fmt.Fprintf(&s, "\n")
	}
	for _, c := range c.contains {
		fmt.Fprintf(&s, "%c ", c)
	}
	return s.String()
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
	// First go through + and ~ ops; we can only understand - after knowing the + positions.
	for i, field := range fields {
		switch field[0] {
		case '+':
			c.position[i] = field[1]
		case '~':
			c.notPosition[i][field[1]-'a'] = true
			c.contains = append(c.contains, field[1])
		}
	}
	// Now that we know the + ops, go through and figure out the - ops.
	for _, field := range fields {
		if field[0] != '-' {
			continue
		}
		for i := 0; i < 5; i++ {
			if c.position[i] == 0 {
				c.notPosition[i][field[1]-'a'] = true
			}
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
		if want := c.position[i]; want != 0 {
			if got != want {
				return false
			}
		} else {
			if c.notPosition[i][got-'a'] {
				return false
			}
		}
	}
	for _, b := range c.contains {
		found := false
		for i := 0; i < 5; i++ {
			if c.position[i] == 0 && word[i] == b {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// suggest suggests  words from the candidate set, words,
// printing the most preferred choice last.
func suggest(words []word) {
	sortWords(words)
	n := 20
	if n >= len(words) {
		n = len(words)
	}
	for _, ws := range words[len(words)-n : len(words)] {
		fmt.Printf("%-8s (exp: %-8.2f freq: %-8d score: %-5d)\n",
			ws.word, ws.exp, ws.freq, ws.score)
	}
	fmt.Printf("%d candidates\n", len(words))
}

// sortWords sorts the words in increasing order or preference.
// The last word is the most preferred.
func sortWords(words []word) {
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
			return words[i].freq < words[j].freq
		}
		return scorei < scorej
	})

	// If the candidate set is not small, only compute next-set size
	// for the topSetSize words by score.
	n := len(words)
	if n > smallSetSize && topSetSize < n {
		n = topSetSize
	}
	top := words[len(words)-n : len(words)]
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
		for j := range c.notPosition[i] {
			c.notPosition[i][j] = false
		}
	}
	c.contains = c.contains[:0]
}

// applyDiffConstraint adds constraints to c assuming we guessed guess
// but the answer was actually answer.
func applyDiffConstraint(c *constraints, guess string, answer string) {
	// First set the + constraints, because - and ~ depend on knowing the + values.
	for i := 0; i < 5; i++ {
		if guess[i] == answer[i] {
			c.position[i] = guess[i]
		}
	}
	for i := 0; i < 5; i++ {
		if c.position[i] != 0 {
			continue
		}
		found := false
		for j := 0; j < 5; j++ {
			if c.position[j] != 0 {
				continue
			}
			if answer[j] == guess[i] {
				found = true
			}
		}
		if found {
			c.notPosition[i][guess[i]-'a'] = true
			c.contains = append(c.contains, guess[i])
		} else {
			for j := 0; j < 5; j++ {
				if c.position[j] == 0 {
					c.notPosition[j][guess[i]-'a'] = true
				}
			}
		}
	}
}
