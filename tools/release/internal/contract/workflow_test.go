// Package contract_test checks that the committed release workflow, the CI workflow,
// the installer and the release tool agree with each other.
package contract_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/RachidChabane/spec-proof/tools/release/internal/domain"
	"github.com/RachidChabane/spec-proof/tools/release/internal/testkit"
)

const (
	verifiesWorkflow = " [verifies SP-1822~1]"
	releaseWorkflow  = ".github/workflows/release.yml"
	ciWorkflow       = ".github/workflows/ci.yml"
	checkWorkflow    = ".github/workflows/release-check.yml"
	installScript    = "script/install"
	releaseTagGlob   = "*/v*"
	publishJob       = "publish"
)

var (
	pinnedAction = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._/-]+@[0-9a-f]{40}$`)
	usesLine     = regexp.MustCompile(`^\s*(?:- )?uses:\s*(\S+)(.*)$`)
	goVersion    = regexp.MustCompile(`(?m)^go (\S+)$`)
)

type workflow struct {
	On          map[string]any `yaml:"on"`
	Permissions map[string]any `yaml:"permissions"`
	Jobs        map[string]job `yaml:"jobs"`
}

type job struct {
	Strategy    strategy          `yaml:"strategy"`
	Needs       any               `yaml:"needs"`
	Uses        string            `yaml:"uses"`
	If          string            `yaml:"if"`
	Permissions map[string]string `yaml:"permissions"`
	With        map[string]any    `yaml:"with"`
	Steps       []step            `yaml:"steps"`
}

type step struct {
	Run string `yaml:"run"`
}

type strategy struct {
	Matrix map[string][]string `yaml:"matrix"`
}

func readRepositoryFile(t *testing.T, relative string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testkit.RepositoryRoot(t), relative))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func parseWorkflow(t *testing.T, relative string) workflow {
	t.Helper()
	var parsed workflow
	if err := yaml.Unmarshal(readRepositoryFile(t, relative), &parsed); err != nil {
		t.Fatalf("%s: %v", relative, err)
	}
	return parsed
}

func needsOf(j job) []string {
	switch value := j.Needs.(type) {
	case string:
		return []string{value}
	case []any:
		var names []string
		for _, item := range value {
			names = append(names, item.(string))
		}
		return names
	}
	return nil
}

func TestReleaseWorkflow(t *testing.T) {
	parsed := parseWorkflow(t, releaseWorkflow)

	t.Run("the workflow starts on pushed tags of the form */v* and on nothing else"+verifiesWorkflow, func(t *testing.T) {
		if len(parsed.On) != 1 {
			t.Fatalf("triggers = %v", parsed.On)
		}
		push, isMap := parsed.On["push"].(map[string]any)
		if !isMap || len(push) != 1 {
			t.Fatalf("push trigger = %v", parsed.On["push"])
		}
		tags, _ := push["tags"].([]any)
		if len(tags) != 1 || tags[0] != releaseTagGlob {
			t.Fatalf("tags = %v", tags)
		}
	})

	t.Run("every permission is denied by default"+verifiesWorkflow, func(t *testing.T) {
		if parsed.Permissions == nil || len(parsed.Permissions) != 0 {
			t.Fatalf("permissions = %v, want {}", parsed.Permissions)
		}
	})

	t.Run("each job names its permissions and only publish writes anything"+verifiesWorkflow, func(t *testing.T) {
		for name, j := range parsed.Jobs {
			if j.Permissions == nil {
				t.Errorf("job %s names no permissions", name)
			}
			if name == publishJob {
				continue
			}
			if !(len(j.Permissions) == 1 && j.Permissions["contents"] == "read") {
				t.Errorf("job %s has permissions %v, want contents: read", name, j.Permissions)
			}
		}
		want := map[string]string{"contents": "write", "id-token": "write", "attestations": "write", "artifact-metadata": "write"}
		got := parsed.Jobs[publishJob].Permissions
		if len(got) != len(want) {
			t.Fatalf("publish permissions = %v", got)
		}
		for scope, level := range want {
			if got[scope] != level {
				t.Errorf("publish permission %s = %q, want %q", scope, got[scope], level)
			}
		}
	})

	t.Run("the jobs run in the order plan, gate, build and reproduce, publish"+verifiesWorkflow, func(t *testing.T) {
		wantNeeds := map[string][]string{
			"plan": nil, "ci": {"plan"}, "build": {"ci", "plan"}, "reproduce": {"ci", "plan"}, publishJob: {"build", "plan", "reproduce"},
		}
		for name, want := range wantNeeds {
			got := needsOf(parsed.Jobs[name])
			slices.Sort(got)
			if !slices.Equal(got, want) {
				t.Errorf("job %s needs %v, want %v", name, got, want)
			}
		}
		if len(parsed.Jobs) != len(wantNeeds) {
			t.Errorf("jobs = %d, want %d", len(parsed.Jobs), len(wantNeeds))
		}
	})

	t.Run("the gate reuses the CI workflow for the format library and the module"+verifiesWorkflow, func(t *testing.T) {
		gate := parsed.Jobs["ci"]
		if gate.Uses != "./"+ciWorkflow || gate.With["modules"] != "${{ needs.plan.outputs.ci_modules }}" {
			t.Fatalf("gate = %+v", gate)
		}
	})

	t.Run("every action is pinned by a full commit SHA and names its version"+verifiesWorkflow, func(t *testing.T) {
		for _, relative := range []string{releaseWorkflow, checkWorkflow, ciWorkflow} {
			checked := 0
			for number, line := range strings.Split(string(readRepositoryFile(t, relative)), "\n") {
				match := usesLine.FindStringSubmatch(line)
				if match == nil || strings.HasPrefix(match[1], "./") {
					continue
				}
				checked++
				if !pinnedAction.MatchString(match[1]) {
					t.Errorf("%s:%d: %s is not pinned by a full commit SHA", relative, number+1, match[1])
				}
				if !regexp.MustCompile(`#\s*v\d`).MatchString(match[2]) {
					t.Errorf("%s:%d: %s carries no version comment", relative, number+1, match[1])
				}
			}
			if checked == 0 {
				t.Errorf("%s uses no action", relative)
			}
		}
	})

	t.Run("publish refuses a private repository, signs the checksums and attests them"+verifiesWorkflow, func(t *testing.T) {
		text := string(readRepositoryFile(t, releaseWorkflow))
		for _, want := range []string{
			"if: github.event.repository.private",
			"cosign sign-blob --yes --bundle dist/" + domain.SignatureBundleName + " dist/" + domain.ChecksumsFileName,
			"subject-checksums: dist/" + domain.ChecksumsFileName,
			"gh release create",
			"--verify-tag",
			"diff dist/" + domain.ChecksumsFileName + " reproduced/" + domain.ChecksumsFileName,
		} {
			if !strings.Contains(text, want) {
				t.Errorf("the workflow lacks %q", want)
			}
		}
	})

	t.Run("every target of the matrix has an SBOM attestation step"+verifiesWorkflow, func(t *testing.T) {
		text := string(readRepositoryFile(t, releaseWorkflow))
		for _, target := range domain.Targets() {
			format := "tar.gz"
			if target.Format() == domain.FormatZip {
				format = "zip"
			}
			want := "dist/${{ env.BINARY }}_${{ env.SEMVER }}_" + target.OS + "_" + target.Arch + "." + format + ".spdx.json"
			if !strings.Contains(text, "sbom-path: "+want) {
				t.Errorf("no SBOM attestation for %s", target)
			}
		}
	})
}

func TestInstallerNamesTheWorkflowIdentity(t *testing.T) {
	script := string(readRepositoryFile(t, installScript))
	value := func(name string) string {
		match := regexp.MustCompile(`(?m)^readonly ` + name + `="([^"]*)"$`).FindStringSubmatch(script)
		if match == nil {
			t.Fatalf("%s is not defined in %s", name, installScript)
		}
		return match[1]
	}

	t.Run("the installer verifies the certificate identity of the release workflow at the tag"+verifiesWorkflow, func(t *testing.T) {
		expand := func(text string) string {
			text = strings.ReplaceAll(text, "${SIGNER_WORKFLOW}", value("SIGNER_WORKFLOW"))
			return strings.ReplaceAll(text, "${REPOSITORY}", domain.RepositorySlug)
		}
		signer := expand(value("SIGNER_WORKFLOW"))
		if signer != domain.RepositorySlug+"/"+domain.ReleaseWorkflow {
			t.Fatalf("signer workflow = %q", signer)
		}
		if got := expand(value("IDENTITY_PREFIX")); got != domain.CertificateIdentityPrefix {
			t.Fatalf("identity prefix = %q, want %q", got, domain.CertificateIdentityPrefix)
		}
		if value("OIDC_ISSUER") != domain.OIDCIssuer {
			t.Fatalf("issuer = %q", value("OIDC_ISSUER"))
		}
		if value("CHECKSUMS_FILE") != domain.ChecksumsFileName || value("BUNDLE_FILE") != domain.SignatureBundleName {
			t.Fatal("the installer names other signature files than the release writes")
		}
	})

	t.Run("the workflow the identity names exists in the repository"+verifiesWorkflow, func(t *testing.T) {
		if _, err := os.Stat(filepath.Join(testkit.RepositoryRoot(t), domain.ReleaseWorkflow)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("the installer downloads from the repository the release is published to"+verifiesWorkflow, func(t *testing.T) {
		if got := strings.NewReplacer("${REPOSITORY}", domain.RepositorySlug).Replace(value("DOWNLOAD_BASE")); got != domain.RepositoryURL+"/releases/download" {
			t.Fatalf("download base = %q", got)
		}
	})
}

func TestToolchainPinMatchesTheWorkspace(t *testing.T) {
	t.Run("the toolchain of the release build is the go version of go.work"+verifiesWorkflow, func(t *testing.T) {
		match := goVersion.FindStringSubmatch(string(readRepositoryFile(t, "go.work")))
		if match == nil || "go"+match[1] != domain.ToolchainVersion {
			t.Fatalf("go.work says %v, the release pins %s", match, domain.ToolchainVersion)
		}
	})
}

func TestCIWorkflowKeepsItsTriggers(t *testing.T) {
	parsed := parseWorkflow(t, ciWorkflow)

	t.Run("CI still runs on pushes to main and on pull requests, and can be called with a modules input"+verifiesWorkflow, func(t *testing.T) {
		push, _ := parsed.On["push"].(map[string]any)
		if branches, _ := push["branches"].([]any); len(branches) != 1 || branches[0] != "main" {
			t.Errorf("push = %v", push)
		}
		if _, present := parsed.On["pull_request"]; !present {
			t.Error("CI no longer runs on pull requests")
		}
		call, _ := parsed.On["workflow_call"].(map[string]any)
		inputs, _ := call["inputs"].(map[string]any)
		modules, _ := inputs["modules"].(map[string]any)
		if modules["type"] != "string" || modules["default"] != "" {
			t.Errorf("modules input = %v", modules)
		}
		secrets, _ := call["secrets"].(map[string]any)
		if _, present := secrets["SHALLNOT_PROOF_GATE_DEPLOY_KEY"]; !present {
			t.Errorf("secrets = %v", secrets)
		}
	})

	t.Run("a call with modules runs only the product job, and a push or pull request runs every job"+verifiesWorkflow, func(t *testing.T) {
		for _, name := range []string{"pilot", "bench", "calibration"} {
			if parsed.Jobs[name].If != "inputs.modules == ''" {
				t.Errorf("job %s runs when %q", name, parsed.Jobs[name].If)
			}
		}
		if parsed.Jobs["product"].If != "" {
			t.Errorf("the product job is conditional: %q", parsed.Jobs["product"].If)
		}
		if groups := parsed.Jobs["product"].Strategy.Matrix["group"]; !slices.Equal(groups, []string{"discpower", "rest"}) {
			t.Errorf("the product job gates the module groups %v", groups)
		}
		gates := false
		for _, s := range parsed.Jobs["product"].Steps {
			if strings.Contains(s.Run, `script/module-group "$GROUP" $MODULES`) && strings.Contains(s.Run, `make ci MODULES="$modules"`) {
				gates = true
			}
		}
		if !gates {
			t.Errorf("no step of the product job gates the modules of its group among those the call names")
		}
	})
}

func TestReleaseCheckWorkflow(t *testing.T) {
	text := string(readRepositoryFile(t, checkWorkflow))

	t.Run("the rehearsal runs on the files that change the release pipeline"+verifiesWorkflow, func(t *testing.T) {
		for _, want := range []string{`"tools/release/**"`, `"script/release*"`, `"script/install"`, `".github/workflows/release.yml"`} {
			if strings.Count(text, want) < 2 {
				t.Errorf("the rehearsal does not trigger on %s for pull requests and pushes", want)
			}
		}
	})

	t.Run("the rehearsal tags locally, runs the dry run and runs actionlint pinned to a version"+verifiesWorkflow, func(t *testing.T) {
		for _, want := range []string{`git tag "$THROWAWAY_TAG"`, `make release-dry-run TAG="$THROWAWAY_TAG"`, "actionlint@${ACTIONLINT_VERSION}", "ACTIONLINT_VERSION: v"} {
			if !strings.Contains(text, want) {
				t.Errorf("the rehearsal lacks %q", want)
			}
		}
		if strings.Contains(text, "git push") {
			t.Error("the rehearsal pushes")
		}
	})
}
