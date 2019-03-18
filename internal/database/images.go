package database

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/Xe/kinq/internal/linkscraper"
	"github.com/asdine/storm/v2"
	"github.com/asdine/storm/v2/q"
	"github.com/rs/xid"
	"golang.org/x/crypto/blake2b"
	"within.website/ln"
)

type Image struct {
	ID         string    `storm:"id"`
	URL        string    `storm:"unique"`
	Added      time.Time `storm:"index"`
	Tags       []string
	Blake2Hash string `storm:"unique"`
	Size       int64
	Deleted    bool
	Data       []byte
	Ext        string
	Mime       string
}

func (i Image) F() ln.F {
	return ln.F{
		"image_id":        i.ID,
		"image_url":       i.URL,
		"image_hash":      i.Blake2Hash,
		"image_deleted":   i.Deleted,
		"image_tag_count": len(i.Tags),
		"image_tags":      i.Tags,
	}
}

type Images interface {
	Insert(url string) (*Image, error)
	One(id string) (*Image, error)
	AddTags(id string, tags []string) error
	RemoveTags(id string, tags []string) error
	Search(numPerPage, pageNumber int, tags []string) ([]Image, error)
	Recent(pageID int) ([]Image, error)
	Delete(id string) error
}

type stormImages struct {
	db *storm.DB
	r  *linkscraper.Rules
}

func NewStormImages(db *storm.DB, r *linkscraper.Rules) Images {
	return &stormImages{db: db, r: r}
}

func validContentType(ct string) bool {
	switch ct {
	case "image/png", "image/jpeg", "image/gif":
		return true
	}

	return false
}

func (s *stormImages) Insert(url string) (*Image, error) {
	id := xid.New().String()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if !validContentType(resp.Header.Get("Content-Type")) {
		return nil, errors.New("bad content type: " + resp.Header.Get("Content-Type"))
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("expected http 200, got: %d", resp.StatusCode)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	log.Printf("%s: %d bytes", url, len(data))

	hsh := blake2b.Sum256(data)
	strhsh := base64.StdEncoding.EncodeToString(hsh[:])

	tags, err := s.r.Test(context.Background(), url)
	if err != nil && err != linkscraper.ErrNotFound {
		ln.Error(context.Background(), err, ln.Action("scrape for tags"))
	}

	i := &Image{
		ID:         id,
		URL:        url,
		Added:      time.Now(),
		Blake2Hash: strhsh,
		Size:       int64(len(data)),
		Tags:       tags,
		Ext:        filepath.Ext(url),
		Data:       data,
		Mime:       resp.Header.Get("Content-Type"),
	}

	err = s.db.Save(i)
	if err == storm.ErrAlreadyExists {
		log.Printf("repeat: %s %v", i.URL, i.Blake2Hash)
		var newImage Image
		err = s.db.One("URL", i.URL, &newImage)
		if err != nil {
			log.Printf("????")
			return nil, err
		}
		i.ID = newImage.ID

		s.db.Update(i)
		err = nil
	}
	if err != nil {
		return nil, err
	}

	return i, nil
}

func (s *stormImages) One(id string) (*Image, error) {
	var i Image
	err := s.db.One("ID", id, &i)
	if err != nil {
		return nil, err
	}

	return &i, nil
}

func (s *stormImages) AddTags(id string, tags []string) error {
	var i Image
	err := s.db.One("ID", id, &i)
	if err != nil {
		return err
	}

	rt := map[string]struct{}{}

	for _, t := range i.Tags {
		rt[t] = struct{}{}
	}

	for _, t := range tags {
		rt[t] = struct{}{}
	}

	res := []string{}
	for tag := range rt {
		res = append(res, tag)
	}

	i.Tags = res

	err = s.db.Save(&i)
	if err != nil {
		return err
	}

	return nil
}

func (s *stormImages) RemoveTags(id string, tags []string) error {
	return errors.New("not implemented")
}

func (s *stormImages) Search(numPerPage, pageNumber int, tags []string) ([]Image, error) {
	qq := q.And(
		q.Eq("Deleted", false),
		q.In("Tags", tags),
	)

	query := s.db.Select(qq)
	query.Limit(numPerPage)
	query.Skip(pageNumber * numPerPage)

	var images []Image
	err := query.Find(&images)
	if err != nil {
		return nil, err
	}

	return images, nil
}

func (s *stormImages) Recent(pageID int) ([]Image, error) {
	var images []Image
	err := s.db.AllByIndex("Added", &images, storm.Reverse(), storm.Limit(30), storm.Skip(30 * pageID))
	if err != nil {
		return nil, err
	}

	return images, nil
}

func (s *stormImages) Delete(id string) error {
	var i Image
	err := s.db.One("ID", id, &i)
	if err != nil {
		return err
	}

	i.Deleted = true

	return s.db.Save(&i)
}
