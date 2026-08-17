package metrics

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	seriesBucket = []byte("series")
	metaBucket   = []byte("meta")
)

func Open(path string) (*DB, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("metrics: open db: %w", err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(seriesBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(metaBucket)
		return err
	}); err != nil {
		db.Close()
		return nil, fmt.Errorf("metrics: init bucket: %w", err)
	}
	return &DB{db: db}, nil
}

type DB struct {
	db *bolt.DB
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) Write(name string, tags map[string]string, ts time.Time, value float64) error {
	hash := tagsHash(tags)
	key := makeSeriesKey(name, tags, ts)
	val := makeValueBytes(value)
	metaKey := makeSeriesPrefix([]byte(name), hash)
	metaVal, _ := json.Marshal(tags)
	return d.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(seriesBucket).Put(key, val); err != nil {
			return err
		}
		return tx.Bucket(metaBucket).Put(metaKey, metaVal)
	})
}

func (d *DB) WriteBatch(metrics []Metric) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(seriesBucket)
		mb := tx.Bucket(metaBucket)
		for _, m := range metrics {
			hash := tagsHash(m.Tags)
			prefix := makeSeriesPrefix([]byte(m.Name), hash)
			metaVal, _ := json.Marshal(m.Tags)
			if err := mb.Put(prefix[:len(prefix):len(prefix)], metaVal); err != nil {
				return err
			}
			for _, p := range m.Points {
				key := append(prefix[:len(prefix):len(prefix)], makeTimestampBytes(p.Timestamp)...)
				val := makeValueBytes(p.Value)
				if err := b.Put(key, val); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (d *DB) Query(name string, tags map[string]string, from, to time.Time, limit int) ([]Metric, error) {
	fromNano := from.UnixNano()
	toNano := to.UnixNano()

	var result []Metric
	var curIdx = -1
	var curKey []byte

	err := d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(seriesBucket)
		c := b.Cursor()

		var prefix []byte
		if len(tags) > 0 {
			prefix = makeSeriesPrefix([]byte(name), tagsHash(tags))
		} else {
			prefix = makeNamePrefix([]byte(name))
		}

		totalPoints := 0
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			ts := int64(binary.BigEndian.Uint64(k[len(k)-8:]))

			if ts < fromNano {
				continue
			}
			if ts > toNano {
				break
			}

			if curIdx < 0 || (len(tags) == 0 && !bytes.Equal(curKey, k[:len(k)-8])) {
				var seriesTags map[string]string
				if len(tags) > 0 {
					seriesTags = copyTags(tags)
				} else {
					seriesTags = tagsFromMeta(tx.Bucket(metaBucket), k[:len(k)-8])
				}
				result = append(result, Metric{
					Name:   name,
					Tags:   seriesTags,
					Points: make([]DataPoint, 0, 64),
				})
				curIdx = len(result) - 1
				curKey = append(curKey[:0], k[:len(k)-8]...)
			}

			result[curIdx].Points = append(result[curIdx].Points, DataPoint{
				Timestamp: ts,
				Value:     math.Float64frombits(binary.LittleEndian.Uint64(v)),
			})

			totalPoints++
			if limit > 0 && totalPoints >= limit {
				break
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

// tagsFromMeta loads the stored tags for a series key prefix (name + hash).
func tagsFromMeta(meta *bolt.Bucket, key []byte) map[string]string {
	if meta == nil {
		return nil
	}
	v := meta.Get(key)
	if v == nil {
		return nil
	}
	var tags map[string]string
	if err := json.Unmarshal(v, &tags); err != nil {
		return nil
	}
	return tags
}

func makeNamePrefix(name []byte) []byte {
	key := make([]byte, len(name)+1)
	copy(key, name)
	key[len(name)] = 0
	return key
}

func (d *DB) ListMetrics() ([]string, error) {
	seen := make(map[string]struct{})

	err := d.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(seriesBucket).Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			idx := bytes.IndexByte(k, 0)
			if idx < 0 {
				continue
			}
			name := string(k[:idx])
			seen[name] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

// ListHosts returns the unique host tag values stored in the series metadata.
func (d *DB) ListHosts() ([]string, error) {
	seen := make(map[string]struct{})

	err := d.db.View(func(tx *bolt.Tx) error {
		mb := tx.Bucket(metaBucket)
		c := mb.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var tags map[string]string
			if err := json.Unmarshal(v, &tags); err != nil {
				continue
			}
			if h := tags["host"]; h != "" {
				seen[h] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(seen))
	for host := range seen {
		result = append(result, host)
	}
	sort.Strings(result)
	return result, nil
}

func (d *DB) Compact(retention time.Duration) error {
	if retention <= 0 {
		return nil
	}
	cutoff := time.Now().Add(-retention)
	cutoffBE := makeTimestampBytes(cutoff.UnixNano())

	return d.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(seriesBucket)
		c := b.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			ts := k[len(k)-8:]
			if bytes.Compare(ts, cutoffBE) < 0 {
				if err := c.Delete(); err != nil {
					return err
				}
			}
		}

		// Prune series metadata with no remaining data points.
		mb := tx.Bucket(metaBucket)
		mc := mb.Cursor()
		sc := b.Cursor()
		for mk, _ := mc.First(); mk != nil; mk, _ = mc.Next() {
			sk, _ := sc.Seek(mk)
			if sk == nil || !bytes.HasPrefix(sk, mk) {
				if err := mc.Delete(); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func makeSeriesKey(name string, tags map[string]string, ts time.Time) []byte {
	hash := tagsHash(tags)
	prefix := makeSeriesPrefix([]byte(name), hash)
	return append(prefix, makeTimestampBytes(ts.UnixNano())...)
}

func makeSeriesPrefix(name []byte, hash uint64) []byte {
	key := make([]byte, 0, len(name)+1+8)
	key = append(key, name...)
	key = append(key, 0)
	key = binary.BigEndian.AppendUint64(key, hash)
	return key
}

func makeTimestampBytes(ns int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(ns))
	return b
}

func makeValueBytes(v float64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, math.Float64bits(v))
	return b
}

func tagsHash(tags map[string]string) uint64 {
	if len(tags) == 0 {
		return 0
	}
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := fnv.New64a()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte("="))
		h.Write([]byte(tags[k]))
		h.Write([]byte{0})
	}
	return h.Sum64()
}

func copyTags(tags map[string]string) map[string]string {
	r := make(map[string]string, len(tags))
	for k, v := range tags {
		r[k] = v
	}
	return r
}

func ParseRetention(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err == nil {
		return d
	}
	if strings.HasSuffix(s, "d") {
		days := 0
		fmt.Sscanf(s, "%dd", &days)
		if days > 0 {
			return time.Duration(days) * 24 * time.Hour
		}
	}
	return 0
}
