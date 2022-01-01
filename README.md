# Wordle

[Inspired by this web implementation.](https://www.powerlanguage.co.uk/wordle/)

Each guess must be a valid word. Submit with Enter: Red letters aren't in the answer, yellow letters are in the answer, green letters are in the answer at that position. Pass `-h` or `--hard` for hard mode which requires that once a letter is green, all future guesses must include those letters in those positions. Pass `-s` or `--stats` to see your current stats without playing. Pass both flags to see hard mode stats. Note: streaks keep track of continuous games regardless of normal vs hard mode.