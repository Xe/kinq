package database

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/Xe/ln"
	"github.com/Xe/uuid"
	"github.com/asdine/storm"
	"github.com/asdine/storm/q"
	"golang.org/x/crypto/blake2b"
)

type Image struct {
	ID         string `storm:"id"`
	URL        string `storm:"unique"`
	Tags       []string
	Blake2Hash string
	Size       int64
	Deleted    bool
}

func (i Image) F() ln.F {
	return ln.F{
		"image_id":      i.ID,
		"image_url":     i.URL,
		"image_hash":    i.Blake2Hash,
		"image_deleted": i.Deleted,
	}
}

type Images interface {
	Insert(url string) (*Image, error)
	AddTags(id string, tags []string) error
	RemoveTags(id string, tags []string) error
	Search(numPerPage, pageNumber int, tags []string) ([]Image, error)
	Untagged() ([]Image, error)
	Delete(id string) error
}

type stormImages struct {
	db *storm.DB
}

func NewStormImages(db *storm.DB) Images {
	return &stormImages{db: db}
}

func validContentType(ct string) bool {
	switch ct {
	case "image/png", "image/jpeg", "image/gif":
		return true
	}

	return false
}

func (s *stormImages) Insert(url string) (*Image, error) {
	resp, err := http.Get(url)
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

	hsh := blake2b.Sum256(data)
	strhsh := base64.StdEncoding.EncodeToString(hsh[:])

	id := uuid.New()

	i := &Image{
		ID:         id,
		URL:        url,
		Blake2Hash: strhsh,
		Size:       int64(len(data)),
	}

	err = s.db.Save(i)
	if err != nil {
		return nil, err
	}

	return i, nil
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

func (s *stormImages) Untagged() ([]Image, error) {
	query := s.db.Select(q.Eq("Tags", []string{}))
	var images []Image
	err := query.Find(&images)
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
