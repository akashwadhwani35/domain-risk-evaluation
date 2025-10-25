package dict

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

var (
	loadOnce sync.Once
	wordSet  map[string]struct{}
)

func ensureLoaded() {
	loadOnce.Do(func() {
		wordSet = make(map[string]struct{})
		path := strings.TrimSpace(os.Getenv("ENGLISH_WORD_LIST"))
		if path == "" {
			path = "/usr/share/dict/words"
		}
		file, err := os.Open(filepath.Clean(path))
		if err != nil {
			logrus.WithError(err).WithField("path", path).Warn("english word list unavailable; dictionary-based checks disabled")
			wordSet = nil
			return
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		count := 0
		for scanner.Scan() {
			token := sanitize(scanner.Text())
			if token == "" {
				continue
			}
			wordSet[token] = struct{}{}
			count++
		}
		if err := scanner.Err(); err != nil {
			logrus.WithError(err).WithField("path", path).Warn("english word list scan error; partial dictionary loaded")
		}
		logrus.WithField("words_loaded", count).WithField("path", path).Info("english dictionary loaded")
	})
}

func sanitize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "")
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "'", "")
	return value
}

// IsEnglishWord reports whether the provided token appears in the English dictionary.
func IsEnglishWord(token string) bool {
	token = sanitize(token)
	if token == "" {
		return false
	}
	ensureLoaded()
	if wordSet == nil {
		return false
	}
	_, ok := wordSet[token]
	return ok
}
