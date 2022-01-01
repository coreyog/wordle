package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"path"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/coreyog/statux"
	"github.com/fatih/color"
	"github.com/jessevdk/go-flags"
	"github.com/mattn/go-tty"
)

const (
	TotalGuesses = 6
	WordLength   = 5

	KeyCodeWinBackspace = 8
	KeyCodeEnter        = 13
	KeyCodeMacBackspace = 127
)

var currentGuess = 0

//go:embed words.txt
var rawWordList string

var wordList []string
var word string
var discovered []bool = make([]bool, WordLength)

type GameStats struct {
	TotalGames     int   `json:"total_games"`
	TotalHardGames int   `json:"total_hard_games"`
	Wins           []int `json:"wins"`
	HardWins       []int `json:"hard_wins"`
	Streak         int   `json:"streak"`
	BestStreak     int   `json:"best_streak"`
}

type Arguments struct {
	HardMode   bool `short:"H" long:"hard" description:"Play in hard mode"`
	PrintStats bool `short:"s" long:"stats" description:"Print stats"`
}

var args Arguments

func main() {
	// parse args
	_, err := flags.Parse(&args)
	if err != nil {
		if flags.WroteHelp(err) {
			printUsage()
			os.Exit(0)
		}

		os.Exit(1)
	}

	gamestats := loadGameStats()

	if args.PrintStats {
		gamestats.print()
		return
	}

	// prepare word list
	wordList = strings.Split(rawWordList, "\n")

	// pick word
	rand.Seed(time.Now().UnixNano())
	word = wordList[rand.Intn(len(wordList))]
	fmt.Println(word) // debugging

	if args.HardMode {
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

	// setup game state
	guess := ""
	win := false

	// listen for interrupts to cleanup terminal trickery
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		<-c
		ty.Close()
		stat.Finish()

		if !win {
			gamestats.Streak = 0
			_ = gamestats.save()
		}

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
	for { // main loop
		// read user input
		pressed, err := ty.ReadRune()
		if err != nil {
			panic(err)
		}

		pressed = unicode.ToUpper(pressed)

		// _, _ = stat.WriteString(TotalGuesses, fmt.Sprintf("%d", int(pressed))) // debugging tty

		// input was backspace
		if (pressed == KeyCodeMacBackspace || pressed == KeyCodeWinBackspace) && len(guess) != 0 {
			guess = guess[:len(guess)-1]
			_, _ = stat.WriteString(currentGuess, formatGuess(guess, false))

			continue
		}

		// input was enter and the guess is filled
		if pressed == KeyCodeEnter && len(guess) == WordLength {
			if !isWord(guess) {
				// guess was not a word, indicate error
				_, _ = stat.WriteString(currentGuess, formatGuess(guess, false)+" (must be a word)")
				continue
			}

			// check hard mode requirements
			if args.HardMode && !hardModeEnforcement(guess) {
				_, _ = stat.WriteString(currentGuess, formatGuess(guess, false)+" (Hard Mode! Must use revealed hints)")
				continue
			}

			// show hints
			_, _ = stat.WriteString(currentGuess, formatGuess(guess, true))

			if currentGuess == 0 {
				// just submitted the first guess, this game officially counts
				if args.HardMode {
					gamestats.TotalHardGames++
				} else {
					gamestats.TotalGames++
				}

				err = gamestats.save()
				if err != nil {
					_, _ = stat.WriteString(TotalGuesses+1, "(problem saving stats)")
				}
			}

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
				break // exit main loop
			}

			// update next line with cursor
			_, _ = stat.WriteString(currentGuess, formatGuess(guess, false))
		}

		// input was letter
		if len(guess) < WordLength && (unicode.IsLetter(pressed)) {
			guess += string(pressed)
			_, _ = stat.WriteString(currentGuess, formatGuess(guess, false))
		}
	} // main loop

	// cleanup terminal
	stat.Finish()
	ty.Close()
	tyOpen = false

	// indicate win or lose, update/save/print stats
	if win {
		if args.HardMode {
			gamestats.HardWins[currentGuess]++
		} else {
			gamestats.Wins[currentGuess]++
		}

		gamestats.Streak++
		gamestats.BestStreak = int(math.Max(float64(gamestats.BestStreak), float64(gamestats.Streak)))

		fmt.Print("You win!\n\n")
	} else {
		gamestats.Streak = 0
		fmt.Printf("The word was %s\n", word)
	}

	_ = gamestats.save()

	gamestats.print()
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
		}
	}

	slots := make([]string, WordLength)

	for i := range guess {
		if clr {
			c := color.RedString
			if guess[i] == word[i] {
				c = color.GreenString
				discovered[i] = true // not elegant, but SUPER convenient
			} else if num := m[guess[i]]; num > 0 {
				m[guess[i]]--
				c = color.YellowString
			}

			slots[i] = c(string(guess[i]))
		} else {
			slots[i] = string(guess[i])
		}
	}

	// add cursor and blanks
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

// hardModeEnforcement checks if the guess is valid by hard mode rules: once a
// character is revealed as in the correct place, it must be used in the guess.
func hardModeEnforcement(guess string) bool {
	for i := range word {
		if discovered[i] && guess[i] != word[i] {
			return false
		}
	}

	return true
}

// mapString maps a string to a count of each characters' occurences.
func mapString(str string) map[byte]int {
	m := make(map[byte]int)
	for _, r := range str {
		m[byte(r)]++
	}

	return m
}

// isWord checks if a string is a word in the wordlist which makes it a valid guess.
func isWord(str string) bool {
	index := sort.SearchStrings(wordList, str)
	return index < len(wordList) && wordList[index] == str
}

func loadGameStats() (gamestats *GameStats) {
	// setup default
	gamestats = &GameStats{
		Wins:     make([]int, TotalGuesses),
		HardWins: make([]int, TotalGuesses),
	}

	// load stats
	home, err := os.UserHomeDir()
	if err != nil {
		return gamestats
	}

	savePath := path.Join(home, ".wordle")

	raw, err := ioutil.ReadFile(savePath)
	if err != nil {
		return gamestats
	}

	err = json.NewDecoder(bytes.NewReader(raw)).Decode(&gamestats)
	if err != nil {
		// file present but unreadable? delete it.
		_ = os.Remove(savePath)
		return gamestats
	}

	return gamestats
}

func (gs *GameStats) save() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	savePath := path.Join(home, ".wordle")

	f, err := os.OpenFile(savePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	err = json.NewEncoder(f).Encode(gs)
	if err != nil {
		return err
	}

	return nil
}

func (gs *GameStats) print() {
	fmt.Print("Game Stats")

	if args.HardMode {
		fmt.Print(" (Hard Mode)")
	}

	fmt.Println()

	if args.HardMode {
		totalWins := 0
		for i := 0; i < TotalGuesses; i++ {
			totalWins += gs.HardWins[i]
		}

		fmt.Printf("   Total Games: %d\n", gs.TotalHardGames)

		if gs.TotalHardGames > 0 {
			fmt.Printf("         Win %%: %.1f\n", float64(totalWins*10000/gs.TotalHardGames)/100)
		} else {
			fmt.Println("         Win %%: 0")
		}

		fmt.Printf("Current Streak: %d\n", gs.Streak)
		fmt.Printf("   Best Streak: %d\n", gs.BestStreak)
		fmt.Println()
		fmt.Println("Guess Distribution:")

		for i := 0; i < TotalGuesses; i++ {
			fmt.Printf("%d: %d\n", i+1, gs.HardWins[i])
		}
	} else {
		totalWins := 0
		for i := 0; i < TotalGuesses; i++ {
			totalWins += gs.Wins[i]
		}

		fmt.Printf("   Total Games: %d\n", gs.TotalGames)

		if gs.TotalGames > 0 {
			fmt.Printf("         Win %%: %.1f\n", float64(totalWins*10000/gs.TotalGames)/100)
		} else {
			fmt.Println("         Win %%: 0")
		}

		fmt.Printf("Current Streak: %d\n", gs.Streak)
		fmt.Printf("   Best Streak: %d\n", gs.BestStreak)
		fmt.Println()
		fmt.Println("Guess Distribution:")

		for i := 0; i < TotalGuesses; i++ {
			fmt.Printf("%d: %d\n", i+1, gs.Wins[i])
		}
	}
}

func printUsage() {
	fmt.Println("Rules:")
	fmt.Println("Each guess must be a valid word. Submit with Enter: Red letters aren't in the answer,")
	fmt.Println("yellow letters are in the answer, green letters are in the answer at that position.")
	fmt.Println("Hard mode: once a letter is green, all future guesses must include those letters in")
	fmt.Println("those positions.")
}
