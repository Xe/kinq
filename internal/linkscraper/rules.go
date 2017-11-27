package linkscraper

import (
	"context"
	"errors"
)

var (
	ErrNotApplicable = errors.New("linkscraper: this url is not applicable for this scraper")
	ErrNotFound      = errors.New("linkscraper: image not found")
)

// Scraper validates and scrapes tags from a given URL by string.
type Scraper interface {
	Valid(url string) bool
	Scrape(ctx context.Context, url string) (tags []string, err error)
}

type Rules []Scraper

func (r *Rules) Add(s Scraper) {
	rs := *r
	rs = append(rs, s)
	r = &rs
}

func (r *Rules) Test(ctx context.Context, url string) (tags []string, err error) {
	for _, rl := range *r {
		if rl.Valid(url) {
			tags, err := rl.Scrape(ctx, url)
			switch err {
			case nil:
			case ErrNotApplicable:
				continue
			default:
				return nil, err
			}

			return tags, nil
		}
	}

	return nil, ErrNotFound
}
