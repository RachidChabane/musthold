package golang_test

import (
	"context"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/golang"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/junitxml"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/process"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/goparser"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/differential"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

const (
	verifiesPackageForms = " [verifies SP-2137~1]"

	ratesModule = "../../../../fixtures/scan/go-rates"
	billingFile = "billing/billing.go"
	ratesFile   = "rates/rates.go"
	// totalSite inlines rates.Parse, so that its form changes with the
	// rates package it is compiled against.
	totalSite = billingFile + "::billing.Total"
	parseSite = ratesFile + "::rates.Parse"
)

// Edits of the rates module: a surcharge added twice, a surcharge laid out
// on more lines, and a bound of Parse moved.
var (
	doubledSurcharge = [2]string{"return amount + amount*rate/percentBase, nil", "return amount + 2*amount*rate/percentBase, nil"}
	spreadSurcharge  = [2]string{"return amount + amount*rate/percentBase, nil", "return amount +\n\t\tamount*rate/percentBase,\n\t\tnil"}
	movedBound       = [2]string{"percent > MaxPercent", "percent >= MaxPercent"}
)

// formsProject is a reference workspace of the rates module and its edited
// copies.
type formsProject struct {
	t       *testing.T
	project scanProject
}

func newFormsProject(t *testing.T) formsProject {
	t.Helper()
	if testing.Short() {
		t.Skip("compiles Go packages")
	}
	return formsProject{t: t, project: openScanModule(t, ratesModule)}
}

func freshCompiler() ports.FormCompiler {
	return golang.New(golang.Dependencies{Process: process.New(), Reader: junitxml.NewReader(), Locator: goparser.NewLocator()}).Compiler()
}

// edited clones the reference and replaces one text of a file.
func (p formsProject) edited(file string, edit [2]string) ecosystem.Workspace {
	p.t.Helper()
	ctx := context.Background()
	clone, err := p.project.manager.Clone(ctx, p.project.workspace, p.project.layout)
	if err != nil {
		p.t.Fatal(err)
	}
	source, err := p.project.manager.ReadFile(clone, file)
	if err != nil {
		p.t.Fatal(err)
	}
	if !strings.Contains(string(source), edit[0]) {
		p.t.Fatalf("%s holds no %q", file, edit[0])
	}
	if err := p.project.manager.WriteFile(clone, p.project.layout, file, []byte(strings.Replace(string(source), edit[0], edit[1], 1))); err != nil {
		p.t.Fatal(err)
	}
	return clone
}

func (p formsProject) forms(compiler ports.FormCompiler, workspace ecosystem.Workspace, sites ...string) differential.CompiledForms {
	p.t.Helper()
	forms, err := compiler.Forms(context.Background(), workspace, p.project.toolchain, sites)
	if err != nil {
		p.t.Fatal(err)
	}
	for _, site := range sites {
		if forms[site] == "" {
			p.t.Fatalf("no form for %s", site)
		}
	}
	return forms
}

func TestPackageForms(t *testing.T) {
	p := newFormsProject(t)
	memoized := freshCompiler()
	reference := p.forms(memoized, p.project.workspace, totalSite)

	t.Run("a package compiled again from what its first compile read gives the forms a fresh compile gives"+verifiesPackageForms, func(t *testing.T) {
		for _, edit := range [][2]string{doubledSurcharge, spreadSurcharge} {
			mutant := p.edited(billingFile, edit)
			if again, fresh := p.forms(memoized, mutant, totalSite), p.forms(freshCompiler(), mutant, totalSite); again[totalSite] != fresh[totalSite] {
				t.Fatalf("%q: memoized %s, fresh %s", edit[1], again[totalSite], fresh[totalSite])
			}
		}
	})
	t.Run("a change of layout keeps the form and a change of behaviour changes it, through the memoized compile"+verifiesPackageForms, func(t *testing.T) {
		if spread := p.forms(memoized, p.edited(billingFile, spreadSurcharge), totalSite); spread[totalSite] != reference[totalSite] {
			t.Fatal("a change of layout changed the form")
		}
		if doubled := p.forms(memoized, p.edited(billingFile, doubledSurcharge), totalSite); doubled[totalSite] == reference[totalSite] {
			t.Fatal("a change of behaviour kept the form")
		}
	})
	t.Run("sites of several packages are compiled against their dependencies as the workspace has them"+verifiesPackageForms, func(t *testing.T) {
		moved := p.edited(ratesFile, movedBound)
		if forms := p.forms(memoized, moved, parseSite, totalSite); forms[totalSite] == reference[totalSite] {
			t.Fatal("the form of a package that inlines a changed dependency kept the reference's")
		}
	})
}
