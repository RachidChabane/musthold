package ecosystemcontract_test

import (
	"path/filepath"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/node"
	javascriptmutation "github.com/RachidChabane/musthold/discpower/internal/adapters/mutation/javascript"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/testkit/ecosystemcontract"
	"github.com/RachidChabane/musthold/discpower/internal/testkit/nodeinstall"
)

const (
	nodeSource     = "src/rates.ts"
	nodeSourceRoot = "src"

	lineTotalSource  = "function lineTotal(unitPrice: number, quantity: number): number {\n  return unitPrice + quantity;\n}"
	lineTotalBroken  = "function lineTotal(unitPrice: number, quantity: number): number {\n  return unitPrice *;\n}"
	clampEquivalent  = "function clampPercent(percent: number): number {\n  return Math.min(MAX_PERCENT, Math.max(0, percent));\n}"
	discountedKilled = "function discounted(total: number, percent: number): number {\n  const kept = MAX_PERCENT + clampPercent(percent);\n  return Math.round(((total * kept) / MAX_PERCENT) * 100) / 100;\n}"

	jestUnbound   = "line totals are zero for no units"
	vitestFile    = "test/rates.test.ts"
	vitestUnbound = "line totals > are zero for no units"
)

// nodeSubject is the contract subject of a Node runner over its fixture project,
// installed from its lock file.
func nodeSubject(t *testing.T, name ecosystem.Name, build func(node.Dependencies) *node.Strategy, unbound ecosystem.TestID) ecosystemcontract.Subject {
	return ecosystemcontract.Subject{
		Name: string(name),
		Build: func(a ecosystemcontract.Adapters) ports.Ecosystem {
			return build(node.Dependencies{Process: a.Process, Reader: a.Reader, Locator: treesitter.NewLocator(), Assignments: javascriptmutation.New()})
		},
		FixtureDir:     nodeinstall.Installed(t, filepath.Join(fixturesRoot, string(name))),
		Sources:        []string{nodeSource},
		SourceRoots:    []string{nodeSourceRoot},
		Bindings:       loadBindings(t, name),
		KillingRef:     killing,
		EquivalentRef:  discount,
		UnaffectedRef:  shipping,
		DisplayRef:     discount,
		UnboundTests:   []ecosystem.TestID{unbound},
		Killing:        edit(nodeSource, "lineTotal", lineTotalSource),
		Equivalent:     edit(nodeSource, "clampPercent", clampEquivalent),
		Broken:         edit(nodeSource, "lineTotal", lineTotalBroken),
		DisplayKilling: edit(nodeSource, "discounted", discountedKilled),
	}
}

// TestEcosystemContractNode runs the contract of the Node strategies, one after
// the other, over npm-installed fixtures.
func TestEcosystemContractNode(t *testing.T) {
	t.Run("jest", func(t *testing.T) {
		nodeinstall.RequireNode(t)
		unbound := ecosystem.NewTestID(jestUnbound, jestUnbound)
		ecosystemcontract.Run(t, nodeSubject(t, ecosystem.Jest, node.NewJest, unbound))
	})
	t.Run("vitest", func(t *testing.T) {
		nodeinstall.RequireNode(t)
		unbound := ecosystem.NewTestID(vitestFile, vitestUnbound)
		ecosystemcontract.Run(t, nodeSubject(t, ecosystem.Vitest, node.NewVitest, unbound))
	})
}
