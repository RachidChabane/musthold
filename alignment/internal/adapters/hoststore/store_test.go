package hoststore

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/alignment/internal/app/ports"
	"github.com/RachidChabane/spec-proof/alignment/internal/platform/atomicfile"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	concurrentStores   = 2
	updatesPerStore    = 15
	hexSecretLength    = SecretBytes * 2
	stalePartialRecord = `{"ticket":`
)

func openStore(t testing.TB) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "host"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func mustOpen(t testing.TB, dir string) *Store {
	t.Helper()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func freshEntry(vote int) hostproto.Entry {
	request := hostprototest.Request()
	return hostproto.NewEntry(hostprototest.Ticket(request, vote, hostproto.FirstEmission), request.Role,
		hostprototest.Digest("schema"), hostproto.DefaultMaxEmissions)
}

func TestStoreContract(t *testing.T) {
	t.Run("the filesystem store satisfies the shared Store contract [verifies SP-341~1]", func(t *testing.T) {
		hostprototest.RunStoreContract(t, func(t *testing.T) hostproto.Store { return openStore(t) })
	})

	t.Run("the filesystem store satisfies the discard contract [verifies SP-342~1]", func(t *testing.T) {
		hostprototest.RunDiscardContract(t, func(t *testing.T) hostprototest.DiscardableStore { return openStore(t) })
	})
}

func TestLayout(t *testing.T) {
	t.Run("the directories are private and hold the documented files [verifies SP-340~1]", func(t *testing.T) {
		store := openStore(t)
		request := hostprototest.Request()
		id, err := store.PutRequest(request)
		if err != nil {
			t.Fatal(err)
		}
		entry := freshEntry(0)
		mustNoError(t, store.PutEntry(entry))
		mustNoError(t, store.PutResponse(hostprototest.Response(entry.Ticket)))
		_, err = store.Append(hostproto.Event{Kind: hostproto.EventEmitted})
		mustNoError(t, err)
		_, err = store.Secret()
		mustNoError(t, err)
		_, err = store.AdvanceWave()
		mustNoError(t, err)

		for _, dir := range []string{store.Dir(), store.path(RequestsDir), store.path(TicketsDir), store.path(ResponsesDir)} {
			assertMode(t, dir, DirMode)
		}
		for _, file := range []string{
			QueueFile, JournalFile, SecretFile,
			filepath.Join(RequestsDir, string(id)+".json"),
			filepath.Join(TicketsDir, string(id)+".0.json"),
			filepath.Join(ResponsesDir, string(id)+".0.json"),
		} {
			assertMode(t, store.path(file), FileMode)
		}
	})

	t.Run("the secret is 32 random bytes in hex, readable by the user only [verifies SP-340~1]", func(t *testing.T) {
		store := openStore(t)
		secret, err := store.Secret()
		mustNoError(t, err)
		if len(secret) != hexSecretLength {
			t.Fatalf("secret has %d characters", len(secret))
		}
		assertMode(t, store.path(SecretFile), FileMode)
		other := openStore(t)
		otherSecret, _ := other.Secret()
		if string(otherSecret) == string(secret) {
			t.Fatal("two installs share a secret")
		}
	})
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s has mode %v, want %v", path, got, want)
	}
}

func mustNoError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestReopen(t *testing.T) {
	t.Run("a reopened store returns everything it held [verifies SP-342~1]", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "host")
		first := mustOpen(t, dir)
		request := hostprototest.Request()
		id, err := first.PutRequest(request)
		mustNoError(t, err)
		entry := freshEntry(1)
		mustNoError(t, first.PutEntry(entry))
		response := hostprototest.Response(entry.Ticket)
		mustNoError(t, first.PutResponse(response))
		_, err = first.Append(hostproto.Event{Kind: hostproto.EventEmitted, Ticket: "a"})
		mustNoError(t, err)
		_, err = first.Append(hostproto.Event{Kind: hostproto.EventFetched, Ticket: "a"})
		mustNoError(t, err)
		secret, err := first.Secret()
		mustNoError(t, err)

		second := mustOpen(t, dir)
		if _, err := second.GetRequest(id); err != nil {
			t.Fatalf("GetRequest: %v", err)
		}
		if got, err := second.GetEntry(entry.Ticket.Key()); err != nil || got != entry {
			t.Fatalf("GetEntry = %+v, %v", got, err)
		}
		if got, err := second.GetResponse(response.Key()); err != nil || got.Nonce != response.Nonce {
			t.Fatalf("GetResponse = %+v, %v", got, err)
		}
		records, err := second.Records()
		mustNoError(t, err)
		if len(records) != 2 || hostproto.VerifyChain(records) != nil {
			t.Fatalf("records = %+v", records)
		}
		if again, _ := second.Secret(); string(again) != string(secret) {
			t.Fatal("the secret changed")
		}
		appended, err := second.Append(hostproto.Event{Kind: hostproto.EventConsumed})
		mustNoError(t, err)
		if appended.Sequence != 2 || appended.Prev != records[1].Digest {
			t.Fatalf("appended = %+v", appended)
		}
	})

	t.Run("a partially written temporary file is never visible as a record [verifies SP-342~1]", func(t *testing.T) {
		store := openStore(t)
		entry := freshEntry(0)
		mustNoError(t, store.PutEntry(entry))
		stray := filepath.Join(store.path(TicketsDir), atomicfile.TempPrefix+"123.json")
		mustNoError(t, os.WriteFile(stray, []byte(stalePartialRecord), FileMode))
		listed, err := store.ListEntries()
		if err != nil || len(listed) != 1 {
			t.Fatalf("ListEntries = %v, %v", listed, err)
		}
	})

	t.Run("a write leaves no temporary file behind [verifies SP-342~1]", func(t *testing.T) {
		store := openStore(t)
		mustNoError(t, store.PutEntry(freshEntry(0)))
		_, err := store.UpdateEntry(freshEntry(0).Ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) { return e, nil })
		mustNoError(t, err)
		names, _ := os.ReadDir(store.path(TicketsDir))
		for _, name := range names {
			if atomicfile.IsTemp(name.Name()) {
				t.Fatalf("temporary file %s remains", name.Name())
			}
		}
	})
}

func TestConcurrency(t *testing.T) {
	t.Run("two stores on one directory serialize updates of an entry [verifies SP-341~1]", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "host")
		stores := make([]*Store, concurrentStores)
		for index := range stores {
			stores[index] = mustOpen(t, dir)
		}
		entry := freshEntry(0)
		mustNoError(t, stores[0].PutEntry(entry))

		var wait sync.WaitGroup
		for _, store := range stores {
			wait.Add(1)
			go func() {
				defer wait.Done()
				for range updatesPerStore {
					_, err := store.UpdateEntry(entry.Ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) {
						e.Corrections++
						return e, nil
					})
					if err != nil {
						t.Error(err)
					}
				}
			}()
		}
		wait.Wait()
		got, err := stores[0].GetEntry(entry.Ticket.Key())
		mustNoError(t, err)
		if want := concurrentStores * updatesPerStore; got.Corrections != want {
			t.Fatalf("corrections = %d, want %d", got.Corrections, want)
		}
	})

	t.Run("two stores on one directory keep a valid journal chain [verifies SP-341~1]", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "host")
		stores := []*Store{mustOpen(t, dir), mustOpen(t, dir)}
		var wait sync.WaitGroup
		for _, store := range stores {
			wait.Add(1)
			go func() {
				defer wait.Done()
				for range updatesPerStore {
					if _, err := store.Append(hostproto.Event{Kind: hostproto.EventFetched}); err != nil {
						t.Error(err)
					}
				}
			}()
		}
		wait.Wait()
		records, err := stores[0].Records()
		mustNoError(t, err)
		if len(records) != concurrentStores*updatesPerStore || hostproto.VerifyChain(records) != nil {
			t.Fatalf("%d records, chain error %v", len(records), hostproto.VerifyChain(records))
		}
	})
}

func TestKeys(t *testing.T) {
	t.Run("a key that is not a digest is refused and never reaches the filesystem [verifies SP-340~1]", func(t *testing.T) {
		store := openStore(t)
		entry := freshEntry(0)
		entry.Ticket.RequestID = "../../escape"
		if err := store.PutEntry(entry); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("PutEntry error = %v", err)
		}
		if _, err := store.GetEntry(entry.Ticket.Key()); !errors.Is(err, hostproto.ErrNotFound) {
			t.Fatalf("GetEntry error = %v", err)
		}
		if _, err := store.GetRequest("../x"); !errors.Is(err, hostproto.ErrNotFound) {
			t.Fatalf("GetRequest error = %v", err)
		}
	})
}

func TestProperties(t *testing.T) {
	t.Run("a stored request comes back with the same id [verifies SP-342~1]", func(t *testing.T) {
		store := openStore(t)
		rapid.Check(t, func(t *rapid.T) {
			user := rapid.String().Draw(t, "user")
			request := hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = user })
			id, err := store.PutRequest(request)
			if err != nil {
				t.Fatal(err)
			}
			got, err := store.GetRequest(id)
			if err != nil {
				t.Fatal(err)
			}
			if gotID, _ := got.ID(); gotID != id || got.Prompt.User != strings.ToValidUTF8(user, "�") {
				t.Fatalf("round trip changed the request")
			}
		})
	})

	t.Run("a journal of any events verifies and survives a reopen [verifies SP-342~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			dir := filepath.Join(os.TempDir(), "hoststore-"+rapid.StringMatching(`[a-z0-9]{12}`).Draw(t, "name"))
			defer func() { _ = os.RemoveAll(dir) }()
			store, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			details := rapid.SliceOfN(rapid.String(), 0, 8).Draw(t, "details")
			for _, detail := range details {
				if _, err := store.Append(hostproto.Event{Kind: hostproto.EventFetched, Detail: detail}); err != nil {
					t.Fatal(err)
				}
			}
			reopened, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			records, err := reopened.Records()
			if err != nil || len(records) != len(details) || hostproto.VerifyChain(records) != nil {
				t.Fatalf("%d records for %d events, %v", len(records), len(details), err)
			}
		})
	})
}

func TestQueue(t *testing.T) {
	item := func(id string, wave int, criteria ...string) ports.QueueItem {
		return ports.QueueItem{RequestID: hostproto.RequestID(id), Criteria: criteria, Wave: wave, Purpose: "validity"}
	}

	t.Run("the wave of the first record is kept and the criteria are merged without duplicates [verifies SP-343~1]", func(t *testing.T) {
		store := openStore(t)
		mustNoError(t, store.Record(item("b", 1, "PAR-2~1")))
		mustNoError(t, store.Record(item("b", 2, "PAR-1~1", "PAR-2~1")))
		items, err := store.Items()
		mustNoError(t, err)
		if len(items) != 1 || items[0].Wave != 1 || !slices.Equal(items[0].Criteria, []string{"PAR-1~1", "PAR-2~1"}) {
			t.Fatalf("items = %+v", items)
		}
	})

	t.Run("items are listed by request id [verifies SP-343~1]", func(t *testing.T) {
		store := openStore(t)
		for _, id := range []string{"c", "a", "b"} {
			mustNoError(t, store.Record(item(id, 1, "PAR-1~1")))
		}
		items, err := store.Items()
		mustNoError(t, err)
		ids := make([]string, len(items))
		for index, listed := range items {
			ids[index] = string(listed.RequestID)
		}
		if !slices.Equal(ids, []string{"a", "b", "c"}) {
			t.Fatalf("ids = %v", ids)
		}
	})

	t.Run("an empty queue has no item and wave zero [verifies SP-343~1]", func(t *testing.T) {
		store := openStore(t)
		items, err := store.Items()
		wave, waveErr := store.Wave()
		if err != nil || waveErr != nil || items == nil || len(items) != 0 || wave != 0 {
			t.Fatalf("items = %v, %v; wave = %d, %v", items, err, wave, waveErr)
		}
	})

	t.Run("the wave advances by consecutive numbers from one and survives a reopen [verifies SP-343~1]", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "host")
		store := mustOpen(t, dir)
		for want := 1; want <= 3; want++ {
			got, err := store.AdvanceWave()
			if err != nil || got != want {
				t.Fatalf("AdvanceWave = %d, %v; want %d", got, err, want)
			}
		}
		wave, err := mustOpen(t, dir).Wave()
		if err != nil || wave != 3 {
			t.Fatalf("Wave = %d, %v", wave, err)
		}
	})

	t.Run("recording keeps the wave counter [verifies SP-343~1]", func(t *testing.T) {
		store := openStore(t)
		_, err := store.AdvanceWave()
		mustNoError(t, err)
		mustNoError(t, store.Record(item("a", 1, "PAR-1~1")))
		if wave, _ := store.Wave(); wave != 1 {
			t.Fatalf("wave = %d", wave)
		}
	})

	t.Run("the criteria of any records merge to their sorted union [verifies SP-343~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			store, err := Open(filepath.Join(os.TempDir(), "hoststore-q-"+rapid.StringMatching(`[a-z0-9]{12}`).Draw(t, "name")))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = os.RemoveAll(store.Dir()) }()
			batches := rapid.SliceOfN(rapid.SliceOfN(rapid.StringMatching(`PAR-[1-4]~1`), 0, 4), 1, 4).Draw(t, "batches")
			var union []string
			for _, batch := range batches {
				union = append(union, batch...)
				if err := store.Record(item("a", 1, batch...)); err != nil {
					t.Fatal(err)
				}
			}
			slices.Sort(union)
			union = slices.Compact(union)
			items, _ := store.Items()
			got := items[0].Criteria
			if len(union) == 0 {
				union = nil
			}
			if !slices.Equal(got, union) && (len(got) != 0 || len(union) != 0) {
				t.Fatalf("criteria = %v, want %v", got, union)
			}
		})
	})
}
