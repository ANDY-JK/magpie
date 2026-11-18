// Package usage keeps the token count of every call the gateway serves, so
// magpie can show what each agent and model consumed and roughly what it cost.
// Records go to one JSON-lines file next to providers.json. Metadata leaves
// the machine only when OTLP export is explicitly enabled.
package usage

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"maps"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yetone/magpie/internal/access"
	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
)

// Record is one call.
type Record struct {
	Operation       string    `json:"operation,omitempty"`
	RouteID         int64     `json:"route_id,omitempty"` // the gateway Route, shared by its attempts
	Time            time.Time `json:"t"`
	Agent           string    `json:"agent"` // magpie agent id, or the client's product name
	Provider        string    `json:"provider"`
	Host            string    `json:"host,omitempty"`          // where the call went: provider.Where then
	ProviderKeyID   string    `json:"providerKeyId,omitempty"` // fingerprint of the API key actually used
	ProviderKeyName string    `json:"providerKeyName,omitempty"`
	// ProviderAccount is the subscription account that answered the call,
	// as the Routing trace names it (provider.Account.User: an email, a
	// login): the one that took over after a failover, the one pinned by
	// X-Magpie-Account. Never a token. "" for a key or a provider without
	// an account, and in a record written before it was kept (#557).
	ProviderAccount string `json:"providerAccount,omitempty"`
	CallerKeyID     string `json:"callerKeyId,omitempty"`
	CallerKeyName   string `json:"callerKeyName,omitempty"`
	// CallerIssuerID is the key CallerKeyID was issued under (an issued,
	// short-lived key: access.IssueKey), whose limits count this call too.
	CallerIssuerID string `json:"callerIssuerId,omitempty"`
	// SessionProvider is the provider ID recorded by the agent. SessionAccount
	// identifies the session's creator, not the account used for an API request.
	// Neither field establishes an upstream route from today's configuration.
	SessionProvider string `json:"session_provider,omitempty"`
	SessionAccount  string `json:"session_account,omitempty"`
	// SessionOfficialLogin marks a matched account's explicit official login
	// metadata, not this call's endpoint or billing authentication.
	SessionOfficialLogin bool   `json:"session_official_login,omitempty"`
	Model                string `json:"model"` // the provider's model id
	// Requested is the model id the agent asked for (a magpie alias, a
	// routing group, provider/model…), and Served the model the vendor's
	// reply says answered, when it named one: a ledger to set beside the
	// vendor's own bill. Neither is in a record written before they were.
	Requested string `json:"req,omitempty"`
	Served    string `json:"served,omitempty"`
	// Upstream: the provider an aggregator (OpenRouter …) said answered
	// behind it — DeepInfra, Novita — when its reply named one
	Upstream string `json:"upstream,omitempty"`
	Input    int    `json:"in"`
	Output   int    `json:"out"`
	// CacheRead is the prompt tokens read from the vendor's cache, and
	// CacheWrite those written to it (Anthropic's cache creation,
	// cache_creation_input_tokens); neither is in Input. Both are written
	// on every record, 0 too, as in and out are, so a call whose prompt
	// missed the cache reads as that rather than as one whose cache wasn't
	// kept (#1839). A record from before they were has neither: 0.
	CacheRead  int `json:"cache_read"`
	CacheWrite int `json:"cache_write"`
	// CacheWrite1h is how many of CacheWrite were written to be kept for
	// an hour (Anthropic's 1-hour TTL, billed at 2× input where 5 minutes'
	// is 1.25×); the rest were for 5 minutes. A record from before it was
	// kept has none: all its writes are counted as 5-minute ones, as they
	// were.
	CacheWrite1h int `json:"cache_write_1h,omitempty"`
	// CacheUnknown: the vendor counted no cache tokens for this call (a
	// plugin's magpie_cache_unknown: Kiro caches but tells only credits),
	// so its CacheRead of 0 is not a miss. A hit rate leaves its prompt
	// out (Totals.CacheUnknown, #3464). A record from before it was kept
	// has none: its 0 still reads as a miss.
	CacheUnknown bool `json:"cache_unknown,omitempty"`
	Reasoning    int  `json:"reasoning,omitempty"`
	// Effort is the reasoning the model was asked for — a routing group's
	// pick for the turn, or the agent's own — as it takes it; "" for none
	Effort string `json:"effort,omitempty"`
	// The service tier, as OpenAI's service_tier words it ("priority",
	// "ultrafast", "flex", "default"; "fast" for Claude's speed "fast"):
	// TierAsked what the client's request asked for, TierSent what went
	// out to the vendor on this try, after a fast pick or a group member
	// set Fast asked for priority and an account whose plan doesn't offer
	// Ultrafast was asked for priority in its place, and TierServed what
	// the vendor's reply said it ran at. Each is "" when there was none:
	// TierServed is never taken from what was asked or sent, only from a
	// reply that names it (#1784)
	TierAsked  string `json:"tier_asked,omitempty"`
	TierSent   string `json:"tier_sent,omitempty"`
	TierServed string `json:"tier_served,omitempty"`
	Millis     int64  `json:"ms"`
	// TTFT: ms from the request to its reply's first content — text,
	// reasoning or a tool call — and FirstText to its first text, counted
	// as Millis is, so Millis-TTFT is how long the reply took to write;
	// none for a reply that wasn't streamed (#196)
	TTFT      int64 `json:"ttft_ms,omitempty"`
	FirstText int64 `json:"first_text_ms,omitempty"`
	// Compaction is Codex's explicit native marker. Nil means no matching
	// native Codex call established whether a gateway row compacted.
	Compaction *bool `json:"compaction,omitempty"`
	// FirstReasoning is the start of paced visible reasoning, absent for hidden reasoning.
	FirstReasoning int64 `json:"first_reasoning_ms,omitempty"`
	// ReasoningHidden: the reply reasoned and its stream doesn't tell how
	// much — sealed or summarised, or streamed with no reasoning count in
	// its usage (#4545: Anthropic's usage never has one), when Reasoning is
	// 0 for unknown, not for none. A reply of no text then tells no speed
	// (Decode).
	ReasoningHidden bool `json:"reasoning_hidden,omitempty"`
	// Sent: ms from the request to its answering try going out to the
	// vendor, its body written, as TTFT is counted: magpie's own time and
	// the tries that failed first are before it, and TTFT-Sent is how
	// long the vendor took to its first content. 0 where it isn't known
	// (a reply not streamed, or a vendor magpie doesn't reach over HTTP).
	Sent int64 `json:"sent_ms,omitempty"`
	// Of Sent, what was magpie's: Own is magpie's own work on the request
	// (reading it, translating it, choosing whom to ask), Retried the
	// tries that failed first and the pauses after them, Queued the wait
	// for a slot of a key's or account's MaxConcurrency or MaxRPM; the
	// rest, Sent-Own-Retried-Queued, is connecting to the vendor and
	// sending it the body. #1532: a vendor that held a try ~100 s and
	// said 503 read as 104 s "In magpie". 0 in a record written before
	// they were, where Sent is all that is known.
	Own int64 `json:"own_ms,omitempty"`
	// Masking: the ms of Own that went on masking the request (Settings
	// → Privacy: the rules, and the local model with its wait for a
	// turn), all of it where Own is unknown; 0 where nothing was masked,
	// or in a record from before it was kept.
	Masking int64 `json:"masking_ms,omitempty"`
	Retried int64 `json:"retried_ms,omitempty"`
	Queued  int64 `json:"queued_ms,omitempty"`
	// Flow: the ms the reply's content took to come, from where its speed
	// is counted (DecodeOf): from a tenth of its content to nine tenths,
	// spread over the whole. A reply a vendor held and let go in one burst
	// has a flow of next to none, however long its end came after its
	// first content, and so tells no speed (John on Discord: a Kimi Code
	// reply read 1,367 tok/s). 0 where it isn't known.
	Flow   int64 `json:"flow_ms,omitempty"`
	Status int   `json:"status"`
	// Error is why a call failed, in the vendor's words and cut short;
	// ErrType what its body called the error (rate_limit_error,
	// usage_limit_reached); RequestID the id the vendor gave the call; and
	// Endpoint the path the agent called, with the one it was sent to when
	// it went out in another protocol (/v1/messages → /v1/chat/completions)
	Error     string `json:"err,omitempty"`
	ErrType   string `json:"err_type,omitempty"`
	RequestID string `json:"rid,omitempty"`
	// ResponseID is the response object ID actually sent to the client.
	ResponseID string `json:"response_id,omitempty"`
	Endpoint   string `json:"ep,omitempty"`
	// Stop is why the upstream said its reply ended, in its own words
	// (stop_reason, finish_reason …): "" when it said none, a stream that
	// just stopped. Told apart by it, a reply that ended too soon is the
	// upstream's own end_turn or a cut (蓝猫 on Discord).
	Stop string `json:"stop,omitempty"`
	// Session is the conversation the call was part of, as its agent names
	// it (X-Magpie-Session, or the session header Claude Code, Codex or
	// OpenCode sends): several sessions on one model told apart
	Session string `json:"session,omitempty"`
	// NativeSession retains the client header when X-Magpie-Session overrides it.
	NativeSession string `json:"native_session,omitempty"`
	// Subagent is the subagent that made the call, as its agent names it
	// (Claude Code's x-claude-code-agent-id, the agentId of its
	// subagents/agent-<id>.jsonl): Session stays the conversation it was
	// spawned in, so its calls are counted there too. ParentAgent is the
	// subagent that spawned it in turn (x-claude-code-parent-agent-id), ""
	// when the conversation itself did or the agent doesn't say.
	Subagent    string `json:"subagent,omitempty"`
	ParentAgent string `json:"parent_agent,omitempty"`
	// Rejected is a request refused locally before an upstream was contacted.
	Rejected bool `json:"rejected,omitempty"`
	// Tries is how many tries the gateway made at the request this row
	// ends, as its route lists them, when there was more than one: a
	// request answered on a later try was answered, not failed (小一 on
	// Discord). HandedOn marks the row of one try that failed and handed
	// the request on to the next (a vendor's refusal kept for what it
	// billed): it is not the request's outcome, which a later row with
	// the same RouteID is, so it is neither a request nor a failure of
	// its own. Neither is in a record written before they were; the log
	// index tells HandedOn from such a record's route (handedOn).
	Tries    int  `json:"tries,omitempty"`
	HandedOn bool `json:"handed_on,omitempty"`
	// UpBytes is how many bytes of this call's request actually went up to
	// the vendor on the wire, its tries summed: the plain body where it
	// goes as read, less where it went compressed (zstd to a ChatGPT
	// backend, provider.CodexCompressed). A try gone over a connection
	// that then failed still counts, as a connection's own counter would,
	// and so does a single ask's second request (RetryOneAuth …). A
	// record from before it was kept has none (#4456).
	UpBytes int `json:"up_bytes,omitempty"`
	// PlainBytes is the request's last try as it went into that ask,
	// before compression, what UpBytes would have been without one: beside
	// it, a compressed turn's discount and a retried request's re-sends
	// both read off the row. None when no try went out.
	PlainBytes int `json:"plain_bytes,omitempty"`
	// Kind is what the agent made the call for when it isn't a turn of
	// the conversation: a Codex subagent's (review, compact, guardian…)
	Kind string `json:"kind,omitempty"`
	// Via is the computer whose magpie passed the call on to this one (a
	// Remote magpie provider there), Agent being the agent's on it; "" when
	// no other magpie forwarded it (including direct LAN/container clients).
	Via string `json:"via,omitempty"`
	// Archive is where the request archive keeps the call, "<date>/<id>"
	// (gateway/archive.go), when it was on: the Usage page reads it back
	// by it long after Recent calls has let the call go (#447)
	Archive string `json:"archive,omitempty"`
	// Computer is the other computer the call was made on, by its id, for
	// a call brought here by sync (#542); "" for one made here, as every
	// call in usage.jsonl is
	Computer string `json:"computer,omitempty"`
	// Source is SourceReported for a call a client told magpie of through
	// POST /v1/magpie/usage/requests (#1874), one that went straight to its
	// vendor and that magpie never saw; "" for the gateway's own. Reported
	// calls count in the totals and are told apart, and filtered, on the
	// page.
	Source string `json:"source,omitempty"`
	// OwnCost is what such a client said the call cost, in USD. nil when it
	// said nothing: the call is then priced at magpie's own prices, as the
	// gateway's calls are, and priced again when those change.
	OwnCost *float64 `json:"cost,omitempty"`
	// Credits is what the vendor said the call cost in its plan's own
	// credits (WorkBuddy's usage.credit, #2512), nil when it said none
	Credits *float64 `json:"credits,omitempty"`
	// Watched is a row of a request an Observer watched (observe.go): it
	// has the request's attempts already, so it can pass the row over.
	// Never persisted.
	Watched bool `json:"-"`
	// Local is true only for verified loopback gateway requests. It is used
	// for local-session deduplication, never persisted or exported.
	Local bool `json:"-"`
	// BodyIn and BodyOut are the request and reply as the gateway
	// captured them, filled only while an Observer wants them (an OTLP
	// export with bodies on, #538) and never written to usage.jsonl.
	BodyIn  string `json:"-"`
	BodyOut string `json:"-"`
	// pricedAs is the provider the call is priced as when it is shown
	// under another: a deleted provider's merged into one on the Usage
	// page, which costs what it did (provider.Renames.Both). Never kept.
	pricedAs string
}

// renameBy has r's provider the one its calls are shown under now
// (Renames.Of), and keeps the one it is priced as beside it when they
// differ.
func (r *Record) renameBy(rn provider.Renames) {
	shown, own := rn.Both(r.Provider, r.Time)
	r.Provider, r.pricedAs = shown, ""
	if own != shown {
		r.pricedAs = own
	}
}

// priceID is the provider r is priced as: its own, not one it is shown
// under after a merge (renameBy).
func (r Record) priceID() string {
	if r.pricedAs != "" {
		return r.pricedAs
	}
	return r.Provider
}

// Path is the log file: ~/.config/magpie/usage.jsonl (XDG-aware).
func Path() string { return filepath.Join(filepath.Dir(provider.Path()), "usage.jsonl") }

var mu sync.Mutex

// openWriteMu serializes the writer's batches and IngestReported's direct
// writes: they share the log's file states (noteLogAppend) and must not
// interleave. It is separate from mu, which Load takes on a read. No request
// goroutine takes it now: Append hands its record to the writer and waits for
// that record alone.
var openWriteMu sync.Mutex

// loads counts Load's reads of the whole log, for the tests: the ledger
// reads the gateway's calls from its index instead.
var loads int

// writesFailed counts every record the writer has not been able to write
// since start: a dropped record is a number the user would quote, not
// "maybe". saidWhen, guarded by openWriteMu, is when a drop was last said,
// so a long failure is spoken of once a minute, and sayEvery how often; a
// success resets saidWhen, so the next drop is said at once.
var (
	writesFailed atomic.Int64
	saidWhen     time.Time
	sayEvery     = time.Minute
)

// AppendsFailed is how many records Append has not written since start, for
// a test and, when it is nonzero, the log.
func AppendsFailed() int64 { return writesFailed.Load() }

// resetAppendsFailed is a test's seam back to nothing dropped.
func resetAppendsFailed() { writesFailed.Store(0) }

// sayFailed says, at most every sayEvery, of the records the writer hasn't
// been able to write: a disk that fills or a folder that goes read-only is
// not silent. The caller holds openWriteMu.
func sayFailed(why error) {
	if time.Since(saidWhen) < sayEvery {
		return
	}
	saidWhen = Clock()
	log.Printf("usage.jsonl: %d record(s) not written since start, latest: %v; calls go on, the ledger keeps what it could open", writesFailed.Load(), why)
}

// ---- appending --------------------------------------------------------------

// Append writes one record. Records go to one background writer, which keeps
// the log open between them and writes the records waiting together, so a
// request's teardown is a queue hand-off rather than an open, two stats and a
// close of usage.jsonl. Append returns once its own record is written or
// counted as dropped, so nothing a later read should see is left in memory.
//
// appendQueueLen records may wait for the writer. When the queue is full — a
// disk that has stopped taking writes — Append waits for room rather than
// dropping the record; the calls go on once it drains. A record that still
// can't be written is counted (writesFailed) and said (sayFailed), as before.
func Append(r Record) {
	if r.Time.IsZero() {
		r.Time = Clock()
	}
	if !r.IsProbe() { // magpie's own test is no call to export
		observeLedger(r)
	}
	defer codexUsedNew(r)
	if !r.IsProbe() && !r.IsReported() && !r.IsRejected() && r.Status == http.StatusOK {
		// F-11's denominator: every answered call, and those of them with no
		// token count at all (the vendor said no usage), for the ledger's
		// coverage to be a number rather than a sample. A local refusal
		// (Rejected: a title, a loop ending) never reached a vendor and is
		// no answered call.
		answered200.Add(1)
		if r.Input+r.Output+r.CacheRead+r.CacheWrite == 0 {
			answered200NoUsage.Add(1)
		}
	}
	b, err := json.Marshal(r)
	if err != nil {
		return
	}
	line := append(b, '\n')
	done := make(chan error, 1)
	startAppendWriter()
	appendQueue <- appendItem{path: Path(), line: line, done: done}
	<-done
}

// Flush waits until every record Append has returned for is on disk. Records
// are written before Append returns, so this is for a process about to exit
// and for tests that read the file directly; a read through Load, the index
// or a summary sees them already.
func Flush() {
	startAppendWriter()
	done := make(chan error, 1)
	appendQueue <- appendItem{done: done}
	<-done
}

// answered200 and answered200NoUsage count the answered calls Append has
// offered the ledger since start, and those of them that recorded no token
// count at all (input, output and cache reads and writes all zero: a 200
// whose vendor said no usage). The second over the first is F-11's "200 but
// no usage" share, said in the log once an hour and read by a test.
var (
	answered200        atomic.Int64
	answered200NoUsage atomic.Int64
)

// Answered200 is how many answered (200) calls Append has offered the ledger
// since start, and how many of them recorded no token usage at all. Reported
// calls and probes are not answered calls of the gateway's own and are left
// out.
func Answered200() (total, withoutUsage int64) {
	return answered200.Load(), answered200NoUsage.Load()
}

// resetAnswered200 is a test's seam back to no calls counted.
func resetAnswered200() {
	answered200.Store(0)
	answered200NoUsage.Store(0)
}

// coverageSaid is when the no-usage share of the answered calls was last
// said, and the numbers then; the writer goroutine owns both.
var coverageSaid time.Time
var coverageTotal, coverageNoUsage int64

// maybeSayCoverage says, at most once an hour, how many of the answered calls
// carried no token usage: F-11's number with the denominator it lacked, in
// the log beside the calls it counts.
func maybeSayCoverage() {
	total, noUsage := Answered200()
	if noUsage == 0 || total == coverageTotal && noUsage == coverageNoUsage || time.Since(coverageSaid) < time.Hour {
		return
	}
	coverageSaid = time.Now()
	coverageTotal, coverageNoUsage = total, noUsage
	log.Printf("usage.jsonl: %d of %d answered call(s) (%.2f%%) recorded no token usage since start", noUsage, total, 100*float64(noUsage)/float64(total))
}

const (
	// appendQueueLen is how many records may wait for the writer before
	// Append waits for room; see Append.
	appendQueueLen = 256
	// appendBatchMax is the most records the writer takes in one write.
	appendBatchMax = 256
)

// appendItem is one record for the writer, or a Flush (line nil) waiting for
// the records before it.
type appendItem struct {
	path string
	line []byte
	done chan error
}

var (
	appendQueue      = make(chan appendItem, appendQueueLen)
	appendWriterOnce sync.Once
)

func startAppendWriter() {
	appendWriterOnce.Do(func() { go appendWriterLoop() })
}

// appendWriterLoop writes what Append hands it, forever: a process that serves
// the gateway appends for its life.
func appendWriterLoop() {
	for {
		writeAppendBatch(takeAppendBatch(<-appendQueue))
		maybeSayCoverage()
	}
}

// takeAppendBatch is first and what already waits for the writer, up to one
// write's worth: records arriving while the last batch was written go out
// together.
func takeAppendBatch(first appendItem) []appendItem {
	batch := []appendItem{first}
	for len(batch) < appendBatchMax {
		select {
		case it := <-appendQueue:
			batch = append(batch, it)
		default:
			return batch
		}
	}
	return batch
}

// writeAppendBatch writes a batch in order: the records of one path in one
// write, and a Flush after everything before it.
func writeAppendBatch(batch []appendItem) {
	for i := 0; i < len(batch); {
		if batch[i].line == nil { // a Flush
			batch[i].done <- nil
			i++
			continue
		}
		j := i + 1
		for j < len(batch) && batch[j].line != nil && batch[j].path == batch[i].path {
			j++
		}
		err := writeAppendLines(batch[i].path, batch[i:j])
		for _, it := range batch[i:j] {
			it.done <- err
		}
		i = j
	}
}

// writeAppendLines writes items' lines to the log at path, under
// openWriteMu, and records the file's states around the write for the index.
func writeAppendLines(path string, items []appendItem) error {
	openWriteMu.Lock()
	defer openWriteMu.Unlock()
	if openFail != nil {
		if err := openFail(); err != nil {
			writesFailed.Add(int64(len(items)))
			sayFailed(err)
			return err
		}
	}
	f, before, err := heldLogFile(path)
	if f == nil {
		writesFailed.Add(int64(len(items)))
		sayFailed(err)
		return err
	}
	var buf []byte
	for _, it := range items {
		buf = append(buf, it.line...)
	}
	n, werr := f.Write(buf)
	if werr != nil {
		var failed int64
		off := 0
		for _, it := range items {
			off += len(it.line)
			if off > n {
				failed++
			}
		}
		writesFailed.Add(failed)
		sayFailed(werr)
		// Seal a line cut short, so the next record starts on a line of its
		// own; best effort, a disk that took nothing takes this too.
		f.Write([]byte{'\n'})
		f.Close()
		heldLog = logHandle{}
		return werr
	}
	saidWhen = time.Time{} // a healed log is quiet: the next drop is said at once
	after, _ := statLogHandle(f)
	noteLogAppend(path, before, after)
	if after != nil && logMaxBytes > 0 && after.Size() >= logMaxBytes && time.Now().After(rotateRetry) {
		rotateLog(path)
	}
	if !holdLogBetweenBatches {
		f.Close()
		heldLog = logHandle{}
	}
	return nil
}

// logHandle is the log the writer keeps open between batches, under
// openWriteMu: an open and a close of usage.jsonl for every request was most
// of Append's time (#3725). It is written to only while Path still names the
// same file, checked as each batch starts, so a log another program rotates,
// removes or replaces is followed before the next record goes anywhere.
// Windows closes it after each batch (holdLogBetweenBatches, log_open_windows.go):
// a file open there cannot be replaced by a rename, and a batch's records
// still share one open instead of one each.
type logHandle struct {
	path string
	f    *os.File
	info os.FileInfo
}

var heldLog logHandle

// openFail, when set, makes the writer fail a batch's open: a test's seam, as
// steady's stat and readFile are, so a write that can't land is read back on
// every platform (a read-only folder's enforcement is not one).
var openFail func() error

// heldLogFile returns the log at path to append to and its state before the
// write, opening or reopening it when the path no longer names the file kept
// from the last batch.
func heldLogFile(path string) (*os.File, os.FileInfo, error) {
	if heldLog.f != nil && heldLog.path == path {
		at, err := statLogFile(path)
		if err == nil && sameLogFile(heldLog.info, at) {
			// the path still names the file kept open, and at is the state
			// the batch writes onto: the boundary state noteLogAppend needs
			return heldLog.f, at, nil
		}
		heldLog.f.Close()
		heldLog = logHandle{}
	}
	// the folder is made only when it is missing: every request appends,
	// and MkdirAll stat'ed it each time (#3725)
	f, err := openAppendLog(path)
	if errors.Is(err, fs.ErrNotExist) && os.MkdirAll(filepath.Dir(path), 0o755) == nil {
		f, err = openAppendLog(path)
	}
	if err != nil {
		return nil, nil, err
	}
	before, err := statLogHandle(f)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	heldLog = logHandle{path: path, f: f, info: before}
	return f, before, nil
}

// closeHeldLog closes the handle the writer keeps, for a test to start from
// none.
func closeHeldLog() {
	openWriteMu.Lock()
	defer openWriteMu.Unlock()
	if heldLog.f != nil {
		heldLog.f.Close()
	}
	heldLog = logHandle{}
}

// ---- rotation ---------------------------------------------------------------

// logMaxBytes is the size at which the writer tries to rotate usage.jsonl.
// This machine's measured rate is 4.7 MB a day (2026-11-10 to 11-18: 37.8 MB,
// 56,514 rows): 256 MiB is about 54 days of history, and because a rotation
// keeps the last logRetainDays in the active log, it triggers about every
// 19 days after the first. A rotation moves only the records older than the
// windows the readers use (logRetainSince), so the active log keeps the
// Usage page's 30 days, Codex's 30-day lead and every gateway key's window,
// and starts again from that retained window rather than from zero. A test
// sets it small.
var logMaxBytes int64 = 256 << 20

// logRetainDays is the least the active log keeps across a rotation: the
// Usage page's longest period and Codex's 30-day lead, with a margin for the
// day boundary and clock skew. A gateway key's window longer than it (up to
// access.MaxDays) makes the retained part longer; when that part alone is
// already over logMaxBytes there is nothing a rotation may move, and it is
// skipped.
const logRetainDays = 35

// renameLog is a test's seam for a rotation that can't rename.
var renameLog = os.Rename

var (
	rotations      atomic.Int64
	rotatedRows    atomic.Int64
	rotateFailures atomic.Int64
	rotateSkips    atomic.Int64
	rotateSaid     time.Time // guarded by openWriteMu
	rotateRetry    time.Time // guarded by openWriteMu
)

// LogRotations is how many times the log has been rotated since start, and
// how many records the rotated files hold; LogRotateFailures is how many
// rotations could not rename or rewrite the log, kept then and nothing
// dropped; LogRotationSkips is how many were skipped because nothing in the
// log was older than the window the readers need (the log then keeps growing
// and says so).
func LogRotations() (files, rows int64) { return rotations.Load(), rotatedRows.Load() }

// LogRotateFailures is how many rotations could not complete. The log is
// then kept: a rotation never drops a record.
func LogRotateFailures() int64 { return rotateFailures.Load() }

// LogRotationSkips is how many rotation attempts found nothing older than
// the retained window.
func LogRotationSkips() int64 { return rotateSkips.Load() }

// resetLogRotations is a test's seam back to no rotation counted.
func resetLogRotations() {
	openWriteMu.Lock()
	defer openWriteMu.Unlock()
	rotations.Store(0)
	rotatedRows.Store(0)
	rotateFailures.Store(0)
	rotateSkips.Store(0)
	rotateSaid, rotateRetry = time.Time{}, time.Time{}
}

// logRetainSince is the first time the active log must keep: the Usage page's
// 30 days (and Codex's 30-day lead) back, or a gateway key's window start
// when that is earlier, since a key's spending is summed from its window's
// start in the log (budget's usage.Visit). It is only read for a rotation, so
// the key file's read is rare.
func logRetainSince(now time.Time) (time.Time, error) {
	since := now.AddDate(0, 0, -logRetainDays)
	keys, err := access.List()
	if err != nil {
		return time.Time{}, err
	}
	for _, k := range keys {
		for _, l := range k.Limits() {
			if l == nil {
				continue
			}
			if start, _ := l.Window(now); start.Before(since) {
				since = start
			}
		}
	}
	return since, nil
}

// rotateLog moves the log at path's records older than the windows the
// readers use into a dated file beside it and keeps the rest in the active
// log, under openWriteMu. If nothing is old enough — a key's window longer
// than the log's history — it is skipped. A rename or rewrite that fails
// leaves the whole log in place and counts the failure; no record is dropped
// by a rotation.
func rotateLog(path string) {
	now := time.Now()
	keepFrom, err := logRetainSince(now)
	if err != nil {
		rotateFailures.Add(1)
		sayRotate(now, "rotation skipped: the gateway keys' windows could not be read (%v); the log is kept", err)
		rotateRetry = now.Add(10 * time.Minute)
		return
	}
	keepTmp, oldTmp, kept, moved, err := splitLog(path, keepFrom)
	if err != nil {
		rotateFailures.Add(1)
		sayRotate(now, "rotation failed, the log is kept: %v", err)
		rotateRetry = now.Add(10 * time.Minute)
		return
	}
	if moved == 0 {
		os.Remove(keepTmp)
		os.Remove(oldTmp)
		rotateSkips.Add(1)
		limit := fmt.Sprintf("%d MiB", logMaxBytes>>20)
		if logMaxBytes < 1<<20 {
			limit = fmt.Sprintf("%d bytes", logMaxBytes)
		}
		sayRotate(now, "the log is over %s but none of its %d record(s) is older than %s, the window the pages and keys read; it is kept and goes on growing", limit, kept, keepFrom.Format("2006-01-02"))
		rotateRetry = now.Add(time.Hour)
		return
	}
	name := rotatedLogName(path, now)
	if heldLog.f != nil { // the path is about to be replaced
		heldLog.f.Close()
		heldLog = logHandle{}
	}
	// The archive first: were the active log replaced before the old lines
	// were safe, a crash would lose them. If the replace then fails, the
	// active log still has every record and the archive is dropped.
	if err := renameLog(oldTmp, name); err != nil {
		os.Remove(keepTmp)
		os.Remove(oldTmp)
		rotateFailures.Add(1)
		sayRotate(now, "rotation failed, the log is kept: %v", err)
		rotateRetry = now.Add(10 * time.Minute)
		return
	}
	if err := renameLog(keepTmp, path); err != nil {
		os.Remove(name)
		os.Remove(keepTmp)
		rotateFailures.Add(1)
		sayRotate(now, "rotation failed and the log was kept whole: %v", err)
		rotateRetry = now.Add(10 * time.Minute)
		return
	}
	rotations.Add(1)
	rotatedRows.Add(moved)
	rotateSaid = time.Time{}
	log.Printf("usage.jsonl: rotated %d record(s) older than %s to %s; %d record(s) kept in the active log", moved, keepFrom.Format("2006-01-02"), name, kept)
	rotateRetry = time.Time{}
}

// sayRotate says a rotation's outcome at most every sayEvery, as sayFailed
// does for a write. The caller holds openWriteMu.
func sayRotate(now time.Time, format string, args ...any) {
	if time.Since(rotateSaid) < sayEvery {
		return
	}
	rotateSaid = now
	log.Printf("usage.jsonl: "+format, args...)
}

// splitLog copies path's lines into two temp files: those whose record time
// is before keepFrom, and the rest (a line whose record time is unknown stays
// with the rest). A last line without its newline is sealed where it lands.
func splitLog(path string, keepFrom time.Time) (keepPath, oldPath string, kept, moved int64, err error) {
	in, err := openLogRead(path)
	if err != nil {
		return "", "", 0, 0, err
	}
	defer in.Close()
	dir := filepath.Dir(path)
	keep, err := os.CreateTemp(dir, "usage-keep-*.tmp")
	if err != nil {
		return "", "", 0, 0, err
	}
	defer func() {
		if err != nil {
			keep.Close()
			os.Remove(keep.Name())
		}
	}()
	old, err := os.CreateTemp(dir, "usage-old-*.tmp")
	if err != nil {
		return "", "", 0, 0, err
	}
	defer func() {
		if err != nil {
			old.Close()
			os.Remove(old.Name())
		}
	}()
	kw := bufio.NewWriterSize(keep, 64<<10)
	ow := bufio.NewWriterSize(old, 64<<10)
	rd := bufio.NewReaderSize(in, 64<<10)
	for {
		line, rerr := rd.ReadBytes('\n')
		if len(line) > 0 {
			if rerr == nil && recordBefore(line, keepFrom) {
				if _, err = ow.Write(line); err != nil {
					return "", "", 0, 0, err
				}
				moved++
			} else {
				if _, err = kw.Write(line); err != nil {
					return "", "", 0, 0, err
				}
				if rerr == io.EOF {
					_, err = kw.WriteString("\n") // seal a line cut short
				}
				if err != nil {
					return "", "", 0, 0, err
				}
				kept++
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			return "", "", 0, 0, rerr
		}
	}
	if err = kw.Flush(); err != nil {
		return "", "", 0, 0, err
	}
	if err = ow.Flush(); err != nil {
		return "", "", 0, 0, err
	}
	if err = keep.Close(); err != nil {
		return "", "", 0, 0, err
	}
	if err = old.Close(); err != nil {
		return "", "", 0, 0, err
	}
	return keep.Name(), old.Name(), kept, moved, nil
}

// recordBefore is whether line's record time is before at; a line that
// doesn't parse, or names no time, is not known to be old and stays.
func recordBefore(line []byte, at time.Time) bool {
	var r struct {
		T time.Time `json:"t"`
	}
	if json.Unmarshal(line, &r) != nil || r.T.IsZero() {
		return false
	}
	return r.T.Before(at)
}

// rotatedLogName is path's dated name beside it, the day and time of the
// rotation, with a count when two rotations land in the same second.
func rotatedLogName(path string, at time.Time) string {
	dir, base := filepath.Split(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	stamp := at.Format("20060102-150405")
	name := filepath.Join(dir, stem+"-"+stamp+ext)
	for n := 2; ; n++ {
		if _, err := os.Stat(name); err != nil {
			return name
		}
		name = filepath.Join(dir, stem+"-"+stamp+"-"+strconv.Itoa(n)+ext)
	}
}

// IsRejected also recognizes local rejections written before the explicit flag.
func (r Record) IsRejected() bool { return r.Rejected || r.Provider == "" && r.Status >= 400 }

// IsReported is a call an app reported itself (#1874), which never went
// through magpie. It is shown and counted on the Usage page only: nothing
// that acts on live traffic (a key's budget, a window's holds, the Sessions
// page's gateway sessions, what a session was routed to, Codex's sealed
// answers or remembered model) reads it, since it may be a backfill of
// calls long past and none of them were magpie's to route.
func (r Record) IsReported() bool { return r.Source == SourceReported }

// Load streams complete records from the log. Historical request bodies are
// not retained by a global cache after the caller finishes with this snapshot.
func Load(since time.Time) []Record {
	mu.Lock()
	loads++
	f, err := openLogRead(Path())
	mu.Unlock()
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []Record
	reader := bufio.NewReaderSize(f, 64<<10)
	for {
		b, err := reader.ReadBytes('\n')
		if err != nil {
			break
		} // an unfinished line is left for the next read
		var r Record
		if json.Unmarshal(b, &r) == nil && !r.IsProbe() && (since.IsZero() || !r.Time.Before(since)) {
			out = append(out, r)
		}
	}
	return out
}

// Known is an agent as its requests name it.
type Known struct {
	ID    string
	Names []string // its id and aliases, which a record may already carry
	UA    []string // what its User-Agent begins with
}

// Agents lists the agents magpie knows. Package agent sets it from each
// agent's own description, so they are listed in one place only.
var Agents func() []Known

// PluginAgents lists the agents plugins add, which come and go as
// plugins do; package agent sets it.
var PluginAgents func() []Known

var builtinAgents = sync.OnceValue(func() []Known {
	if Agents == nil {
		return nil
	}
	return Agents()
})

// knownAgents are magpie's own agents, then those plugins add.
func knownAgents() []Known {
	own := builtinAgents()
	if PluginAgents == nil {
		return own
	}
	more := PluginAgents()
	if len(more) == 0 {
		return own
	}
	return append(slices.Clip(own), more...)
}

// IsAgent says whether id is an agent magpie knows, its own or a plugin's,
// rather than the product name of some other client (Python-urllib).
func IsAgent(id string) bool {
	return slices.ContainsFunc(knownAgents(), func(k Known) bool { return k.ID == id || slices.Contains(k.Names, id) })
}

// libraries are the product names of HTTP libraries, runtimes and API
// SDKs as AgentOf names a client by its User-Agent, lower-cased. A
// request that says only one of these is from a script, or from a tool
// built on the library, and says nothing of which: it is no agent of its
// own (theforeveriris on X: Bun, curl and python-httpx each had an
// agent's scorecard on the Context tab).
var libraries = map[string]bool{
	// command lines and API tools
	"curl": true, "wget": true, "httpie": true, "postmanruntime": true, "insomnia": true,
	// JavaScript
	"bun": true, "node": true, "deno": true, "node-fetch": true, "undici": true, "axios": true, "got": true, "ky": true,
	// Python
	"python": true, "python-httpx": true, "httpx": true, "python-requests": true, "requests": true,
	"python-urllib": true, "urllib3": true, "aiohttp": true,
	// Go, JVM, Ruby, PHP, Dart, Rust
	"go-http-client": true, "go-resty": true, "resty": true, "okhttp": true, "java": true, "java-http-client": true,
	"apache-httpclient": true, "reactor-netty": true, "ruby": true, "faraday": true, "rest-client": true,
	"guzzlehttp": true, "dart": true, "reqwest": true, "libcurl": true,
	// the vendors' SDKs, which an app or a script calls a model with
	"openai": true, "asyncopenai": true, "anthropic": true, "asyncanthropic": true, "ai-sdk": true, "google-genai-sdk": true,
}

// IsLibrary says whether a client, by the name AgentOf gives it (a
// record's Agent, old records' too), is an HTTP library, a runtime or an
// API SDK rather than an agent: curl, Bun, python-httpx, Go-http-client,
// OpenAI/Python.
func IsLibrary(id string) bool { return libraries[strings.ToLower(strings.TrimSpace(id))] }

// AgentOf names the agent behind a client User-Agent. Known agents map to
// their magpie id; anything else keeps its product name.
func AgentOf(ua string) string {
	ua = strings.TrimSpace(ua)
	name, _, _ := strings.Cut(ua, "/")
	name, _, _ = strings.Cut(name, " ")
	l := strings.ToLower(name)
	// a name first: cursor-local is its own, though Cursor's UA (cursor)
	// begins it
	for _, k := range knownAgents() {
		if slices.Contains(k.Names, l) {
			return k.ID
		}
	}
	for _, k := range knownAgents() {
		if slices.ContainsFunc(k.UA, func(p string) bool { return strings.HasPrefix(l, p) }) {
			return k.ID
		}
	}
	if name == "" {
		return "other"
	}
	return name
}

// ---- summaries --------------------------------------------------------------

// Period is a window of time to sum over.
type Period string

const (
	Today Period = "today"
	Week  Period = "7d"
	Month Period = "30d"
	All   Period = "all"
)

// Periods lists the windows in order.
var Periods = []Period{Today, Week, Month, All}

// CostAt is what the call costs at a price: at the tier its input reaches,
// its 1-hour cache writes at theirs, and at the hour it was made (an
// off-peak price doubles at peak, Price.In). A reported call that said
// what it cost costs that, at any price.
func (r Record) CostAt(p catalog.Price) float64 {
	if r.OwnCost != nil {
		return *r.OwnCost // a reported call's own cost (#1874)
	}
	return p.In(r.Time).CostSplit(r.Input, r.Output, r.CacheRead, r.CacheWrite, r.CacheWrite1h)
}

// Totals is a sum of calls.
type Totals struct {
	// Calls are the requests, Errors those the agent got an error for, and
	// Retried those answered after a try that failed first (Record.Tries):
	// a try handed on is in neither count, but its tokens are in the sums
	Calls   int `json:"calls"`
	Errors  int `json:"errors"`
	Retried int `json:"retried,omitempty"`
	// Loops are those of Errors magpie ended for looping (LoopErrType):
	// a model's count of them says how often it got stuck (#1858)
	Loops      int `json:"loops,omitempty"`
	Input      int `json:"input"`
	Output     int `json:"output"`
	CacheRead  int `json:"cache_read"`
	CacheWrite int `json:"cache_write"`
	// CacheWrite1h is how many of CacheWrite were 1-hour writes
	CacheWrite1h int `json:"cache_write_1h,omitempty"`
	// CacheUnknown is how many of the prompt's tokens (Input, CacheRead
	// and CacheWrite) were of calls whose vendor counted no cache
	// (Record.CacheUnknown), CacheUnknownCalls how many calls they were:
	// a hit rate is CacheRead over the prompt less CacheUnknown, and none
	// when nothing is left (#3464)
	CacheUnknown      int     `json:"cache_unknown,omitempty"`
	CacheUnknownCalls int     `json:"cache_unknown_calls,omitempty"`
	Reasoning         int     `json:"reasoning"`
	Cost              float64 `json:"cost"`     // USD at the effective price, for the priced calls
	Unpriced          int     `json:"unpriced"` // calls with tokens but no known price
	// Credits are what the vendors said Credited of the calls cost in
	// their plans' own credits (Record.Credits): the rest said nothing
	Credits  float64 `json:"credits,omitempty"`
	Credited int     `json:"credited,omitempty"`
	// Timed: the answered calls whose first token was timed (streamed),
	// TTFT the sum of their ttft_ms; DecodeMs the time from it to the end
	// of those that wrote any, over which DecodeOut tokens came (a
	// reasoning reply's answer, Record.Decode): their mean wait, and how
	// fast they wrote (#196)
	Timed     int   `json:"timed,omitempty"`
	TTFT      int64 `json:"ttft_ms,omitempty"`
	DecodeMs  int64 `json:"decode_ms,omitempty"`
	DecodeOut int   `json:"decode_out,omitempty"`
	// DecodeN is how many replies DecodeMs and DecodeOut are of: the ones
	// whose speed was measured, which a speed is shown with, so a group of
	// 260 calls whose speed is one reply's says so (Fallmonkey on Discord)
	DecodeN int `json:"decode_n,omitempty"`
	// FloorOut tokens over FloorMs are the least FloorN replies read from a
	// session file wrote at, those whose speed wasn't measured (Row.Floor):
	// what a group shows, as "≥", beside how many it is of, after its
	// measured speed when it has one. They are never in Speed.
	FloorOut int   `json:"floor_out,omitempty"`
	FloorMs  int64 `json:"floor_ms,omitempty"`
	FloorN   int   `json:"floor_n,omitempty"`
}

// Tokens is what went in and out, excluding cache traffic.
func (t Totals) Tokens() int { return t.Input + t.Output }

// MeanTTFT is the mean ms to the first token of the timed calls, 0 for none.
func (t Totals) MeanTTFT() int64 {
	if t.Timed == 0 {
		return 0
	}
	return t.TTFT / int64(t.Timed)
}

// Speed is how fast the timed calls wrote, in tokens a second after their
// first: 0 for none.
func (t Totals) Speed() float64 {
	if t.DecodeMs <= 0 {
		return 0
	}
	return float64(t.DecodeOut) / (float64(t.DecodeMs) / 1000)
}

// Floor is the least the replies whose speed wasn't measured wrote at, in
// tokens a second (Row.Floor): 0 for none.
func (t Totals) Floor() float64 {
	if t.FloorMs <= 0 {
		return 0
	}
	return float64(t.FloorOut) / (float64(t.FloorMs) / 1000)
}

// A reply's speed is its output tokens over its decode window, the ms
// from its first content to its end (#196; one that reasoned, DecodeOf); a window counts only when it
// timed the writing (#731). One that is shorter than MinDecodeMs, or over
// which the tokens would have come faster than MaxDecodeSpeed a second,
// didn't: the reply came in one burst at its end, written before the
// window opened — a turn that is one big tool call from Gemini, whose
// functionCall comes whole, has its first content a millisecond before
// its end, and 8264 tokens in 1 ms read 8,264,000 tok/s. Such a reply
// tells no speed and is left out of the summed one, where its tokens
// over next to no time would lift everyone's; its TTFT still counts.
// The bounds are wide of any real stream: the fastest vendors write a few
// thousand tokens a second, and a tenth of a second is the least a
// stream's timing tells anything by. routing/client.js's speedOf has the same.
const (
	MinDecodeMs    = 100
	MaxDecodeSpeed = 10000
)

// DecodeWindow is the ms a reply of out tokens, ms long with its first
// content at ttft, took to write them: 0 when it wasn't timed, wrote
// nothing, or came too fast to tell a speed by (above).
func DecodeWindow(out int, ms, ttft int64) int64 {
	w := ms - ttft
	if out <= 0 || ttft <= 0 || w < MinDecodeMs || int64(out)*1000 > MaxDecodeSpeed*w {
		return 0
	}
	return w
}

// DecodeOf is what a reply's speed is told by: the tokens it was seen to
// write and the ms it took (tony on Discord: gpt-6.1-sol read 163 tok/s,
// with a 14 s first token). A reply that reasoned counts only its answer —
// its output less the reasoning, from its first text — as most vendors
// keep the reasoning out of the stream (OpenAI's is encrypted, its summary
// sent when it's done; Claude's may come with no text): its tokens were
// written before the first content, over time the window from it doesn't
// hold, and counting them read a 2,000-token think and a 200-token answer
// in 1.5 s as 1,467 tok/s. Its time goes to the wait before the answer.
// With no answer text, paced visible reasoning can time all output from
// firstReasoning. Missing evidence (including older records), hidden reasoning
// and an unknown flow leave the speed untold. The
// window is DecodeWindow's: 0 for a reply that tells none.
//
// A reply its vendor held back and let go in a burst at its end tells
// none either (John on Discord: Kimi Code answering OpenCode read 1,367,
// then 2,237 and 2,012 tok/s). The burst is told two ways, as the reply's
// text came before it or with it:
//
//   - flow, when known (Record.Flow), is how long its content took to come
//     from where its window starts. A window HeldShare times longer had
//     its content come in a small part of it: the rest was a wait while
//     the vendor wrote and held it. The time it took to come is not the
//     time it took to write (a burst let go over 236 ms read 528 tokens at
//     2,237 tok/s), and that time is not seen, so it tells no speed. A
//     stream that came as it was written has a flow about its window.
//   - a reply that reasoned is timed from its first text, its reasoning
//     written before. When the answer would have been written over
//     ThinkPace times faster than its reasoning came before it, from the
//     first content to the first text, its first text came with the burst
//     and the wait before it was the answer being written and held (22
//     reasoning tokens over 10.5 s, then 528 answer tokens in 236 ms). A
//     reasoning hidden from the stream (OpenAI's, Claude's omitted) comes
//     before the first content and leaves this time short; one that
//     streams comes at the pace its answer does.
func DecodeOf(out, reasoning int, ms, ttft, firstText, flow int64, firstReasoning ...int64) (tokens int, w int64) {
	start := ttft
	if reasoning > 0 {
		if firstText > 0 {
			out, start = out-reasoning, firstText
		} else if len(firstReasoning) > 0 && firstReasoning[0] >= ttft && flow > 0 && reasoning <= out {
			start = firstReasoning[0]
		} else {
			return 0, 0
		}
	}
	if ttft <= 0 {
		return 0, 0
	}
	if w = DecodeWindow(out, ms, start); w == 0 {
		return 0, 0
	}
	if flow > 0 && flow*HeldShare < w {
		return 0, 0
	}
	if reasoning > 0 && firstText > 0 && int64(out)*(firstText-ttft) > ThinkPace*int64(reasoning)*w {
		return 0, 0
	}
	return out, w
}

// HeldShare and ThinkPace tell a reply held back and let go in a burst
// (DecodeOf): its content came in under a quarter of its window, or its
// answer at over 20 times the pace its reasoning came. routing/client.js's
// decodeOf and app.js's ledDecode have the same.
const (
	HeldShare = 4
	ThinkPace = 20
)

// Decode is the record's DecodeOf.
func (r Record) Decode() (tokens int, w int64) {
	if r.ReasoningHidden && r.FirstText <= 0 {
		return 0, 0
	}
	return DecodeOf(r.Output, r.Reasoning, r.Millis, r.TTFT, r.FirstText, r.Flow, r.FirstReasoning)
}

// FormatCredits is what t's credited calls cost in their plans' credits,
// as their replies said (#2512): "" when none said.
func FormatCredits(t Totals) string {
	switch {
	case t.Credited == 0:
		return ""
	case t.Credits > 0 && t.Credits < 0.01:
		return "<0.01 credits"
	}
	return strconv.FormatFloat(math.Round(t.Credits*100)/100, 'f', -1, 64) + " credits"
}

// FormatCost renders an effective-price cost, kept in USD everywhere it's
// stored, as the CLI and TUI show it: at amountUSD's own price when
// currency isn't "cny", else converted at rate (CNY per one USD, from
// internal/fx; a rate of 0 or below also falls back to USD, a stale or
// missing rate being no reason to hide the number). Whole dollars or yuan
// above 100, cents above 1, else thousandths, so a fraction of a cent
// still shows as something.
func FormatCost(amountUSD float64, currency string, rate float64) string {
	amount, sign := amountUSD, "$"
	if currency == "cny" && rate > 0 {
		amount, sign = amountUSD*rate, "¥"
	}
	switch {
	case amount >= 100:
		return fmt.Sprintf("%s%.0f", sign, amount)
	case amount >= 1:
		return fmt.Sprintf("%s%.2f", sign, amount)
	default:
		return fmt.Sprintf("%s%.3f", sign, amount)
	}
}

// countCache counts r's prompt as one whose cache wasn't counted when its
// vendor said so, or never counts it (a record kept before it said so).
func (t *Totals) countCache(r Record) {
	if r.cacheNotCounted() {
		t.CacheUnknown += r.cacheUnknownIn()
		t.CacheUnknownCalls++
	}
}

// CountsNoCache is whether a provider's vendor never counts cache tokens,
// so a call of it with none recorded was not a miss: Kiro caches but tells
// only credits. Calls kept before its plugin said so (opencode-kiro-auth
// 0.1.16's magpie_cache_unknown) carry no CacheUnknown, and are read as
// having it (#3464). The provider is the built-in's id, or the plugin's
// own when it is signed in under that.
func CountsNoCache(provider string) bool {
	return provider == "kiro" || provider == "kiro-plugin"
}

// cacheNotCounted is whether r's vendor counted no cache for it: it said
// so, or it never counts any and r has none (kept before it said so).
func (r Record) cacheNotCounted() bool {
	return r.CacheUnknown || r.CacheRead+r.CacheWrite == 0 && CountsNoCache(r.Provider)
}

// cacheUnknownIn is r's prompt when its vendor counted no cache, else 0.
func (r Record) cacheUnknownIn() int {
	if !r.cacheNotCounted() {
		return 0
	}
	return r.Input + r.CacheRead + r.CacheWrite
}

// HitRate is the share of the prompt read from the cache, of the calls
// whose vendor counted it; false when none did (#3464).
func (t Totals) HitRate() (float64, bool) {
	p := t.Input + t.CacheRead + t.CacheWrite - t.CacheUnknown
	if p <= 0 {
		return 0, false
	}
	return float64(t.CacheRead) / float64(p), true
}

func (t *Totals) add(r Record, price *catalog.Price) { t.addFrom(Row{Record: r}, price) }

// addFrom is add for a ledger row, whose speed is told as Row.Decode tells
// it (a Codex turn of one call read from its session file, by its own rule).
func (t *Totals) addFrom(row Row, price *catalog.Price) {
	r := row.Record
	t.count(r)
	t.Input += r.Input
	t.Output += r.Output
	t.CacheRead += r.CacheRead
	t.CacheWrite += r.CacheWrite
	t.CacheWrite1h += r.CacheWrite1h
	t.countCache(r)
	t.Reasoning += r.Reasoning
	if r.TTFT > 0 && !r.Failed() {
		t.Timed++
		t.TTFT += r.TTFT
		if n, w := row.Decode(); w > 0 {
			t.DecodeMs += w
			t.DecodeOut += n
			t.DecodeN++
		}
	}
	if r.Input+r.Output == 0 {
		return
	}
	if price == nil {
		t.Unpriced++
		return
	}
	t.Cost += r.CostAt(*price)
}

// LoopErrType is a record's ErrType for a reply magpie ended because the
// model was stuck in a loop (#1359), told apart from what vendors call
// their errors.
const LoopErrType = "reply_loop"

// count counts r as the request it ends: failed, answered on a retry, or
// answered; a try handed on to another is no request of its own.
func (t *Totals) count(r Record) {
	// a try handed on is billed its credits too, though it is no request
	if r.Credits != nil {
		t.Credits += *r.Credits
		t.Credited++
	}
	if r.HandedOn {
		return
	}
	t.Calls++
	switch {
	case r.Failed():
		t.Errors++
		if r.ErrType == LoopErrType {
			t.Loops++
		}
	case r.Tries > 1:
		t.Retried++
	}
}

// LostRequest is whether r ends a request the agent got an error for: a
// failed row that no later try took over from.
func (r Record) LostRequest() bool { return r.Failed() && !r.HandedOn }

// RetriedRequest is whether r ends a request that was answered after a try
// that failed first.
func (r Record) RetriedRequest() bool { return !r.HandedOn && !r.Failed() && r.Tries > 1 }

// Group is the share of one agent or model.
type Group struct {
	ID       string `json:"id"`
	Provider string `json:"provider,omitempty"` // models only
	Model    string `json:"model,omitempty"`
	// ModelName is what the user calls this model at this provider
	// (provider.ModelName), beside the id the counts are keyed on (Model):
	// the Usage page shows it in the id's place, and a clear of the model's
	// usage still goes by the id. "" when the model goes by its id.
	ModelName       string `json:"modelName,omitempty"`
	ProviderKeyID   string `json:"providerKeyId,omitempty"`
	ProviderKeyName string `json:"providerKeyName,omitempty"`
	// Account is the subscription account of an Accounts group: "" for
	// the calls of an account's provider whose record names none
	Account       string `json:"account,omitempty"`
	CallerKeyID   string `json:"callerKeyId,omitempty"`
	CallerKeyName string `json:"callerKeyName,omitempty"`
	// Host is where the calls went, when the provider's id has gone to
	// more than one place, or elsewhere than the provider goes now: its
	// calls are then told apart by it, not summed under the id.
	Host string `json:"host,omitempty"`
	// Agent is the agent a session is of (sessions only).
	Agent string `json:"agent,omitempty"`
	Totals
	// Own is the share of these calls magpie made by itself (Record.ByMagpie):
	// a routing classifier's, a web search's or an image description's,
	// counted in Totals too, since they went to the same key or account
	// (#1839). Filled for models, keys and accounts; a value, so a
	// summary's copies share nothing.
	Own OwnShare `json:"own,omitzero"`
}

// OwnShare is what magpie's own calls of a group came to.
type OwnShare struct {
	Calls  int     `json:"calls"`
	Tokens int     `json:"tokens"` // in and out, cache aside, as Totals.Tokens
	Cost   float64 `json:"cost"`
}

// addOwn counts r in g's own share when magpie made it by itself.
func (g *Group) addOwn(r Record, price *catalog.Price) {
	if !r.ByMagpie() {
		return
	}
	if !r.HandedOn {
		g.Own.Calls++
	}
	g.Own.Tokens += r.Input + r.Output
	if price != nil && r.Input+r.Output > 0 {
		g.Own.Cost += r.CostAt(*price)
	}
}

// ByMagpie is whether magpie made the call by itself rather than an agent:
// its routing classifier, System One's decision, a web search or an
// image description it asked for (#1839).
func (r Record) ByMagpie() bool {
	id := AgentOf(r.Agent)
	// "magpie" as the agents know it; its own User-Agents (magpie-router/1,
	// magpie-search/1) where they aren't loaded
	return id == "magpie" || strings.HasPrefix(id, "magpie-")
}

// who is where a subscription's calls went as the account signed in
// ("dee@example.com" of "api.factory.ai as dee@example.com"), the host
// alone for the rest. The account is what tells one place from another: a
// built-in moved onto its plugin sends the same account's calls through
// plugin://…, which isn't another place.
func who(where string) string {
	if i := strings.LastIndex(where, " as "); i >= 0 {
		return where[i+4:]
	}
	return where
}

// Account is the subscription account that answered the call: the one the
// record names, or, in a record written before it named one, the account
// its Host said the call went out as then ("chatgpt.com as dee@example.com")
// — what the record itself says, never today's sign-in. "" when neither
// tells (#557).
func (r Record) Account() string {
	if r.ProviderAccount != "" {
		return r.ProviderAccount
	}
	if i := strings.LastIndex(r.Host, " as "); i >= 0 {
		return r.Host[i+4:]
	}
	return ""
}

// Point is one bar of the timeline.
type Point struct {
	Label string    `json:"label"`
	Time  time.Time `json:"time"`
	Totals
}

// Summary is a period, summed up.
type Summary struct {
	Period Period    `json:"period"`
	Since  time.Time `json:"since"`
	Totals
	Bucket       string  `json:"bucket"` // hour | day | week
	Series       []Point `json:"series"`
	Agents       []Group `json:"agents"`
	Models       []Group `json:"models"`
	ProviderKeys []Group `json:"providerKeys"`
	// Accounts are the subscription accounts' calls, by provider and the
	// account that answered (#557)
	Accounts   []Group `json:"accounts"`
	CallerKeys []Group `json:"callerKeys"`
	// Sessions are the calls that named their session, by session, the
	// most tokens first: all of them, or the top few of SessionCount where
	// it was asked for that way (SummarizeTop).
	Sessions     []Group `json:"sessions"`
	SessionCount int     `json:"sessionCount"`
}

// Summarize caches the four periods over an indexed log snapshot. Callers
// receive their own result slices, without retaining historical Records.
func Summarize(p Period) Summary {
	return indexedSummary(p, Clock())
}

func summarize(p Period, now time.Time, recs []Record) Summary {
	var first time.Time
	keys := map[string]bool{}
	for _, r := range recs {
		if !r.IsRejected() && (first.IsZero() || r.Time.Before(first)) {
			first = r.Time
		}
		if r.ProviderKeyID != "" {
			keys[r.Provider] = true
		}
	}
	return summarizeFrom(p, now, first, keys, func(fn func(Record)) {
		for _, r := range recs {
			fn(r)
		}
	})
}

func summarizeFrom(p Period, now time.Time, first time.Time, historicalKeys map[string]bool, read func(func(Record))) Summary {
	u := newSummaryRun(p, now, first, historicalKeys)
	hosts := newSummaryHosts()
	u.read(read, func(r Record) { hosts.note(r, u.since) })
	keyed := u.keying(hosts)
	counts := u.newCounts()
	u.read(read, func(r Record) { counts.add(u, &keyed, r) })
	return counts.summary(u)
}

// ---- last seen --------------------------------------------------------------

// seen is when each agent's latest request reached the gateway in this
// process — at its start, where a record is written only once it is answered.
var seen sync.Map // agent id → time.Time

// Via is where the gateway sent a session's calls: one model at one
// reasoning effort, how often and how much.
type Via struct {
	Provider string    `json:"provider"`
	Model    string    `json:"model"`
	Effort   string    `json:"effort,omitempty"`
	Calls    int       `json:"calls"`
	Tokens   int       `json:"tokens"` // in and out, as Totals counts them
	Last     time.Time `json:"last"`
	// Credits are what Credited of the calls cost in the plan's own
	// credits, as their replies said (Record.Credits)
	Credits  float64 `json:"credits,omitempty"`
	Credited int     `json:"credited,omitempty"`
}

// Vias is what the gateway sent each session's calls to since a time, by
// agent id and the session's id ("codex|<id>"), the most calls first.
func Vias(since time.Time) ViaIndex {
	return cachedVias(since)
}

// LogVersion is a number for the log as it stands, which changes when a
// call is written to it, and whether there is one: a synthetic log
// (LogCalls), or one not read to its end, has none; no log yet is one. The Sessions page
// keys an unchanged list on it.
func LogVersion() (uint64, bool) {
	if LogCalls != nil {
		return 0, false
	}
	s := readLogSnapshot()
	return s.version, s.info == nil || s.complete()
}

// A provider renamed since is listed by the id it has now (renames): the
// Sessions tab named a renamed one by its old id (cybercaius).
func buildVias(snapshot *logSnapshot, since time.Time, renames provider.Renames) *viaLists {
	counted := map[string]viaAcc{}
	countVias(snapshot.blocks, since, renames, nil, counted, keptStrings{})
	return listedVias(&viaLists{m: map[string][]Via{}, first: map[string]time.Time{}}, counted)
}

// viaAcc is one session's lists as counted, in the order its calls first
// named each, and when its first call was.
type viaAcc struct {
	vs    []Via
	first time.Time
}

// countVias counts blocks' calls into own, a session's counts begun from
// a copy of base's: base is never written.
func countVias(blocks []*rowChunk, since time.Time, renames provider.Renames, base, own map[string]viaAcc, keep keptStrings) {
	visitBlocks(blocks, since, func(r Record) {
		if r.IsRejected() || r.IsReported() || r.Session == "" || r.Model == "" {
			return
		}
		r.Provider, r.Model, r.Effort = keep.of(renames.Of(r.Provider, r.Time)), keep.of(r.Model), keep.of(r.Effort)
		k := ReportedAs(r.Agent) + "|" + r.Session
		a, seen := own[k]
		if !seen {
			if b, ok := base[k]; ok {
				a, seen = viaAcc{vs: slices.Clone(b.vs), first: b.first}, true
			}
		}
		if !seen || r.Time.Before(a.first) {
			a.first = r.Time
		}
		vs := a.vs
		i := slices.IndexFunc(vs, func(v Via) bool { return v.Provider == r.Provider && v.Model == r.Model && v.Effort == r.Effort })
		if i < 0 {
			vs, i = append(vs, Via{Provider: r.Provider, Model: r.Model, Effort: r.Effort}), len(vs)
		}
		vs[i].Calls++
		vs[i].Tokens += r.Input + r.Output
		if r.Credits != nil {
			vs[i].Credits += *r.Credits
			vs[i].Credited++
		}
		if r.Time.After(vs[i].Last) {
			vs[i].Last = r.Time
		}
		a.vs = vs
		own[k] = a
	})
}

// listedVias is lists with each session counted in counted listed in
// place of its own, the most calls first; lists itself is not written.
func listedVias(lists *viaLists, counted map[string]viaAcc) *viaLists {
	if len(counted) == 0 {
		return lists
	}
	out := &viaLists{m: maps.Clone(lists.m), first: maps.Clone(lists.first)}
	for k, a := range counted {
		vs := slices.Clone(a.vs)
		sort.SliceStable(vs, func(i, j int) bool { return vs[i].Calls > vs[j].Calls })
		out.m[k], out.first[k] = vs, a.first
	}
	return out
}

// Saw notes a request from an agent arriving now.
func Saw(agent string) { seen.Store(agent, time.Now()) }

// LastSeen is when a request from the agent last reached the gateway: this
// process's own, else the newest in the log's last stretch. Zero if none.
func LastSeen(agent string) time.Time {
	var last time.Time
	if t, ok := seen.Load(agent); ok {
		last = t.(time.Time)
	}
	// Read the tail without the write lock: a slow write holds openWriteMu,
	// not the reads below, so a request's ledger scan is not queued behind it.
	// A tail boundary may land mid-line; the partial first line fails to
	// unmarshal and is skipped, and that record is still read from the kept
	// seen note or on the next call.
	f, err := openLogRead(Path())
	if err != nil {
		return last
	}
	defer f.Close()
	const tail = 256 << 10
	if st, err := f.Stat(); err == nil && st.Size() > tail {
		f.Seek(st.Size()-tail, 0)
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var r Record
		if json.Unmarshal(sc.Bytes(), &r) == nil && r.Agent == agent && r.Via == "" && !r.IsProbe() && r.Time.After(last) {
			last = r.Time
		}
	}
	return last
}
