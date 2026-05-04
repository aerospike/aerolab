package db

import "testing"

// mapGetter is a colGetter backed by a plain map (no schema required) for
// unit tests of the Expr AST in isolation.
func mapGetter(m map[string]Value) colGetter {
	return func(name string) (Value, bool) {
		v, ok := m[name]
		return v, ok
	}
}

func TestFilterEq(t *testing.T) {
	g := mapGetter(map[string]Value{"x": Int(5), "s": Str("abc")})
	if !Eq("x", Int(5)).eval(g) {
		t.Error("x==5 should match")
	}
	if Eq("x", Int(6)).eval(g) {
		t.Error("x==6 should not match")
	}
	if Eq("missing", Int(5)).eval(g) {
		t.Error("missing col Eq should not match")
	}
	if Eq("s", Int(5)).eval(g) {
		t.Error("type mismatch must not match")
	}
	if !Eq("s", Str("abc")).eval(g) {
		t.Error("string Eq")
	}
}

func TestFilterIn(t *testing.T) {
	g := mapGetter(map[string]Value{"cluster": Int(2)})
	if !In("cluster", Int(1), Int(2), Int(3)).eval(g) {
		t.Error("In should match 2")
	}
	if In("cluster", Int(10), Int(11)).eval(g) {
		t.Error("In should not match")
	}
	if In("missing", Int(1)).eval(g) {
		t.Error("In on missing col should not match")
	}
	if In("cluster").eval(g) {
		t.Error("empty In should never match")
	}
}

func TestFilterBetween(t *testing.T) {
	g := mapGetter(map[string]Value{"t": Int(50)})
	if !Between("t", Int(10), Int(100)).eval(g) {
		t.Error("50 in [10,100]")
	}
	if Between("t", Int(60), Int(100)).eval(g) {
		t.Error("50 not in [60,100]")
	}
	if !Between("t", Int(50), Int(50)).eval(g) {
		t.Error("50 in [50,50]")
	}
	if Between("missing", Int(1), Int(2)).eval(g) {
		t.Error("Between on missing col")
	}
}

func TestFilterExists(t *testing.T) {
	g := mapGetter(map[string]Value{"a": Int(0)})
	if !Exists("a").eval(g) {
		t.Error("Exists(a) should match even if value is zero int")
	}
	if Exists("b").eval(g) {
		t.Error("Exists(b) should not match")
	}
}

func TestFilterNot(t *testing.T) {
	g := mapGetter(map[string]Value{"a": Int(1)})
	if Not(Eq("a", Int(1))).eval(g) {
		t.Error("!true = false expected")
	}
	if !Not(Eq("a", Int(2))).eval(g) {
		t.Error("!false = true expected")
	}
	if !Not(Exists("b")).eval(g) {
		t.Error("!Exists(missing) = true expected")
	}
}

func TestFilterAndOr(t *testing.T) {
	g := mapGetter(map[string]Value{"a": Int(1), "b": Int(2)})
	if !And(Eq("a", Int(1)), Eq("b", Int(2))).eval(g) {
		t.Error("And both true")
	}
	if And(Eq("a", Int(1)), Eq("b", Int(99))).eval(g) {
		t.Error("And one false must fail")
	}
	if !Or(Eq("a", Int(99)), Eq("b", Int(2))).eval(g) {
		t.Error("Or one true must match")
	}
	if Or(Eq("a", Int(99)), Eq("b", Int(99))).eval(g) {
		t.Error("Or all false must fail")
	}
	// Empty And => true, empty Or => false.
	if !And().eval(g) {
		t.Error("empty And should be true")
	}
	if Or().eval(g) {
		t.Error("empty Or should be false")
	}
}

func TestFilterColumnNamesGathered(t *testing.T) {
	expr := And(
		Eq("a", Int(1)),
		Or(Exists("b"), Between("c", Int(0), Int(10))),
		Not(In("d", Int(1), Int(2))),
	)
	names := expr.columns(nil)
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
	}
	for _, want := range []string{"a", "b", "c", "d"} {
		if !seen[want] {
			t.Errorf("column %q not reported", want)
		}
	}
}

func TestFilterMirrorsPluginTimeseriesShape(t *testing.T) {
	// The plugin builds: MustExist => And(BinExists, Or(Eq...))
	// Optional         => Or(Not(BinExists), Or(Eq...))
	row := map[string]Value{"ClusterName": Int(0), "latency": Int(500)}
	g := mapGetter(row)

	mustExist := And(Exists("ClusterName"), Or(Eq("ClusterName", Int(0)), Eq("ClusterName", Int(1))))
	if !mustExist.eval(g) {
		t.Error("MustExist with matching value must evaluate true")
	}

	optional := Or(Not(Exists("service")), Or(Eq("service", Int(2))))
	if !optional.eval(g) {
		t.Error("optional with absent column should be true (Not(Exists) branch)")
	}

	// Required bin exists check (for the value bin itself).
	valReq := And(Exists("latency"), mustExist)
	if !valReq.eval(g) {
		t.Error("bin existence for required bin")
	}
}
