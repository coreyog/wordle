package main

import (
	_ "embed"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/coreyog/statux"
	"github.com/fatih/color"
	"github.com/mattn/go-tty"
)

const (
	TotalGuesses = 6
	WordLength   = 5

	KeyCodeBackspace = 127
	KeyCodeEnter     = 13
)

var currentGuess = 0

//go:embed words.txt
var rawWordList string

var wordList []string
var word string
var discovered []bool = make([]bool, WordLength)

var hardMode bool

func main() {
	// process args
	for _, arg := range os.Args[1:] {
		arg = strings.TrimLeft(arg, "-")
		if strings.EqualFold(arg, "hard") || strings.EqualFold(arg, "h") {
			hardMode = true
		}
	}
	// prepare word list
	wordList = strings.Split(rawWordList, "\n")

	// pick word
	rand.Seed(time.Now().UnixNano())
	word = wordList[rand.Intn(len(wordList))]
	// fmt.Println(word)

	if hardMode {
		fmt.Println("Hard mode enabled. Guesses must use all revealed (green) hints.")
	}

	// prepare key listener
	ty, err := tty.Open()
	if err != nil {
		panic(err)
	}

	tyOpen := true
	defer func() {
		if tyOpen {
			ty.Close()
		}
	}()

	// prepare output
	stat, err := statux.New(TotalGuesses + 1)
	if err != nil {
		panic(err)
	}
	defer func() {
		if !stat.IsFinished() {
			stat.Finish()
		}
	}()

	// listen for interrupts
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		<-c
		ty.Close()
		stat.Finish()
		os.Exit(0)
	}()

	// print the initial game state
	for i := 0; i < TotalGuesses; i++ {
		if i == 0 {
			_, _ = stat.WriteString(i, "█ _ _ _ _")
		} else {
			_, _ = stat.WriteString(i, "_ _ _ _ _")
		}
	}

	// start the game
	guess := ""
	win := false
	for {
		// read user input
		pressed, err := ty.ReadRune()
		if err != nil {
			panic(err)
		}

		// _, _ = stat.WriteString(TotalGuesses, fmt.Sprintf("%d", byte(pressed)))

		// input was backspace
		if pressed == KeyCodeBackspace && len(guess) != 0 {
			guess = guess[:len(guess)-1]
			_, _ = stat.WriteString(currentGuess, formatGuess(guess, false))
			continue
		}

		// input was enter
		if pressed == KeyCodeEnter && len(guess) == WordLength {
			if isWord(guess) {
				// check hard mode requirements
				if hardMode && !hardModeEnforcement(guess) {
					_, _ = stat.WriteString(currentGuess, formatGuess(guess, false)+" (Hard Mode! Must use revealed hints)")
					continue
				}

				// show hints
				_, _ = stat.WriteString(currentGuess, formatGuess(guess, true))

				// check for win
				if guess == word {
					win = true
					break
				}

				// prepare next guess
				currentGuess++
				guess = ""

				// check for lose
				if currentGuess == TotalGuesses {
					break
				}

				// update next line
				_, _ = stat.WriteString(currentGuess, formatGuess(guess, false))
			} else {
				// guess was not a word, indicate error
				_, _ = stat.WriteString(currentGuess, formatGuess(guess, false)+" (must be a word)")
			}
		}

		// input was letter
		pressed = unicode.ToUpper(pressed)

		if len(guess) < WordLength && (unicode.IsLetter(pressed)) {
			guess += string(pressed)
			_, _ = stat.WriteString(currentGuess, formatGuess(guess, false))
		}
	}

	// cleanup terminal
	stat.Finish()
	ty.Close()
	tyOpen = false

	if win {
		fmt.Println("You win!")
	} else {
		fmt.Printf("The word was %s\n", word)
	}
}

func formatGuess(guess string, clr bool) string {
	if guess == "" {
		return "█ _ _ _ _"
	}

	// map and remove correct guesses
	m := mapString(word)
	for i := range guess {
		if guess[i] == word[i] {
			m[word[i]]--
			discovered[i] = true // not elegant, but super convenient
		}
	}

	slots := make([]string, WordLength)
	for i := range guess {
		if clr {
			if guess[i] == word[i] {
				slots[i] = color.GreenString(string(guess[i]))
			} else if num := m[guess[i]]; num > 0 {
				m[guess[i]]--
				slots[i] = color.YellowString(string(guess[i]))
			} else {
				slots[i] = color.RedString(string(guess[i]))
			}
		} else {
			slots[i] = string(guess[i])
		}
	}

	first := true
	for i := len(guess); i < WordLength; i++ {
		if first {
			slots[i] = "█"
			first = false
		} else {
			slots[i] = "_"
		}
	}

	return strings.Join(slots, " ")
}

func hardModeEnforcement(guess string) bool {
	for i := range word {
		if discovered[i] && guess[i] != word[i] {
			return false
		}
	}

	return true
}

func mapString(str string) map[byte]int {
	m := make(map[byte]int)
	for _, r := range str {
		m[byte(r)]++
	}
	return m
}

func isWord(str string) bool {
	index := sort.SearchStrings(wordList, str)
	return index < len(wordList) && wordList[index] == str
}
