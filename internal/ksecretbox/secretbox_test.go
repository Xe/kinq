package ksecretbox

import "testing"

func TestSecretBox(t *testing.T) {
	var (
		key *[32]byte
		sk  string
	)

	t.Run("generate key", func(t *testing.T) {
		var err error
		key, err = GenerateKey()
		if err != nil {
			t.Fatalf("can't generate key: %v", err)
		}
	})

	if key == nil {
		t.Fatal("can't continue")
	}

	t.Run("show key", func(t *testing.T) {
		sk = ShowKey(key)
		if len(sk) == 0 {
			t.Fatal("expected output to be a nonzero length string")
		}
	})

	t.Run("read key", func(t *testing.T) {
		readKey, err := ParseKey(sk)
		if err != nil {
			t.Fatal(err)
		}

		if *key != *readKey {
			t.Fatal("key did not parse out correctly")
		}
	})
}
