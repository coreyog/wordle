package main

import (
	"bufio"
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
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/coreyog/statux"
	"github.com/fatih/color"
	"github.com/jessevdk/go-flags"
	"github.com/mattn/go-tty"
)

const (
	TotalGuesses          = 6
	WordLength            = 5
	MaxHistogramBarLength = float64(15)

	KeyCodeWinBackspace = 8
	KeyCodeEnter        = 13
	KeyCodeMacBackspace = 127

	EmojiNotInWord = '⬛'
	EmojiSomewhere = '🟨'
	EmojiLocated   = '🟩'
)

type KeyHint byte
type ColorFunc func(string, ...interface{}) string

const (
	KeyHintUnknown KeyHint = iota
	KeyHintNotInWord
	KeyHintSomewhere
	KeyHintLocated
)

var hintColorFns = map[KeyHint]ColorFunc{
	KeyHintUnknown:   fmt.Sprintf,
	KeyHintNotInWord: color.RedString,
	KeyHintSomewhere: color.YellowString,
	KeyHintLocated:   color.GreenString,
}

var currentGuess = 0

//go:embed good_words.txt
var rawGoodWordList string

//go:embed bad_words.txt
var rawBadWordList string

//go:embed VERSION
var version string

var wordList []string
var allowedWords []string
var word string
var discovered []bool = make([]bool, WordLength)

var keyboard map[rune]KeyHint
var emojiStack []string = []string{}
var dayOffset int

type GameStats struct {
	TotalGames               int        `json:"total_games"`
	TotalHardGames           int        `json:"total_hard_games"`
	Wins                     []int      `json:"wins"`
	HardWins                 []int      `json:"hard_wins"`
	Streak                   int        `json:"streak"`
	BestStreak               int        `json:"best_streak"`
	LastDaily                *time.Time `json:"last_daily"`
	ExperimentalEmojiSupport bool       `json:"experimental_emoji_support"`
}

type Arguments struct {
	HardMode     bool `short:"H" long:"hard" description:"Play in hard mode"`
	PrintStats   bool `short:"s" long:"stats" description:"Print stats"`
	PrintVersion bool `short:"v" long:"version" description:"Prints the version"`
}

var args Arguments

func main() {
	// parse flags
	_, err := flags.Parse(&args)
	if err != nil {
		if flags.WroteHelp(err) {
			printUsage()
			os.Exit(0)
		}

		os.Exit(1)
	}

	if args.PrintVersion {
		fmt.Printf("v%s\n", version)
		os.Exit(0)
	}

	gamestats := loadGameStats()

	if args.PrintStats {
		gamestats.print(nil)
		return
	}

	// parse word list deterministically even if compiled on windows
	parseWordLists()

	shouldPlayDaily := gamestats.LastDaily == nil || time.Since(*gamestats.LastDaily) > 24*time.Hour

	// calculate day offset
	dayOffset = int(time.Since(time.Date(2021, time.June, 19, 0, 0, 0, 0, time.UTC)).Hours() / 24)

	// pick word
	if shouldPlayDaily {
		fmt.Println("   Daily Puzzle!")

		index := dayOffset % len(wordList)
		word = wordList[index]

		now := time.Now().UTC()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		gamestats.LastDaily = &today
	} else {
		rand.Seed(time.Now().UnixNano())
		word = wordList[rand.Intn(len(wordList))]
	}

	sort.Strings(wordList)
	// fmt.Println(word) // debugging

	initKeyboard()

	if args.HardMode {
		fmt.Println("     Hard Mode")
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
	stat, err := statux.New(TotalGuesses + 4) // +1 for "status" line, +3 for keyboard
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

		if tyOpen {
			ty.Close()
			tyOpen = false
		}

		stat.Finish()

		if !win && currentGuess != 0 {
			gamestats.Streak = 0
			_ = gamestats.save()

			fmt.Printf("\nThe word was %s\n", word)
		}

		os.Exit(0)
	}()

	// print the initial game state
	for i := 0; i < TotalGuesses; i++ {
		if i == 0 {
			_, _ = stat.WriteString(i, "     █ _ _ _ _")
		} else {
			_, _ = stat.WriteString(i, "     _ _ _ _ _")
		}
	}

	printKeyboard(stat)

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
				_, _ = stat.WriteString(currentGuess, formatGuess(guess, false)+" (must use revealed hints)")
				continue
			}

			// show hints
			_, _ = stat.WriteString(currentGuess, formatGuess(guess, true))

			printKeyboard(stat)

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
		fmt.Printf("\nThe word was %s\n\n", word)
	}

	_ = gamestats.save()

	gamestats.print(&win)
}

func initKeyboard() {
	keyboard = map[rune]KeyHint{}

	for i := 'A'; i <= 'Z'; i++ {
		keyboard[i] = KeyHintUnknown
	}
}

func printKeyboard(stat *statux.Statux) {
	rows := []string{
		"QWERTYUIOP",
		"ASDFGHJKL",
		"ZXCVBNM",
	}

	for i, row := range rows {
		letters := make([]string, len(row))

		for j, key := range row {
			sprintf := hintColorFns[keyboard[key]]
			letters[j] = sprintf(string(key))
		}

		lineNumber := TotalGuesses + 1 + i
		_, _ = stat.WriteString(lineNumber, strings.Repeat(" ", i)+strings.Join(letters, " "))
	}
}

func parseWordLists() {
	// prepare scanner to read embedded memory
	scanner := bufio.NewScanner(bytes.NewBuffer([]byte(rawGoodWordList)))
	scanner.Split(bufio.ScanLines)

	// read in words, we already know how many there are
	wordList = make([]string, 0, 2309)
	for scanner.Scan() {
		wordList = append(wordList, scanner.Text())
	}

	// do it again, but keep these words separate
	scanner = bufio.NewScanner(bytes.NewBuffer([]byte(rawBadWordList)))
	scanner.Split(bufio.ScanLines)

	allowedWords = make([]string, 0, 10657)
	for scanner.Scan() {
		allowedWords = append(allowedWords, scanner.Text())
	}
}

func formatGuess(guess string, clr bool) string {
	// map and remove correct guesses
	m := mapString(word)

	for i := range guess {
		if guess[i] == word[i] {
			m[word[i]]--
		}
	}

	slots := make([]string, WordLength)
	emoji := make([]rune, 0, WordLength)

	for i := range guess {
		if clr {
			c := color.RedString
			if guess[i] == word[i] {
				c = color.GreenString
				discovered[i] = true // not elegant, but SUPER convenient

				setKeyHint(rune(guess[i]), KeyHintLocated)

				emoji = append(emoji, EmojiLocated)
			} else if num := m[guess[i]]; num > 0 {
				m[guess[i]]--
				c = color.YellowString

				setKeyHint(rune(guess[i]), KeyHintSomewhere)
				emoji = append(emoji, EmojiSomewhere)
			} else {
				setKeyHint(rune(guess[i]), KeyHintNotInWord)
				emoji = append(emoji, EmojiNotInWord)
			}

			slots[i] = c(string(guess[i]))
		} else {
			slots[i] = string(guess[i])
		}
	}

	if clr {
		emojiStack = append(emojiStack, string(emoji))
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

	return "     " + strings.Join(slots, " ")
}

func setKeyHint(r rune, hint KeyHint) {
	existing := keyboard[r]
	if hint > existing {
		keyboard[r] = hint
	}
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
	found := index < len(wordList) && wordList[index] == str

	if found {
		return true
	}

	index = sort.SearchStrings(allowedWords, str)

	return index < len(allowedWords) && allowedWords[index] == str
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

func (gs *GameStats) print(win *bool) {
	// setup for easy mode
	wins := gs.Wins
	totalGames := gs.TotalGames
	hardInd := ""

	fmt.Print("Game Stats")

	if args.HardMode {
		fmt.Print(" (Hard Mode)")

		wins = gs.HardWins
		totalGames = gs.TotalHardGames

		hardInd = "*"
	}

	fmt.Print("\n\n")

	totalWins := 0
	for i := 0; i < TotalGuesses; i++ {
		totalWins += wins[i]
	}

	fmt.Printf("   Total Games: %d\n", totalGames)

	if totalGames > 0 {
		rawPercent := float64(totalWins*10000) / float64(totalGames) / 100
		strPercent := strconv.FormatFloat(rawPercent, 'f', 1, 64)
		strPercent = strings.TrimRight(strPercent, "0")
		strPercent = strings.TrimRight(strPercent, ".")
		fmt.Printf("         Win %%: %s\n", strPercent)
	} else {
		fmt.Println("         Win %%: 0")
	}

	fmt.Printf("Current Streak: %d\n", gs.Streak)
	fmt.Printf("   Best Streak: %d\n", gs.BestStreak)
	fmt.Println()
	fmt.Print("Guess Distribution:\n\n")

	// prepare histogram
	hist := make([]float64, TotalGuesses)
	max := float64(-1)

	winPadding := 0

	for i := 0; i < TotalGuesses; i++ {
		hist[i] = float64(wins[i]) / float64(totalWins)
		if max < hist[i] {
			max = hist[i]
		}

		winWord := strconv.Itoa(wins[i])

		if len(winWord) > winPadding {
			winPadding = len(winWord)
		}
	}

	mult := MaxHistogramBarLength / max

	// histogram
	for i := 0; i < TotalGuesses; i++ {
		count := strconv.Itoa(wins[i])
		count = strings.Repeat(" ", winPadding-len(count)) + count
		fmt.Printf("%d: %s %s\n", i+1, count, strings.Repeat("█", int(math.Min(MaxHistogramBarLength, hist[i]*mult))))
	}

	if gs.ExperimentalEmojiSupport && win != nil {
		fmt.Println()

		turn := "X"

		if *win {
			turn = strconv.Itoa(currentGuess + 1)
		}

		fmt.Printf("Wordle %d %s/6%s\n\n", dayOffset, turn, hardInd)

		for _, line := range emojiStack {
			fmt.Println(line)
		}
	}
}

func printUsage() {
	fmt.Println("Rules:")
	fmt.Println("Each guess must be a valid word. Submit with Enter: Red letters aren't in the answer,")
	fmt.Println("yellow letters are in the answer, green letters are in the answer at that position.")
	fmt.Println("Hard mode: once a letter is green, all future guesses must include those letters in")
	fmt.Println("those positions.")
	fmt.Println()
	fmt.Printf("Wordle v%s\n", version)
}
