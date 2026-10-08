package domain_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/tools/release/internal/domain"
)

const (
	verifiesDist      = " [verifies SP-1818~1]"
	verifiesArchiving = " [verifies SP-1805~2]"
	distCommit        = "3333333333333333333333333333333333333333"
)

// validDist builds the facts of a dist directory that holds exactly the release of
// a tag for the targets.
func validDist(t *testing.T, tagText string, targets []domain.Target, hasNotice bool) domain.DistFacts {
	t.Helper()
	tag := mustTag(t, tagText)
	module := tag.Module
	instant := domain.CommitTime(1_700_000_000)
	facts := domain.DistFacts{Tag: tag, Commit: distCommit, CommitTime: instant, Targets: targets, HasNotice: hasNotice, Files: map[string]domain.DistFile{}}
	var sums []domain.Checksum
	add := func(name string, file domain.DistFile) {
		file.Digest = domain.DigestOf([]byte(name))
		facts.Files[name] = file
		sums = append(sums, domain.Checksum{Digest: file.Digest, Name: name})
	}
	for _, target := range targets {
		archive := target.ArchiveName(module.Binary, tag.Version)
		var entries []domain.EntryFacts
		for _, name := range domain.ExpectedEntryNames(target, module.Binary, hasNotice) {
			mode := domain.FileMode
			if name == target.ExecutableName(module.Binary) {
				mode = domain.ExecutableMode
			}
			entries = append(entries, domain.EntryFacts{Name: name, Mode: mode, ModTime: instant})
		}
		binary := &domain.BinaryFacts{MainPath: module.ModulePath() + "/cmd/" + module.Binary, Settings: map[string]string{
			"-trimpath": "true", "CGO_ENABLED": "0", "GOOS": target.OS, "GOARCH": target.Arch,
		}, Contains: func(needle string) bool { return needle == tag.Semver() || needle == distCommit }}
		add(archive, domain.DistFile{Entries: entries, Binary: binary})
		header := domain.SPDXHeader{Version: "SPDX-2.3", Namespace: domain.SBOMNamespace(tag, archive), Created: domain.SPDXCreated(instant)}
		add(domain.SBOMName(archive), domain.DistFile{SBOM: &header})
	}
	add(domain.InstallerFileName, domain.DistFile{})
	facts.Checksums = domain.RenderChecksums(sums)
	return facts
}

func linuxAndWindows(t *testing.T) []domain.Target {
	t.Helper()
	var targets []domain.Target
	for _, text := range []string{"linux/amd64", "windows/arm64"} {
		target, err := domain.ParseTarget(text)
		if err != nil {
			t.Fatal(err)
		}
		targets = append(targets, target)
	}
	return targets
}

func anyFinding(findings []domain.Finding, fragment string) bool {
	for _, finding := range findings {
		if strings.Contains(finding.Message, fragment) {
			return true
		}
	}
	return false
}

func TestVerifyDist(t *testing.T) {
	targets := linuxAndWindows(t)

	t.Run("a dist directory that holds exactly the release has no finding"+verifiesDist, func(t *testing.T) {
		if findings := domain.VerifyDist(validDist(t, "discpower/v0.2.0", targets, false)); len(findings) != 0 {
			t.Fatalf("findings = %v", findings)
		}
	})

	t.Run("the signature bundle and the formula may sit beside the release files"+verifiesDist, func(t *testing.T) {
		facts := validDist(t, "discpower/v0.2.0", targets, false)
		facts.Files[domain.SignatureBundleName] = domain.DistFile{}
		facts.Files[domain.FormulaFileName("spec-proof-dp")] = domain.DistFile{}
		if findings := domain.VerifyDist(facts); len(findings) != 0 {
			t.Fatalf("findings = %v", findings)
		}
	})

	t.Run("a dist directory of a work with a NOTICE holds it in every archive"+verifiesArchiving, func(t *testing.T) {
		if findings := domain.VerifyDist(validDist(t, "discpower/v0.2.0", targets, true)); len(findings) != 0 {
			t.Fatalf("findings = %v", findings)
		}
	})

	t.Run("an archive that lacks the NOTICE of the work is a finding"+verifiesArchiving, func(t *testing.T) {
		facts := validDist(t, "discpower/v0.2.0", targets, false)
		facts.HasNotice = true
		if findings := domain.VerifyDist(facts); !anyFinding(findings, "NOTICE") {
			t.Fatalf("findings = %v", findings)
		}
	})

	t.Run("an archive that holds a NOTICE the work does not have is a finding"+verifiesArchiving, func(t *testing.T) {
		facts := validDist(t, "discpower/v0.2.0", targets, true)
		facts.HasNotice = false
		if findings := domain.VerifyDist(facts); !anyFinding(findings, "NOTICE") {
			t.Fatalf("findings = %v", findings)
		}
	})

	t.Run("an archive that lacks the third-party notices is a finding"+verifiesArchiving, func(t *testing.T) {
		facts := validDist(t, "discpower/v0.2.0", targets, false)
		name := targets[0].ArchiveName("spec-proof-dp", facts.Tag.Version)
		file := facts.Files[name]
		file.Entries = slices.DeleteFunc(slices.Clone(file.Entries), func(entry domain.EntryFacts) bool { return entry.Name == domain.ThirdPartyNoticesFile })
		facts.Files[name] = file
		if findings := domain.VerifyDist(facts); !anyFinding(findings, domain.ThirdPartyNoticesFile) {
			t.Fatalf("findings = %v", findings)
		}
	})

	mutations := map[string]struct {
		change   func(*domain.DistFacts)
		fragment string
	}{
		"a missing archive": {func(f *domain.DistFacts) {
			delete(f.Files, targets[0].ArchiveName("spec-proof-dp", f.Tag.Version))
		}, "is missing"},
		"a missing checksum file": {func(f *domain.DistFacts) { f.Checksums = "" }, "SHA256SUMS is missing"},
		"an unlisted file":        {func(f *domain.DistFacts) { f.Files["extra.bin"] = domain.DistFile{} }, "extra.bin is in the dist directory"},
		"a digest that differs": {func(f *domain.DistFacts) {
			name := domain.SBOMName(targets[0].ArchiveName("spec-proof-dp", f.Tag.Version))
			file := f.Files[name]
			file.Digest = strings.Repeat("0", 64)
			f.Files[name] = file
		}, "lists"},
		"a listed file that is not part of the release": {func(f *domain.DistFacts) {
			f.Checksums += strings.Repeat("a", 64) + "  surprise.zip\n"
		}, "surprise.zip"},
		"an archive entry owned by someone": {func(f *domain.DistFacts) {
			name := targets[0].ArchiveName("spec-proof-dp", f.Tag.Version)
			file := f.Files[name]
			file.Entries = append([]domain.EntryFacts(nil), file.Entries...)
			file.Entries[0].UID = 1000
			f.Files[name] = file
		}, "owned by 1000:0"},
		"an archive entry with another mode": {func(f *domain.DistFacts) {
			name := targets[0].ArchiveName("spec-proof-dp", f.Tag.Version)
			file := f.Files[name]
			file.Entries = append([]domain.EntryFacts(nil), file.Entries...)
			file.Entries[0].Mode = 0o600
			f.Files[name] = file
		}, "has mode 600"},
		"an archive entry with another time": {func(f *domain.DistFacts) {
			name := targets[0].ArchiveName("spec-proof-dp", f.Tag.Version)
			file := f.Files[name]
			file.Entries = append([]domain.EntryFacts(nil), file.Entries...)
			file.Entries[1].ModTime = time.Unix(5, 0)
			f.Files[name] = file
		}, "commit time"},
		"an archive with an entry missing": {func(f *domain.DistFacts) {
			name := targets[1].ArchiveName("spec-proof-dp", f.Tag.Version)
			file := f.Files[name]
			file.Entries = file.Entries[:2]
			f.Files[name] = file
		}, "entries are"},
		"a binary linked with another version": {func(f *domain.DistFacts) {
			name := targets[0].ArchiveName("spec-proof-dp", f.Tag.Version)
			file := f.Files[name]
			file.Binary.Contains = func(needle string) bool { return needle == distCommit }
			f.Files[name] = file
		}, "linked into main.version"},
		"a binary built with cgo": {func(f *domain.DistFacts) {
			name := targets[0].ArchiveName("spec-proof-dp", f.Tag.Version)
			f.Files[name].Binary.Settings["CGO_ENABLED"] = "1"
		}, "CGO_ENABLED"},
		"a binary for another platform": {func(f *domain.DistFacts) {
			name := targets[0].ArchiveName("spec-proof-dp", f.Tag.Version)
			f.Files[name].Binary.Settings["GOARCH"] = "riscv64"
		}, "GOARCH"},
		"a binary with VCS stamping": {func(f *domain.DistFacts) {
			name := targets[0].ArchiveName("spec-proof-dp", f.Tag.Version)
			f.Files[name].Binary.Settings["vcs"] = "git"
		}, "VCS stamping"},
		"a binary of another package": {func(f *domain.DistFacts) {
			name := targets[0].ArchiveName("spec-proof-dp", f.Tag.Version)
			f.Files[name].Binary.MainPath = "example.com/other"
		}, "main package is example.com/other"},
		"an archive with no build information": {func(f *domain.DistFacts) {
			name := targets[0].ArchiveName("spec-proof-dp", f.Tag.Version)
			file := f.Files[name]
			file.Binary = nil
			f.Files[name] = file
		}, "no readable build information"},
		"an archive that cannot be read": {func(f *domain.DistFacts) {
			name := targets[0].ArchiveName("spec-proof-dp", f.Tag.Version)
			file := f.Files[name]
			file.Unreadable = "gzip: invalid checksum"
			file.Entries, file.Binary = nil, nil
			f.Files[name] = file
		}, "cannot be read: gzip: invalid checksum"},
		"an SBOM of another archive": {func(f *domain.DistFacts) {
			name := domain.SBOMName(targets[0].ArchiveName("spec-proof-dp", f.Tag.Version))
			f.Files[name].SBOM.Namespace = "https://example.com/other"
		}, "documentNamespace"},
		"an SBOM of another time": {func(f *domain.DistFacts) {
			name := domain.SBOMName(targets[0].ArchiveName("spec-proof-dp", f.Tag.Version))
			f.Files[name].SBOM.Created = "2001-01-01T00:00:00Z"
		}, "created is"},
		"an SBOM of another SPDX version": {func(f *domain.DistFacts) {
			name := domain.SBOMName(targets[0].ArchiveName("spec-proof-dp", f.Tag.Version))
			f.Files[name].SBOM.Version = "SPDX-2.2"
		}, "spdxVersion"},
		"an SBOM that cannot be read": {func(f *domain.DistFacts) {
			name := domain.SBOMName(targets[0].ArchiveName("spec-proof-dp", f.Tag.Version))
			file := f.Files[name]
			file.SBOM = nil
			f.Files[name] = file
		}, "cannot be read"},
	}
	for name, mutation := range mutations {
		t.Run("the check finds "+name+verifiesDist, func(t *testing.T) {
			facts := validDist(t, "discpower/v0.2.0", targets, false)
			mutation.change(&facts)
			findings := domain.VerifyDist(facts)
			if !anyFinding(findings, mutation.fragment) {
				t.Fatalf("findings %v do not mention %q", findings, mutation.fragment)
			}
			for _, finding := range findings {
				if finding.Scope != domain.ScopeAlways || finding.Rule != domain.RuleDist {
					t.Errorf("finding = %+v", finding)
				}
			}
		})
	}
}

func TestArchiveEntries(t *testing.T) {
	content := domain.ArchiveContent{
		Binary: []byte("binary"), License: []byte("licence"), Readme: []byte("readme"),
		Notice: []byte("notice"), HasNotice: true, ThirdPartyNotices: []byte("third party"),
	}
	withoutNotice := content
	withoutNotice.HasNotice = false

	t.Run("an archive lists the binary, the licence, the README, the NOTICE and the third-party notices sorted by name with fixed modes"+verifiesArchiving, func(t *testing.T) {
		for _, target := range domain.Targets() {
			entries := domain.ArchiveEntries(target, "tool", content)
			var names []string
			for _, entry := range entries {
				names = append(names, entry.Name)
				wantMode := domain.FileMode
				if entry.Name == target.ExecutableName("tool") {
					wantMode = domain.ExecutableMode
				}
				if entry.Mode != wantMode {
					t.Errorf("%s %s: mode %o", target, entry.Name, entry.Mode)
				}
			}
			want := domain.ExpectedEntryNames(target, "tool", true)
			wantOrder := []string{"LICENSE", "NOTICE", "README.md", "THIRD_PARTY_NOTICES.md", target.ExecutableName("tool")}
			if !slices.Equal(names, want) || !slices.Equal(names, wantOrder) {
				t.Errorf("%s: names %v, want %v", target, names, wantOrder)
			}
		}
	})

	t.Run("an archive of a work with no NOTICE holds no NOTICE entry"+verifiesArchiving, func(t *testing.T) {
		for _, target := range domain.Targets() {
			var names []string
			for _, entry := range domain.ArchiveEntries(target, "tool", withoutNotice) {
				names = append(names, entry.Name)
			}
			if want := domain.ExpectedEntryNames(target, "tool", false); !slices.Equal(names, want) || slices.Contains(names, domain.NoticeFile) {
				t.Errorf("%s: names %v, want %v", target, names, want)
			}
		}
	})
}
