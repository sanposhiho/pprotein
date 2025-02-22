package collect

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kaz/pprotein/internal/storage"
)

type (
	Snapshot struct {
		store storage.Storage

		*SnapshotMeta
		*SnapshotTarget
	}
	SnapshotMeta struct {
		Type        string
		ID          string
		Datetime    time.Time
		GitRevision string
	}
	SnapshotTarget struct {
		GroupId  string
		Label    string
		URL      string
		Duration int
	}
)

func newSnapshot(store storage.Storage, typ string, ext string, target *SnapshotTarget) *Snapshot {
	ts := time.Now()
	id := strconv.FormatInt(ts.UnixNano(), 36) + ext

	return &Snapshot{
		store: store,

		SnapshotMeta: &SnapshotMeta{
			Type:        typ,
			ID:          id,
			Datetime:    ts,
			GitRevision: "",
		},
		SnapshotTarget: target,
	}
}

func (s *Snapshot) unmarshal(raw []byte) error {
	return json.Unmarshal(raw, s)
}
func (s *Snapshot) marshal() ([]byte, error) {
	return json.Marshal(s)
}

func (s *Snapshot) Collect() error {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s?seconds=%d", s.URL, s.Duration), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("http error: %w", err)
	}
	defer resp.Body.Close()

	var r io.Reader = resp.Body
	if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
		cr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to initialize gzip reader: %w", err)
		}
		defer cr.Close()

		r = cr
	}

	bodyContent, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("failed to read body: %w", err)
	}
	s.GitRevision = resp.Header.Get("X-GIT-REVISION")

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http error: status=%v, body=%v", resp.StatusCode, string(bodyContent))
	}
	if len(bodyContent) == 0 {
		return fmt.Errorf("received empty content")
	}

	serialized, err := s.marshal()
	if err != nil {
		return fmt.Errorf("failed to serialize: %w", err)
	}
	if err := s.store.PutSnapshot(s.ID, s.Type, serialized); err != nil {
		return fmt.Errorf("failed to write meta: %w", err)
	}
	if err := s.store.PutBlob(s.ID, bodyContent); err != nil {
		return fmt.Errorf("failed to write body: %w", err)
	}
	return nil
}

func (s *Snapshot) BodyPath() (string, error) {
	return s.store.GetBlobPath(s.ID)
}

func (s *Snapshot) Prune() error {
	return s.store.DeleteSnapshot(s.ID)
}
