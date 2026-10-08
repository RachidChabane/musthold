package adapters

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/RachidChabane/spec-proof/tools/release/internal/domain"
)

const permissionMask = 0o777

// ArchivePacker writes deterministic release archives: entries in the order
// given, fixed modes, owner root, the commit time, and no name or time in the
// compression header.
type ArchivePacker struct{}

// Pack packs the entries into the container of the format.
func (ArchivePacker) Pack(format domain.ArchiveFormat, entries []domain.ArchiveEntry, modTime time.Time) ([]byte, error) {
	switch format {
	case domain.FormatTarGz:
		return packTarGz(entries, modTime)
	case domain.FormatZip:
		return packZip(entries, modTime)
	}
	return nil, fmt.Errorf("unknown archive format %q", format)
}

func packTarGz(entries []domain.ArchiveEntry, modTime time.Time) ([]byte, error) {
	var out bytes.Buffer
	compressor := gzip.NewWriter(&out)
	archive := tar.NewWriter(compressor)
	for _, entry := range entries {
		header := &tar.Header{
			Typeflag: tar.TypeReg, Name: entry.Name, Mode: entry.Mode, Size: int64(len(entry.Data)),
			Uid: domain.ArchiveOwnerID, Gid: domain.ArchiveOwnerID, ModTime: modTime.UTC(), Format: tar.FormatUSTAR,
		}
		if err := archive.WriteHeader(header); err != nil {
			return nil, err
		}
		if _, err := archive.Write(entry.Data); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	if err := compressor.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func packZip(entries []domain.ArchiveEntry, modTime time.Time) ([]byte, error) {
	var out bytes.Buffer
	archive := zip.NewWriter(&out)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.Name, Method: zip.Deflate, Modified: modTime.UTC()}
		header.SetMode(fs.FileMode(entry.Mode))
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write(entry.Data); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// unpackedEntry is an entry read back from an archive.
type unpackedEntry struct {
	facts domain.EntryFacts
	data  []byte
}

// unpack reads every entry of an archive.
func unpack(format domain.ArchiveFormat, archive []byte) ([]unpackedEntry, error) {
	switch format {
	case domain.FormatTarGz:
		return unpackTarGz(archive)
	case domain.FormatZip:
		return unpackZip(archive)
	}
	return nil, fmt.Errorf("unknown archive format %q", format)
}

func unpackTarGz(archive []byte) ([]unpackedEntry, error) {
	decompressor, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	reader := tar.NewReader(decompressor)
	var entries []unpackedEntry
	for {
		header, err := reader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return entries, nil
			}
			return nil, err
		}
		data := make([]byte, header.Size)
		if _, err := io.ReadFull(reader, data); err != nil {
			return nil, err
		}
		entries = append(entries, unpackedEntry{
			facts: domain.EntryFacts{Name: header.Name, Mode: header.Mode & permissionMask, UID: header.Uid, GID: header.Gid, ModTime: header.ModTime.UTC()},
			data:  data,
		})
	}
}

func unpackZip(archive []byte) ([]unpackedEntry, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, err
	}
	var entries []unpackedEntry
	for _, file := range reader.File {
		opened, err := file.Open()
		if err != nil {
			return nil, err
		}
		data := make([]byte, file.UncompressedSize64)
		_, readErr := io.ReadFull(opened, data)
		closeErr := opened.Close()
		if readErr != nil || closeErr != nil {
			return nil, fmt.Errorf("read %s: %v %v", file.Name, readErr, closeErr)
		}
		entries = append(entries, unpackedEntry{
			facts: domain.EntryFacts{Name: file.Name, Mode: int64(file.Mode().Perm()), UID: domain.ArchiveOwnerID, GID: domain.ArchiveOwnerID, ModTime: file.Modified.UTC()},
			data:  data,
		})
	}
	return entries, nil
}
