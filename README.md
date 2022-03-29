# Wordle

[Inspired by the original web implementation.](https://www.powerlanguage.co.uk/wordle/) I do strive to keep the wordlist and behavior updated with the official implementation.

Each guess must be a valid word. Submit your guesses with Enter: Red letters aren't in the answer, yellow letters are in the answer, green letters are in the answer at that position. Pass `-H` or `--hard` for hard mode which requires that once a letter is green, all future guesses must include those letters in those positions. Pass `-s` or `--stats` to see your current stats without playing. Pass both flags to see hard mode stats. Note: streaks keep track of continuous games regardless of normal vs hard mode, as in there's only one streak. Streaks, dailies, and other tidbits are kept track in a file located at `~/.wordle`.

It's a go app, so installation looks like the usual:

    go install github.com/coreyog/wordle@latest

Or download from the Releases page.