package crm

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// NewID returns a random RFC 4122 version 4 UUID.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ValidID reports whether s is a canonical lowercase UUID. Malformed IDs are treated as not found.
func ValidID(s string) bool { return uuidPattern.MatchString(s) }

var phonePattern = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

// NormalizePhone converts common Romanian and international formats to E.164.
func NormalizePhone(p string) string {
	p = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", ".", "", "/", "").Replace(strings.TrimSpace(p))
	if strings.HasPrefix(p, "00") {
		p = "+" + p[2:]
	}
	if len(p) == 10 && strings.HasPrefix(p, "0") {
		p = "+40" + p[1:]
	}
	if len(p) == 11 && strings.HasPrefix(p, "40") {
		p = "+" + p
	}
	return p
}

func ValidPhone(p string) bool { return phonePattern.MatchString(p) }

// PhoneSearchFragment turns typed search input into a digit fragment comparable with stored
// E.164 numbers, or "" when the input is not phone-like.
func PhoneSearchFragment(q string) string {
	q = strings.TrimSpace(q)
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, q)
	if len(digits) < 3 || strings.IndexFunc(q, func(r rune) bool { return !strings.ContainsRune("+0123456789 ()-./", r) }) >= 0 {
		return ""
	}
	if strings.HasPrefix(digits, "00") {
		digits = digits[2:]
	} else if strings.HasPrefix(digits, "0") {
		digits = "40" + digits[1:]
	}
	return digits
}

var folder = strings.NewReplacer("ă", "a", "â", "a", "î", "i", "ș", "s", "ş", "s", "ț", "t", "ţ", "t", "é", "e", "ö", "o", "ü", "u")

// FoldName lowercases and removes Romanian diacritics for accent-insensitive search.
func FoldName(s string) string {
	return folder.Replace(strings.ToLower(strings.Join(strings.Fields(s), " ")))
}

func cleanText(s string) string { return strings.TrimSpace(s) }

func lengthBetween(s string, min, max int) bool {
	n := utf8.RuneCountInString(s)
	return n >= min && n <= max
}

// ParseDate validates a YYYY-MM-DD calendar date.
func ParseDate(s string) (time.Time, bool) {
	t, err := time.Parse(time.DateOnly, s)
	return t, err == nil
}

func validTags(tags []string) ([]string, bool) {
	if len(tags) > 10 {
		return nil, false
	}
	out := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, t := range tags {
		t = cleanText(t)
		if !lengthBetween(t, 1, 40) {
			return nil, false
		}
		if !seen[strings.ToLower(t)] {
			seen[strings.ToLower(t)] = true
			out = append(out, t)
		}
	}
	return out, true
}

func validStage(s string) bool {
	for _, v := range Stages {
		if v == s {
			return true
		}
	}
	return false
}

func clampPage(offset, limit, def, max int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = def
	}
	if limit > max {
		limit = max
	}
	return offset, limit
}
