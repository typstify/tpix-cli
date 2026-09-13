package deps

import "testing"

func TestDependencyIsValid(t *testing.T) {
	cases := []struct {
		name   string
		dep    Dependency
		expect bool
	}{
		{"only namespace", Dependency{Namespace: "ns-01"}, false},
		{"namespace and package", Dependency{Namespace: "ns-01", Name: "pkg-test"}, true},
		{"all field", Dependency{Namespace: "ns-01", Name: "pkg-test", Version: "v0.1.1"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.dep.IsValid() != tc.expect {
				t.Fail()
			}
		})
	}
}

func TestDependencyIsPartial(t *testing.T) {
	cases := []struct {
		name   string
		dep    Dependency
		expect bool
	}{
		{"only namespace", Dependency{Namespace: "ns-01"}, true},
		{"namespace and package", Dependency{Namespace: "ns-01", Name: "pkg-test"}, true},
		{"all field", Dependency{Namespace: "ns-01", Name: "pkg-test", Version: "v0.1.1"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.dep.Partial() != tc.expect {
				t.Fail()
			}
		})
	}
}
