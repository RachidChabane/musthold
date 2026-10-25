package app

import (
	"fmt"
	"path"
	"sync"

	"github.com/RachidChabane/spec-proof/tools/release/internal/domain"
)

// DefaultJobs is how many targets build at once.
const DefaultJobs = 2

const (
	stagingDirectory = ".stage"
	installerSource  = "script/install"
	sumsMode         = 0o644
)

// BuildRequest is one build of the release assets of a tag.
type BuildRequest struct {
	Tag        domain.Tag
	Commit     string
	CommitTime int64
	Targets    []domain.Target
	// Jobs is how many targets build at once.
	Jobs int
	// DistDir receives the archives, the SBOMs and SHA256SUMS.
	DistDir string
}

// Builder cross-builds a binary module into the dist directory.
type Builder struct {
	Compiler  Compiler
	Archiver  Archiver
	Inspector BinaryInspector
	Files     Files
}

// Build writes, for every target, the archive and the SBOM of the module's binary,
// then the installer and SHA256SUMS. A module that ships no binary builds nothing.
func (b Builder) Build(request BuildRequest) ([]domain.Checksum, error) {
	module := request.Tag.Module
	if !module.ShipsBinary() {
		return nil, nil
	}
	if err := b.Files.RemoveAll(request.DistDir); err != nil {
		return nil, fmt.Errorf("clear %s: %w", request.DistDir, err)
	}
	defer func() { _ = b.Files.RemoveAll(path.Join(request.DistDir, stagingDirectory)) }()

	built, err := b.buildAll(request)
	if err != nil {
		return nil, err
	}
	installer, err := b.Files.ReadFile(installerSource)
	if err != nil {
		return nil, fmt.Errorf("read the installer: %w", err)
	}
	if err := b.Files.WriteFile(path.Join(request.DistDir, domain.InstallerFileName), installer, domain.ExecutableMode); err != nil {
		return nil, err
	}
	sums := append(built, domain.Checksum{Digest: domain.DigestOf(installer), Name: domain.InstallerFileName})
	text := domain.RenderChecksums(sums)
	if err := b.Files.WriteFile(path.Join(request.DistDir, domain.ChecksumsFileName), []byte(text), sumsMode); err != nil {
		return nil, err
	}
	parsed, err := domain.ParseChecksums(text)
	if err != nil {
		return nil, err
	}
	return parsed, nil
}

// buildAll builds the targets with at most Jobs at once and returns the checksums
// of what they wrote, in the order of the targets.
func (b Builder) buildAll(request BuildRequest) ([]domain.Checksum, error) {
	jobs := max(request.Jobs, 1)
	slots := make(chan struct{}, jobs)
	results := make([][]domain.Checksum, len(request.Targets))
	errs := make([]error, len(request.Targets))
	var group sync.WaitGroup
	for index, target := range request.Targets {
		group.Add(1)
		go func() {
			defer group.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			results[index], errs[index] = b.buildTarget(request, target)
		}()
	}
	group.Wait()
	var sums []domain.Checksum
	for index, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("build %s: %w", request.Targets[index], err)
		}
		sums = append(sums, results[index]...)
	}
	return sums, nil
}

func (b Builder) buildTarget(request BuildRequest, target domain.Target) ([]domain.Checksum, error) {
	module := request.Tag.Module
	staged := path.Join(request.DistDir, stagingDirectory, target.OS+"_"+target.Arch, target.ExecutableName(module.Binary))
	spec := domain.BuildSpec{Tag: request.Tag, Commit: request.Commit, Target: target, Output: staged}
	if err := b.Compiler.Compile(spec); err != nil {
		return nil, err
	}
	binary, err := b.Files.ReadFile(staged)
	if err != nil {
		return nil, err
	}
	content, err := b.archiveContent(module, binary)
	if err != nil {
		return nil, err
	}
	modTime := domain.CommitTime(request.CommitTime)
	archive, err := b.Archiver.Pack(target.Format(), domain.ArchiveEntries(target, module.Binary, content), modTime)
	if err != nil {
		return nil, err
	}
	facts, err := b.Inspector.Inspect(binary)
	if err != nil {
		return nil, err
	}
	archiveName := target.ArchiveName(module.Binary, request.Tag.Version)
	sbom, err := domain.RenderSPDX(domain.SBOMInput{Tag: request.Tag, ArchiveName: archiveName, Build: facts, Created: modTime})
	if err != nil {
		return nil, err
	}
	written := []struct {
		name string
		data []byte
	}{{archiveName, archive}, {domain.SBOMName(archiveName), sbom}}
	var sums []domain.Checksum
	for _, file := range written {
		if err := b.Files.WriteFile(path.Join(request.DistDir, file.name), file.data, domain.FileMode); err != nil {
			return nil, err
		}
		sums = append(sums, domain.Checksum{Digest: domain.DigestOf(file.data), Name: file.name})
	}
	return sums, nil
}

func (b Builder) archiveContent(module domain.Module, binary []byte) (domain.ArchiveContent, error) {
	content := domain.ArchiveContent{Binary: binary}
	var err error
	if content.License, err = b.Files.ReadFile(domain.LicenseFile); err != nil {
		return content, fmt.Errorf("read the licence: %w", err)
	}
	if content.Readme, err = b.Files.ReadFile(path.Join(module.Name, domain.ReadmeFile)); err != nil {
		return content, fmt.Errorf("read the README of %s: %w", module.Name, err)
	}
	if content.ThirdPartyNotices, err = b.Files.ReadFile(domain.ThirdPartyNoticesFile); err != nil {
		return content, fmt.Errorf("read the third-party notices: %w", err)
	}
	if content.Notice, err = b.Files.ReadFile(domain.NoticeFile); err != nil {
		return content, fmt.Errorf("read the notice: %w", err)
	}
	return content, nil
}
