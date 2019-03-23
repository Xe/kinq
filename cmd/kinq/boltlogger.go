package main

import (
	"context"
	"net/http"
	"time"

	"github.com/celrenheit/sandflake"
	bbolt "go.etcd.io/bbolt"
	bolt "go.etcd.io/bbolt"
	"within.website/ln"
)

type boltLogger struct {
	db         *bbolt.DB
	g          sandflake.Generator
	bucketName string
	f          ln.Formatter
}

// BoltLogger logs everything to a given boltdb database and bucket.
func BoltLogger(db *bbolt.DB, bucketName string, f ln.Formatter) *boltLogger {
	_ = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		return err
	})

	return &boltLogger{
		db:         db,
		g:          sandflake.Generator{},
		bucketName: bucketName,
		f:          f,
	}
}

func (b boltLogger) Apply(ctx context.Context, e ln.Event) bool {
	id := b.g.Next()

	b.db.Update(func(tx *bbolt.Tx) error {
		bk := tx.Bucket([]byte(b.bucketName))
		data, err := b.f.Format(ctx, e)
		if err != nil {
			return err
		}
		return bk.Put([]byte(id.String()), data)
	})

	return true
}

func (b boltLogger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=\"UTF-8\"")
	now := time.Now()

	err := b.db.View(func(tx *bolt.Tx) error {
		bk := tx.Bucket([]byte(b.bucketName))

		if err := bk.ForEach(func(k, v []byte) error {
			id := sandflake.MustParse(string(k))
			if id.Time().Day() == now.Day() {
				w.Write(v)
			}

			return nil
		}); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		ln.Error(r.Context(), err)
	}
}

func (b boltLogger) Close() {}
func (b boltLogger) Run()   {}
