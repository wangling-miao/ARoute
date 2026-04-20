package content

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	"github.com/mozillazg/go-pinyin"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

var (
	slugInvalidRegex = regexp.MustCompile(`[^a-z0-9\-]`)
	slugHyphenRegex  = regexp.MustCompile(`-{2,}`)
)

// GenerateSlug converts a title string into a URL-safe slug.
// Chinese characters are converted to pinyin before slugification.
func GenerateSlug(title string) string {
	a := pinyin.NewArgs()
	a.Style = pinyin.Normal

	var b strings.Builder
	for _, r := range title {
		if unicode.IsSpace(r) {
			b.WriteRune('-')
		} else if r < 128 {
			b.WriteRune(r)
		} else if unicode.Is(unicode.Han, r) {
			b.WriteRune('-')
			b.WriteString(pinyin.LazyPinyin(string(r), a)[0])
			b.WriteRune('-')
		}
	}

	s := strings.ToLower(b.String())
	s = slugInvalidRegex.ReplaceAllString(s, "")
	s = slugHyphenRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")

	return s
}

// GenerateUniqueSlug generates a slug from title ensuring uniqueness in the content type's table.
func (s *Service) GenerateUniqueSlug(ctx context.Context, ct *interfaces.ContentType, title string) (string, error) {
	base := GenerateSlug(title)
	if base == "" {
		base = "untitled"
	}

	slug, err := s.store.GetNextSlug(ctx, ct.TableName, base)
	if err != nil {
		return "", err
	}
	return slug, nil
}
