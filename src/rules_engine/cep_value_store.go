package rules_engine

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/cockroachdb/pebble"
)

// CEPValueStore stores CEP event snapshots outside in-memory state.
// Local cache mode can keep only lightweight pointers in memory and read values back on completion.
type CEPValueStore interface {
	PutSnapshot(data map[string]interface{}, expiresAtNs int64) (string, error)
	GetSnapshot(ref string) (map[string]interface{}, error)
	DeleteSnapshot(ref string) error
	Close() error
}

type pebbleCEPValueRecord struct {
	Data        map[string]interface{} `json:"d"`
	ExpiresAtNs int64                  `json:"e"`
}

type pebblePutRequest struct {
	ref         string
	payload     []byte
	expiresAtNs int64
}

const (
	pebbleWriteQueueSize = 4096
	pebbleWriteBatchSize = 128
	pebbleCleanupTick    = 30 * time.Second
	pebbleCleanupLimit   = 5000
)

// PebbleCEPValueStore is a local disk-backed value store for CEP snapshots.
type PebbleCEPValueStore struct {
	db          *pebble.DB
	dbPath      string
	counter     uint64
	writeQueue  chan *pebblePutRequest
	inflight    sync.Map // ref -> []byte (payload pending batch commit)
	stopCh      chan struct{}
	wg          sync.WaitGroup
	closed      atomic.Bool
	closeSignal sync.Once
}

var pebbleStoreInstanceCounter uint64

// NewPebbleCEPValueStore creates/opens a Pebble DB for a specific ruleset.
func NewPebbleCEPValueStore(rulesetID string) (*PebbleCEPValueStore, error) {
	baseDir := filepath.Join(os.TempDir(), "agentsmith-hub", "cep_values")
	safeRulesetID := sanitizePathComponent(rulesetID)
	if safeRulesetID == "" {
		safeRulesetID = "default"
	}
	instanceID := atomic.AddUint64(&pebbleStoreInstanceCounter, 1)
	dbPath := filepath.Join(
		baseDir,
		safeRulesetID,
		fmt.Sprintf("inst-%d-%d-%d", os.Getpid(), time.Now().UnixNano(), instanceID),
	)

	if err := os.MkdirAll(dbPath, 0o755); err != nil {
		return nil, fmt.Errorf("create pebble dir failed: %w", err)
	}

	// This store is an ephemeral TTL cache for CEP snapshots. Prefer smooth
	// write latency over long compaction work: disable compactions and raise
	// stop-write thresholds to avoid write stalls under burst traffic.
	db, err := pebble.Open(dbPath, &pebble.Options{
		DisableAutomaticCompactions: true,
		DisableWAL:                  true,
		MemTableSize:                64 << 20, // 64MB
		MemTableStopWritesThreshold: 32,
		L0CompactionThreshold:       1 << 30,
		L0StopWritesThreshold:       1 << 30,
	})
	if err != nil {
		return nil, fmt.Errorf("open pebble failed: %w", err)
	}

	store := &PebbleCEPValueStore{
		db:         db,
		dbPath:     dbPath,
		writeQueue: make(chan *pebblePutRequest, pebbleWriteQueueSize),
		stopCh:     make(chan struct{}),
	}
	store.wg.Add(2)
	go store.writeLoop()
	go store.cleanupLoop()
	return store, nil
}

// PutSnapshot serializes data and enqueues an asynchronous write to Pebble.
// The payload is kept in an inflight map so that an immediate GetSnapshot
// can serve it before the batch commits. If the write queue is full the
// call returns an error and the caller should keep data inline.
func (s *PebbleCEPValueStore) PutSnapshot(data map[string]interface{}, expiresAtNs int64) (string, error) {
	if s == nil || s.db == nil || s.closed.Load() {
		return "", fmt.Errorf("pebble value store not initialized")
	}
	// Encode expiresAtNs in ref so deleteSnapshotDirect can reconstruct the
	// expiry index key without an extra refIndex lookup (saves one key per snapshot).
	ref := fmt.Sprintf("%d|%d-%d", expiresAtNs, time.Now().UnixNano(), atomic.AddUint64(&s.counter, 1))
	record := pebbleCEPValueRecord{
		Data:        data,
		ExpiresAtNs: expiresAtNs,
	}
	payload, err := sonic.Marshal(record)
	if err != nil {
		return "", err
	}

	// Keep payload in inflight map so immediate GetSnapshot can return it
	// before the writeLoop commits the batch to Pebble.
	s.inflight.Store(ref, payload)

	req := &pebblePutRequest{
		ref:         ref,
		payload:     payload,
		expiresAtNs: expiresAtNs,
	}
	select {
	case s.writeQueue <- req:
		return ref, nil
	case <-s.stopCh:
		s.inflight.Delete(ref)
		return "", fmt.Errorf("pebble value store stopping")
	default:
		// Queue full – caller will keep data inline as fallback.
		s.inflight.Delete(ref)
		return "", fmt.Errorf("write queue full")
	}
}

func (s *PebbleCEPValueStore) GetSnapshot(ref string) (map[string]interface{}, error) {
	if s == nil || s.db == nil || s.closed.Load() {
		return nil, fmt.Errorf("pebble value store not initialized")
	}

	// Check inflight map first (covers the async window before batch commit).
	var val []byte
	if raw, ok := s.inflight.Load(ref); ok {
		val = raw.([]byte)
	} else {
		v, closer, err := s.db.Get(dataKey(ref))
		if err != nil {
			return nil, err
		}
		val = append([]byte(nil), v...)
		closer.Close()
	}

	var record pebbleCEPValueRecord
	if err := sonic.Unmarshal(val, &record); err != nil {
		return nil, err
	}

	if record.ExpiresAtNs > 0 && time.Now().UnixNano() > record.ExpiresAtNs {
		_ = s.DeleteSnapshot(ref)
		return nil, fmt.Errorf("snapshot expired")
	}
	return record.Data, nil
}

func (s *PebbleCEPValueStore) DeleteSnapshot(ref string) error {
	if s == nil || s.db == nil || s.closed.Load() {
		return nil
	}
	s.inflight.Delete(ref)
	return s.deleteSnapshotDirect(ref)
}

func (s *PebbleCEPValueStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	s.closeSignal.Do(func() {
		s.closed.Store(true)
		close(s.stopCh)
	})
	s.wg.Wait()
	err := s.db.Close()
	dbPath := s.dbPath
	s.db = nil
	s.dbPath = ""
	if dbPath != "" {
		if removeErr := os.RemoveAll(dbPath); removeErr != nil && err == nil {
			err = removeErr
		}
	}
	return err
}

func (s *PebbleCEPValueStore) writeLoop() {
	defer s.wg.Done()
	for {
		select {
		case <-s.stopCh:
			s.flushPendingWrites()
			return
		case first := <-s.writeQueue:
			if first == nil {
				continue
			}
			reqs := make([]*pebblePutRequest, 0, pebbleWriteBatchSize)
			reqs = append(reqs, first)
		drain:
			for len(reqs) < pebbleWriteBatchSize {
				select {
				case req := <-s.writeQueue:
					if req != nil {
						reqs = append(reqs, req)
					}
				default:
					break drain
				}
			}
			_ = s.commitPutBatch(reqs)
			// Remove committed entries from inflight map.
			for _, req := range reqs {
				s.inflight.Delete(req.ref)
			}
		}
	}
}

func (s *PebbleCEPValueStore) flushPendingWrites() {
	for {
		select {
		case req := <-s.writeQueue:
			if req == nil {
				continue
			}
			_ = s.commitPutBatch([]*pebblePutRequest{req})
			s.inflight.Delete(req.ref)
		default:
			return
		}
	}
}

// commitPutBatch writes data and expiry-index keys for each request.
// The refIndex key is no longer needed because expiresAtNs is encoded
// in the ref string itself (2 keys per snapshot instead of 3).
func (s *PebbleCEPValueStore) commitPutBatch(reqs []*pebblePutRequest) error {
	if len(reqs) == 0 {
		return nil
	}
	batch := s.db.NewBatch()
	defer batch.Close()

	for _, req := range reqs {
		if err := batch.Set(dataKey(req.ref), req.payload, nil); err != nil {
			return err
		}
		if err := batch.Set(expiryIndexKey(req.expiresAtNs, req.ref), []byte{}, nil); err != nil {
			return err
		}
	}
	return batch.Commit(pebble.NoSync)
}

func (s *PebbleCEPValueStore) cleanupLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(pebbleCleanupTick)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			_ = s.cleanupExpired(time.Now().UnixNano(), pebbleCleanupLimit)
		}
	}
}

func (s *PebbleCEPValueStore) cleanupExpired(nowNs int64, limit int) error {
	lower := expiryPrefix()
	upper := expiryIndexUpperBound(nowNs)
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: lower,
		UpperBound: upper,
	})
	if err != nil {
		return err
	}
	defer iter.Close()

	batch := s.db.NewBatch()
	defer batch.Close()

	count := 0
	for iter.First(); iter.Valid(); iter.Next() {
		expKey := append([]byte(nil), iter.Key()...)
		ref := parseRefFromExpiryKey(expKey)
		if ref == "" {
			continue
		}
		if err := batch.Delete(expKey, nil); err != nil {
			return err
		}
		if err := batch.Delete(dataKey(ref), nil); err != nil {
			return err
		}
		s.inflight.Delete(ref)
		count++
		if limit > 0 && count >= limit {
			break
		}
	}
	if count == 0 {
		return nil
	}
	return batch.Commit(pebble.NoSync)
}

// deleteSnapshotDirect deletes a snapshot and its expiry index entry.
// The expiresAtNs is parsed from the ref itself instead of performing
// an extra db.Get on a refIndex key.
func (s *PebbleCEPValueStore) deleteSnapshotDirect(ref string) error {
	batch := s.db.NewBatch()
	defer batch.Close()

	if expNs, ok := parseExpiresNsFromRef(ref); ok {
		_ = batch.Delete(expiryIndexKey(expNs, ref), nil)
	}
	if err := batch.Delete(dataKey(ref), nil); err != nil {
		return err
	}
	return batch.Commit(pebble.NoSync)
}

func dataKey(ref string) []byte {
	return []byte("d:" + ref)
}

func expiryPrefix() []byte {
	return []byte("e:")
}

func expiryIndexKey(expiresAtNs int64, ref string) []byte {
	// Zero-padded timestamp keeps lexicographic order equal to time order.
	return []byte(fmt.Sprintf("e:%020d:%s", expiresAtNs, ref))
}

func expiryIndexUpperBound(nowNs int64) []byte {
	// '\xff' ensures all refs at this timestamp are included.
	return []byte(fmt.Sprintf("e:%020d:\xff", nowNs))
}

func parseRefFromExpiryKey(k []byte) string {
	// format: e:<20-digit-ts>:<ref>
	parts := bytes.SplitN(k, []byte(":"), 3)
	if len(parts) != 3 {
		return ""
	}
	return string(parts[2])
}

// parseExpiresNsFromRef extracts expiresAtNs encoded in the ref string.
// Ref format: "<expiresAtNs>|<ts>-<counter>".
func parseExpiresNsFromRef(ref string) (int64, bool) {
	idx := strings.IndexByte(ref, '|')
	if idx <= 0 {
		return 0, false
	}
	ns, err := strconv.ParseInt(ref[:idx], 10, 64)
	if err != nil {
		return 0, false
	}
	return ns, true
}

func sanitizePathComponent(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	// Keep path-friendly chars only.
	out := strings.Builder{}
	out.Grow(len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			out.WriteRune(r)
		} else {
			out.WriteByte('_')
		}
	}
	return out.String()
}
