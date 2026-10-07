package ecosystemcontract_test

import (
	"path/filepath"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem/jvm"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/testkit/ecosystemcontract"
	"github.com/RachidChabane/spec-proof/discpower/internal/testkit/jvmenv"
)

const (
	mavenFixture     = "junit5-maven"
	gradleFixture    = "junit5-gradle"
	kotlinFixture    = "junit5-kotlin"
	gradleBindings   = ecosystem.Name("gradle")
	kotlinBindings   = ecosystem.Name("gradle-kotlin")
	unboundTestName  = "is zero for no units"
	unboundTestClass = "RatesTest"

	mavenSource     = "src/main/java/com/example/parcel/Rates.java"
	gradleSource    = "src/main/java/parcel/Rates.java"
	kotlinSource    = "src/main/kotlin/parcel/Rates.kt"
	mavenClasses    = "target/classes/com/example/parcel/*.class"
	gradleClasses   = "build/classes/java/main/parcel/*.class"
	kotlinClasses   = "build/classes/kotlin/main/parcel/*.class"
	sourceRootsMain = "src/main"
)

func jvmDependencies(a ecosystemcontract.Adapters) jvm.Dependencies {
	return jvm.Dependencies{Process: a.Process, Reader: a.Reader, Locator: treesitter.NewLocator()}
}

func mavenSubject(t *testing.T) ecosystemcontract.Subject {
	const rates = "Rates."
	return ecosystemcontract.Subject{
		Name: "maven",
		Build: func(a ecosystemcontract.Adapters) ports.Ecosystem {
			return jvm.NewMaven(jvmDependencies(a))
		},
		FixtureDir:    filepath.Join(fixturesRoot, mavenFixture),
		Sources:       []string{mavenSource},
		SourceRoots:   []string{sourceRootsMain},
		BuildOutputs:  mavenClasses,
		Bindings:      loadBindings(t, ecosystem.Maven),
		KillingRef:    killing,
		EquivalentRef: discount,
		UnaffectedRef: shipping,
		DisplayRef:    discount,
		UnboundTests:  []ecosystem.TestID{ecosystem.NewTestID("com.example.parcel."+unboundTestClass, unboundTestName)},
		Killing: edit(mavenSource, rates+"lineTotal",
			"public static BigDecimal lineTotal(BigDecimal unitPrice, int quantity) {\n        return unitPrice.add(BigDecimal.valueOf(quantity));\n    }"),
		Equivalent: edit(mavenSource, rates+"clampPercent",
			"static int clampPercent(int percent) {\n        return Math.min(MAX_PERCENT, Math.max(0, percent));\n    }"),
		Broken: edit(mavenSource, rates+"lineTotal",
			"public static BigDecimal lineTotal(BigDecimal unitPrice, int quantity) {\n        return unitPrice *;\n    }"),
		DisplayKilling: edit(mavenSource, rates+"discounted",
			"public static BigDecimal discounted(BigDecimal total, int percent) {\n        int kept = MAX_PERCENT + clampPercent(percent);\n        return total.multiply(BigDecimal.valueOf(kept))\n                .divide(BigDecimal.valueOf(MAX_PERCENT), 2, RoundingMode.HALF_UP);\n    }"),
	}
}

func gradleSubject(t *testing.T) ecosystemcontract.Subject {
	const rates = "Rates."
	return ecosystemcontract.Subject{
		Name: "gradle",
		Build: func(a ecosystemcontract.Adapters) ports.Ecosystem {
			return jvm.NewGradle(jvmDependencies(a))
		},
		FixtureDir:    filepath.Join(fixturesRoot, gradleFixture),
		Sources:       []string{gradleSource},
		SourceRoots:   []string{sourceRootsMain},
		BuildOutputs:  gradleClasses,
		Bindings:      loadBindings(t, gradleBindings),
		KillingRef:    killing,
		EquivalentRef: discount,
		UnaffectedRef: shipping,
		DisplayRef:    discount,
		UnboundTests:  []ecosystem.TestID{ecosystem.NewTestID("parcel."+unboundTestClass, unboundTestName)},
		Killing: edit(gradleSource, rates+"lineTotal",
			"public static int lineTotal(int unitPrice, int quantity) {\n        return unitPrice + quantity;\n    }"),
		Equivalent: edit(gradleSource, rates+"clampPercent",
			"static int clampPercent(int percent) {\n        return Math.min(MAX_PERCENT, Math.max(0, percent));\n    }"),
		Broken: edit(gradleSource, rates+"lineTotal",
			"public static int lineTotal(int unitPrice, int quantity) {\n        return unitPrice *;\n    }"),
		DisplayKilling: edit(gradleSource, rates+"discounted",
			"public static int discounted(int total, int percent) {\n        int kept = MAX_PERCENT + clampPercent(percent);\n        return total * kept / MAX_PERCENT;\n    }"),
	}
}

func kotlinSubject(t *testing.T) ecosystemcontract.Subject {
	return ecosystemcontract.Subject{
		Name: "gradle-kotlin",
		Build: func(a ecosystemcontract.Adapters) ports.Ecosystem {
			return jvm.NewGradle(jvmDependencies(a))
		},
		FixtureDir:    filepath.Join(fixturesRoot, kotlinFixture),
		Sources:       []string{kotlinSource},
		SourceRoots:   []string{sourceRootsMain},
		BuildOutputs:  kotlinClasses,
		Bindings:      loadBindings(t, kotlinBindings),
		KillingRef:    killing,
		EquivalentRef: discount,
		UnaffectedRef: shipping,
		DisplayRef:    discount,
		UnboundTests:  []ecosystem.TestID{ecosystem.NewTestID("parcel."+unboundTestClass, unboundTestName)},
		Killing: edit(kotlinSource, "lineTotal",
			"fun lineTotal(unitPrice: Int, quantity: Int): Int {\n    return unitPrice + quantity\n}"),
		Equivalent: edit(kotlinSource, "clampPercent",
			"internal fun clampPercent(percent: Int): Int {\n    return minOf(MAX_PERCENT, maxOf(0, percent))\n}"),
		Broken: edit(kotlinSource, "lineTotal",
			"fun lineTotal(unitPrice: Int, quantity: Int): Int {\n    return unitPrice *\n}"),
		DisplayKilling: edit(kotlinSource, "discounted",
			"fun discounted(total: Int, percent: Int): Int {\n    val kept = MAX_PERCENT + clampPercent(percent)\n    return total * kept / MAX_PERCENT\n}"),
	}
}

// TestEcosystemContractJVM runs the contract of the JVM strategies one after the
// other; each build spawns a JVM and the machine runs one at a time.
func TestEcosystemContractJVM(t *testing.T) {
	t.Run("maven with java", func(t *testing.T) {
		jvmenv.Require(t)
		jvmenv.FetchMaven(t, filepath.Join(fixturesRoot, mavenFixture))
		ecosystemcontract.Run(t, mavenSubject(t))
	})
	t.Run("gradle with java", func(t *testing.T) {
		jvmenv.Require(t)
		jvmenv.FetchGradle(t, filepath.Join(fixturesRoot, gradleFixture))
		ecosystemcontract.Run(t, gradleSubject(t))
	})
	t.Run("gradle with kotlin", func(t *testing.T) {
		jvmenv.Require(t)
		jvmenv.FetchGradle(t, filepath.Join(fixturesRoot, kotlinFixture))
		ecosystemcontract.Run(t, kotlinSubject(t))
	})
}
