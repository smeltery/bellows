// Package resultcache reuses explicitly pure checks from attested local runs.
// Its private directory owns regular private <64-hex>.json slots: corruption is
// pruned and replaced only after fresh attested execution. Other paths are untouched.
package resultcache

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	v "github.com/smeltery/bellows/internal/verification"
	"github.com/smeltery/bellows/internal/verification/receipt"
	"github.com/smeltery/bellows/internal/verification/run"
)

const TTL = 24 * time.Hour
const maxEntries, maxBytes, maxEntry = 128, 16 << 20, 128 << 10

type Store struct {
	path, repository, producer string
	directory                  os.FileInfo
	key                        []byte
	now                        func() time.Time
}
type entry struct {
	Version                int
	Key, Producer, CheckID string
	Origin                 v.Reuse
}
type envelope struct {
	Payload json.RawMessage
	MAC     string
}

func Open(path, repository string) (store *Store, err error) {
	root, info, err := openDirectory(path, repository)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	unlock, err := locked(root)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, unlock()) }()
	key, err := readPrivate(root, ".key", 32)
	if os.IsNotExist(err) {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err == nil {
			err = atomicCreate(root, ".key", key)
		}
	}
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("invalid result cache authentication key")
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	file, err := os.Open(executable)
	if err != nil {
		return nil, err
	}
	hash := sha256.New()
	_, err = io.Copy(hash, file)
	err = errors.Join(err, file.Close())
	if err != nil {
		return nil, err
	}
	return &Store{path: root.Name(), repository: repository, directory: info, key: key, producer: hex.EncodeToString(hash.Sum(nil)), now: time.Now}, nil
}

func (s *Store) access() (*os.Root, func() error, error) {
	root, info, err := openDirectory(s.path, s.repository)
	if err != nil {
		return nil, nil, err
	}
	if !os.SameFile(s.directory, info) {
		_ = root.Close()
		return nil, nil, fmt.Errorf("result cache directory replaced")
	}
	unlock, err := locked(root)
	if err != nil {
		_ = root.Close()
		return nil, nil, err
	}
	return root, func() error { return errors.Join(unlock(), root.Close()) }, nil
}

func (s *Store) planKey(plan run.CachePlan, id string) (string, error) {
	if plan.Identity.Executor != v.ExecutorFlox || !plan.Cacheable[id] {
		return "", fmt.Errorf("check is not eligible for result reuse")
	}
	checks := make([]v.Check, 0, len(plan.Targets))
	eligible := false
	for _, target := range plan.Targets {
		checks = append(checks, target.Check)
		if target.Check.ID == id && !target.Skipped {
			eligible = true
		}
	}
	if !eligible {
		return "", fmt.Errorf("check is absent or skipped")
	}
	if err := receipt.Validate(v.Receipt{Version: v.ReceiptVersion, RunID: "cache-key", StartedAt: s.now().UTC(), Input: v.Ready, Identity: plan.Identity, Checks: checks, Results: []v.Result{}}); err != nil {
		return "", err
	}
	data, err := json.Marshal(struct {
		Version           int
		Producer, CheckID string
		Plan              run.CachePlan
	}{1, s.producer, id, plan})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

func (s *Store) Lookup(plan run.CachePlan, id string) (result v.Result, hit bool, err error) {
	key, err := s.planKey(plan, id)
	if err != nil {
		return result, false, nil
	}
	root, close, err := s.access()
	if err != nil {
		return result, false, err
	}
	defer func() { err = errors.Join(err, close()) }()
	cached, err := s.read(root, key+".json", true)
	if err != nil || cached.Key != key || cached.CheckID != id {
		return result, false, nil
	}
	code := 0
	return v.Result{ID: id, State: v.Passed, ExitCode: &code, DurationMS: 0, Reuse: &cached.Origin}, true, nil
}

func (s *Store) Save(out run.Outcome) (err error) {
	plan, data, executed, err := out.CacheEvidence()
	if err != nil {
		return err
	}
	completed, err := receipt.Decode(data)
	if err != nil || completed.CompletedAt == nil {
		return fmt.Errorf("invalid attested cache evidence")
	}
	now := s.now().UTC()
	expires := completed.CompletedAt.Add(TTL)
	if completed.CompletedAt.After(now) || !now.Before(expires) {
		return nil
	}
	origin := v.Reuse{ReceiptID: fmt.Sprintf("%x", sha256.Sum256(data)), RunID: completed.RunID, CompletedAt: *completed.CompletedAt, ExpiresAt: expires}
	root, close, err := s.access()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, close()) }()
	if err = s.prune(root, 0); err != nil {
		return err
	}
	for _, id := range executed {
		key, keyErr := s.planKey(plan, id)
		if keyErr != nil {
			return keyErr
		}
		name := key + ".json"
		if _, err = s.read(root, name, true); err == nil {
			continue
		}
		if _, statErr := root.Lstat(name); statErr == nil {
			continue
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
		payload, marshalErr := json.Marshal(entry{1, key, s.producer, id, origin})
		if marshalErr != nil {
			return marshalErr
		}
		mac := hmac.New(sha256.New, s.key)
		_, _ = mac.Write(payload)
		encoded, marshalErr := json.Marshal(envelope{payload, hex.EncodeToString(mac.Sum(nil))})
		if marshalErr != nil {
			return marshalErr
		}
		if err = s.prune(root, int64(len(encoded))); err != nil {
			return err
		}
		if err = atomicCreate(root, name, encoded); err != nil {
			return err
		}
	}
	return nil
}

func strict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("trailing cache data")
	}
	return nil
}
func (s *Store) read(root *os.Root, name string, fresh bool) (cached entry, err error) {
	data, err := readPrivate(root, name, maxEntry)
	if err != nil {
		return cached, err
	}
	var sealed envelope
	if err = strict(data, &sealed); err != nil {
		return cached, err
	}
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write(sealed.Payload)
	signature, err := hex.DecodeString(sealed.MAC)
	if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		return cached, fmt.Errorf("cache authentication failed")
	}
	if err = strict(sealed.Payload, &cached); err != nil {
		return cached, err
	}
	o := cached.Origin
	now := s.now().UTC()
	if cached.Version != 1 || cached.Key+".json" != name || o.RunID == "" || len(o.ReceiptID) != 64 || o.CompletedAt.IsZero() || o.CompletedAt.After(now) || !o.ExpiresAt.Equal(o.CompletedAt.Add(TTL)) || (fresh && (cached.Producer != s.producer || !now.Before(o.ExpiresAt))) {
		return cached, fmt.Errorf("expired or invalid cache origin")
	}
	if _, err = hex.DecodeString(o.ReceiptID); err != nil {
		return cached, err
	}
	return cached, nil
}

func (s *Store) prune(root *os.Root, incoming int64) error {
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	files, err := dir.ReadDir(-1)
	err = errors.Join(err, dir.Close())
	if err != nil {
		return err
	}
	type candidate struct {
		name string
		size int64
		time time.Time
	}
	var entries []candidate
	var total int64
	for _, file := range files {
		name := file.Name()
		if len(name) != 69 || !strings.HasSuffix(name, ".json") {
			continue
		}
		if _, err := hex.DecodeString(strings.TrimSuffix(name, ".json")); err != nil {
			continue
		}
		info, err := root.Lstat(name)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			continue
		}
		cached, err := s.read(root, name, false)
		if err != nil {
			if err = root.Remove(name); err != nil {
				return err
			}
			continue
		}
		if !s.now().Before(cached.Origin.ExpiresAt) || cached.Producer != s.producer {
			if err = root.Remove(name); err != nil {
				return err
			}
			continue
		}
		entries = append(entries, candidate{name, info.Size(), cached.Origin.CompletedAt})
		total += info.Size()
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].time.Before(entries[j].time) })
	for len(entries) >= maxEntries || total+incoming > maxBytes {
		if len(entries) == 0 {
			return fmt.Errorf("result cache capacity exceeded")
		}
		if err = root.Remove(entries[0].name); err != nil {
			return err
		}
		total -= entries[0].size
		entries = entries[1:]
	}
	return nil
}
