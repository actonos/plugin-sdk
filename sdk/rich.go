package sdk

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// SplitMessage splits a long string into chunks of at most maxChunkSize runes,
// preferring line breaks ('\n') or spaces (' ') to keep code blocks and sentences intact.
func SplitMessage(text string, maxChunkSize int) []string {
	if maxChunkSize <= 0 {
		maxChunkSize = 4000
	}

	runes := []rune(text)
	if len(runes) <= maxChunkSize {
		return []string{text}
	}

	var chunks []string

	for len(runes) > 0 {
		if len(runes) <= maxChunkSize {
			chunks = append(chunks, string(runes))
			break
		}

		splitIdx := maxChunkSize
		foundBreak := false

		// Search window for clean line breaks (last 25% of the chunk)
		searchWindow := maxChunkSize / 4
		minSearch := maxChunkSize - searchWindow
		if minSearch < 0 {
			minSearch = 0
		}

		for i := maxChunkSize - 1; i >= minSearch; i-- {
			if runes[i] == '\n' {
				splitIdx = i + 1
				foundBreak = true
				break
			}
		}

		if !foundBreak {
			for i := maxChunkSize - 1; i >= minSearch; i-- {
				if runes[i] == ' ' {
					splitIdx = i + 1
					foundBreak = true
					break
				}
			}
		}

		chunks = append(chunks, string(runes[:splitIdx]))
		runes = runes[splitIdx:]
	}

	// Add part indicators if multiple chunks
	if len(chunks) > 1 {
		for i := range chunks {
			suffix := fmt.Sprintf("\n\n📄 _(Part %d/%d)_", i+1, len(chunks))
			if i > 0 {
				chunks[i] = fmt.Sprintf("_(Part %d/%d continued)_\n\n", i+1, len(chunks)) + chunks[i]
			} else {
				chunks[i] = chunks[i] + suffix
			}
		}
	}

	return chunks
}

// EscapeTelegramMarkdownV2 escapes characters that require escaping in Telegram MarkdownV2.
func EscapeTelegramMarkdownV2(text string) string {
	special := []string{"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}
	result := text
	for _, ch := range special {
		result = strings.ReplaceAll(result, ch, "\\"+ch)
	}
	return result
}

// TruncateRunes safely truncates a string to maxRunes without breaking UTF-8 characters.
func TruncateRunes(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "..."
}
