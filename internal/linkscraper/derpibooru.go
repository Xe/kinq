package linkscraper

import (
	"context"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"within.website/derpigo"
)

type derpiDirectScraper struct {
	dg *derpigo.Connection
}

func NewDerpiDirectScraper(apik string) Scraper {
	return &derpiDirectScraper{
		dg: derpigo.New(derpigo.WithAPIKey(apik)),
	}
}

func (d *derpiDirectScraper) Valid(u string) bool {
	ur, err := url.Parse(u)
	if err != nil {
		return false
	}

	if ur.Host == "derpibooru.org" {
		return true
	}

	return false
}

func (d *derpiDirectScraper) Scrape(ctx context.Context, uri string) ([]string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	id := u.Path[1:]
	i, err := strconv.Atoi(id)
	if err != nil {
		return nil, ErrNotApplicable
	}

	img, _, err := d.dg.GetImage(ctx, i)
	if err != nil {
		return nil, err
	}

	tags := strings.Split(img.Tags, ", ")

	return tags, nil
}

type derpiCDNScraper struct {
	dg *derpigo.Connection
}

func NewDerpiCDNScraper(apik string) Scraper {
	return &derpiCDNScraper{
		dg: derpigo.New(derpigo.WithAPIKey(apik)),
	}
}

func (d *derpiCDNScraper) Valid(u string) bool {
	ur, err := url.Parse(u)
	if err != nil {
		return false
	}

	if ur.Host == "derpicdn.net" {
		return true
	}

	return false
}

func (d *derpiCDNScraper) Scrape(ctx context.Context, u string) ([]string, error) {
	b := filepath.Base(u)
	if strings.Contains(b, "__") {
		sp := strings.Split(b, "__")
		if len(sp) != 2 {
			return nil, ErrNotApplicable
		}

		id := sp[0]
		i, err := strconv.Atoi(id)
		if err != nil {
			return nil, err
		}

		img, _, err := d.dg.GetImage(ctx, i)
		if err != nil {
			return nil, err
		}

		tags := strings.Split(img.Tags, ", ")

		return tags, nil
	}

	return nil, ErrNotApplicable
}
