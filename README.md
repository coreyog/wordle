# Wordle

[Inspired by the original web implementation.](https://www.powerlanguage.co.uk/wordle/) I strive to keep the wordlist and behavior updated with the official implementation.

Each guess must be a valid word. Submit your guesses with Enter: Red letters aren't in the answer, yellow letters are in the answer, green letters are in the answer at that position. Pass `-H` or `--hard` for hard mode which requires that once a letter is green, all future guesses must include those letters in those positions. Pass `-s` or `--stats` to see your current stats without playing. Pass both flags to see hard mode stats. Note: streaks keep track of continuous games regardless of normal vs hard mode, as in there's only one streak.

It's a go app, so installation looks like the usual:

    go install github.com/coreyog/wordle@latest

Or download from the Releases page.

## Config

Streaks, dailies, and other tidbits are kept track in a file located at `~/.wordle`. There's 2 config values in the file that can only be modified by changing the file manually: `experimental_emoji_support` and `default_to_hard_mode`. It's not easy for a terminal application to know if printing emoji will be printed correctly or not so by default emoji support is disabled. Setting it to `true` in the config will tell the program to attempt to print a sharable set of emojis representing how you did, for example:

```
Wordle 278 3/6*

🟨⬛⬛🟨🟩
🟨⬛⬛🟩🟩
🟩🟩🟩🟩🟩
```

If you always play with hard mode enabled then you can update the config by setting `default_to_hard_mode` to `true` so that the next time you start a game, it'll behave like you passed the `-H` flag. This also affects the `-s` flag. With this config value set to `true`, it's impossible to start a standard game. If you would like to do that, you'll need to update the config again and set the value back to `false`.

### Note to self about deploys:

Once everything is checked in and ready for a release:

    gitsem [major, minor, patch]
    git push
    goreleaser release
    rm -rf dist