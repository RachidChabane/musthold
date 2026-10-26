package adapters_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/tools/release/internal/adapters"
	"github.com/RachidChabane/musthold/tools/release/internal/domain"
)

const verifiesArchive = " [verifies SP-1805~3]"

var archiveTime = time.Unix(1_700_000_000, 0).UTC()

func sampleEntries(target domain.Target) []domain.ArchiveEntry {
	return domain.ArchiveEntries(target, "tool", domain.ArchiveContent{
		Binary: bytes.Repeat([]byte("binary "), 500), License: []byte("licence\n"), Readme: []byte("readme\n"),
		Notice: []byte("notice\n"), ThirdPartyNotices: []byte("third party\n"),
	})
}

// failer is what packFor needs of a test: both testing.T and rapid.T have it.
type failer interface {
	Helper()
	Fatal(args ...any)
}

func packFor(t failer, target domain.Target, entries []domain.ArchiveEntry, instant time.Time) []byte {
	t.Helper()
	packed, err := adapters.ArchivePacker{}.Pack(target.Format(), entries, instant)
	if err != nil {
		t.Fatal(err)
	}
	return packed
}

func TestArchivePacker(t *testing.T) {
	linux := domain.Target{OS: domain.OSLinux, Arch: domain.ArchAmd64}
	windows := domain.Target{OS: domain.OSWindows, Arch: domain.ArchArm64}

	t.Run("packing the same entries twice gives the same bytes, for both containers"+verifiesArchive, func(t *testing.T) {
		for _, target := range []domain.Target{linux, windows} {
			entries := sampleEntries(target)
			if !bytes.Equal(packFor(t, target, entries, archiveTime), packFor(t, target, entries, archiveTime)) {
				t.Errorf("%s: two packs differ", target)
			}
		}
	})

	t.Run("packing is deterministic for any content"+verifiesArchive, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			target := rapid.SampledFrom(domain.Targets()).Draw(t, "target")
			entries := domain.ArchiveEntries(target, "tool", domain.ArchiveContent{
				Binary: rapid.SliceOf(rapid.Byte()).Draw(t, "binary"), License: rapid.SliceOf(rapid.Byte()).Draw(t, "licence"), Readme: rapid.SliceOf(rapid.Byte()).Draw(t, "readme"),
				Notice:            rapid.SliceOf(rapid.Byte()).Draw(t, "notice"),
				ThirdPartyNotices: rapid.SliceOf(rapid.Byte()).Draw(t, "third party notices"),
			})
			if !bytes.Equal(packFor(t, target, entries, archiveTime), packFor(t, target, entries, archiveTime)) {
				t.Fatal("two packs differ")
			}
		})
	})

	t.Run("a tar.gz holds the entries in order with fixed mode, owner and time and no name in its gzip header"+verifiesArchive, func(t *testing.T) {
		entries := sampleEntries(linux)
		packed := packFor(t, linux, entries, archiveTime)
		compressed, err := gzip.NewReader(bytes.NewReader(packed))
		if err != nil {
			t.Fatal(err)
		}
		if compressed.Header.Name != "" || compressed.Header.Comment != "" || !compressed.Header.ModTime.IsZero() || len(compressed.Header.Extra) != 0 {
			t.Errorf("gzip header = %+v", compressed.Header)
		}
		reader := tar.NewReader(compressed)
		for index := 0; ; index++ {
			header, err := reader.Next()
			if err == io.EOF {
				if index != len(entries) {
					t.Fatalf("%d entries, want %d", index, len(entries))
				}
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			want := entries[index]
			data, _ := io.ReadAll(reader)
			if header.Name != want.Name || header.Mode != want.Mode || header.Uid != 0 || header.Gid != 0 || header.Uname != "" || header.Gname != "" ||
				!header.ModTime.Equal(archiveTime) || !header.AccessTime.IsZero() || !header.ChangeTime.IsZero() || !bytes.Equal(data, want.Data) {
				t.Errorf("entry %d = %+v", index, header)
			}
		}
	})

	t.Run("a zip holds the entries in order with fixed mode and the commit time"+verifiesArchive, func(t *testing.T) {
		entries := sampleEntries(windows)
		packed := packFor(t, windows, entries, archiveTime)
		reader, err := zip.NewReader(bytes.NewReader(packed), int64(len(packed)))
		if err != nil {
			t.Fatal(err)
		}
		if len(reader.File) != len(entries) {
			t.Fatalf("%d entries, want %d", len(reader.File), len(entries))
		}
		for index, file := range reader.File {
			want := entries[index]
			opened, _ := file.Open()
			data, _ := io.ReadAll(opened)
			_ = opened.Close()
			if file.Name != want.Name || int64(file.Mode().Perm()) != want.Mode || !file.Modified.Equal(archiveTime) || !bytes.Equal(data, want.Data) {
				t.Errorf("entry %d = %s %v %v", index, file.Name, file.Mode(), file.Modified)
			}
		}
	})

	t.Run("another commit time gives another archive"+verifiesArchive, func(t *testing.T) {
		entries := sampleEntries(linux)
		if bytes.Equal(packFor(t, linux, entries, archiveTime), packFor(t, linux, entries, archiveTime.Add(time.Hour))) {
			t.Fatal("the commit time does not reach the archive")
		}
	})

	t.Run("an unknown container is refused"+verifiesArchive, func(t *testing.T) {
		if _, err := (adapters.ArchivePacker{}).Pack("rar", nil, archiveTime); err == nil {
			t.Fatal("packed")
		}
	})
}
