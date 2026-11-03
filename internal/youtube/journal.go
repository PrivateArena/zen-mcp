package youtube

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Journal is the durable, human-readable local ledger of channel mutations (§4.6), and the only
// subsystem that may enforce the daily upload cap.
//
// The order of operations is the whole point:
//
//	Claim(slot) -> save -> attach file -> Complete(slot, videoId)
//
// Claiming *before* the attach is what closes the "abandoned upload consumed the slot but was never
// recorded" hole: an attach creates the video and Studio autosaves it as a private draft, so a flow that
// recorded the slot afterwards would leave a real draft on the channel that nothing knows about. A slot
// left uncompleted is surfaced as abandoned and stays charged against today's limit.
type Journal struct {
	path string
	now  func() time.Time
	loc  *time.Location

	Date         string         `json:"date"`
	MaxPerDay    int            `json:"maxPerDay"`
	UploadsToday int            `json:"uploadsToday"`
	Slots        []JournalSlot  `json:"slots"`
	Events       []JournalEvent `json:"events,omitempty"`
}

// JournalSlot is one claimed or completed mutation.
type JournalSlot struct {
	SlotID    string    `json:"slotId"`
	Kind      string    `json:"kind"` // upload | comment | edit | delete
	VideoID   string    `json:"videoId,omitempty"`
	Status    string    `json:"status"` // claimed | completed | abandoned
	Detail    string    `json:"detail,omitempty"`
	ClaimedAt time.Time `json:"claimedAt"`
	Dated     string    `json:"date"`
}

// JournalEvent is an append-only note: a video that went out unrecorded, a restart seen, a refusal.
type JournalEvent struct {
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"`
	VideoID string    `json:"videoId,omitempty"`
	Detail  string    `json:"detail,omitempty"`
}

// OpenJournal loads ~/.zen-mcp/youtube/journal.json, creating it on first use, and re-rolls the day. An
// existing file that cannot be parsed is left untouched on disk and reported — a corrupt ledger must not
// silently reset the cap.
func OpenJournal(path string, maxPerDay int, loc *time.Location) (*Journal, error) {
	if loc == nil {
		loc = time.Local
	}
	j := &Journal{path: path, now: func() time.Time { return time.Now().In(loc) }, loc: loc, MaxPerDay: maxPerDay}
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		j.rollDate()
		return j, nil
	case err != nil:
		return nil, fmt.Errorf("read journal %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, j); err != nil {
		return nil, fmt.Errorf("parse journal %s: %w (fix or move the file; the cap is not reset automatically)", path, err)
	}
	j.path, j.now, j.loc, j.MaxPerDay = path, func() time.Time { return time.Now().In(loc) }, loc, maxPerDay
	j.rollDate()
	return j, nil
}

// rollDate resets the counter when the configured date has turned, and re-flags anything still claimed
// from an earlier day as abandoned.
func (j *Journal) rollDate() {
	today := j.now().Format("2006-01-02")
	if j.Date == today {
		j.UploadsToday = j.countOpenUploads()
		return
	}
	for i := range j.Slots {
		if j.Slots[i].Status == "claimed" {
			j.Slots[i].Status = "abandoned"
			j.Slots[i].Detail = "unfinished across a date boundary"
			j.Events = append(j.Events, JournalEvent{At: j.now(), Kind: "slot_abandoned", VideoID: j.Slots[i].VideoID, Detail: j.Slots[i].SlotID})
		}
	}
	j.Date = today
	j.UploadsToday = 0
}

func (j *Journal) countOpenUploads() int {
	n := 0
	for _, s := range j.Slots {
		if s.Dated == j.Date && s.Kind == "upload" && s.Status != "released" {
			n++
		}
	}
	return n
}

// Claim takes a slot for one channel mutation, before the mutation happens. It refuses at the cap.
func (j *Journal) Claim(kind string) (JournalSlot, error) {
	j.rollDate()
	if kind == "upload" && j.MaxPerDay > 0 && j.countOpenUploads() >= j.MaxPerDay {
		return JournalSlot{}, fmt.Errorf("%w: %d of %d used today (journal %s)", ErrDailyLimit, j.UploadsToday, j.MaxPerDay, j.path)
	}
	slot := JournalSlot{
		SlotID:    fmt.Sprintf("s_%s_%d", j.now().Format("20060102T150405"), len(j.Slots)+1),
		Kind:      kind,
		Status:    "claimed",
		ClaimedAt: j.now(),
		Dated:     j.Date,
	}
	j.Slots = append(j.Slots, slot)
	j.UploadsToday = j.countOpenUploads()
	return slot, j.Save()
}

// Complete records the video id the page reported, which is the only trustworthy binding between a slot
// and a real upload — the caller never supplies it.
func (j *Journal) Complete(slotID, videoID string) error {
	for i := range j.Slots {
		if j.Slots[i].SlotID != slotID {
			continue
		}
		j.Slots[i].Status = "completed"
		j.Slots[i].VideoID = videoID
		return j.Save()
	}
	return fmt.Errorf("journal: no slot %s to complete", slotID)
}

// Abandon marks a claimed slot as finished-without-success, keeping its video id when the page revealed
// one. It stays charged against the daily cap: the draft exists on the channel whether or not we
// completed the wizard.
func (j *Journal) Abandon(slotID, videoID, reason string) error {
	for i := range j.Slots {
		if j.Slots[i].SlotID != slotID {
			continue
		}
		j.Slots[i].Status = "abandoned"
		if videoID != "" {
			j.Slots[i].VideoID = videoID
		}
		j.Slots[i].Detail = reason
		j.Events = append(j.Events, JournalEvent{At: j.now(), Kind: "slot_abandoned", VideoID: j.Slots[i].VideoID, Detail: reason})
		return j.Save()
	}
	return fmt.Errorf("journal: no slot %s to abandon", slotID)
}

// Release returns a claimed slot to the day, for a run that provably created nothing — it never reached the
// attach, so there is no draft on the channel. The plan's rule is that an *abandoned upload* stays charged,
// because the video exists; a run that could not open a tab is not that. The event is still recorded.
func (j *Journal) Release(slotID, reason string) error {
	for i := range j.Slots {
		if j.Slots[i].SlotID != slotID {
			continue
		}
		j.Slots[i].Status = "released"
		j.Slots[i].Detail = reason
		j.Events = append(j.Events, JournalEvent{At: j.now(), Kind: "slot_released", Detail: reason})
		j.UploadsToday = j.countOpenUploads()
		return j.Save()
	}
	return fmt.Errorf("journal: no slot %s to release", slotID)
}

// NoteUnrecorded records a video id that the page revealed but the ledger never claimed a slot for — the
// evidence that the cap is only an estimate.
func (j *Journal) NoteUnrecorded(videoID, detail string) error {
	j.Events = append(j.Events, JournalEvent{At: j.now(), Kind: "unrecorded", VideoID: videoID, Detail: detail})
	return j.Save()
}

// Today lists today's slots, newest first — what `upload_status` consults before trusting its memory.
func (j *Journal) Today() []JournalSlot {
	j.rollDate()
	var out []JournalSlot
	for _, s := range j.Slots {
		if s.Dated == j.Date {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ClaimedAt.After(out[k].ClaimedAt) })
	return out
}

// All returns the whole ledger.
func (j *Journal) All() []JournalSlot { return append([]JournalSlot(nil), j.Slots...) }

// Limit is the configured daily cap.
func (j *Journal) Limit() int { return j.MaxPerDay }

// TodayUploads counts today's upload slots, abandoned ones included — an abandoned upload is still a video
// that exists on the channel.
func (j *Journal) TodayUploads() int {
	j.rollDate()
	return j.countOpenUploads()
}

// SlotForVideo finds the most recent slot bound to a video id.
func (j *Journal) SlotForVideo(videoID string) (JournalSlot, bool) {
	for i := len(j.Slots) - 1; i >= 0; i-- {
		if j.Slots[i].VideoID == videoID && videoID != "" {
			return j.Slots[i], true
		}
	}
	return JournalSlot{}, false
}

// Save writes atomically: a half-written journal is worse than no journal, because it silently raises or
// lowers the daily cap.
func (j *Journal) Save() error {
	if j.path == "" {
		return nil
	}
	if dir := filepath.Dir(j.path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create journal dir: %w", err)
		}
	}
	raw, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	tmp := j.path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write journal: %w", err)
	}
	return os.Rename(tmp, j.path)
}

// DefaultEvidenceDir is where a run writes its screenshots — the plan requires an evidence path in every
// mutation reply, so the location is fixed rather than asked for.
func DefaultEvidenceDir() string {
	base, err := JournalPath()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(base), "evidence")
}

// JournalPath is the default location, honouring XDG_STATE_HOME (§4.6).
func JournalPath() (string, error) {
	if custom := os.Getenv("ZEN_YOUTUBE_JOURNAL"); custom != "" {
		return custom, nil
	}
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve journal path: %w", err)
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "zen-mcp", "youtube", "journal.json"), nil
}
